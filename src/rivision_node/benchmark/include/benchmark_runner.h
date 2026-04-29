#pragma once

#include "benchmark_common.h"
#include "data_source.h"
#include "inference_backend.h"
#include "metrics_collector.h"
#include <memory>
#include <atomic>

namespace rivision {
namespace benchmark {

// ============================================================
// 测试模式
// ============================================================
enum class BenchmarkMode {
    SINGLE_FRAME,       // 单帧测试 (延迟测量)
    MAX_THROUGHPUT,     // 最大吞吐测试
    FIXED_FPS,          // 固定帧率测试
    STRESS_TEST,        // 压力测试 (长时间运行)
    LATENCY_PROFILE,    // 延迟分布测试
    CONCURRENCY_SWEEP   // 并发数扫描
};

// ============================================================
// 测试运行器配置
// ============================================================
struct RunnerConfig {
    BenchmarkMode mode = BenchmarkMode::MAX_THROUGHPUT;
    
    // 基本参数
    int warmup_runs = 5;
    int main_runs = 100;
    int cooldown_sec = 2;
    
    // 并发
    int concurrency = 1;
    std::vector<int> concurrency_sweep = {1, 2, 4, 8};
    
    // 固定帧率模式
    int target_fps = 30;
    
    // 压力测试
    int stress_duration_sec = 3600;  // 1小时
    
    // 输出
    std::string output_dir = "./results";
    bool realtime_display = true;
    bool save_json = true;
    bool save_csv = true;
};

// ============================================================
// 基准测试运行器
// ============================================================
class BenchmarkRunner {
public:
    BenchmarkRunner();
    ~BenchmarkRunner();
    
    // 设置组件
    void set_data_source(std::unique_ptr<DataSource> source);
    void set_backend(std::unique_ptr<InferenceBackend> backend);
    void set_config(const RunnerConfig& config);
    
    // 运行测试
    BenchmarkResult run();
    BenchmarkResult run_single_frame();
    BenchmarkResult run_max_throughput();
    BenchmarkResult run_fixed_fps();
    BenchmarkResult run_stress_test();
    std::vector<BenchmarkResult> run_concurrency_sweep();
    
    // 控制
    void stop();
    bool is_running() const { return running_; }
    
    // 进度回调
    void set_progress_callback(ProgressCallback callback);
    
    // 获取指标收集器
    MetricsCollector& metrics() { return metrics_; }

private:
    // 预热
    void warmup();
    
    // 冷却
    void cooldown();
    
    // 单次推理并记录
    void infer_and_record(const Frame& frame);
    
    // 并发推理
    void run_concurrent(int concurrency, int total_runs);
    
    std::unique_ptr<DataSource> source_;
    std::unique_ptr<InferenceBackend> backend_;
    RunnerConfig config_;
    MetricsCollector metrics_;
    
    ProgressCallback progress_callback_;
    
    std::atomic<bool> running_{false};
    std::atomic<bool> stop_requested_{false};
};

// ============================================================
// YOLO 基准测试器
// ============================================================
class YOLOBenchmark {
public:
    YOLOBenchmark();
    ~YOLOBenchmark();
    
    // 配置
    void set_image(const std::string& image_path);
    void set_video(const std::string& video_path);
    void set_config(const BenchmarkConfig& config);
    
    // 本地推理测试
    BenchmarkResult run_local(const std::string& model_path, int threads = 4);
    
    // HTTP 服务测试
    BenchmarkResult run_http(const std::string& url, int concurrency = 1);
    std::vector<BenchmarkResult> run_http_concurrency_sweep(
        const std::string& url, const std::vector<int>& concurrency_levels);
    
    // 完整测试 (本地 + HTTP)
    void run_all(ResultAggregator& aggregator);

private:
    BenchmarkConfig config_;
    std::string image_path_;
    std::string video_path_;
};

// ============================================================
// VLM 基准测试器
// ============================================================
class VLMBenchmark {
public:
    VLMBenchmark();
    ~VLMBenchmark();
    
    // 配置
    void set_image(const std::string& image_path);
    void set_prompt(const std::string& prompt);
    void set_config(const BenchmarkConfig& config);
    
    // 本地推理测试
    BenchmarkResult run_local(const std::string& model_path, 
                              const std::string& mmproj_path,
                              int threads = 6);
    
    // HTTP 服务测试
    BenchmarkResult run_http(const std::string& url);
    
    // 完整测试
    void run_all(ResultAggregator& aggregator);

private:
    BenchmarkConfig config_;
    std::string image_path_;
    std::string prompt_ = "Describe this image in detail.";
};

// ============================================================
// 工具函数
// ============================================================

// 打印平台信息 (可选传入模型路径)
void print_platform_info(const std::string& model_path = "");

// 打印测试结果
void print_result(const BenchmarkResult& result);

// 打印对比表格
void print_comparison_table(const std::vector<BenchmarkResult>& results);

// 计算多路视频支持能力
void print_multi_stream_capacity(double max_fps, const std::vector<int>& target_fps = {30, 15, 10, 5});

}  // namespace benchmark
}  // namespace rivision
