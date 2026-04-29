#pragma once

#include "benchmark_common.h"
#include <mutex>
#include <atomic>
#include <memory>
#include <map>
#include <fstream>
#include <thread>

namespace rivision {
namespace benchmark {

// ============================================================
// 直方图
// ============================================================
class Histogram {
public:
    Histogram(const std::vector<double>& buckets = {10, 20, 50, 100, 200, 500, 1000, 2000, 5000});
    
    void add(double value);
    void reset();
    
    // 获取各桶计数
    std::map<double, int64_t> get_buckets() const;
    
    // 获取统计
    Statistics get_statistics() const;
    
    // 百分位数
    double percentile(double p) const;

private:
    std::vector<double> bucket_bounds_;
    std::unique_ptr<std::atomic<int64_t>[]> bucket_counts_;
    size_t bucket_count_size_ = 0;
    
    mutable std::mutex mutex_;
    std::vector<double> all_values_;
    
    std::atomic<double> sum_{0};
    std::atomic<double> sum_sq_{0};
    std::atomic<int64_t> count_{0};
    std::atomic<double> min_{1e18};
    std::atomic<double> max_{0};
};

// ============================================================
// 资源监控器
// ============================================================
class ResourceMonitor {
public:
    ResourceMonitor();
    ~ResourceMonitor();
    
    void start();
    void stop();
    
    // 获取当前资源使用
    double get_cpu_percent() const;
    double get_memory_mb() const;
    double get_memory_percent() const;
    
    // 获取峰值
    double get_peak_memory_mb() const { return peak_memory_mb_; }
    double get_avg_cpu_percent() const;
    
    // NPU 利用率 (K3 SpacemiT)
    double get_npu_utilization() const;

private:
    void run_monitor();
    
    std::thread monitor_thread_;
    std::atomic<bool> running_{false};
    
    std::atomic<double> current_cpu_{0};
    std::atomic<double> current_memory_mb_{0};
    std::atomic<double> peak_memory_mb_{0};
    
    std::vector<double> cpu_samples_;
    mutable std::mutex samples_mutex_;
    
    // CPU 计算用
    int64_t prev_total_ = 0;
    int64_t prev_idle_ = 0;
};

// ============================================================
// 指标收集器
// ============================================================
class MetricsCollector {
public:
    MetricsCollector();
    ~MetricsCollector();
    
    // 开始/停止收集
    void start(const std::string& test_name);
    void stop();
    void reset();
    
    // 记录推理结果
    void record(const InferenceResult& result);
    
    // 记录批量结果
    void record_batch(const std::vector<InferenceResult>& results);
    
    // 实时显示
    void enable_realtime_display(bool enable) { realtime_display_ = enable; }
    void display_progress();
    
    // 获取结果
    BenchmarkResult get_result() const;
    
    // 导出
    void export_json(const std::string& filepath) const;
    void export_csv(const std::string& filepath) const;
    void export_markdown(std::ostream& os) const;
    
    // 直方图访问
    const Histogram& latency_histogram() const { return latency_hist_; }
    const Histogram& inference_histogram() const { return inference_hist_; }

private:
    std::string test_name_;
    TimePoint start_time_;
    TimePoint end_time_;
    
    // 计数器
    std::atomic<int64_t> total_count_{0};
    std::atomic<int64_t> success_count_{0};
    std::atomic<int64_t> error_count_{0};
    
    // 直方图
    Histogram latency_hist_;
    Histogram inference_hist_;
    Histogram ttft_hist_;
    Histogram tps_hist_;  // tokens per second
    
    // 累计值
    std::atomic<double> total_latency_ms_{0};
    std::atomic<double> total_inference_ms_{0};
    std::atomic<int64_t> total_tokens_{0};
    
    // 资源监控
    ResourceMonitor resource_monitor_;
    
    // 配置
    bool realtime_display_ = true;
    std::atomic<bool> running_{false};
    
    // 上次显示时间
    mutable TimePoint last_display_time_;
    mutable std::mutex display_mutex_;
};

// ============================================================
// 多测试结果聚合器
// ============================================================
class ResultAggregator {
public:
    void add_result(const BenchmarkResult& result);
    
    // 获取汇总
    std::vector<BenchmarkResult> get_all_results() const { return results_; }
    
    // 导出报告
    void export_report(const std::string& output_dir) const;
    void print_summary() const;
    
    // 对比分析
    void compare_backends(std::ostream& os) const;
    void compare_concurrency(std::ostream& os) const;

private:
    std::vector<BenchmarkResult> results_;
    mutable std::mutex mutex_;
};

}  // namespace benchmark
}  // namespace rivision
