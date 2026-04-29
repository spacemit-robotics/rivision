#include "metrics_collector.h"
#include <algorithm>
#include <cmath>
#include <iomanip>
#include <sstream>
#include <fstream>
#include <limits>
#include <thread>
#include <chrono>
#include <unistd.h>
#include <sys/utsname.h>
#include <cstdlib>

namespace rivision {
namespace benchmark {

// ============================================================
// Statistics 实现
// ============================================================
void Statistics::compute(const std::vector<double>& values) {
    if (values.empty()) return;
    
    count = static_cast<int64_t>(values.size());
    
    // 排序用于百分位数
    std::vector<double> sorted = values;
    std::sort(sorted.begin(), sorted.end());
    
    min = sorted.front();
    max = sorted.back();
    
    // 平均值
    double sum = 0;
    for (double v : sorted) sum += v;
    avg = sum / count;
    
    // 标准差
    double sq_sum = 0;
    for (double v : sorted) sq_sum += (v - avg) * (v - avg);
    stddev = std::sqrt(sq_sum / count);
    
    // 百分位数
    p50 = sorted[static_cast<size_t>(count * 0.50)];
    p95 = sorted[static_cast<size_t>(count * 0.95)];
    p99 = sorted[std::min(static_cast<size_t>(count * 0.99), sorted.size() - 1)];
}

// ============================================================
// Histogram 实现
// ============================================================
Histogram::Histogram(const std::vector<double>& buckets) : bucket_bounds_(buckets) {
    bucket_count_size_ = buckets.size() + 1;
    bucket_counts_ = std::make_unique<std::atomic<int64_t>[]>(bucket_count_size_);
    for (size_t i = 0; i < bucket_count_size_; i++) {
        bucket_counts_[i] = 0;
    }
}

void Histogram::add(double value) {
    // 找到对应的桶
    size_t idx = bucket_bounds_.size();
    for (size_t i = 0; i < bucket_bounds_.size(); i++) {
        if (value <= bucket_bounds_[i]) {
            idx = i;
            break;
        }
    }
    bucket_counts_[idx]++;
    
    // 更新统计
    double old_sum = sum_.load();
    while (!sum_.compare_exchange_weak(old_sum, old_sum + value)) {}
    
    double old_sq = sum_sq_.load();
    while (!sum_sq_.compare_exchange_weak(old_sq, old_sq + value * value)) {}
    
    count_++;
    
    // 更新 min/max
    double old_min = min_.load();
    while (value < old_min && !min_.compare_exchange_weak(old_min, value)) {}
    
    double old_max = max_.load();
    while (value > old_max && !max_.compare_exchange_weak(old_max, value)) {}
    
    // 保存原始值用于百分位数
    {
        std::lock_guard<std::mutex> lock(mutex_);
        all_values_.push_back(value);
    }
}

void Histogram::reset() {
    for (size_t i = 0; i < bucket_count_size_; i++) {
        bucket_counts_[i] = 0;
    }
    sum_ = 0;
    sum_sq_ = 0;
    count_ = 0;
    min_ = 1e18;
    max_ = 0;
    
    std::lock_guard<std::mutex> lock(mutex_);
    all_values_.clear();
}

std::map<double, int64_t> Histogram::get_buckets() const {
    std::map<double, int64_t> result;
    for (size_t i = 0; i < bucket_bounds_.size(); i++) {
        result[bucket_bounds_[i]] = bucket_counts_[i].load();
    }
    result[std::numeric_limits<double>::infinity()] = bucket_counts_[bucket_count_size_ - 1].load();
    return result;
}

Statistics Histogram::get_statistics() const {
    Statistics stats;
    stats.count = count_;
    stats.min = min_;
    stats.max = max_;
    
    if (count_ > 0) {
        stats.avg = sum_ / count_;
        double variance = (sum_sq_ / count_) - (stats.avg * stats.avg);
        stats.stddev = std::sqrt(std::max(0.0, variance));
    }
    
    // 百分位数
    std::lock_guard<std::mutex> lock(mutex_);
    if (!all_values_.empty()) {
        std::vector<double> sorted = all_values_;
        std::sort(sorted.begin(), sorted.end());
        
        stats.p50 = sorted[static_cast<size_t>(sorted.size() * 0.50)];
        stats.p95 = sorted[static_cast<size_t>(sorted.size() * 0.95)];
        stats.p99 = sorted[std::min(static_cast<size_t>(sorted.size() * 0.99), sorted.size() - 1)];
    }
    
    return stats;
}

double Histogram::percentile(double p) const {
    std::lock_guard<std::mutex> lock(mutex_);
    if (all_values_.empty()) return 0;
    
    std::vector<double> sorted = all_values_;
    std::sort(sorted.begin(), sorted.end());
    
    size_t idx = static_cast<size_t>(sorted.size() * p / 100.0);
    return sorted[std::min(idx, sorted.size() - 1)];
}

// ============================================================
// ResourceMonitor 实现
// ============================================================
ResourceMonitor::ResourceMonitor() = default;

ResourceMonitor::~ResourceMonitor() {
    stop();
}

void ResourceMonitor::start() {
    running_ = true;
    monitor_thread_ = std::thread(&ResourceMonitor::run_monitor, this);
}

void ResourceMonitor::stop() {
    running_ = false;
    if (monitor_thread_.joinable()) {
        monitor_thread_.join();
    }
}

void ResourceMonitor::run_monitor() {
    while (running_) {
        // 读取 CPU 使用率
        std::ifstream stat("/proc/stat");
        if (stat) {
            std::string line;
            std::getline(stat, line);
            
            int64_t user, nice, system, idle, iowait, irq, softirq;
            sscanf(line.c_str(), "cpu %ld %ld %ld %ld %ld %ld %ld",
                   &user, &nice, &system, &idle, &iowait, &irq, &softirq);
            
            int64_t total = user + nice + system + idle + iowait + irq + softirq;
            
            if (prev_total_ > 0) {
                int64_t total_diff = total - prev_total_;
                int64_t idle_diff = idle - prev_idle_;
                
                if (total_diff > 0) {
                    double cpu = 100.0 * (1.0 - static_cast<double>(idle_diff) / total_diff);
                    current_cpu_ = cpu;
                    
                    std::lock_guard<std::mutex> lock(samples_mutex_);
                    cpu_samples_.push_back(cpu);
                }
            }
            
            prev_total_ = total;
            prev_idle_ = idle;
        }
        
        // 读取内存使用
        std::ifstream meminfo("/proc/meminfo");
        if (meminfo) {
            int64_t total = 0, available = 0;
            std::string line;
            
            while (std::getline(meminfo, line)) {
                if (line.find("MemTotal:") == 0) {
                    sscanf(line.c_str(), "MemTotal: %ld kB", &total);
                } else if (line.find("MemAvailable:") == 0) {
                    sscanf(line.c_str(), "MemAvailable: %ld kB", &available);
                }
            }
            
            double used_mb = (total - available) / 1024.0;
            current_memory_mb_ = used_mb;
            
            double old_peak = peak_memory_mb_.load();
            while (used_mb > old_peak && !peak_memory_mb_.compare_exchange_weak(old_peak, used_mb)) {}
        }
        
        std::this_thread::sleep_for(std::chrono::milliseconds(500));
    }
}

double ResourceMonitor::get_cpu_percent() const {
    return current_cpu_;
}

double ResourceMonitor::get_memory_mb() const {
    return current_memory_mb_;
}

double ResourceMonitor::get_memory_percent() const {
    std::ifstream meminfo("/proc/meminfo");
    if (meminfo) {
        int64_t total = 0, available = 0;
        std::string line;
        while (std::getline(meminfo, line)) {
            if (line.find("MemTotal:") == 0) {
                sscanf(line.c_str(), "MemTotal: %ld kB", &total);
            } else if (line.find("MemAvailable:") == 0) {
                sscanf(line.c_str(), "MemAvailable: %ld kB", &available);
            }
        }
        if (total > 0) {
            return 100.0 * (total - available) / total;
        }
    }
    return 0;
}

double ResourceMonitor::get_avg_cpu_percent() const {
    std::lock_guard<std::mutex> lock(samples_mutex_);
    if (cpu_samples_.empty()) return 0;
    
    double sum = 0;
    for (double v : cpu_samples_) sum += v;
    return sum / cpu_samples_.size();
}

double ResourceMonitor::get_npu_utilization() const {
    // SpacemiT NPU 利用率 (如果可用)
    std::ifstream npu("/sys/class/spacemit-npu/npu0/utilization");
    if (npu) {
        double util;
        npu >> util;
        return util;
    }
    return -1;  // 不可用
}

// ============================================================
// MetricsCollector 实现
// ============================================================
MetricsCollector::MetricsCollector() 
    : latency_hist_({10, 20, 50, 100, 200, 500, 1000, 2000, 5000}),
      inference_hist_({5, 10, 20, 50, 100, 200, 500, 1000}),
      ttft_hist_({100, 200, 500, 1000, 2000, 5000, 10000}),
      tps_hist_({1, 2, 5, 10, 20, 50, 100}) {}

MetricsCollector::~MetricsCollector() {
    stop();
}

void MetricsCollector::start(const std::string& test_name) {
    test_name_ = test_name;
    start_time_ = Clock::now();
    last_display_time_ = start_time_;
    running_ = true;
    
    reset();
    resource_monitor_.start();
}

void MetricsCollector::stop() {
    end_time_ = Clock::now();
    running_ = false;
    resource_monitor_.stop();
}

void MetricsCollector::reset() {
    total_count_ = 0;
    success_count_ = 0;
    error_count_ = 0;
    total_latency_ms_ = 0;
    total_inference_ms_ = 0;
    total_tokens_ = 0;
    
    latency_hist_.reset();
    inference_hist_.reset();
    ttft_hist_.reset();
    tps_hist_.reset();
}

void MetricsCollector::record(const InferenceResult& result) {
    total_count_++;
    
    if (result.success) {
        success_count_++;
        
        latency_hist_.add(result.total_ms);
        inference_hist_.add(result.inference_ms);
        
        total_latency_ms_ = total_latency_ms_ + result.total_ms;
        total_inference_ms_ = total_inference_ms_ + result.inference_ms;
        
        // VLM 指标
        if (result.ttft_ms > 0) {
            ttft_hist_.add(result.ttft_ms);
        }
        if (result.tokens_per_sec > 0) {
            tps_hist_.add(result.tokens_per_sec);
        }
        if (result.tokens_generated > 0) {
            total_tokens_ = total_tokens_ + result.tokens_generated;
        }
    } else {
        error_count_++;
    }
    
    // 实时显示
    if (realtime_display_) {
        display_progress();
    }
}

void MetricsCollector::record_batch(const std::vector<InferenceResult>& results) {
    for (const auto& r : results) {
        record(r);
    }
}

void MetricsCollector::display_progress() {
    std::lock_guard<std::mutex> lock(display_mutex_);
    
    auto now = Clock::now();
    auto elapsed = Duration(now - last_display_time_).count();
    
    // 每秒更新一次
    if (elapsed < 1000) return;
    last_display_time_ = now;
    
    auto total_elapsed = Duration(now - start_time_).count() / 1000.0;
    double fps = success_count_ / total_elapsed;
    auto stats = latency_hist_.get_statistics();
    
    printf("\r[%s] %ld/%ld (%.1f%% err) | FPS: %.1f | Latency: %.1f/%.1f/%.1f ms (avg/p95/p99) | CPU: %.0f%% | Mem: %.0f MB    ",
           test_name_.c_str(),
           static_cast<long>(success_count_.load()),
           static_cast<long>(total_count_.load()),
           total_count_ > 0 ? 100.0 * error_count_ / total_count_ : 0,
           fps,
           stats.avg, stats.p95, stats.p99,
           resource_monitor_.get_cpu_percent(),
           resource_monitor_.get_memory_mb());
    fflush(stdout);
}

BenchmarkResult MetricsCollector::get_result() const {
    BenchmarkResult result;
    result.test_name = test_name_;
    result.total_frames = total_count_;
    result.success_count = success_count_;
    result.error_count = error_count_;
    
    auto elapsed = Duration(end_time_ - start_time_).count() / 1000.0;
    result.total_time_sec = elapsed;
    result.fps = elapsed > 0 ? success_count_ / elapsed : 0;
    
    result.latency = latency_hist_.get_statistics();
    result.inference_time = inference_hist_.get_statistics();
    result.ttft = ttft_hist_.get_statistics();
    result.tokens_per_sec = tps_hist_.get_statistics();
    
    result.peak_memory_mb = resource_monitor_.get_peak_memory_mb();
    result.avg_cpu_percent = resource_monitor_.get_avg_cpu_percent();
    
    // NPU 信息 (K3 A100)
    const char* yolo_threads = std::getenv("YOLO_THREADS");
    if (yolo_threads) {
        result.npu_threads = std::atoi(yolo_threads);
    } else {
        result.npu_threads = 4;  // 默认 4 核
    }
    
    // 获取 Session 数
    int num_sessions = 1;
    const char* dual_session = std::getenv("YOLO_DUAL_SESSION");
    if (dual_session && std::atoi(dual_session) > 0 && result.npu_threads >= 4) {
        num_sessions = 2;
    }
    
    // ★ NPU 真实利用率计算
    // 公式: NPU利用率 = 总推理时间 / (测试时间 × Session数) × 100%
    // 这反映 NPU 在测试期间的实际繁忙程度
    if (result.npu_threads > 0) {
        double total_infer = total_inference_ms_.load();
        double test_duration_ms = Duration(end_time_ - start_time_).count();
        
        if (total_infer > 0 && test_duration_ms > 0) {
            // 真实 NPU 利用率 = 总推理时间 / (测试时间 × Session数)
            result.npu_efficiency = std::min(100.0, 
                (total_infer / (test_duration_ms * num_sessions)) * 100.0);
        } else {
            // 回退: 基于理论最大 FPS
            double theoretical_fps = result.npu_threads * 3.5;
            result.npu_efficiency = std::min(100.0, result.fps / theoretical_fps * 100.0);
        }
    }
    
    // 模型内存 (峰值内存 - 基础进程内存约 100MB)
    result.model_memory_mb = std::max(0.0, result.peak_memory_mb - 100.0);
    
    // 时间戳
    auto start_t = std::chrono::system_clock::to_time_t(
        std::chrono::system_clock::now() - (Clock::now() - start_time_));
    auto end_t = std::chrono::system_clock::to_time_t(
        std::chrono::system_clock::now() - (Clock::now() - end_time_));
    
    char buf[64];
    strftime(buf, sizeof(buf), "%Y-%m-%d %H:%M:%S", localtime(&start_t));
    result.start_time = buf;
    strftime(buf, sizeof(buf), "%Y-%m-%d %H:%M:%S", localtime(&end_t));
    result.end_time = buf;
    
    return result;
}

void MetricsCollector::export_json(const std::string& filepath) const {
    auto result = get_result();
    
    std::ofstream file(filepath);
    file << "{\n";
    file << "  \"test_name\": \"" << result.test_name << "\",\n";
    file << "  \"backend_type\": \"" << result.backend_type << "\",\n";
    file << "  \"concurrency\": " << result.concurrency << ",\n";
    file << "  \"total_frames\": " << result.total_frames << ",\n";
    file << "  \"success_count\": " << result.success_count << ",\n";
    file << "  \"error_count\": " << result.error_count << ",\n";
    file << "  \"total_time_sec\": " << std::fixed << std::setprecision(2) << result.total_time_sec << ",\n";
    file << "  \"fps\": " << std::fixed << std::setprecision(2) << result.fps << ",\n";
    file << "  \"latency\": {\n";
    file << "    \"avg\": " << result.latency.avg << ",\n";
    file << "    \"min\": " << result.latency.min << ",\n";
    file << "    \"max\": " << result.latency.max << ",\n";
    file << "    \"p50\": " << result.latency.p50 << ",\n";
    file << "    \"p95\": " << result.latency.p95 << ",\n";
    file << "    \"p99\": " << result.latency.p99 << ",\n";
    file << "    \"stddev\": " << result.latency.stddev << "\n";
    file << "  },\n";
    file << "  \"inference_time\": {\n";
    file << "    \"avg\": " << result.inference_time.avg << ",\n";
    file << "    \"p95\": " << result.inference_time.p95 << ",\n";
    file << "    \"p99\": " << result.inference_time.p99 << "\n";
    file << "  },\n";
    file << "  \"peak_memory_mb\": " << result.peak_memory_mb << ",\n";
    file << "  \"avg_cpu_percent\": " << result.avg_cpu_percent << ",\n";
    file << "  \"npu\": {\n";
    file << "    \"threads\": " << result.npu_threads << ",\n";
    file << "    \"efficiency\": " << std::fixed << std::setprecision(1) << result.npu_efficiency << ",\n";
    file << "    \"model_memory_mb\": " << std::fixed << std::setprecision(1) << result.model_memory_mb << "\n";
    file << "  },\n";
    file << "  \"start_time\": \"" << result.start_time << "\",\n";
    file << "  \"end_time\": \"" << result.end_time << "\"\n";
    file << "}\n";
}

void MetricsCollector::export_csv(const std::string& filepath) const {
    auto result = get_result();
    
    bool write_header = !std::ifstream(filepath).good();
    
    std::ofstream file(filepath, std::ios::app);
    
    if (write_header) {
        file << "test_name,backend_type,concurrency,total_frames,success_count,error_count,";
        file << "total_time_sec,fps,latency_avg,latency_p95,latency_p99,";
        file << "inference_avg,peak_memory_mb,avg_cpu_percent,start_time,end_time\n";
    }
    
    file << result.test_name << ",";
    file << result.backend_type << ",";
    file << result.concurrency << ",";
    file << result.total_frames << ",";
    file << result.success_count << ",";
    file << result.error_count << ",";
    file << std::fixed << std::setprecision(2) << result.total_time_sec << ",";
    file << std::fixed << std::setprecision(2) << result.fps << ",";
    file << std::fixed << std::setprecision(2) << result.latency.avg << ",";
    file << std::fixed << std::setprecision(2) << result.latency.p95 << ",";
    file << std::fixed << std::setprecision(2) << result.latency.p99 << ",";
    file << std::fixed << std::setprecision(2) << result.inference_time.avg << ",";
    file << std::fixed << std::setprecision(1) << result.peak_memory_mb << ",";
    file << std::fixed << std::setprecision(1) << result.avg_cpu_percent << ",";
    file << result.start_time << ",";
    file << result.end_time << "\n";
}

void MetricsCollector::export_markdown(std::ostream& os) const {
    auto result = get_result();
    
    os << "## " << result.test_name << "\n\n";
    os << "| Metric | Value |\n";
    os << "|--------|-------|\n";
    os << "| Total Frames | " << result.total_frames << " |\n";
    os << "| Success | " << result.success_count << " |\n";
    os << "| Errors | " << result.error_count << " |\n";
    os << "| Duration | " << std::fixed << std::setprecision(2) << result.total_time_sec << " s |\n";
    os << "| FPS | " << std::fixed << std::setprecision(2) << result.fps << " |\n";
    os << "| Latency (avg) | " << std::fixed << std::setprecision(2) << result.latency.avg << " ms |\n";
    os << "| Latency (p95) | " << std::fixed << std::setprecision(2) << result.latency.p95 << " ms |\n";
    os << "| Latency (p99) | " << std::fixed << std::setprecision(2) << result.latency.p99 << " ms |\n";
    os << "| Peak Memory | " << std::fixed << std::setprecision(1) << result.peak_memory_mb << " MB |\n";
    os << "| Avg CPU | " << std::fixed << std::setprecision(1) << result.avg_cpu_percent << " % |\n";
    os << "\n";
}

// ============================================================
// ResultAggregator 实现
// ============================================================
void ResultAggregator::add_result(const BenchmarkResult& result) {
    std::lock_guard<std::mutex> lock(mutex_);
    results_.push_back(result);
}

void ResultAggregator::export_report(const std::string& output_dir) const {
    std::lock_guard<std::mutex> lock(mutex_);
    
    // JSON
    std::ofstream json(output_dir + "/benchmark_results.json");
    json << "[\n";
    for (size_t i = 0; i < results_.size(); i++) {
        const auto& r = results_[i];
        json << "  {\n";
        json << "    \"test_name\": \"" << r.test_name << "\",\n";
        json << "    \"model_name\": \"" << r.model_name << "\",\n";
        json << "    \"backend_type\": \"" << r.backend_type << "\",\n";
        json << "    \"concurrency\": " << r.concurrency << ",\n";
        json << "    \"fps\": " << r.fps << ",\n";
        json << "    \"latency_avg\": " << r.latency.avg << ",\n";
        json << "    \"latency_p95\": " << r.latency.p95 << ",\n";
        json << "    \"latency_p99\": " << r.latency.p99 << ",\n";
        json << "    \"npu_threads\": " << r.npu_threads << ",\n";
        json << "    \"npu_efficiency\": " << r.npu_efficiency << ",\n";
        json << "    \"peak_memory_mb\": " << r.peak_memory_mb << "\n";
        json << "  }" << (i + 1 < results_.size() ? "," : "") << "\n";
    }
    json << "]\n";
    
    // Markdown
    std::ofstream md(output_dir + "/benchmark_report.md");
    md << "# RiVision Benchmark Report\n\n";
    md << "**Generated:** " << results_.front().start_time << "\n\n";
    
    // Platform info
    struct utsname uts;
    uname(&uts);
    md << "## Platform Information\n\n";
    md << "| Item | Value |\n";
    md << "|------|-------|\n";
    md << "| System | " << uts.sysname << " |\n";
    md << "| Machine | " << uts.machine << " |\n";
    md << "| Kernel | " << uts.release << " |\n";
    
    // NPU info
    bool is_riscv = std::string(uts.machine).find("riscv") != std::string::npos;
    if (is_riscv && !results_.empty()) {
        md << "| NPU | K3 A100 (8 cores) |\n";
        md << "| NPU Threads | " << results_.front().npu_threads << " |\n";
        md << "| Backend | SpacemiT EP (ORT) |\n";
    }
    md << "\n";
    
    // Results table (matching image 1 format)
    md << "## Benchmark Results\n\n";
    md << "| Model | FPS | Latency (avg) | Latency (p95) | Latency (p99) | Memory | CPU | NPU Efficiency |\n";
    md << "|-------|-----|---------------|---------------|---------------|--------|-----|----------------|\n";
    
    for (const auto& r : results_) {
        std::string model = r.model_name.empty() ? r.test_name : r.model_name;
        md << "| " << model << " | "
           << std::fixed << std::setprecision(2) << r.fps << " | "
           << std::fixed << std::setprecision(2) << r.latency.avg << " ms | "
           << std::fixed << std::setprecision(2) << r.latency.p95 << " ms | "
           << std::fixed << std::setprecision(2) << r.latency.p99 << " ms | "
           << std::fixed << std::setprecision(0) << r.peak_memory_mb << " MB | "
           << std::fixed << std::setprecision(0) << r.avg_cpu_percent << "% | "
           << std::fixed << std::setprecision(0) << r.npu_efficiency << "% |\n";
    }
    md << "\n";
    
    // Latency metrics explanation
    md << "## Latency Metrics Explanation\n\n";
    md << "| Metric | Description | NPU Performance Relevance |\n";
    md << "|--------|-------------|---------------------------|\n";
    md << "| **avg** | Average latency across all inferences | Reflects NPU throughput under typical load |\n";
    md << "| **p95** | 95th percentile (95% complete within) | Reflects NPU stability under high load |\n";
    md << "| **p99** | 99th percentile (99% complete within) | Reflects NPU handling of extreme cases |\n";
    md << "\n";
    md << "> **Note:** Small gap between p95/p99 and avg indicates stable NPU scheduling.\n\n";
    
    compare_backends(md);
}

void ResultAggregator::print_summary() const {
    std::lock_guard<std::mutex> lock(mutex_);
    
    printf("\n");
    printf("╔═══════════════════════════════════════════════════════════════════╗\n");
    printf("║                    Benchmark Summary                              ║\n");
    printf("╠═══════════════════════════════════════════════════════════════════╣\n");
    printf("║ %-20s │ %8s │ %8s │ %8s │ %8s ║\n", 
           "Test", "FPS", "Lat(avg)", "Lat(p95)", "Lat(p99)");
    printf("╠══════════════════════╪══════════╪══════════╪══════════╪══════════╣\n");
    
    for (const auto& r : results_) {
        printf("║ %-20s │ %8.1f │ %6.1f ms│ %6.1f ms│ %6.1f ms║\n",
               r.test_name.c_str(), r.fps, r.latency.avg, r.latency.p95, r.latency.p99);
    }
    
    printf("╚═══════════════════════════════════════════════════════════════════╝\n");
}

void ResultAggregator::compare_backends(std::ostream& os) const {
    os << "## Backend Comparison\n\n";
    os << "| Backend | FPS | Latency (avg) | Latency (p95) | Latency (p99) |\n";
    os << "|---------|-----|---------------|---------------|---------------|\n";
    
    for (const auto& r : results_) {
        os << "| " << r.test_name << " | "
           << std::fixed << std::setprecision(1) << r.fps << " | "
           << std::fixed << std::setprecision(1) << r.latency.avg << " ms | "
           << std::fixed << std::setprecision(1) << r.latency.p95 << " ms | "
           << std::fixed << std::setprecision(1) << r.latency.p99 << " ms |\n";
    }
    os << "\n";
}

void ResultAggregator::compare_concurrency(std::ostream& os) const {
    os << "## Concurrency Impact\n\n";
    os << "| Concurrency | FPS | Latency (avg) | Efficiency |\n";
    os << "|-------------|-----|---------------|------------|\n";
    
    double base_fps = 0;
    for (const auto& r : results_) {
        if (r.concurrency == 1) {
            base_fps = r.fps;
        }
        
        double efficiency = base_fps > 0 ? (r.fps / r.concurrency) / base_fps * 100 : 100;
        
        os << "| " << r.concurrency << " | "
           << std::fixed << std::setprecision(1) << r.fps << " | "
           << std::fixed << std::setprecision(1) << r.latency.avg << " ms | "
           << std::fixed << std::setprecision(0) << efficiency << "% |\n";
    }
    os << "\n";
}

}  // namespace benchmark
}  // namespace rivision
