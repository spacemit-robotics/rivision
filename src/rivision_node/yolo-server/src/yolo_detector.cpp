/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "yolo_detector.h"

#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstdio>
#include <memory>
#include <string>
#include <unordered_map>
#include <utility>
#include <vector>

#include "image_utils.h"

#ifdef USE_ONNX
#include <onnxruntime_cxx_api.h>
#ifdef USE_SPACEMIT_EP
#include "spacemit_ort_env.h"
#endif
#ifdef USE_CUDA_EP
#include <onnxruntime_c_api.h>
#endif
#endif

namespace yolo {

const std::vector<std::string> COCO_CLASSES = {
    "person",         "bicycle",    "car",           "motorcycle",    "airplane",     "bus",           "train",
    "truck",          "boat",       "traffic light", "fire hydrant",  "stop sign",    "parking meter", "bench",
    "bird",           "cat",        "dog",           "horse",         "sheep",        "cow",           "elephant",
    "bear",           "zebra",      "giraffe",       "backpack",      "umbrella",     "handbag",       "tie",
    "suitcase",       "frisbee",    "skis",          "snowboard",     "sports ball",  "kite",          "baseball bat",
    "baseball glove", "skateboard", "surfboard",     "tennis racket", "bottle",       "wine glass",    "cup",
    "fork",           "knife",      "spoon",         "bowl",          "banana",       "apple",         "sandwich",
    "orange",         "broccoli",   "carrot",        "hot dog",       "pizza",        "donut",         "cake",
    "chair",          "couch",      "potted plant",  "bed",           "dining table", "toilet",        "tv",
    "laptop",         "mouse",      "remote",        "keyboard",      "cell phone",   "microwave",     "oven",
    "toaster",        "sink",       "refrigerator",  "book",          "clock",        "vase",          "scissors",
    "teddy bear",     "hair drier", "toothbrush"};

static constexpr float CONF_THRESHOLD = 0.35f;  // ★ 提高置信度阈值，减少误检
static constexpr float NMS_THRESHOLD = 0.45f;
static constexpr int MAX_DETECTIONS_BEFORE_NMS = 300;  // ★ NMS 前最大检测数

// ★ 关键修复: 全局共享 Ort::Env (ONNX Runtime 要求进程级单例)
// 多个 Env 实例会导致资源竞争和 SpacemiT EP 初始化冲突
#ifdef USE_ONNX
static Ort::Env& get_global_env() {
    static Ort::Env env(ORT_LOGGING_LEVEL_WARNING, "yolo-server");
    return env;
}

// ★ SpacemiT EP 只初始化一次 (NPU 资源是全局的)
static std::atomic<bool> g_spacemit_initialized{false};
static std::mutex g_spacemit_init_mutex;
#endif

struct YOLODetector::Impl {
#ifdef USE_ONNX
    // 不再创建独立 Env，使用全局共享
    Ort::SessionOptions session_options;
    std::unique_ptr<Ort::Session> session;
    Ort::AllocatorWithDefaultOptions allocator;
    std::string input_name;
    std::string output_name;
    int64_t batch_size = 1;
    int64_t input_c = 3;
    int64_t input_h = 640;
    int64_t input_w = 640;
    // 动态输出形状
    int64_t out_dim1 = 84;
    int64_t out_dim2 = 8400;
    int num_classes = 80;
    int num_boxes = 8400;
#endif
    std::string model_path;
};

YOLODetector::YOLODetector() : impl_(std::make_unique<Impl>()) {}
YOLODetector::~YOLODetector() = default;

bool YOLODetector::load(const std::string& model_path) {
#ifdef USE_ONNX
    try {
        // ★ K3 SpacemiT EP: SPACEMIT_EP_INTRA_THREAD_NUM=4 → 4 个 A100 核心 (8-11)
        int intra_threads = 4;  // 默认 4 线程
        int inter_threads = 1;

        const char* env_intra = std::getenv("YOLO_THREADS");
        const char* env_inter = std::getenv("YOLO_INTER_THREADS");
        if (env_intra) {
            intra_threads = std::atoi(env_intra);
            if (intra_threads < 1) intra_threads = 1;
            if (intra_threads > 8) intra_threads = 8;  // K3 A100 最多 8 核心
        }
        if (env_inter) {
            inter_threads = std::atoi(env_inter);
            if (inter_threads < 1) inter_threads = 1;
            if (inter_threads > 4) inter_threads = 1;
        }

        // ★ SpacemiT EP: ORT 线程数设为 1，由 SPACEMIT_EP_INTRA_THREAD_NUM 控制
        // SPACEMIT_EP_INTRA_THREAD_NUM=4 → 4 个 A100 核心 (8-11)
#ifdef USE_SPACEMIT_EP
        impl_->session_options.SetIntraOpNumThreads(1);
        impl_->session_options.SetInterOpNumThreads(1);
#else
        impl_->session_options.SetIntraOpNumThreads(intra_threads);
        impl_->session_options.SetInterOpNumThreads(inter_threads);
#endif
        impl_->session_options.SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_ALL);

        // ★ P8: 禁用线程缓存清理，提高连续推理性能
        impl_->session_options.AddConfigEntry("session.use_env_allocators", "1");

        printf("[YOLODetector] YOLO_THREADS=%d (A100 cores)\n", intra_threads);

#ifdef USE_CUDA_EP
        // CUDA EP for NVIDIA Jetson Orin
        try {
            OrtCUDAProviderOptionsV2* cuda_options = nullptr;
            Ort::GetApi().CreateCUDAProviderOptions(&cuda_options);

            std::vector<const char*> keys = {"device_id", "arena_extend_strategy", "cudnn_conv_algo_search"};
            std::vector<const char*> values = {"0", "kNextPowerOfTwo", "EXHAUSTIVE"};
            Ort::GetApi().UpdateCUDAProviderOptions(cuda_options, keys.data(), values.data(), keys.size());

            impl_->session_options.AppendExecutionProvider_CUDA_V2(*cuda_options);
            Ort::GetApi().ReleaseCUDAProviderOptions(cuda_options);
            printf("[YOLODetector] \u2713 CUDA EP enabled (Jetson Orin GPU)\n");
        } catch (const Ort::Exception& e) {
            printf("[YOLODetector] \u2717 CUDA EP unavailable, using CPU fallback: %s\n", e.what());
        }
#elif defined(USE_SPACEMIT_EP)
        // SpacemiT EP for RISC-V K3 NPU
        // ★ SPACEMIT_EP_INTRA_THREAD_NUM=4 → 4 个 A100 核心 (8-11)
        // ★ SPACEMIT_EP_USE_GLOBAL_INTRA_THREAD=1 → 多 Session 共享线程池
        {
            std::lock_guard<std::mutex> lock(g_spacemit_init_mutex);
            std::unordered_map<std::string, std::string> provider_options;
            provider_options["SPACEMIT_EP_INTRA_THREAD_NUM"] = std::to_string(intra_threads);
            provider_options["SPACEMIT_EP_USE_GLOBAL_INTRA_THREAD"] = "1";

            try {
                SessionOptionsSpaceMITEnvInit(impl_->session_options, provider_options);
                if (!g_spacemit_initialized.load()) {
                    g_spacemit_initialized.store(true);
                    printf("[YOLODetector] SpacemiT EP initialized (threads=%d, global_pool=ON)\n", intra_threads);
                } else {
                    printf("[YOLODetector] SpacemiT EP session added (threads=%d, global_pool=ON)\n", intra_threads);
                }
            } catch (const Ort::Exception& e) {
                printf("[YOLODetector] SpacemiT EP error: %s\n", e.what());
            }
        }
#else
        printf("[YOLODetector] No hardware EP compiled, using CPU inference\n");
#endif

        // ★ 关键修复: 使用全局共享 Env 创建 Session
        impl_->session = std::make_unique<Ort::Session>(get_global_env(), model_path.c_str(), impl_->session_options);

        // 缓存输入/输出名称
        auto input_name_ptr = impl_->session->GetInputNameAllocated(0, impl_->allocator);
        auto output_name_ptr = impl_->session->GetOutputNameAllocated(0, impl_->allocator);
        impl_->input_name = input_name_ptr.get();
        impl_->output_name = output_name_ptr.get();

        // 查询模型输入形状
        auto input_info = impl_->session->GetInputTypeInfo(0);
        auto tensor_info = input_info.GetTensorTypeAndShapeInfo();
        auto shape = tensor_info.GetShape();
        if (shape.size() == 4) {
            impl_->batch_size = shape[0];
            impl_->input_c = shape[1];
            impl_->input_h = shape[2];
            impl_->input_w = shape[3];
        }

        // 查询模型输出形状
        auto out_info = impl_->session->GetOutputTypeInfo(0);
        auto out_tensor = out_info.GetTensorTypeAndShapeInfo();
        auto out_shape = out_tensor.GetShape();
        if (out_shape.size() == 3) {
            impl_->out_dim1 = out_shape[1];
            impl_->out_dim2 = out_shape[2];
        }

        printf("[YOLODetector] Model: %s\n", model_path.c_str());
        printf("[YOLODetector] Input: %s [%ld,%ld,%ld,%ld]\n", impl_->input_name.c_str(), impl_->batch_size,
               impl_->input_c, impl_->input_h, impl_->input_w);
        printf("[YOLODetector] Output: %s [%ld,%ld]\n", impl_->output_name.c_str(), impl_->out_dim1, impl_->out_dim2);

        impl_->model_path = model_path;
        loaded_ = true;
        return true;
    } catch (const Ort::Exception& e) {
        printf("[YOLODetector] Failed to load: %s\n", e.what());
        return false;
    }
#else
    printf("[YOLODetector] ONNX Runtime not available (stub mode)\n");
    impl_->model_path = model_path;
    loaded_ = true;
    return true;
#endif
}

std::vector<float> YOLODetector::preprocess(const std::vector<uint8_t>& jpeg_data, int& orig_width, int& orig_height) {
    image::ImageData img;
    if (!image::decode_jpeg(jpeg_data, img)) return {};
    orig_width = img.width;
    orig_height = img.height;
    return image::resize_and_normalize(img, impl_->input_w, impl_->input_h);
}

std::vector<Detection> YOLODetector::postprocess(const std::vector<float>& output, int orig_width, int orig_height) {
    std::vector<Detection> detections;

    // ★ 修复：动态计算 num_boxes 和 num_classes（从实际输出形状推导）
    // YOLOv8/11 输出: [batch, 4+num_classes, num_boxes]
    // out_dim1 = 4 + num_classes, out_dim2 = num_boxes
    int num_classes = static_cast<int>(impl_->out_dim1 - 4);
    int num_boxes = static_cast<int>(impl_->out_dim2);

    // ★ 安全检查：防止异常输出形状
    if (num_classes <= 0 || num_classes > 1000 || num_boxes <= 0 || num_boxes > 50000) {
        printf("[YOLO] ⚠️ 异常输出形状: classes=%d, boxes=%d\n", num_classes, num_boxes);
        return detections;
    }

    // ★ 验证输出大小
    size_t expected_size = static_cast<size_t>((4 + num_classes) * num_boxes);
    if (output.size() < expected_size) {
        printf("[YOLO] ⚠️ 输出大小不匹配: got %zu, expected %zu\n", output.size(), expected_size);
        return detections;
    }

    // Letterbox 参数还原
    float scale = std::min(static_cast<float>(impl_->input_w) / orig_width,
                          static_cast<float>(impl_->input_h) / orig_height);
    float new_w = orig_width * scale;
    float new_h = orig_height * scale;
    float pad_x = (impl_->input_w - new_w) / 2.0f;
    float pad_y = (impl_->input_h - new_h) / 2.0f;

    for (int i = 0; i < num_boxes; i++) {
        float max_conf = 0;
        int max_class = 0;
        for (int c = 0; c < num_classes; c++) {
            float conf = output[(4 + c) * num_boxes + i];
            if (conf > max_conf) {
                max_conf = conf;
                max_class = c;
            }
        }
        if (max_conf < CONF_THRESHOLD) continue;

        float cx = output[0 * num_boxes + i];
        float cy = output[1 * num_boxes + i];
        float w = output[2 * num_boxes + i];
        float h = output[3 * num_boxes + i];

        Detection det;
        det.x1 = (cx - w / 2 - pad_x) / new_w;
        det.y1 = (cy - h / 2 - pad_y) / new_h;
        det.x2 = (cx + w / 2 - pad_x) / new_w;
        det.y2 = (cy + h / 2 - pad_y) / new_h;

        det.x1 = std::max(0.0f, std::min(1.0f, det.x1));
        det.y1 = std::max(0.0f, std::min(1.0f, det.y1));
        det.x2 = std::max(0.0f, std::min(1.0f, det.x2));
        det.y2 = std::max(0.0f, std::min(1.0f, det.y2));

        // ★ 过滤无效框：太小(<1%)或太大(>95%)
        float box_w = det.x2 - det.x1;
        float box_h = det.y2 - det.y1;
        if (box_w < 0.01f || box_h < 0.01f) continue;  // 太小
        if (box_w > 0.95f || box_h > 0.95f) continue;  // 太大（几乎满屏）

        det.confidence = max_conf;
        det.class_id = max_class;
        det.class_name = (max_class < static_cast<int>(COCO_CLASSES.size())) ? COCO_CLASSES[max_class] : "unknown";
        detections.push_back(det);

        // ★ 限制 NMS 前的检测数量，防止 O(n²) 爆炸
        if (detections.size() >= MAX_DETECTIONS_BEFORE_NMS) break;
    }

    // ★ 警告：检测数量异常多
    if (detections.size() > 100) {
        printf("[YOLO] ⚠️ 检测数量异常: %zu (阈值前), 可能是模型输出问题\n", detections.size());
    }

    // NMS - 按置信度排序
    std::sort(detections.begin(), detections.end(),
              [](const Detection& a, const Detection& b) { return a.confidence > b.confidence; });

    std::vector<Detection> result;
    result.reserve(50);  // ★ 预分配，避免重复扩容
    std::vector<bool> suppressed(detections.size(), false);

    for (size_t i = 0; i < detections.size(); i++) {
        if (suppressed[i]) continue;
        result.push_back(detections[i]);

        // ★ 限制最终结果数量
        if (result.size() >= 50) break;

        for (size_t j = i + 1; j < detections.size(); j++) {
            if (suppressed[j] || detections[j].class_id != detections[i].class_id) continue;
            float ix1 = std::max(detections[i].x1, detections[j].x1);
            float iy1 = std::max(detections[i].y1, detections[j].y1);
            float ix2 = std::min(detections[i].x2, detections[j].x2);
            float iy2 = std::min(detections[i].y2, detections[j].y2);
            float inter = std::max(0.0f, ix2 - ix1) * std::max(0.0f, iy2 - iy1);
            float area_i = (detections[i].x2 - detections[i].x1) * (detections[i].y2 - detections[i].y1);
            float area_j = (detections[j].x2 - detections[j].x1) * (detections[j].y2 - detections[j].y1);
            if (inter / (area_i + area_j - inter + 1e-6f) > NMS_THRESHOLD) suppressed[j] = true;
        }
    }
    return result;
}

DetectionResult YOLODetector::detect(const std::vector<uint8_t>& jpeg_data) {
    DetectionResult result;
    result.success = false;

    if (!loaded_) {
        result.error = "Model not loaded";
        return result;
    }

    auto start = std::chrono::high_resolution_clock::now();

    int orig_width, orig_height;
    auto input_tensor = preprocess(jpeg_data, orig_width, orig_height);
    if (input_tensor.empty()) {
        result.error = "Failed to preprocess image";
        return result;
    }

#ifdef USE_ONNX
    try {
        int64_t bs = impl_->batch_size;
        std::array<int64_t, 4> input_shape = {bs, 3, impl_->input_h, impl_->input_w};
        auto memory_info = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);

        // batch > 1 时复制填充
        std::vector<float> batch_data;
        float* tensor_data = input_tensor.data();
        size_t tensor_size = input_tensor.size();

        if (bs > 1) {
            size_t single = input_tensor.size();
            batch_data.resize(single * bs);
            for (int64_t b = 0; b < bs; b++)
                std::copy(input_tensor.begin(), input_tensor.end(), batch_data.begin() + b * single);
            tensor_data = batch_data.data();
            tensor_size = batch_data.size();
        }

        auto input = Ort::Value::CreateTensor<float>(memory_info, tensor_data, tensor_size, input_shape.data(),
                                                     input_shape.size());

        const char* input_names[] = {impl_->input_name.c_str()};
        const char* output_names[] = {impl_->output_name.c_str()};

        auto outputs = impl_->session->Run(Ort::RunOptions{nullptr}, input_names, &input, 1, output_names, 1);

        auto& output_tensor = outputs[0];
        const float* output_data = output_tensor.GetTensorData<float>();
        auto output_shape = output_tensor.GetTensorTypeAndShapeInfo().GetShape();

        size_t output_size = 1;
        for (auto dim : output_shape) output_size *= dim;

        // 只取第一个 batch 的输出
        size_t single_batch_size = output_size / bs;
        std::vector<float> output_vec(output_data, output_data + single_batch_size);
        result.detections = postprocess(output_vec, orig_width, orig_height);
        result.success = true;
#else
    Detection stub_det;
    stub_det.x1 = 0.1f;
    stub_det.y1 = 0.2f;
    stub_det.x2 = 0.3f;
    stub_det.y2 = 0.4f;
    stub_det.confidence = 0.85f;
    stub_det.class_id = 0;
    stub_det.class_name = "person";
    result.detections.push_back(stub_det);
    result.success = true;
#endif
    } catch (const Ort::Exception& e) {
        result.error = std::string("ONNX inference failed: ") + e.what();
    } catch (const std::exception& e) {
        result.error = std::string("Detection failed: ") + e.what();
    } catch (...) {
        result.error = "Detection failed: unknown error";
    }

    auto end = std::chrono::high_resolution_clock::now();
    result.inference_ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();

    return result;
}

// ============================================================
// ★ 从预处理张量直接推理 (用于流水线模式，跳过预处理)
// ============================================================
DetectionResult YOLODetector::detect_from_tensor(const std::vector<float>& tensor, int orig_width, int orig_height) {
    DetectionResult result;
    result.success = false;

    if (!loaded_) {
        result.error = "Model not loaded";
        return result;
    }

    if (tensor.empty()) {
        result.error = "Empty tensor";
        return result;
    }

    auto start = std::chrono::high_resolution_clock::now();

#ifdef USE_ONNX
    try {
        int64_t bs = impl_->batch_size;
        std::array<int64_t, 4> input_shape = {bs, 3, impl_->input_h, impl_->input_w};
        auto memory_info = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);

        // 直接使用传入的张量
        std::vector<float> batch_data;
        const float* tensor_data = tensor.data();
        size_t tensor_size = tensor.size();

        if (bs > 1) {
            batch_data.resize(tensor_size * bs);
            for (int64_t b = 0; b < bs; b++) {
                std::copy(tensor.begin(), tensor.end(), batch_data.begin() + b * tensor_size);
            }
            tensor_data = batch_data.data();
            tensor_size = batch_data.size();
        }

        Ort::Value input_tensor = Ort::Value::CreateTensor<float>(memory_info, const_cast<float*>(tensor_data),
                                                                  tensor_size, input_shape.data(), input_shape.size());

        const char* input_names[] = {impl_->input_name.c_str()};
        const char* output_names[] = {impl_->output_name.c_str()};

        auto outputs = impl_->session->Run(Ort::RunOptions{nullptr}, input_names, &input_tensor, 1, output_names, 1);

        auto& out_tensor = outputs.front();
        float* output_data = out_tensor.GetTensorMutableData<float>();
        size_t output_size = out_tensor.GetTensorTypeAndShapeInfo().GetElementCount();

        size_t single_batch_size = output_size / bs;
        std::vector<float> output_vec(output_data, output_data + single_batch_size);
        result.detections = postprocess(output_vec, orig_width, orig_height);
        result.success = true;
    } catch (const std::exception& e) {
        result.error = std::string("Inference failed: ") + e.what();
    }
#else
    result.success = true;
#endif

    auto end = std::chrono::high_resolution_clock::now();
    result.inference_ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();

    return result;
}

// ============================================================
// ★ P7: YOLOWorkerPool 实现 - 多 Session 并发推理
// ============================================================

YOLOWorkerPool::YOLOWorkerPool(int num_workers)
    : num_workers_(num_workers > 0 ? num_workers : 2), detector_mutexes_(num_workers_ > 0 ? num_workers_ : 2) {
    printf("[YOLOWorkerPool] 初始化 %d 个并发 worker\n", num_workers_);
}

YOLOWorkerPool::~YOLOWorkerPool() { printf("[YOLOWorkerPool] 销毁 %d 个 worker\n", num_workers_); }

bool YOLOWorkerPool::load(const std::string& model_path) {
    model_path_ = model_path;
    detectors_.clear();
    detectors_.reserve(num_workers_);

    printf("[YOLOWorkerPool] 加载 %d 个独立 Session...\n", num_workers_);

    for (int i = 0; i < num_workers_; i++) {
        auto detector = std::make_unique<YOLODetector>();
        if (!detector->load(model_path)) {
            printf("[YOLOWorkerPool] ❌ Worker %d 加载失败\n", i);
            return false;
        }
        detectors_.push_back(std::move(detector));
        printf("[YOLOWorkerPool] ✓ Worker %d 就绪\n", i);
    }

    loaded_ = true;
    printf("[YOLOWorkerPool] ✓ 所有 %d 个 Session 加载完成\n", num_workers_);
    return true;
}

DetectionResult YOLOWorkerPool::detect(const std::vector<uint8_t>& jpeg_data) {
    if (!loaded_ || detectors_.empty()) {
        DetectionResult result;
        result.success = false;
        result.error = "WorkerPool not loaded";
        return result;
    }

    // Round-robin 选择 worker (无锁快速路径)
    int worker_id = next_worker_.fetch_add(1) % num_workers_;

    // 尝试获取该 worker 的锁，如果被占用则尝试下一个
    int attempts = 0;
    while (attempts < num_workers_) {
        if (detector_mutexes_[worker_id].try_lock()) {
            active_workers_.fetch_add(1);

            // 执行推理
            auto result = detectors_[worker_id]->detect(jpeg_data);

            active_workers_.fetch_sub(1);
            detector_mutexes_[worker_id].unlock();
            return result;
        }
        // 该 worker 忙，尝试下一个
        worker_id = (worker_id + 1) % num_workers_;
        attempts++;
    }

    // 所有 worker 都忙，等待第一个可用的
    worker_id = next_worker_.fetch_add(1) % num_workers_;
    std::lock_guard<std::mutex> lock(detector_mutexes_[worker_id]);
    active_workers_.fetch_add(1);

    auto result = detectors_[worker_id]->detect(jpeg_data);

    active_workers_.fetch_sub(1);
    return result;
}

}  // namespace yolo
