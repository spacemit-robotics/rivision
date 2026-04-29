#pragma once

#include <string>
#include <vector>
#include <chrono>
#include <cstdint>
#include <functional>
#include <memory>
#include <atomic>
#include <mutex>

namespace rivision {
namespace benchmark {

// ============================================================
// 时间工具
// ============================================================
using Clock = std::chrono::high_resolution_clock;
using TimePoint = Clock::time_point;
using Duration = std::chrono::duration<double, std::milli>;

inline int64_t now_ms() {
    return std::chrono::duration_cast<std::chrono::milliseconds>(
        Clock::now().time_since_epoch()).count();
}

inline int64_t now_us() {
    return std::chrono::duration_cast<std::chrono::microseconds>(
        Clock::now().time_since_epoch()).count();
}

// ============================================================
// 帧数据结构
// ============================================================
struct Frame {
    std::vector<uint8_t> data;      // 图像数据 (JPEG/Raw)
    int width = 0;
    int height = 0;
    int channels = 3;
    int64_t timestamp_us = 0;       // 帧时间戳
    int64_t frame_id = 0;           // 帧序号
    bool is_jpeg = true;            // true=JPEG, false=Raw RGB
    
    bool empty() const { return data.empty(); }
    size_t size() const { return data.size(); }
};

// ============================================================
// 检测结果
// ============================================================
struct Detection {
    float x1, y1, x2, y2;           // bbox
    int class_id;
    std::string class_name;
    float confidence;
};

// ============================================================
// 推理结果
// ============================================================
struct InferenceResult {
    bool success = false;
    std::string error;
    
    // YOLO 结果
    std::vector<Detection> detections;
    
    // VLM 结果
    std::string text_output;
    int tokens_generated = 0;
    
    // 时间指标 (ms)
    double preprocess_ms = 0;
    double inference_ms = 0;
    double postprocess_ms = 0;
    double total_ms = 0;
    
    // VLM 特有
    double vision_encode_ms = 0;
    double prompt_eval_ms = 0;
    double token_gen_ms = 0;
    double ttft_ms = 0;             // 首 token 延迟
    double tokens_per_sec = 0;
    
    // 网络相关 (HTTP 模式)
    double network_ms = 0;
    int http_status = 0;
    
    // 元数据
    int64_t frame_id = 0;
    int64_t timestamp_us = 0;
};

// ============================================================
// 后端配置
// ============================================================
struct BackendConfig {
    std::string type;               // "local_yolo", "local_vlm", "http_yolo", "http_vlm"
    
    // 本地模式
    std::string model_path;
    std::string mmproj_path;        // VLM mmproj
    int cpu_threads = 8;            // ★ CPU 线程数 (预处理/后处理, 推荐8)
    int npu_threads = 4;            // ★ NPU 核心数 (A100: 当前可用4核)
    bool dual_session = true;       // ★ 双 Session (true=最高吞吐，4核也支持2+2配置)
    int batch_size = 1;
    float conf_threshold = 0.25f;
    float nms_threshold = 0.45f;
    
    // HTTP 模式
    std::string http_url;
    int http_timeout_ms = 30000;
    int http_pool_size = 8;
    
    // VLM 参数
    std::string prompt = "Describe this image.";
    int max_tokens = 512;
    float temperature = 0.3f;
};

// ============================================================
// 后端信息
// ============================================================
struct BackendInfo {
    std::string name;
    std::string version;
    std::string backend_type;       // "local" / "http"
    std::string model_name;
    int workers = 1;
    bool is_loaded = false;
};

// ============================================================
// 测试配置
// ============================================================
struct BenchmarkConfig {
    std::string name = "RiVision Benchmark";
    
    // 测试参数
    int warmup_runs = 5;
    int main_runs = 100;
    int cooldown_sec = 2;
    
    // 并发
    std::vector<int> concurrency_levels = {1, 2, 4, 8};
    
    // ★ 线程/核心配置 (最优默认值)
    // 测试验证: CPU_THREADS=4 + DUAL_SESSION=1 → 47.76 FPS
    int cpu_threads = 4;            // CPU 线程数 (4核足够，NPU是瓶颈)
    int npu_threads = 4;            // NPU 核心数 (A100: 当前可用4核)
    bool dual_session = true;       // ★ 双 Session (4核=2+2, 提高NPU利用率)
    
    // 数据源
    std::string source_type;        // "video_file", "video_stream", "image_folder"
    std::string source_path;
    int fps_limit = 0;              // 0 = 无限制
    bool loop = true;
    
    // 输出
    std::string output_dir = "./results";
    bool output_json = true;
    bool output_csv = true;
    bool realtime_display = true;
};

// ============================================================
// 统计结构
// ============================================================
struct Statistics {
    double min = 0;
    double max = 0;
    double avg = 0;
    double stddev = 0;
    double p50 = 0;
    double p95 = 0;
    double p99 = 0;
    int64_t count = 0;
    int64_t errors = 0;
    
    void compute(const std::vector<double>& values);
};

// ============================================================
// 基准测试结果
// ============================================================
struct BenchmarkResult {
    std::string test_name;
    std::string backend_type;
    int concurrency = 1;
    
    // 吞吐量
    double fps = 0;
    double total_time_sec = 0;
    int64_t total_frames = 0;
    int64_t success_count = 0;
    int64_t error_count = 0;
    
    // 延迟统计
    Statistics latency;
    Statistics inference_time;
    
    // VLM 特有
    Statistics ttft;
    Statistics tokens_per_sec;
    
    // 资源使用
    double peak_memory_mb = 0;
    double avg_cpu_percent = 0;
    
    // NPU 相关 (K3 A100)
    int npu_threads = 0;              // 使用的 NPU 核心数
    double npu_efficiency = 0;        // NPU 效率估算 (%)
    double model_memory_mb = 0;       // 模型内存占用
    std::string model_name;           // 模型名称
    
    // 时间戳
    std::string start_time;
    std::string end_time;
};

// ============================================================
// 回调函数类型
// ============================================================
using InferenceCallback = std::function<void(const InferenceResult&)>;
using ProgressCallback = std::function<void(int current, int total, const std::string& status)>;

}  // namespace benchmark
}  // namespace rivision
