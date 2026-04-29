#include "benchmark_common.h"
#include "data_source.h"
#include "inference_backend.h"
#include "metrics_collector.h"
#include "benchmark_runner.h"

#include <iostream>
#include <cstring>
#include <cstdlib>
#include <filesystem>

using namespace rivision::benchmark;

void print_usage(const char* prog) {
    printf("RiVision YOLO/VLM Benchmark Tool\n\n");
    printf("Usage: %s <command> [options]\n\n", prog);
    printf("Commands:\n");
    printf("  yolo-local     Run local YOLO benchmark\n");
    printf("  yolo-http      Run HTTP YOLO benchmark\n");
    printf("  vlm-local      Run local VLM benchmark\n");
    printf("  vlm-http       Run HTTP VLM benchmark\n");
    printf("  all            Run all benchmarks\n");
    printf("\n");
    printf("Options:\n");
    printf("  --image PATH       Test image path (required)\n");
    printf("  --video PATH       Test video path (optional)\n");
    printf("  --model PATH       Model file path (for local)\n");
    printf("  --mmproj PATH      VLM mmproj path (for local VLM)\n");
    printf("  --url URL          HTTP service URL\n");
    printf("  --runs N           Number of test runs (default: 100)\n");
    printf("  --warmup N         Warmup runs (default: 5)\n");
    printf("  --threads N        Number of threads (default: 4)\n");
    printf("  --concurrency N    Concurrency level (default: 1)\n");
    printf("  --sweep            Run concurrency sweep (1,2,4,8)\n");
    printf("  --output DIR       Output directory (default: ./results)\n");
    printf("  --quiet            Disable realtime display\n");
    printf("  --help             Show this help\n");
    printf("\n");
    printf("Examples:\n");
    printf("  %s yolo-http --image test.jpg --url http://localhost:9081 --sweep\n", prog);
    printf("  %s yolo-local --image test.jpg --model models/yolov8n.onnx --threads 4\n", prog);
    printf("  %s vlm-local --image test.jpg --model models/minicpm.gguf --mmproj smt\n", prog);
}

int main(int argc, char* argv[]) {
    if (argc < 2) {
        print_usage(argv[0]);
        return 1;
    }
    
    std::string command = argv[1];
    
    if (command == "--help" || command == "-h") {
        print_usage(argv[0]);
        return 0;
    }
    
    // 解析参数
    std::string image_path;
    std::string video_path;
    std::string model_path;
    std::string mmproj_path;
    std::string url = "http://127.0.0.1:9081";
    std::string output_dir = "./results";
    int runs = 100;
    int warmup = 5;
    int threads = 4;
    int concurrency = 1;
    bool sweep = false;
    bool quiet = false;
    
    for (int i = 2; i < argc; i++) {
        if (strcmp(argv[i], "--image") == 0 && i + 1 < argc) {
            image_path = argv[++i];
        } else if (strcmp(argv[i], "--video") == 0 && i + 1 < argc) {
            video_path = argv[++i];
        } else if (strcmp(argv[i], "--model") == 0 && i + 1 < argc) {
            model_path = argv[++i];
        } else if (strcmp(argv[i], "--mmproj") == 0 && i + 1 < argc) {
            mmproj_path = argv[++i];
        } else if (strcmp(argv[i], "--url") == 0 && i + 1 < argc) {
            url = argv[++i];
        } else if (strcmp(argv[i], "--runs") == 0 && i + 1 < argc) {
            runs = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--warmup") == 0 && i + 1 < argc) {
            warmup = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--threads") == 0 && i + 1 < argc) {
            threads = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--concurrency") == 0 && i + 1 < argc) {
            concurrency = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--output") == 0 && i + 1 < argc) {
            output_dir = argv[++i];
        } else if (strcmp(argv[i], "--sweep") == 0) {
            sweep = true;
        } else if (strcmp(argv[i], "--quiet") == 0) {
            quiet = true;
        } else if (strcmp(argv[i], "--help") == 0) {
            print_usage(argv[0]);
            return 0;
        }
    }
    
    // 验证参数
    if (image_path.empty()) {
        fprintf(stderr, "Error: --image is required\n");
        return 1;
    }
    
    if (!std::filesystem::exists(image_path)) {
        fprintf(stderr, "Error: Image not found: %s\n", image_path.c_str());
        return 1;
    }
    
    // 创建输出目录
    std::filesystem::create_directories(output_dir);
    
    // 打印平台信息 (传入模型路径)
    print_platform_info(model_path);
    
    // 配置
    BenchmarkConfig config;
    config.warmup_runs = warmup;
    config.main_runs = runs;
    config.realtime_display = !quiet;
    config.output_dir = output_dir;
    
    ResultAggregator aggregator;
    
    // 执行测试
    if (command == "yolo-local") {
        if (model_path.empty()) {
            fprintf(stderr, "Error: --model is required for yolo-local\n");
            return 1;
        }
        
        YOLOBenchmark benchmark;
        benchmark.set_image(image_path);
        benchmark.set_config(config);
        
        auto result = benchmark.run_local(model_path, threads);
        print_result(result);
        aggregator.add_result(result);
        
    } else if (command == "yolo-http") {
        YOLOBenchmark benchmark;
        benchmark.set_image(image_path);
        benchmark.set_config(config);
        
        if (sweep) {
            std::vector<int> levels = {1, 2, 4, 8};
            auto results = benchmark.run_http_concurrency_sweep(url, levels);
            
            for (const auto& r : results) {
                print_result(r);
                aggregator.add_result(r);
            }
            
            print_comparison_table(results);
            
            if (!results.empty()) {
                double max_fps = 0;
                for (const auto& r : results) {
                    if (r.fps > max_fps) max_fps = r.fps;
                }
                print_multi_stream_capacity(max_fps, {30, 15, 10, 5, 3});
            }
        } else {
            auto result = benchmark.run_http(url, concurrency);
            print_result(result);
            aggregator.add_result(result);
        }
        
    } else if (command == "vlm-local") {
        if (model_path.empty()) {
            fprintf(stderr, "Error: --model is required for vlm-local\n");
            return 1;
        }
        
        VLMBenchmark benchmark;
        benchmark.set_image(image_path);
        benchmark.set_config(config);
        
        auto result = benchmark.run_local(model_path, mmproj_path, threads);
        print_result(result);
        aggregator.add_result(result);
        
    } else if (command == "vlm-http") {
        VLMBenchmark benchmark;
        benchmark.set_image(image_path);
        benchmark.set_config(config);
        
        auto result = benchmark.run_http(url);
        print_result(result);
        aggregator.add_result(result);
        
    } else if (command == "all") {
        printf("Running all benchmarks...\n\n");
        
        // YOLO HTTP with sweep
        {
            YOLOBenchmark benchmark;
            benchmark.set_image(image_path);
            benchmark.set_config(config);
            
            auto results = benchmark.run_http_concurrency_sweep(url, {1, 2, 4, 8});
            for (const auto& r : results) {
                aggregator.add_result(r);
            }
        }
        
        // VLM HTTP (如果有 VLM 服务)
        // ...
        
        aggregator.print_summary();
        aggregator.export_report(output_dir);
        
    } else {
        fprintf(stderr, "Unknown command: %s\n", command.c_str());
        print_usage(argv[0]);
        return 1;
    }
    
    // 导出结果
    printf("\nResults saved to: %s/\n", output_dir.c_str());
    
    return 0;
}
