#include "inference_backend.h"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <unistd.h>
#include <array>
#include <regex>
#include <fstream>
#include <sstream>
#include <thread>
#include <filesystem>

namespace rivision {
namespace benchmark {

// ============================================================
// LocalVLMBackend 实现 (通过 llama-mtmd-cli)
// ============================================================
struct LocalVLMBackend::Impl {
    std::string cli_path;
    std::string model_path;
    std::string mmproj_path;
    std::string prompt;
    int threads = 6;
    int max_tokens = 512;
    float temperature = 0.3f;
    
    // 临时文件
    std::string temp_image_path;
};

LocalVLMBackend::LocalVLMBackend() : impl_(std::make_unique<Impl>()) {}
LocalVLMBackend::~LocalVLMBackend() { shutdown(); }

bool LocalVLMBackend::init(const BackendConfig& config) {
    config_ = config;
    
    impl_->model_path = config.model_path;
    impl_->mmproj_path = config.mmproj_path;
    impl_->prompt = config.prompt;
    impl_->max_tokens = config.max_tokens;
    impl_->temperature = config.temperature;
    
    // VLM 线程数：优先使用环境变量，然后使用配置值
    // K3 A100: llama.cpp 支持全部 8 核
    impl_->threads = config.cpu_threads;
    const char* env_threads = std::getenv("VLM_THREADS");
    if (env_threads) {
        impl_->threads = std::atoi(env_threads);
        if (impl_->threads < 1) impl_->threads = 1;
        if (impl_->threads > 8) impl_->threads = 8;
    }
    printf("[LocalVLMBackend] VLM_THREADS=%d (A100 cores)\n", impl_->threads);
    
    // 查找 llama-mtmd-cli
    // 优先级: bin/ 目录 > 当前目录 > PATH > 系统路径
    std::vector<std::string> cli_paths = {
        "./bin/llama-mtmd-cli",                    // 打包部署目录
        "llama-mtmd-cli",                          // PATH 中
        "./llama-mtmd-cli",                        // 当前目录
        "../engines/llama.cpp/llama-mtmd-cli",    // 开发目录
        "/opt/rivision/rivision_node/engines/llama.cpp/llama-mtmd-cli"  // 系统安装
    };
    
    for (const auto& path : cli_paths) {
        if (std::filesystem::exists(path) || system(("which " + path + " > /dev/null 2>&1").c_str()) == 0) {
            impl_->cli_path = path;
            break;
        }
    }
    
    if (impl_->cli_path.empty()) {
        fprintf(stderr, "[LocalVLMBackend] llama-mtmd-cli not found\n");
        return false;
    }
    
    // 检查模型文件
    if (!std::filesystem::exists(config.model_path)) {
        fprintf(stderr, "[LocalVLMBackend] Model not found: %s\n", config.model_path.c_str());
        return false;
    }
    
    // 创建临时图片路径
    impl_->temp_image_path = "/tmp/vlm_benchmark_" + std::to_string(getpid()) + ".jpg";
    
    is_loaded_ = true;
    printf("[LocalVLMBackend] Initialized: %s (cpu_threads=%d)\n", 
           config.model_path.c_str(), config.cpu_threads);
    
    return true;
}

InferenceResult LocalVLMBackend::infer(const Frame& frame) {
    InferenceResult result;
    result.frame_id = frame.frame_id;
    result.timestamp_us = frame.timestamp_us;
    
    if (!is_loaded_) {
        result.error = "Model not loaded";
        return result;
    }
    
    auto total_start = Clock::now();
    
    // 保存帧到临时文件
    {
        std::ofstream file(impl_->temp_image_path, std::ios::binary);
        file.write(reinterpret_cast<const char*>(frame.data.data()), frame.data.size());
    }
    
    // 构建命令
    std::ostringstream cmd;
    cmd << impl_->cli_path
        << " -m " << impl_->model_path
        << " --image " << impl_->temp_image_path
        << " -p \"" << impl_->prompt << "\""
        << " -t " << impl_->threads
        << " -n " << impl_->max_tokens
        << " --temp " << impl_->temperature
        << " --seed 42"
        << " --no-warmup"
        << " 2>&1";
    
    // mmproj 参数
    if (impl_->mmproj_path == "smt") {
        std::string config_dir = std::filesystem::path(impl_->model_path).parent_path().string();
        cmd << " --media-backend smt --smt-config-dir " << config_dir;
    } else if (!impl_->mmproj_path.empty() && std::filesystem::exists(impl_->mmproj_path)) {
        cmd << " --mmproj " << impl_->mmproj_path;
    }
    
    // 执行
    std::array<char, 4096> buffer;
    std::string output;
    
    FILE* pipe = popen(cmd.str().c_str(), "r");
    if (!pipe) {
        result.error = "Failed to execute llama-mtmd-cli";
        return result;
    }
    
    while (fgets(buffer.data(), buffer.size(), pipe) != nullptr) {
        output += buffer.data();
    }
    
    int exit_code = pclose(pipe);
    
    auto total_end = Clock::now();
    result.total_ms = Duration(total_end - total_start).count();
    
    if (exit_code != 0) {
        result.error = "llama-mtmd-cli failed with code " + std::to_string(exit_code);
        return result;
    }
    
    // 解析输出
    // Vision 编码时间
    std::regex vision_regex(R"(image slice encoded in (\d+) ms)");
    std::smatch match;
    std::string::const_iterator search_start(output.cbegin());
    
    while (std::regex_search(search_start, output.cend(), match, vision_regex)) {
        result.vision_encode_ms += std::stod(match[1].str());
        search_start = match.suffix().first;
    }
    
    // Prompt eval
    std::regex prompt_regex(R"(prompt eval time =\s+(\d+\.?\d*) ms)");
    if (std::regex_search(output, match, prompt_regex)) {
        result.prompt_eval_ms = std::stod(match[1].str());
    }
    
    // Eval (token generation)
    std::regex eval_regex(R"(eval time =\s+(\d+\.?\d*) ms / (\d+) tokens)");
    if (std::regex_search(output, match, eval_regex)) {
        result.token_gen_ms = std::stod(match[1].str());
        result.tokens_generated = std::stoi(match[2].str());
    }
    
    // Tokens per second
    std::regex tps_regex(R"((\d+\.?\d*) tokens per second)");
    search_start = output.cbegin();
    while (std::regex_search(search_start, output.cend(), match, tps_regex)) {
        result.tokens_per_sec = std::stod(match[1].str());  // 取最后一个
        search_start = match.suffix().first;
    }
    
    // Total time
    std::regex total_regex(R"(total time =\s+(\d+\.?\d*) ms)");
    if (std::regex_search(output, match, total_regex)) {
        result.inference_ms = std::stod(match[1].str());
    }
    
    // TTFT = Vision + Prompt eval
    result.ttft_ms = result.vision_encode_ms + result.prompt_eval_ms;
    
    // 提取生成的文本 (简化：最后几行)
    std::istringstream iss(output);
    std::string line;
    std::vector<std::string> lines;
    while (std::getline(iss, line)) {
        if (!line.empty() && line[0] != '[' && line.find("time =") == std::string::npos) {
            lines.push_back(line);
        }
    }
    
    if (lines.size() > 3) {
        for (size_t i = lines.size() - 3; i < lines.size(); i++) {
            result.text_output += lines[i] + "\n";
        }
    }
    
    result.success = true;
    return result;
}

BackendInfo LocalVLMBackend::info() const {
    BackendInfo info;
    info.name = "LocalVLM";
    info.version = "1.0.0";
    info.backend_type = "local";
    info.model_name = config_.model_path;
    info.workers = 1;
    info.is_loaded = is_loaded_;
    return info;
}

double LocalVLMBackend::get_memory_usage_mb() const {
    std::ifstream statm("/proc/self/statm");
    if (statm) {
        long pages;
        statm >> pages;
        statm >> pages;
        return pages * 4096.0 / (1024 * 1024);
    }
    return 0;
}

void LocalVLMBackend::shutdown() {
    // 清理临时文件
    if (!impl_->temp_image_path.empty()) {
        std::filesystem::remove(impl_->temp_image_path);
    }
    is_loaded_ = false;
}

}  // namespace benchmark
}  // namespace rivision
