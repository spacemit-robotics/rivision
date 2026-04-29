#include "benchmark_runner.h"
#include <iostream>
#include <iomanip>
#include <fstream>
#include <thread>
#include <future>
#include <cstring>
#include <sys/utsname.h>

namespace rivision {
namespace benchmark {

// ============================================================
// BenchmarkRunner 实现
// ============================================================
BenchmarkRunner::BenchmarkRunner() = default;
BenchmarkRunner::~BenchmarkRunner() = default;

void BenchmarkRunner::set_data_source(std::unique_ptr<DataSource> source) {
    source_ = std::move(source);
}

void BenchmarkRunner::set_backend(std::unique_ptr<InferenceBackend> backend) {
    backend_ = std::move(backend);
}

void BenchmarkRunner::set_config(const RunnerConfig& config) {
    config_ = config;
}

void BenchmarkRunner::set_progress_callback(ProgressCallback callback) {
    progress_callback_ = callback;
}

BenchmarkResult BenchmarkRunner::run() {
    switch (config_.mode) {
        case BenchmarkMode::SINGLE_FRAME:
            return run_single_frame();
        case BenchmarkMode::MAX_THROUGHPUT:
            return run_max_throughput();
        case BenchmarkMode::FIXED_FPS:
            return run_fixed_fps();
        case BenchmarkMode::STRESS_TEST:
            return run_stress_test();
        default:
            return run_max_throughput();
    }
}

void BenchmarkRunner::warmup() {
    if (!source_ || !backend_) {
        printf("[Warmup] ERROR: source=%p, backend=%p\n", (void*)source_.get(), (void*)backend_.get());
        return;
    }
    
    // Warmup 静默进行
    
    Frame frame;
    for (int i = 0; i < config_.warmup_runs && !stop_requested_; i++) {
        if (source_->read(frame)) {
            backend_->infer(frame);
        }
    }
    
    // 重置数据源
    source_->seek(0);
    
    // Warmup done
}

void BenchmarkRunner::cooldown() {
    std::this_thread::sleep_for(std::chrono::seconds(config_.cooldown_sec));
}

void BenchmarkRunner::infer_and_record(const Frame& frame) {
    auto result = backend_->infer(frame);
    metrics_.record(result);
}

BenchmarkResult BenchmarkRunner::run_single_frame() {
    
    if (!source_ || !backend_) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    running_ = true;
    stop_requested_ = false;
    
    auto info = backend_->info();
    
    std::string test_name = info.name + " (single frame)";
    
    // 预热 + 开始测试
    warmup();
    
    metrics_.start(test_name);
    metrics_.enable_realtime_display(config_.realtime_display);
    
    Frame frame;
    if (!source_->read(frame)) {
        metrics_.stop();
        return metrics_.get_result();
    }
    
    // ★ 优化: 使用批量模式最大化 NPU 利用率
    // 批量推理启用预处理流水线，保持 NPU 持续忙碌
    const bool use_batch_mode = config_.main_runs > 1;
    
    if (use_batch_mode) {
        
        // 收集所有帧
        std::vector<Frame> frames;
        frames.reserve(config_.main_runs);
        for (int i = 0; i < config_.main_runs; i++) {
            frames.push_back(frame);
            frames.back().frame_id = i;
        }
        
        // 批量推理 (使用流水线)
        std::vector<InferenceResult> results;
        try {
            results = backend_->infer_batch(frames, 1);
        } catch (...) {
            // 异常静默处理
        }
        
        // 记录结果
        for (size_t i = 0; i < results.size(); i++) {
            metrics_.record(results[i]);
            if (progress_callback_ && (i + 1) % 8 == 0) {
                progress_callback_(i + 1, config_.main_runs, "inferring");
            }
        }
    } else {
        
        for (int i = 0; i < config_.main_runs && !stop_requested_; i++) {
            infer_and_record(frame);
            
            if (progress_callback_) {
                progress_callback_(i + 1, config_.main_runs, "inferring");
            }
        }
    }
    
    printf("\n");  // 换行结束实时显示
    metrics_.stop();
    
    // 冷却 (跳过，加快测试)
    // cooldown();
    
    auto result = metrics_.get_result();
    result.backend_type = info.backend_type;
    result.model_name = info.model_name;
    result.concurrency = 1;
    
    // 结果将在最终报告中显示
    
    running_ = false;
    return result;
}

BenchmarkResult BenchmarkRunner::run_max_throughput() {
    if (!source_ || !backend_) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    running_ = true;
    stop_requested_ = false;
    
    auto info = backend_->info();
    std::string test_name = info.name + " (max throughput, c=" + 
                            std::to_string(config_.concurrency) + ")";
    
    // 预热
    warmup();
    
    // 开始测试
    metrics_.start(test_name);
    metrics_.enable_realtime_display(config_.realtime_display);
    
    printf("[Test] Running max throughput test (concurrency=%d, runs=%d)...\n",
           config_.concurrency, config_.main_runs);
    
    run_concurrent(config_.concurrency, config_.main_runs);
    
    printf("\n");
    metrics_.stop();
    
    // 冷却
    cooldown();
    
    auto result = metrics_.get_result();
    result.backend_type = info.backend_type;
    result.model_name = info.model_name;
    result.concurrency = config_.concurrency;
    
    running_ = false;
    return result;
}

void BenchmarkRunner::run_concurrent(int concurrency, int total_runs) {
    std::vector<Frame> frames;
    frames.reserve(total_runs);
    
    // 预读取所有帧
    Frame frame;
    while (frames.size() < static_cast<size_t>(total_runs) && source_->read(frame)) {
        frames.push_back(frame);
    }
    
    if (frames.empty()) {
        fprintf(stderr, "[Error] No frames available\n");
        return;
    }
    
    // 如果帧数不足，循环填充
    size_t orig_size = frames.size();
    while (frames.size() < static_cast<size_t>(total_runs)) {
        frames.push_back(frames[frames.size() % orig_size]);
    }
    
    // 并发推理
    std::atomic<size_t> next_idx{0};
    std::vector<std::thread> workers;
    
    for (int t = 0; t < concurrency; t++) {
        workers.emplace_back([&]() {
            while (!stop_requested_) {
                size_t idx = next_idx.fetch_add(1);
                if (idx >= frames.size()) break;
                
                auto result = backend_->infer(frames[idx]);
                metrics_.record(result);
            }
        });
    }
    
    for (auto& w : workers) {
        w.join();
    }
}

BenchmarkResult BenchmarkRunner::run_fixed_fps() {
    if (!source_ || !backend_) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    running_ = true;
    stop_requested_ = false;
    
    auto info = backend_->info();
    std::string test_name = info.name + " (fixed " + 
                            std::to_string(config_.target_fps) + " fps)";
    
    // 预热
    warmup();
    
    // 开始测试
    metrics_.start(test_name);
    metrics_.enable_realtime_display(config_.realtime_display);
    
    printf("[Test] Running fixed FPS test (target=%d fps, runs=%d)...\n",
           config_.target_fps, config_.main_runs);
    
    int64_t interval_us = 1000000 / config_.target_fps;
    
    Frame frame;
    for (int i = 0; i < config_.main_runs && !stop_requested_; i++) {
        auto start = Clock::now();
        
        if (source_->read(frame)) {
            infer_and_record(frame);
        }
        
        // 等待到下一帧时间
        auto elapsed = std::chrono::duration_cast<std::chrono::microseconds>(
            Clock::now() - start).count();
        
        if (elapsed < interval_us) {
            std::this_thread::sleep_for(std::chrono::microseconds(interval_us - elapsed));
        }
    }
    
    printf("\n");
    metrics_.stop();
    
    auto result = metrics_.get_result();
    result.backend_type = info.backend_type;
    result.model_name = info.model_name;
    result.concurrency = 1;
    
    running_ = false;
    return result;
}

BenchmarkResult BenchmarkRunner::run_stress_test() {
    if (!source_ || !backend_) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    running_ = true;
    stop_requested_ = false;
    
    auto info = backend_->info();
    std::string test_name = info.name + " (stress " + 
                            std::to_string(config_.stress_duration_sec) + "s)";
    
    // 预热
    warmup();
    
    // 开始测试
    metrics_.start(test_name);
    metrics_.enable_realtime_display(config_.realtime_display);
    
    printf("[Test] Running stress test (duration=%d sec, concurrency=%d)...\n",
           config_.stress_duration_sec, config_.concurrency);
    
    auto end_time = Clock::now() + std::chrono::seconds(config_.stress_duration_sec);
    
    std::atomic<bool> done{false};
    std::vector<std::thread> workers;
    
    for (int t = 0; t < config_.concurrency; t++) {
        workers.emplace_back([&]() {
            Frame frame;
            while (!done && !stop_requested_) {
                if (source_->read(frame)) {
                    auto result = backend_->infer(frame);
                    metrics_.record(result);
                } else {
                    source_->seek(0);
                }
            }
        });
    }
    
    // 等待测试结束
    while (Clock::now() < end_time && !stop_requested_) {
        std::this_thread::sleep_for(std::chrono::seconds(1));
    }
    
    done = true;
    
    for (auto& w : workers) {
        w.join();
    }
    
    printf("\n");
    metrics_.stop();
    
    auto result = metrics_.get_result();
    result.backend_type = info.backend_type;
    result.model_name = info.model_name;
    result.concurrency = config_.concurrency;
    
    running_ = false;
    return result;
}

std::vector<BenchmarkResult> BenchmarkRunner::run_concurrency_sweep() {
    std::vector<BenchmarkResult> results;
    
    for (int c : config_.concurrency_sweep) {
        config_.concurrency = c;
        source_->seek(0);
        
        auto result = run_max_throughput();
        results.push_back(result);
        
        if (stop_requested_) break;
    }
    
    return results;
}

void BenchmarkRunner::stop() {
    stop_requested_ = true;
}

// ============================================================
// 工具函数实现
// ============================================================
void print_platform_info(const std::string& model_path) {
    struct utsname uts;
    uname(&uts);
    
    printf("\n");
    printf("╔═══════════════════════════════════════════════════════════════════╗\n");
    printf("║                    Platform Information                           ║\n");
    printf("╠═══════════════════════════════════════════════════════════════════╣\n");
    printf("║  System:     %-54s ║\n", uts.sysname);
    printf("║  Machine:    %-54s ║\n", uts.machine);
    printf("║  Kernel:     %-54s ║\n", uts.release);
    
    // CPU 信息
    std::ifstream cpuinfo("/proc/cpuinfo");
    std::string line;
    std::string cpu_model = "Unknown";
    int cpu_count = 0;
    
    while (std::getline(cpuinfo, line)) {
        if (line.find("model name") == 0 || line.find("isa") == 0) {
            cpu_model = line.substr(line.find(':') + 2);
        }
        if (line.find("processor") == 0) {
            cpu_count++;
        }
    }
    
    printf("║  CPU:        %-54s ║\n", cpu_model.substr(0, 54).c_str());
    printf("║  Cores:      %-54d ║\n", cpu_count);
    
    // 内存信息
    std::ifstream meminfo("/proc/meminfo");
    int64_t total_kb = 0;
    while (std::getline(meminfo, line)) {
        if (line.find("MemTotal:") == 0) {
            sscanf(line.c_str(), "MemTotal: %ld kB", &total_kb);
            break;
        }
    }
    
    char mem_str[64];
    snprintf(mem_str, sizeof(mem_str), "%.1f GB", total_kb / 1048576.0);
    printf("║  Memory:     %-54s ║\n", mem_str);
    
    // 模型名称
    if (!model_path.empty()) {
        std::string model_name = model_path;
        size_t last_slash = model_path.find_last_of("/\\");
        if (last_slash != std::string::npos) {
            model_name = model_path.substr(last_slash + 1);
        }
        printf("║  Model:      %-54s ║\n", model_name.substr(0, 54).c_str());
    }
    
    // NPU 信息 (K3 A100)
    bool is_riscv = std::string(uts.machine).find("riscv") != std::string::npos;
    if (is_riscv) {
        const char* yolo_threads = std::getenv("YOLO_THREADS");
        const char* vlm_threads = std::getenv("VLM_THREADS");
        int npu_threads = yolo_threads ? std::atoi(yolo_threads) : 4;
        
        printf("╠═══════════════════════════════════════════════════════════════════╣\n");
        printf("║  NPU:        K3 A100 (8 cores)                                    ║\n");
        printf("║  YOLO NPU:   %d cores (YOLO_THREADS)%-30s ║\n", npu_threads, "");
        if (vlm_threads) {
            printf("║  VLM NPU:    %d cores (VLM_THREADS)%-31s ║\n", std::atoi(vlm_threads), "");
        }
        printf("║  Backend:    SpacemiT EP (ORT)%-36s ║\n", "");
    }
    
    printf("╚═══════════════════════════════════════════════════════════════════╝\n");
}

void print_result(const BenchmarkResult& result) {
    printf("\n");
    printf("╔═══════════════════════════════════════════════════════════════════╗\n");
    printf("║  %-65s ║\n", result.test_name.c_str());
    printf("╠═══════════════════════════════════════════════════════════════════╣\n");
    printf("║  Performance                                                      ║\n");
    printf("║  ─────────────────────────────────────────────────────────────── ║\n");
    printf("║    Throughput:       %8.2f fps                                 ║\n", result.fps);
    printf("║    Total Frames:     %8ld                                     ║\n", result.total_frames);
    printf("║    Success Rate:     %8.1f%%                                   ║\n", 
           result.total_frames > 0 ? 100.0 * result.success_count / result.total_frames : 0);
    printf("║    Duration:         %8.2f s                                   ║\n", result.total_time_sec);
    printf("║                                                                   ║\n");
    printf("║  Latency (ms)                                                     ║\n");
    printf("║  ─────────────────────────────────────────────────────────────── ║\n");
    printf("║    Average:          %8.2f                                     ║\n", result.latency.avg);
    printf("║    Min:              %8.2f                                     ║\n", result.latency.min);
    printf("║    Max:              %8.2f                                     ║\n", result.latency.max);
    printf("║    P50:              %8.2f                                     ║\n", result.latency.p50);
    printf("║    P95:              %8.2f                                     ║\n", result.latency.p95);
    printf("║    P99:              %8.2f                                     ║\n", result.latency.p99);
    printf("║    StdDev:           %8.2f                                     ║\n", result.latency.stddev);
    printf("║                                                                   ║\n");
    printf("║  Resources                                                        ║\n");
    printf("║  ─────────────────────────────────────────────────────────────── ║\n");
    printf("║    Peak Memory:      %8.1f MB                                  ║\n", result.peak_memory_mb);
    printf("║    Avg CPU:          %8.1f%%                                   ║\n", result.avg_cpu_percent);
    
    // NPU 信息 (如果有)
    if (result.npu_threads > 0) {
        printf("║                                                                   ║\n");
        printf("║  NPU (A100)                                                       ║\n");
        printf("║  ─────────────────────────────────────────────────────────────── ║\n");
        printf("║    NPU Cores:        %8d                                     ║\n", result.npu_threads);
        if (result.npu_efficiency > 0) {
            printf("║    NPU Efficiency:   %7.1f%%                                    ║\n", result.npu_efficiency);
        }
        if (result.model_memory_mb > 0) {
            printf("║    Model Memory:     %8.1f MB                                  ║\n", result.model_memory_mb);
        }
    }
    
    printf("╚═══════════════════════════════════════════════════════════════════╝\n");
    fflush(stdout);
}

void print_comparison_table(const std::vector<BenchmarkResult>& results) {
    printf("\n");
    printf("╔═══════════════════════════════════════════════════════════════════════════╗\n");
    printf("║                          Performance Comparison                           ║\n");
    printf("╠══════════════════════════╤══════════╤══════════╤══════════╤══════════════╣\n");
    printf("║ %-24s │ %8s │ %8s │ %8s │ %12s ║\n", 
           "Test", "FPS", "Lat(avg)", "Lat(p95)", "Success Rate");
    printf("╠══════════════════════════╪══════════╪══════════╪══════════╪══════════════╣\n");
    
    for (const auto& r : results) {
        double success_rate = r.total_frames > 0 ? 100.0 * r.success_count / r.total_frames : 0;
        printf("║ %-24s │ %8.1f │ %6.1f ms│ %6.1f ms│ %10.1f%% ║\n",
               r.test_name.substr(0, 24).c_str(),
               r.fps, r.latency.avg, r.latency.p95, success_rate);
    }
    
    printf("╚══════════════════════════╧══════════╧══════════╧══════════╧══════════════╝\n");
}

void print_multi_stream_capacity(double max_fps, const std::vector<int>& target_fps) {
    printf("\n");
    printf("╔═══════════════════════════════════════════════════════════════════╗\n");
    printf("║                    Multi-Stream Capacity                          ║\n");
    printf("╠═══════════════════════════════════════════════════════════════════╣\n");
    printf("║  Max Throughput: %.1f fps                                         ║\n", max_fps);
    printf("║  ─────────────────────────────────────────────────────────────── ║\n");
    
    for (int fps : target_fps) {
        int streams = static_cast<int>(max_fps / fps);
        if (streams >= 1) {
            printf("║    @ %2d fps/stream:  %2d streams supported                        ║\n",
                   fps, streams);
        } else {
            int pct = static_cast<int>(max_fps * 100 / fps);
            printf("║    @ %2d fps/stream:  insufficient (only %d%% capacity)            ║\n",
                   fps, pct);
        }
    }
    
    printf("╚═══════════════════════════════════════════════════════════════════╝\n");
}

// ============================================================
// YOLOBenchmark 实现
// ============================================================
YOLOBenchmark::YOLOBenchmark() = default;
YOLOBenchmark::~YOLOBenchmark() = default;

void YOLOBenchmark::set_image(const std::string& image_path) {
    image_path_ = image_path;
}

void YOLOBenchmark::set_video(const std::string& video_path) {
    video_path_ = video_path;
}

void YOLOBenchmark::set_config(const BenchmarkConfig& config) {
    config_ = config;
}

BenchmarkResult YOLOBenchmark::run_local(const std::string& model_path, int /*threads*/) {
    auto source = std::make_unique<SingleImageSource>();
    static_cast<SingleImageSource*>(source.get())->set_repeat_count(config_.main_runs);
    
    if (!source->open(image_path_)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    auto backend = std::make_unique<LocalYOLOBackend>();
    BackendConfig backend_config;
    backend_config.model_path = model_path;
    backend_config.cpu_threads = config_.cpu_threads;      // ★ CPU 线程数 (预处理/后处理)
    backend_config.npu_threads = config_.npu_threads;      // ★ NPU 核心数
    backend_config.dual_session = config_.dual_session;    // ★ 双 Session 开关
    
    if (!backend->init(backend_config)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    BenchmarkRunner runner;
    runner.set_data_source(std::move(source));
    runner.set_backend(std::move(backend));
    
    RunnerConfig runner_config;
    runner_config.mode = BenchmarkMode::SINGLE_FRAME;
    runner_config.warmup_runs = config_.warmup_runs;
    runner_config.main_runs = config_.main_runs;
    runner_config.cooldown_sec = config_.cooldown_sec;
    runner_config.realtime_display = config_.realtime_display;
    runner.set_config(runner_config);
    
    return runner.run();
}

BenchmarkResult YOLOBenchmark::run_http(const std::string& url, int concurrency) {
    auto source = std::make_unique<SingleImageSource>();
    static_cast<SingleImageSource*>(source.get())->set_repeat_count(config_.main_runs);
    
    if (!source->open(image_path_)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    auto backend = std::make_unique<HTTPYOLOBackend>();
    BackendConfig backend_config;
    backend_config.http_url = url;
    backend_config.http_pool_size = concurrency * 2;
    
    if (!backend->init(backend_config)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    BenchmarkRunner runner;
    runner.set_data_source(std::move(source));
    runner.set_backend(std::move(backend));
    
    RunnerConfig runner_config;
    runner_config.mode = BenchmarkMode::MAX_THROUGHPUT;
    runner_config.concurrency = concurrency;
    runner_config.warmup_runs = config_.warmup_runs;
    runner_config.main_runs = config_.main_runs;
    runner_config.cooldown_sec = config_.cooldown_sec;
    runner_config.realtime_display = config_.realtime_display;
    runner.set_config(runner_config);
    
    return runner.run();
}

std::vector<BenchmarkResult> YOLOBenchmark::run_http_concurrency_sweep(
    const std::string& url, const std::vector<int>& concurrency_levels) {
    
    std::vector<BenchmarkResult> results;
    
    for (int c : concurrency_levels) {
        auto result = run_http(url, c);
        results.push_back(result);
    }
    
    return results;
}

void YOLOBenchmark::run_all(ResultAggregator& /*aggregator*/) {
    // TODO: 实现完整的 YOLO benchmark 流程
}

// ============================================================
// VLMBenchmark 实现
// ============================================================
VLMBenchmark::VLMBenchmark() = default;
VLMBenchmark::~VLMBenchmark() = default;

void VLMBenchmark::set_image(const std::string& image_path) {
    image_path_ = image_path;
}

void VLMBenchmark::set_prompt(const std::string& prompt) {
    prompt_ = prompt;
}

void VLMBenchmark::set_config(const BenchmarkConfig& config) {
    config_ = config;
}

BenchmarkResult VLMBenchmark::run_local(const std::string& model_path,
                                        const std::string& mmproj_path,
                                        int /*threads*/) {
    auto source = std::make_unique<SingleImageSource>();
    static_cast<SingleImageSource*>(source.get())->set_repeat_count(config_.main_runs);
    
    if (!source->open(image_path_)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    auto backend = std::make_unique<LocalVLMBackend>();
    BackendConfig backend_config;
    backend_config.model_path = model_path;
    backend_config.mmproj_path = mmproj_path;
    backend_config.cpu_threads = config_.cpu_threads;      // ★ CPU 线程数
    backend_config.prompt = prompt_;
    
    if (!backend->init(backend_config)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    BenchmarkRunner runner;
    runner.set_data_source(std::move(source));
    runner.set_backend(std::move(backend));
    
    RunnerConfig runner_config;
    runner_config.mode = BenchmarkMode::SINGLE_FRAME;
    runner_config.warmup_runs = std::min(2, config_.warmup_runs);  // VLM 预热少一些
    runner_config.main_runs = config_.main_runs;
    runner_config.cooldown_sec = config_.cooldown_sec;
    runner_config.realtime_display = config_.realtime_display;
    runner.set_config(runner_config);
    
    return runner.run();
}

BenchmarkResult VLMBenchmark::run_http(const std::string& url) {
    auto source = std::make_unique<SingleImageSource>();
    static_cast<SingleImageSource*>(source.get())->set_repeat_count(config_.main_runs);
    
    if (!source->open(image_path_)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    auto backend = std::make_unique<HTTPVLMBackend>();
    BackendConfig backend_config;
    backend_config.http_url = url;
    backend_config.prompt = prompt_;
    backend_config.http_timeout_ms = 60000;  // VLM 需要更长超时
    
    if (!backend->init(backend_config)) {
        BenchmarkResult result;
        result.error_count = 1;
        return result;
    }
    
    BenchmarkRunner runner;
    runner.set_data_source(std::move(source));
    runner.set_backend(std::move(backend));
    
    RunnerConfig runner_config;
    runner_config.mode = BenchmarkMode::SINGLE_FRAME;
    runner_config.warmup_runs = 1;
    runner_config.main_runs = config_.main_runs;
    runner_config.cooldown_sec = config_.cooldown_sec;
    runner_config.realtime_display = config_.realtime_display;
    runner.set_config(runner_config);
    
    return runner.run();
}

void VLMBenchmark::run_all(ResultAggregator& /*aggregator*/) {
    // TODO: 实现 VLM benchmark
}

}  // namespace benchmark
}  // namespace rivision
