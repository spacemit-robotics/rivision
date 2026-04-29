#include "inference_backend.h"
#include <opencv2/opencv.hpp>
#include <onnxruntime_cxx_api.h>
#include <fstream>
#include <queue>
#include <thread>
#include <mutex>
#include <condition_variable>
#include <cstring>
#include <unordered_map>
#include <atomic>
#include <future>

// SpacemiT NPU 支持 (K3 A100)
#ifdef USE_SPACEMIT_EP
#include "spacemit_ort_env.h"
#endif

// ★★★ K3 硬件加速预处理 ★★★
// 方案1: MPP (K1 JPU + V2D) - K3 不完全支持
// 方案2: FFmpeg (mjpeg_stcodec) - K3 推荐，VPU 硬件 JPEG 解码
#ifdef USE_K3_MPP
#include "k3_mpp_preprocessor.h"
#endif

#ifdef USE_K3_FFMPEG
#include "../preprocessing/ffmpeg_preprocessor.h"
#endif

// ★ 优化 3: OpenCV 多线程初始化
// RISC-V Vector 扩展加速 + 多线程并行
static void init_opencv_optimization(int cpu_threads = 4) {
    static std::once_flag flag;
    static int configured_threads = 4;
    
    // 首次调用时初始化
    std::call_once(flag, [cpu_threads]() {
        configured_threads = cpu_threads;
        
        // 环境变量优先级最高
        const char* env = std::getenv("OPENCV_THREADS");
        if (env) {
            configured_threads = std::max(1, std::min(8, std::atoi(env)));
        }
        
        cv::setNumThreads(configured_threads);
        cv::setUseOptimized(true);
        
        // OpenCV 初始化完成
    });
    
    // 后续调用如果线程数不同，更新 OpenCV 线程数
    if (cpu_threads != configured_threads) {
        const char* env = std::getenv("OPENCV_THREADS");
        if (!env) {  // 仅在没有环境变量时才更新
            configured_threads = cpu_threads;
            cv::setNumThreads(configured_threads);
        }
    }
}

// ★ 关键修复: 全局共享 Ort::Env (ONNX Runtime 要求进程级单例)
// 多个 Env 实例会导致资源竞争和 SpacemiT EP 初始化冲突
static Ort::Env& get_global_env() {
    static Ort::Env env(ORT_LOGGING_LEVEL_WARNING, "rivision-benchmark");
    return env;
}

// SpacemiT EP 只初始化一次
static std::atomic<bool> g_spacemit_initialized{false};
static std::mutex g_spacemit_init_mutex;

namespace rivision {
namespace benchmark {

// ============================================================
// COCO 类别名称
// ============================================================
static const char* COCO_NAMES[] = {
    "person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck",
    "boat", "traffic light", "fire hydrant", "stop sign", "parking meter", "bench",
    "bird", "cat", "dog", "horse", "sheep", "cow", "elephant", "bear", "zebra",
    "giraffe", "backpack", "umbrella", "handbag", "tie", "suitcase", "frisbee",
    "skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove",
    "skateboard", "surfboard", "tennis racket", "bottle", "wine glass", "cup",
    "fork", "knife", "spoon", "bowl", "banana", "apple", "sandwich", "orange",
    "broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair", "couch",
    "potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
    "remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink",
    "refrigerator", "book", "clock", "vase", "scissors", "teddy bear", "hair drier",
    "toothbrush"
};
static const int NUM_CLASSES = 80;

// ============================================================
// ★★★ 优化 5: 内存池 (减少内存分配开销) ★★★
// ============================================================
// 预分配 tensor 缓冲区，避免每帧都分配/释放内存
// 典型 YOLO 输入: 640x640x3 = 1.2MB，输出: 1x84x8400 = 2.8MB
template<typename T>
class MemoryPool {
public:
    explicit MemoryPool(size_t buffer_size, size_t pool_size = 16) 
        : buffer_size_(buffer_size) {
        for (size_t i = 0; i < pool_size; i++) {
            auto buf = std::make_shared<std::vector<T>>();
            buf->reserve(buffer_size);
            pool_.push(buf);
        }
        // MemoryPool 初始化完成
    }
    
    // 获取一个缓冲区 (如果池空则创建新的)
    std::shared_ptr<std::vector<T>> acquire() {
        std::lock_guard<std::mutex> lock(mutex_);
        if (pool_.empty()) {
            auto buf = std::make_shared<std::vector<T>>();
            buf->reserve(buffer_size_);
            alloc_count_++;
            return buf;
        }
        auto buf = pool_.front();
        pool_.pop();
        return buf;
    }
    
    // 归还缓冲区到池中
    void release(std::shared_ptr<std::vector<T>> buf) {
        if (!buf) return;
        buf->clear();  // 清空内容但保留容量
        std::lock_guard<std::mutex> lock(mutex_);
        pool_.push(buf);
    }
    
    size_t extra_allocs() const { return alloc_count_; }
    
private:
    size_t buffer_size_;
    std::queue<std::shared_ptr<std::vector<T>>> pool_;
    std::mutex mutex_;
    std::atomic<size_t> alloc_count_{0};
};

// ============================================================
// 预处理缓存 (用于异步流水线)
// ============================================================
struct PreprocData {
    std::shared_ptr<std::vector<float>> tensor;  // ★ 使用内存池分配的共享缓冲区
    cv::Mat original_img;
    int64_t frame_id;
    int64_t timestamp_us;
    double preprocess_ms;
    bool valid = false;
    std::string error;
};

// ============================================================
// LocalYOLOBackend 实现
// ============================================================
struct LocalYOLOBackend::Impl {
    // ★★★ 双 Session 支持 (根据 NPU 核心数自动启用) ★★★
    // A100 8核: 启用双 Session，每个 4 核
    // A100 4核: 单 Session，使用全部 4 核
    static constexpr int MAX_SESSIONS = 2;
    std::vector<Ort::SessionOptions> session_options_list;
    std::vector<std::unique_ptr<Ort::Session>> sessions;
    int num_sessions = 1;       // 实际使用的 Session 数量
    int cores_per_session = 4;  // 每个 Session 的 NPU 核心数
    int total_npu_cores = 4;    // 总 NPU 核心数
    
    // 输入输出信息
    std::vector<const char*> input_names;
    std::vector<const char*> output_names;
    std::vector<int64_t> input_shape;
    
    // Worker 池
    std::queue<int> available_workers;
    std::mutex worker_mutex;
    std::condition_variable worker_cv;
    
    // ★ 预处理队列 (保持 NPU 持续忙碌)
    std::queue<PreprocData> preproc_queue;
    std::mutex preproc_mutex;
    std::condition_variable preproc_cv;
    std::condition_variable preproc_space_cv;
    std::atomic<bool> preproc_running{false};
    std::thread preproc_thread;
    static constexpr int PREPROC_QUEUE_SIZE = 8;
    
    // ★ 双 Session 推理统计
    std::array<std::atomic<int64_t>, MAX_SESSIONS> session_infer_count;
    
    // ★★★ 优化 5: 内存池 ★★★
    std::unique_ptr<MemoryPool<float>> input_tensor_pool;   // 输入 tensor 缓冲池
    std::unique_ptr<MemoryPool<float>> output_tensor_pool;  // 输出 tensor 缓冲池
    
    // ★★★ 优化 6: K3 硬件加速预处理 ★★★
    // 方案1: FFmpeg mjpeg_stcodec (K3 推荐，VPU 硬件 JPEG 解码)
    // 方案2: MPP (K1 JPU + V2D，K3 不完全支持)
#ifdef USE_K3_FFMPEG
    std::unique_ptr<IPreprocessor> ffmpeg_preprocessor;
    bool use_ffmpeg_preprocess = false;
    std::mutex ffmpeg_mutex;  // ★★★ FFmpeg 不是线程安全的，需要互斥锁 ★★★
    std::atomic<int> ffmpeg_success_count{0};  // ★ 成功计数（用于日志）
    std::atomic<int> ffmpeg_fail_count{0};     // ★ 失败计数（用于日志）
    
    // ★★★ 方案 2: 多 FFmpeg 实例 (测试) ★★★
    // 环境变量 FFMPEG_MULTI_INSTANCE=N 启用 N 个解码器实例
    // 注意: K3 硬件解码器可能不支持多实例 (只有一个 /dev/video12)
    static constexpr int MAX_FFMPEG_INSTANCES = 4;
    std::vector<std::unique_ptr<FFmpegPreprocessor>> ffmpeg_instances;
    std::vector<std::mutex> ffmpeg_instance_mutexes;
    std::atomic<int> ffmpeg_instance_idx{0};  // 轮询分配
    bool use_multi_ffmpeg = false;
#endif
#ifdef USE_K3_MPP
    std::unique_ptr<K3MppPreprocessor> mpp_preprocessor;
    bool use_mpp_preprocess = false;
#endif
    
    // 配置
    float conf_threshold = 0.25f;
    float nms_threshold = 0.45f;
    int input_width = 640;
    int input_height = 640;
    int cpu_threads = 6;           // ★ CPU 线程数 (预处理/后处理, K3推荐6)
    bool async_preprocess = false;
    
    // ★ 优化 4+5+6: 减少内存拷贝 + 使用内存池 + MPP 硬件加速
    // 原始流程: decode → resize → cvtColor → convertTo → split → memcpy (6次分配)
    // 优化流程: decode → resize+cvtColor 合并 → 直接写入内存池 tensor (0次分配)
    // MPP 流程: JPU解码 → V2D(resize+CSC) → CPU归一化 (硬件加速)
    PreprocData do_preprocess(const Frame& frame) {
        PreprocData data;
        data.frame_id = frame.frame_id;
        data.timestamp_us = frame.timestamp_us;
        
        auto start = Clock::now();
        
        // ★ 从内存池获取 tensor 缓冲区 (避免每帧分配)
        const int plane_size = input_height * input_width;
        data.tensor = input_tensor_pool->acquire();
        data.tensor->resize(3 * plane_size);
        
#ifdef USE_K3_FFMPEG
        // ★★★ 方案 2: 多 FFmpeg 实例 (真正并行) ★★★
        if (use_ffmpeg_preprocess && use_multi_ffmpeg && !ffmpeg_instances.empty()) {
            // 轮询选择实例
            int idx = ffmpeg_instance_idx.fetch_add(1) % static_cast<int>(ffmpeg_instances.size());
            auto* ff = ffmpeg_instances[idx].get();
            if (!ff) {
                data.valid = false;
                data.error = "FFmpeg instance is null";
                return data;
            }
            
            // 每个实例有独立的 mutex (真正并行)
            FFmpegPreprocessor::DecodeResult decode_result;
            {
                std::lock_guard<std::mutex> lock(ffmpeg_instance_mutexes[idx]);
                if (!ff->decodeOnly(frame.data.data(), frame.data.size(), decode_result)) {
                    ffmpeg_fail_count.fetch_add(1);
                    data.valid = false;
                    data.error = "FFmpeg multi-instance decode failed";
                    return data;
                }
            }
            
            // scale+convert 可以并行
            if (!ff->scaleAndConvert(decode_result, data.tensor->data(), input_width, input_height)) {
                data.valid = false;
                data.error = "FFmpeg scale/convert failed";
                return data;
            }
            
            int count = ffmpeg_success_count.fetch_add(1) + 1;
            double decode_ms = ff->getLastDecodeTimeMs();
            double scale_ms = ff->getLastScaleTimeMs();
            double convert_ms = ff->getLastConvertTimeMs();
            data.preprocess_ms = decode_ms + scale_ms + convert_ms;
            data.original_img = cv::Mat(decode_result.height, decode_result.width, CV_8UC3);
            
            (void)count;  // 移除日志输出
            
            data.valid = true;
            return data;
        }
        
        // ★★★ 方案 1: 单实例 + 分离 mutex 粒度 ★★★
        // 优化: 只在 mutex 内做硬件解码 (~0.8ms)
        //       scale+convert 在 mutex 外并行执行 (~5ms)
        if (use_ffmpeg_preprocess && ffmpeg_preprocessor) {
            auto* ff = dynamic_cast<FFmpegPreprocessor*>(ffmpeg_preprocessor.get());
            if (!ff) {
                data.valid = false;
                data.error = "FFmpeg preprocessor cast failed";
                return data;
            }
            
            // ★ Step 1: 硬件解码 (需要 mutex 保护, ~0.8ms)
            FFmpegPreprocessor::DecodeResult decode_result;
            {
                std::lock_guard<std::mutex> lock(ffmpeg_mutex);
                if (!ff->decodeOnly(frame.data.data(), frame.data.size(), decode_result)) {
                    ffmpeg_fail_count.fetch_add(1);
                    if (ffmpeg_fail_count.load() <= 3) {
                        printf("[FFmpeg] ERROR: Hardware decode failed (frame %ld)\n", frame.frame_id);
                        fflush(stdout);
                    }
                    data.valid = false;
                    data.error = "FFmpeg hardware decode failed";
                    return data;
                }
            }
            // ★ mutex 已释放，其他线程可以开始解码下一帧
            
            // ★ Step 2: 缩放+转换 (无需 mutex, ~5ms, 可并行)
            if (!ff->scaleAndConvert(decode_result, data.tensor->data(), input_width, input_height)) {
                data.valid = false;
                data.error = "FFmpeg scale/convert failed";
                return data;
            }
            
            int count = ffmpeg_success_count.fetch_add(1) + 1;
            double decode_ms = ff->getLastDecodeTimeMs();
            double scale_ms = ff->getLastScaleTimeMs();
            double convert_ms = ff->getLastConvertTimeMs();
            data.preprocess_ms = decode_ms + scale_ms + convert_ms;
            data.original_img = cv::Mat(decode_result.height, decode_result.width, CV_8UC3);
            
            (void)count;  // 移除日志输出
            
            data.valid = true;
            return data;
        }
#endif

#ifdef USE_K3_MPP
        // ★★★ 尝试 MPP 硬件加速路径 ★★★
        if (use_mpp_preprocess && mpp_preprocessor) {
            auto result = mpp_preprocessor->preprocess(
                frame.data.data(), frame.data.size(), data.tensor->data());
            
            if (result.success) {
                // 硬件加速成功
                data.preprocess_ms = result.total_ms;
                data.valid = true;
                // 创建占位 Mat 保存原始尺寸 (用于后处理缩放计算)
                data.original_img = cv::Mat(result.orig_height, result.orig_width, CV_8UC3);
                return data;
            }
            // 硬件失败，静默回退到 OpenCV
        }
#endif
        
        // ★ OpenCV 软件路径 (回退或默认)
        // 1. 解码 JPEG
        data.original_img = cv::imdecode(frame.data, cv::IMREAD_COLOR);
        if (data.original_img.empty()) {
            data.error = "Failed to decode image";
            return data;
        }
        
        // 2. Resize (如果尺寸相同则跳过)
        cv::Mat resized;
        if (data.original_img.cols == input_width && data.original_img.rows == input_height) {
            resized = data.original_img;  // 零拷贝引用
        } else {
            cv::resize(data.original_img, resized, cv::Size(input_width, input_height), 
                       0, 0, cv::INTER_LINEAR);
        }
        
        // 3. ★★★ 优化: 并行 BGR→RGB + 归一化 + HWC→CHW ★★★
        // 使用多线程分块处理，大幅降低预处理时间
        // 
        // 理论分析:
        //   串行: T_convert = H × W × 3 ops ≈ 5ms
        //   并行: T_convert = (H × W × 3 ops) / N_threads ≈ 0.6ms (8线程)
        //
        // 最优条件: T_pre ≤ T_npu 时，可同时达成 最低延时 + 最高 NPU 利用率
        //
        float* tensor_data = data.tensor->data();
        const float scale = 1.0f / 255.0f;
        const int num_threads = std::min(cpu_threads, cv::getNumThreads());  // ★ 使用配置的线程数
        
        // 使用 OpenCV 的 parallel_for_ 实现行级并行
        cv::parallel_for_(cv::Range(0, input_height), [&](const cv::Range& range) {
            for (int y = range.start; y < range.end; y++) {
                const uint8_t* row = resized.ptr<uint8_t>(y);
                float* ptr_r = tensor_data + y * input_width;
                float* ptr_g = tensor_data + plane_size + y * input_width;
                float* ptr_b = tensor_data + 2 * plane_size + y * input_width;
                
                for (int x = 0; x < input_width; x++) {
                    // BGR → RGB + 归一化 + HWC→CHW
                    ptr_r[x] = row[2] * scale;  // R
                    ptr_g[x] = row[1] * scale;  // G
                    ptr_b[x] = row[0] * scale;  // B
                    row += 3;
                }
            }
        }, num_threads);
        
        data.preprocess_ms = Duration(Clock::now() - start).count();
        data.valid = true;
        return data;
    }
};

LocalYOLOBackend::LocalYOLOBackend() : impl_(std::make_unique<Impl>()) {}
LocalYOLOBackend::~LocalYOLOBackend() { shutdown(); }

bool LocalYOLOBackend::init(const BackendConfig& config) {
    config_ = config;
    
    impl_->conf_threshold = config.conf_threshold;
    impl_->nms_threshold = config.nms_threshold;
    
    // ★ CPU 线程配置 (用于预处理/后处理/OpenCV)
    impl_->cpu_threads = config.cpu_threads;
    const char* env_cpu_threads = std::getenv("CPU_THREADS");
    if (env_cpu_threads) {
        impl_->cpu_threads = std::max(1, std::min(8, std::atoi(env_cpu_threads)));
    }
    
    // ★ 初始化 OpenCV 多线程 (使用配置的 CPU 线程数)
    init_opencv_optimization(impl_->cpu_threads);
    
    // ============================================================
    // ★★★ 双 Session 自动配置 (基于 A100 核心数) ★★★
    // ============================================================
    // 配置优先级: 环境变量 > BackendConfig > YAML配置 > 默认值
    //
    // 配置方式:
    //   1. 环境变量: YOLO_THREADS=8 ./benchmark
    //   2. BackendConfig: config.npu_threads = 8
    //   3. YAML 配置: npu.yolo_threads: 8 (通过 shell 脚本传递)
    //   4. 默认值: 4 核
    //
    // 双 Session 规则:
    //   - 4核以上自动启用双 Session (每个 Session 使用一半核心)
    //   - YOLO_DUAL_SESSION=0 可强制禁用
    // ============================================================
    
    // 1. 读取 NPU 核心数 (优先级: 环境变量 > config > 默认)
    int total_cores = config.npu_threads;  // 从 BackendConfig 读取 (默认 4)
    const char* env_threads = std::getenv("YOLO_THREADS");
    if (env_threads) {
        total_cores = std::atoi(env_threads);  // 环境变量覆盖
    }
    // 限制范围
    if (total_cores < 1) total_cores = 1;
    if (total_cores > 8) total_cores = 8;  // K3 A100 最多 8 核心
    impl_->total_npu_cores = total_cores;
    
    // 2. 决定 Session 数量 (优先级: 环境变量 > config > 自动)
    impl_->num_sessions = 1;
    impl_->cores_per_session = total_cores;
    
    bool enable_dual = config.dual_session;  // 从 BackendConfig 读取 (默认 true)
    const char* env_dual = std::getenv("YOLO_DUAL_SESSION");
    if (env_dual) {
        enable_dual = (std::atoi(env_dual) > 0);  // 环境变量覆盖
    }
    
    // ★★★ Session 策略选择 ★★★
    // 
    // 策略对比:
    //   单 Session (4×1): 最低延时，NPU效率取决于预处理速度
    //   双 Session (2×2): 更高吞吐，NPU利用率更高
    //
    // 环境变量控制:
    //   YOLO_DUAL_SESSION=0: 强制单 Session (最低延时)
    //   YOLO_DUAL_SESSION=1: 强制双 Session (最高吞吐，默认)
    //   不设置: 自动启用双Session
    //
    // ★ 优化: 4核也启用双Session (2+2)，提高NPU利用率
    if (enable_dual && total_cores >= 4) {
        impl_->num_sessions = 2;
        impl_->cores_per_session = total_cores / 2;  // 4核 → 2×2核, 8核 → 2×4核
    }
    num_workers_ = impl_->num_sessions;
    
    // 初始化推理统计
    for (int i = 0; i < Impl::MAX_SESSIONS; i++) {
        impl_->session_infer_count[i].store(0);
    }
    
    // ============================================================
    // 为每个 Session 创建独立的 SessionOptions
    // ============================================================
    impl_->session_options_list.resize(impl_->num_sessions);
    impl_->sessions.resize(impl_->num_sessions);
    
    for (int s = 0; s < impl_->num_sessions; s++) {
        auto& opts = impl_->session_options_list[s];
        
#ifdef USE_SPACEMIT_EP
        opts.SetIntraOpNumThreads(1);
        opts.SetInterOpNumThreads(1);
#else
        opts.SetIntraOpNumThreads(impl_->cores_per_session);
        opts.SetInterOpNumThreads(1);
#endif
        opts.SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_ALL);
        opts.AddConfigEntry("session.use_env_allocators", "1");
        
#ifdef USE_SPACEMIT_EP
        // ★ 为每个 Session 配置独立的 NPU 核心
        {
            std::lock_guard<std::mutex> lock(g_spacemit_init_mutex);
            std::unordered_map<std::string, std::string> provider_options;
            provider_options["SPACEMIT_EP_INTRA_THREAD_NUM"] = std::to_string(impl_->cores_per_session);
            
            // 双 Session 模式: 使用独立线程池避免竞争
            if (impl_->num_sessions > 1) {
                provider_options["SPACEMIT_EP_USE_GLOBAL_INTRA_THREAD"] = "0";
            } else {
                provider_options["SPACEMIT_EP_USE_GLOBAL_INTRA_THREAD"] = "1";
            }
            
            try {
                SessionOptionsSpaceMITEnvInit(opts, provider_options);
            } catch (const Ort::Exception& e) {
                // EP 初始化失败
            }
        }
#endif
    }
    
#ifndef USE_SPACEMIT_EP
    // No hardware EP, CPU inference
#endif
    
    try {
        // ★ 为每个 Session 创建独立的 ONNX Session
        for (int s = 0; s < impl_->num_sessions; s++) {
            impl_->sessions[s] = std::make_unique<Ort::Session>(
                get_global_env(), config.model_path.c_str(), impl_->session_options_list[s]);
            impl_->available_workers.push(s);
        }
        
        // 获取输入输出信息
        Ort::AllocatorWithDefaultOptions allocator;
        auto& session = *impl_->sessions[0];
        
        // 输入
        size_t num_inputs = session.GetInputCount();
        for (size_t i = 0; i < num_inputs; i++) {
            auto name = session.GetInputNameAllocated(i, allocator);
            impl_->input_names.push_back(strdup(name.get()));
        }
        
        // 输入形状
        auto input_info = session.GetInputTypeInfo(0);
        auto tensor_info = input_info.GetTensorTypeAndShapeInfo();
        impl_->input_shape = tensor_info.GetShape();
        
        if (impl_->input_shape.size() == 4) {
            impl_->input_height = static_cast<int>(impl_->input_shape[2]);
            impl_->input_width = static_cast<int>(impl_->input_shape[3]);
        }
        
        // 输出
        size_t num_outputs = session.GetOutputCount();
        for (size_t i = 0; i < num_outputs; i++) {
            auto name = session.GetOutputNameAllocated(i, allocator);
            impl_->output_names.push_back(strdup(name.get()));
        }
        
        // ★★★ 优化 5: 初始化内存池 ★★★
        size_t input_tensor_size = 3 * impl_->input_width * impl_->input_height;
        size_t output_tensor_size = 1 * 100 * 10000;
        size_t pool_size = impl_->cpu_threads + impl_->num_sessions + 16;
        
        impl_->input_tensor_pool = std::make_unique<MemoryPool<float>>(input_tensor_size, pool_size);
        impl_->output_tensor_pool = std::make_unique<MemoryPool<float>>(output_tensor_size, pool_size);
        
        // ★★★ 优化 6: 初始化 K3 硬件加速预处理 ★★★
        
#ifdef USE_K3_FFMPEG
        {
            const char* env_ffmpeg = std::getenv("USE_FFMPEG_PREPROCESS");
            bool enable_ffmpeg = !(env_ffmpeg && std::atoi(env_ffmpeg) == 0);
            
            if (enable_ffmpeg) {
                // ★ 方案 2: 检查是否启用多 FFmpeg 实例
                const char* env_multi = std::getenv("FFMPEG_MULTI_INSTANCE");
                int num_instances = env_multi ? std::atoi(env_multi) : 0;
                if (num_instances > 0 && num_instances <= Impl::MAX_FFMPEG_INSTANCES) {
                    impl_->ffmpeg_instances.resize(num_instances);
                    impl_->ffmpeg_instance_mutexes = std::vector<std::mutex>(num_instances);
                    
                    int hw_count = 0;
                    for (int i = 0; i < num_instances; i++) {
                        auto* pp = new FFmpegPreprocessor();
                        FFmpegPreprocessor::Config cfg;
                        cfg.target_width = impl_->input_width;
                        cfg.target_height = impl_->input_height;
                        cfg.cpu_threads = impl_->cpu_threads;
                        cfg.prefer_hw_decoder = true;
                        
                        if (pp->init(cfg)) {
                            impl_->ffmpeg_instances[i].reset(pp);
                            if (pp->isHardwareAccelerated()) hw_count++;
                        } else {
                            delete pp;
                        }
                    }
                    
                    if (hw_count > 0) {
                        impl_->use_multi_ffmpeg = true;
                        impl_->use_ffmpeg_preprocess = true;
                    }
                }
                
                // 单实例模式 (默认)
                if (!impl_->use_multi_ffmpeg) {
                    impl_->ffmpeg_preprocessor = create_ffmpeg_preprocessor(
                        impl_->input_width, impl_->input_height, 
                        impl_->cpu_threads, true);
                    
                    if (impl_->ffmpeg_preprocessor) {
                        impl_->use_ffmpeg_preprocess = true;
                    }
                }
            }
        }
#endif

#ifdef USE_K3_MPP
        // ★★★ MPP 硬件加速 (备选) ★★★
#ifdef USE_K3_FFMPEG
        if (!impl_->use_ffmpeg_preprocess) {
#endif
        {
            const char* env_mpp = std::getenv("USE_MPP_PREPROCESS");
            bool enable_mpp = !(env_mpp && std::atoi(env_mpp) == 0);
            
            if (enable_mpp) {
                MppPreprocessorConfig mpp_config;
                mpp_config.target_width = impl_->input_width;
                mpp_config.target_height = impl_->input_height;
                mpp_config.cpu_threads = impl_->cpu_threads;
                mpp_config.enable_hw_decode = true;
                mpp_config.enable_hw_resize = true;
                
                impl_->mpp_preprocessor = create_mpp_preprocessor(mpp_config);
                
                if (impl_->mpp_preprocessor && impl_->mpp_preprocessor->is_hw_available()) {
                    impl_->use_mpp_preprocess = true;
                }
            }
        }
#ifdef USE_K3_FFMPEG
        }
#endif
#endif
        
        // ★ 打印汇总信息
        const char* preproc_name = "opencv";
#ifdef USE_K3_FFMPEG
        if (impl_->use_ffmpeg_preprocess) {
            preproc_name = impl_->use_multi_ffmpeg ? "ffmpeg-multi" : "ffmpeg-hw";
        }
#endif
#ifdef USE_K3_MPP
        if (impl_->use_mpp_preprocess) preproc_name = "mpp-hw";
#endif
        printf("[Backend] %s | NPU=%d×%d | Preproc=%s | Input=%dx%d\n",
               config.model_path.c_str(), impl_->num_sessions, impl_->cores_per_session,
               preproc_name, impl_->input_width, impl_->input_height);
        fflush(stdout);
        
        is_loaded_ = true;
        return true;
        
    } catch (const Ort::Exception& e) {
        // ★ 与 yolo-server 一致: 仅捕获 Ort::Exception
        printf("[LocalYOLOBackend] Failed to load: %s\n", e.what());
        return false;
    }
}

InferenceResult LocalYOLOBackend::infer(const Frame& frame) {
    InferenceResult result;
    result.frame_id = frame.frame_id;
    result.timestamp_us = frame.timestamp_us;
    
    if (!is_loaded_) {
        result.error = "Model not loaded";
        return result;
    }
    
    // 静默推理
    
    auto total_start = Clock::now();
    
    // 获取可用 worker
    int worker_id;
    {
        std::unique_lock<std::mutex> lock(impl_->worker_mutex);
        impl_->worker_cv.wait(lock, [this]() { return !impl_->available_workers.empty(); });
        worker_id = impl_->available_workers.front();
        impl_->available_workers.pop();
    }
    
    try {
        auto& session = *impl_->sessions[worker_id];
        
        // 1. ★★★ 使用统一的预处理路径 (包含 MPP 硬件加速) ★★★
        auto preproc_data = impl_->do_preprocess(frame);
        
        if (!preproc_data.valid) {
            result.error = preproc_data.error.empty() ? "Preprocess failed" : preproc_data.error;
            impl_->available_workers.push(worker_id);
            impl_->worker_cv.notify_one();
            return result;
        }
        
        result.preprocess_ms = preproc_data.preprocess_ms;
        
        // 获取预处理后的 tensor
        const int plane_size = impl_->input_height * impl_->input_width;
        float* input_tensor_data = preproc_data.tensor->data();
        
        // 2. 推理
        auto inference_start = Clock::now();
        
        std::vector<int64_t> input_shape = {1, 3, impl_->input_height, impl_->input_width};
        Ort::MemoryInfo memory_info = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);
        
        Ort::Value input_tensor_ort = Ort::Value::CreateTensor<float>(
            memory_info, input_tensor_data, 3 * plane_size,
            input_shape.data(), input_shape.size());
        
        auto output_tensors = session.Run(Ort::RunOptions{nullptr},
            impl_->input_names.data(), &input_tensor_ort, 1,
            impl_->output_names.data(), impl_->output_names.size());
        
        auto inference_end = Clock::now();
        result.inference_ms = Duration(inference_end - inference_start).count();
        
        // 3. 后处理 (标准 YOLO 融合格式: [1, 84, 8400])
        auto postprocess_start = Clock::now();
        
        float* output_data = output_tensors[0].GetTensorMutableData<float>();
        auto output_shape = output_tensors[0].GetTensorTypeAndShapeInfo().GetShape();
        
        // YOLO 输出: [1, 84, 8400] (features × detections)
        // 或转置布局: [1, 8400, 84] (detections × features)
        // 84 = 4 (cx, cy, w, h) + 80 (classes)
        
        if (output_shape.size() < 3) {
            result.error = "Invalid output shape: expected 3 dimensions";
            impl_->available_workers.push(worker_id);
            impl_->worker_cv.notify_one();
            return result;
        }
        
        int dim1 = static_cast<int>(output_shape[1]);
        int dim2 = static_cast<int>(output_shape[2]);
        
        // 静默后处理
        
        // 判断布局: 84 是 features，8400 是 detections
        bool transposed = (dim1 > dim2);  // [1, 8400, 84] 时 dim1 > dim2
        int num_detections = transposed ? dim1 : dim2;
        int num_features = transposed ? dim2 : dim1;
        
        // 安全检查
        if (num_features < 4 + NUM_CLASSES) {
            result.error = "Invalid output features: " + std::to_string(num_features) + 
                           " (expected >= 84). Model may use unsupported DFL format.";
            impl_->available_workers.push(worker_id);
            impl_->worker_cv.notify_one();
            return result;
        }
        
        std::vector<cv::Rect> boxes;
        std::vector<float> confidences;
        std::vector<int> class_ids;
        
        // 使用预处理返回的原始图像尺寸计算缩放比例
        float scale_x = static_cast<float>(preproc_data.original_img.cols) / impl_->input_width;
        float scale_y = static_cast<float>(preproc_data.original_img.rows) / impl_->input_height;
        
        for (int i = 0; i < num_detections; i++) {
            // 获取所有类别的置信度
            float max_conf = 0;
            int max_class = 0;
            for (int c = 0; c < NUM_CLASSES; c++) {
                float conf;
                if (transposed) {
                    // [1, 8400, 84]: data[det_idx * features + feat_idx]
                    conf = output_data[i * num_features + (4 + c)];
                } else {
                    // [1, 84, 8400]: data[feat_idx * detections + det_idx]
                    conf = output_data[(4 + c) * num_detections + i];
                }
                if (conf > max_conf) {
                    max_conf = conf;
                    max_class = c;
                }
            }
            
            if (max_conf < impl_->conf_threshold) continue;
            
            // 获取 bbox (cx, cy, w, h)
            float cx, cy, w, h;
            if (transposed) {
                cx = output_data[i * num_features + 0];
                cy = output_data[i * num_features + 1];
                w  = output_data[i * num_features + 2];
                h  = output_data[i * num_features + 3];
            } else {
                cx = output_data[0 * num_detections + i];
                cy = output_data[1 * num_detections + i];
                w  = output_data[2 * num_detections + i];
                h  = output_data[3 * num_detections + i];
            }
            
            int x1 = static_cast<int>((cx - w / 2) * scale_x);
            int y1 = static_cast<int>((cy - h / 2) * scale_y);
            int x2 = static_cast<int>((cx + w / 2) * scale_x);
            int y2 = static_cast<int>((cy + h / 2) * scale_y);
            
            boxes.emplace_back(x1, y1, x2 - x1, y2 - y1);
            confidences.push_back(max_conf);
            class_ids.push_back(max_class);
        }
        
        // NMS
        std::vector<int> indices;
        cv::dnn::NMSBoxes(boxes, confidences, impl_->conf_threshold, impl_->nms_threshold, indices);
        
        for (int idx : indices) {
            Detection det;
            det.x1 = static_cast<float>(boxes[idx].x);
            det.y1 = static_cast<float>(boxes[idx].y);
            det.x2 = static_cast<float>(boxes[idx].x + boxes[idx].width);
            det.y2 = static_cast<float>(boxes[idx].y + boxes[idx].height);
            det.class_id = class_ids[idx];
            det.class_name = (class_ids[idx] < NUM_CLASSES) ? COCO_NAMES[class_ids[idx]] : "unknown";
            det.confidence = confidences[idx];
            result.detections.push_back(det);
        }
        
        auto postprocess_end = Clock::now();
        result.postprocess_ms = Duration(postprocess_end - postprocess_start).count();
        
        result.success = true;
        
    } catch (const Ort::Exception& e) {
        result.error = std::string("ONNX Runtime error: ") + e.what();
    } catch (const std::exception& e) {
        result.error = std::string("Error: ") + e.what();
    }
    
    // 归还 worker
    {
        std::lock_guard<std::mutex> lock(impl_->worker_mutex);
        impl_->available_workers.push(worker_id);
    }
    impl_->worker_cv.notify_one();
    
    auto total_end = Clock::now();
    result.total_ms = Duration(total_end - total_start).count();
    
    return result;
}

std::vector<InferenceResult> LocalYOLOBackend::infer_batch(
    const std::vector<Frame>& frames, int concurrency) {
    (void)concurrency;  // 使用内部流水线配置，不使用此参数
    
    std::vector<InferenceResult> results(frames.size());
    
    if (frames.empty() || !is_loaded_) {
        return results;
    }
    
    // ★ 优化: 使用异步预处理流水线
    // 当 NPU 推理时，CPU 同时准备下一帧
    const bool use_pipeline = (frames.size() > 1);
    
#ifdef USE_K3_FFMPEG
    // ★ 重置计数器，确保打印批量推理的前几帧时间
    impl_->ffmpeg_success_count.store(0);
    impl_->ffmpeg_fail_count.store(0);
#endif
    
    if (use_pipeline) {
        // ============================================================
        // ★★★ 三级异步流水线 (100% NPU 效率目标) ★★★
        // ============================================================
        // Stage 1: 预处理线程池 → Queue1 → Stage 2: NPU 推理 → Queue2 → Stage 3: 后处理线程池
        
        // ============================================================
        // ★ 队列大小和线程数配置 (基于 CPU 线程数自动调整)
        // ============================================================
        // 队列深度: 足够大以隐藏延迟，保证 NPU 不等待
        size_t PREPROC_QUEUE_SIZE = 24;   // ★ 增大: 16→24 (双 Session 需要更深的队列)
        size_t POSTPROC_QUEUE_SIZE = 24;  // ★ 增大: 16→24
        const char* env_queue = std::getenv("YOLO_QUEUE_SIZE");
        if (env_queue) {
            int q = std::max(4, std::min(32, std::atoi(env_queue)));
            PREPROC_QUEUE_SIZE = q;
            POSTPROC_QUEUE_SIZE = q;
        }
        
        // ★ 线程数配置 (优化: 提高并行度)
        // FFmpeg 硬件解码: 预处理线程数 = 3 (一个解码，两个等待 mutex，充分隐藏延迟)
        // OpenCV 软件解码: 预处理线程数 = CPU 核心数 (真正并行)
        int num_postproc_threads = std::max(4, impl_->cpu_threads / 2);  // ★ 增加: 4→6
        
#ifdef USE_K3_FFMPEG
        // ★ FFmpeg 硬件解码时，3个线程充分隐藏 mutex 等待时间
        int num_preproc_threads = impl_->use_ffmpeg_preprocess ? 3 : impl_->cpu_threads;
#else
        int num_preproc_threads = impl_->cpu_threads;
#endif
        
        const char* env_preproc = std::getenv("YOLO_PREPROC_THREADS");
        const char* env_postproc = std::getenv("YOLO_POSTPROC_THREADS");
        if (env_preproc) {
            num_preproc_threads = std::max(1, std::min(8, std::atoi(env_preproc)));
        }
        if (env_postproc) {
            num_postproc_threads = std::max(1, std::min(8, std::atoi(env_postproc)));
        }
        
        // Pipeline 配置完成
        
        // ============================================================
        // Stage 1: 预处理队列
        // ============================================================
        std::queue<PreprocData> preproc_queue;
        std::mutex preproc_mutex;
        std::condition_variable preproc_has_data_cv;
        std::condition_variable preproc_has_space_cv;
        std::atomic<size_t> preproc_idx{0};
        std::atomic<int> preproc_active{0};
        
        // ============================================================
        // Stage 3: 后处理队列 (NPU 输出 → 后处理)
        // ============================================================
        struct PostprocData {
            size_t result_idx;
            std::shared_ptr<std::vector<float>> input_tensor;   // ★ 输入 tensor (用于归还内存池)
            std::shared_ptr<std::vector<float>> output_data;    // ★ 零拷贝: 使用内存池缓冲
            int64_t output_shape[3];
            cv::Mat original_img;
            int64_t frame_id;
            int64_t timestamp_us;
            double preprocess_ms;
            double inference_ms;
            Clock::time_point total_start;
            bool valid = false;
            std::string error;
        };
        
        std::queue<PostprocData> postproc_queue;
        std::mutex postproc_mutex;
        std::condition_variable postproc_has_data_cv;
        std::condition_variable postproc_has_space_cv;
        std::atomic<size_t> postproc_done{0};
        std::atomic<bool> inference_done{false};
        
        // ============================================================
        // 预处理 Worker
        // ============================================================
        auto preproc_worker = [&]() {
            while (true) {
                size_t idx = preproc_idx.fetch_add(1);
                if (idx >= frames.size()) break;
                
                preproc_active.fetch_add(1);
                auto data = impl_->do_preprocess(frames[idx]);
                preproc_active.fetch_sub(1);
                
                {
                    std::unique_lock<std::mutex> lock(preproc_mutex);
                    preproc_has_space_cv.wait(lock, [&]() {
                        return preproc_queue.size() < PREPROC_QUEUE_SIZE;
                    });
                    preproc_queue.push(std::move(data));
                }
                preproc_has_data_cv.notify_one();
            }
        };
        
        // ============================================================
        // 后处理 Worker (异步执行 NMS 等)
        // ============================================================
        auto postproc_worker = [&]() {
            while (true) {
                PostprocData data;
                {
                    std::unique_lock<std::mutex> lock(postproc_mutex);
                    postproc_has_data_cv.wait(lock, [&]() {
                        return !postproc_queue.empty() || inference_done.load();
                    });
                    if (postproc_queue.empty() && inference_done.load()) break;
                    if (postproc_queue.empty()) continue;
                    data = std::move(postproc_queue.front());
                    postproc_queue.pop();
                }
                postproc_has_space_cv.notify_one();
                
                InferenceResult& result = results[data.result_idx];
                result.frame_id = data.frame_id;
                result.timestamp_us = data.timestamp_us;
                result.preprocess_ms = data.preprocess_ms;
                result.inference_ms = data.inference_ms;
                
                if (!data.valid) {
                    result.error = data.error;
                    // ★ 归还内存池缓冲
                    impl_->input_tensor_pool->release(data.input_tensor);
                    impl_->output_tensor_pool->release(data.output_data);
                    postproc_done.fetch_add(1);
                    continue;
                }
                
                auto postprocess_start = Clock::now();
                
                int dim1 = static_cast<int>(data.output_shape[1]);
                int dim2 = static_cast<int>(data.output_shape[2]);
                bool transposed = (dim1 > dim2);
                int num_detections = transposed ? dim1 : dim2;
                int num_features = transposed ? dim2 : dim1;
                
                std::vector<cv::Rect> boxes;
                std::vector<float> confidences;
                std::vector<int> class_ids;
                
                float scale_x = static_cast<float>(data.original_img.cols) / impl_->input_width;
                float scale_y = static_cast<float>(data.original_img.rows) / impl_->input_height;
                const float* output_ptr = data.output_data->data();  // ★ 使用 shared_ptr
                
                for (int i = 0; i < num_detections; i++) {
                    float max_conf = 0;
                    int max_class = 0;
                    for (int c = 0; c < NUM_CLASSES; c++) {
                        float conf = transposed ? 
                            output_ptr[i * num_features + (4 + c)] :
                            output_ptr[(4 + c) * num_detections + i];
                        if (conf > max_conf) {
                            max_conf = conf;
                            max_class = c;
                        }
                    }
                    
                    if (max_conf < impl_->conf_threshold) continue;
                    
                    float cx, cy, w, h;
                    if (transposed) {
                        cx = output_ptr[i * num_features + 0];
                        cy = output_ptr[i * num_features + 1];
                        w  = output_ptr[i * num_features + 2];
                        h  = output_ptr[i * num_features + 3];
                    } else {
                        cx = output_ptr[0 * num_detections + i];
                        cy = output_ptr[1 * num_detections + i];
                        w  = output_ptr[2 * num_detections + i];
                        h  = output_ptr[3 * num_detections + i];
                    }
                    
                    int x1 = static_cast<int>((cx - w / 2) * scale_x);
                    int y1 = static_cast<int>((cy - h / 2) * scale_y);
                    int x2 = static_cast<int>((cx + w / 2) * scale_x);
                    int y2 = static_cast<int>((cy + h / 2) * scale_y);
                    
                    boxes.emplace_back(x1, y1, x2 - x1, y2 - y1);
                    confidences.push_back(max_conf);
                    class_ids.push_back(max_class);
                }
                
                std::vector<int> indices;
                cv::dnn::NMSBoxes(boxes, confidences, impl_->conf_threshold, impl_->nms_threshold, indices);
                
                for (int idx : indices) {
                    Detection det;
                    det.x1 = static_cast<float>(boxes[idx].x);
                    det.y1 = static_cast<float>(boxes[idx].y);
                    det.x2 = static_cast<float>(boxes[idx].x + boxes[idx].width);
                    det.y2 = static_cast<float>(boxes[idx].y + boxes[idx].height);
                    det.class_id = class_ids[idx];
                    det.class_name = (class_ids[idx] < NUM_CLASSES) ? COCO_NAMES[class_ids[idx]] : "unknown";
                    det.confidence = confidences[idx];
                    result.detections.push_back(det);
                }
                
                result.postprocess_ms = Duration(Clock::now() - postprocess_start).count();
                result.total_ms = Duration(Clock::now() - data.total_start).count() + data.preprocess_ms;
                result.success = true;
                
                // ★★★ 优化 5: 归还内存池缓冲 ★★★
                impl_->input_tensor_pool->release(data.input_tensor);
                impl_->output_tensor_pool->release(data.output_data);
                
                postproc_done.fetch_add(1);
            }
        };
        
        // ============================================================
        // 启动线程
        // ============================================================
        std::vector<std::thread> preproc_threads;
        std::vector<std::thread> postproc_threads;
        
        for (int t = 0; t < num_preproc_threads; t++) {
            preproc_threads.emplace_back(preproc_worker);
        }
        for (int t = 0; t < num_postproc_threads; t++) {
            postproc_threads.emplace_back(postproc_worker);
        }
        
        // ============================================================
        // ★★★ Stage 2: 双 Session 并行 NPU 推理 ★★★
        // ============================================================
        // 当 num_sessions=2 时，启动两个推理线程交替使用 NPU
        // Session1 推理时，Session2 准备数据；Session2 推理时，Session1 准备数据
        // 目标: NPU 100% 利用率
        // ============================================================
        
        const int num_infer_sessions = impl_->num_sessions;
        std::vector<int64_t> input_shape = {1, 3, impl_->input_height, impl_->input_height};
        
        // 推理任务索引 (原子操作保证线程安全)
        std::atomic<size_t> infer_task_idx{0};
        std::atomic<bool> all_preproc_done{false};
        
        // ★ 推理线程函数 (每个 Session 一个线程)
        auto infer_worker = [&](int session_id) {
            auto& session = *impl_->sessions[session_id];
            Ort::MemoryInfo memory_info = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);
            
            while (true) {
                PreprocData preproc_data;
                size_t task_idx;
                
                // 从预处理队列获取数据
                {
                    std::unique_lock<std::mutex> lock(preproc_mutex);
                    preproc_has_data_cv.wait(lock, [&]() {
                        return !preproc_queue.empty() || 
                               (preproc_idx.load() >= frames.size() && preproc_active.load() == 0);
                    });
                    if (preproc_queue.empty()) {
                        all_preproc_done.store(true);
                        preproc_has_data_cv.notify_all();  // ★ 修复: 通知所有等待的推理线程退出
                        break;
                    }
                    preproc_data = std::move(preproc_queue.front());
                    preproc_queue.pop();
                    task_idx = infer_task_idx.fetch_add(1);
                }
                preproc_has_space_cv.notify_one();
                
                PostprocData postproc_data;
                postproc_data.result_idx = task_idx;
                postproc_data.frame_id = preproc_data.frame_id;
                postproc_data.timestamp_us = preproc_data.timestamp_us;
                postproc_data.preprocess_ms = preproc_data.preprocess_ms;
                postproc_data.original_img = std::move(preproc_data.original_img);
                postproc_data.input_tensor = preproc_data.tensor;  // ★ 保存输入 tensor 用于后续归还
                postproc_data.total_start = Clock::now();
                
                if (!preproc_data.valid) {
                    postproc_data.valid = false;
                    postproc_data.error = preproc_data.error;
                } else {
                    try {
                        // ★★★ NPU 推理 (Session[session_id]) ★★★
                        auto inference_start = Clock::now();
                        
                        Ort::Value input_tensor_ort = Ort::Value::CreateTensor<float>(
                            memory_info, preproc_data.tensor->data(), preproc_data.tensor->size(),
                            input_shape.data(), input_shape.size());
                        
                        auto output_tensors = session.Run(Ort::RunOptions{nullptr},
                            impl_->input_names.data(), &input_tensor_ort, 1,
                            impl_->output_names.data(), impl_->output_names.size());
                        
                        postproc_data.inference_ms = Duration(Clock::now() - inference_start).count();
                        
                        // ★★★ 优化 5: 零拷贝输出 - 使用内存池缓冲 ★★★
                        auto output_shape = output_tensors[0].GetTensorTypeAndShapeInfo().GetShape();
                        if (output_shape.size() >= 3) {
                            postproc_data.output_shape[0] = output_shape[0];
                            postproc_data.output_shape[1] = output_shape[1];
                            postproc_data.output_shape[2] = output_shape[2];
                            
                            const float* src = output_tensors[0].GetTensorData<float>();
                            size_t total = output_shape[0] * output_shape[1] * output_shape[2];
                            
                            // ★ 从内存池获取输出缓冲，而非每次分配
                            postproc_data.output_data = impl_->output_tensor_pool->acquire();
                            postproc_data.output_data->resize(total);
                            std::memcpy(postproc_data.output_data->data(), src, total * sizeof(float));
                            postproc_data.valid = true;
                        } else {
                            postproc_data.valid = false;
                            postproc_data.error = "Invalid output shape";
                        }
                        
                        // 更新推理计数
                        impl_->session_infer_count[session_id].fetch_add(1);
                        
                    } catch (const std::exception& e) {
                        postproc_data.valid = false;
                        postproc_data.error = e.what();
                    }
                }
                
                // 推送到后处理队列
                {
                    std::unique_lock<std::mutex> lock(postproc_mutex);
                    postproc_has_space_cv.wait(lock, [&]() {
                        return postproc_queue.size() < POSTPROC_QUEUE_SIZE;
                    });
                    postproc_queue.push(std::move(postproc_data));
                }
                postproc_has_data_cv.notify_one();
            }
        };
        
        // ★ 启动推理线程 (双 Session 模式启动 2 个线程)
        std::vector<std::thread> infer_threads;
        for (int s = 0; s < num_infer_sessions; s++) {
            infer_threads.emplace_back(infer_worker, s);
        }
        
        // 等待所有推理线程完成
        for (int i = 0; i < (int)infer_threads.size(); i++) {
            if (infer_threads[i].joinable()) {
                infer_threads[i].join();
            }
        }
        
        // 清理
        inference_done.store(true);
        postproc_has_data_cv.notify_all();
        
        for (auto& t : preproc_threads) {
            if (t.joinable()) t.join();
        }
        for (auto& t : postproc_threads) {
            if (t.joinable()) t.join();
        }
        
    } else {
        // 单帧: 直接调用 infer
        for (size_t i = 0; i < frames.size(); i++) {
            results[i] = infer(frames[i]);
        }
    }
    
    return results;
}

BackendInfo LocalYOLOBackend::info() const {
    BackendInfo info;
    info.name = "LocalYOLO";
    info.version = "2.0.0";  // 双 Session 版本
    info.backend_type = "local";
    info.model_name = config_.model_path;
    info.workers = num_workers_;
    info.is_loaded = is_loaded_;
    // 附加双 Session 信息
    if (impl_->num_sessions > 1) {
        info.backend_type = "local_dual_session";
    }
    return info;
}

double LocalYOLOBackend::get_memory_usage_mb() const {
    // 读取 /proc/self/statm
    std::ifstream statm("/proc/self/statm");
    if (statm) {
        long pages;
        statm >> pages;  // total
        statm >> pages;  // resident
        return pages * 4096.0 / (1024 * 1024);  // 假设 4KB 页
    }
    return 0;
}

void LocalYOLOBackend::shutdown() {
    // ★ 与 yolo-server 一致: 简单清理，不捕获异常
    impl_->sessions.clear();
    is_loaded_ = false;
}

// ============================================================
// InferenceBackend 默认实现
// ============================================================
std::vector<InferenceResult> InferenceBackend::infer_batch(
    const std::vector<Frame>& frames, int concurrency) {
    (void)concurrency;  // 默认实现不使用并发参数
    
    std::vector<InferenceResult> results;
    results.reserve(frames.size());
    
    for (const auto& frame : frames) {
        results.push_back(infer(frame));
    }
    
    return results;
}

std::future<InferenceResult> InferenceBackend::infer_async(const Frame& frame) {
    return std::async(std::launch::async, [this, frame]() {
        return infer(frame);
    });
}

// ============================================================
// InferenceBackend 工厂方法
// ============================================================
std::unique_ptr<InferenceBackend> InferenceBackend::create(const std::string& type) {
    if (type == "local_yolo") {
        return std::make_unique<LocalYOLOBackend>();
    } else if (type == "local_vlm") {
        return std::make_unique<LocalVLMBackend>();
    } else if (type == "http_yolo") {
        return std::make_unique<HTTPYOLOBackend>();
    } else if (type == "http_vlm") {
        return std::make_unique<HTTPVLMBackend>();
    }
    return nullptr;
}

}  // namespace benchmark
}  // namespace rivision
