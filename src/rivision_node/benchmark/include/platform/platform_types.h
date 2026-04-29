#pragma once

#include <string>
#include <map>
#include <vector>

namespace rivision {
namespace benchmark {

// ============================================================
// 目标平台枚举
// ============================================================
enum class Platform {
    UNKNOWN = 0,
    Plat_X86_64,         // x86_64 CPU (AVX2/AVX512)
    Plat_RISCV64,        // RISC-V K3 (SpacemiT NPU)
    Plat_ARM64_GPU       // ARM64 + NVIDIA GPU (Jetson Orin)
};

// ============================================================
// YOLO 后端类型
// ============================================================
enum class YOLOBackendType {
    AUTO = 0,       // 自动选择最佳后端
    ORT_CPU,        // ONNX Runtime CPU ExecutionProvider
    ORT_NPU,        // ONNX Runtime SpacemiT NPU ExecutionProvider
    TENSORRT,       // TensorRT (CUDA)
    HTTP            // HTTP 服务调用
};

// ============================================================
// VLM 后端类型
// ============================================================
enum class VLMBackendType {
    AUTO = 0,       // 自动选择最佳后端
    LLAMA_CPU,      // llama.cpp CPU
    LLAMA_NPU,      // llama.cpp SpacemiT smt
    LLAMA_CUDA,     // llama.cpp CUDA
    HTTP            // HTTP 服务调用 (llama-server)
};

// ============================================================
// 平台信息
// ============================================================
struct PlatformInfo {
    Platform platform = Platform::UNKNOWN;
    std::string name;           // "x86_64", "riscv64", "arm64"
    std::string arch;           // "amd64", "riscv64", "arm64"
    std::string description;    // "Intel Core i7 @ 3.6GHz"
    
    // CPU 信息
    int cpu_cores = 0;
    int cpu_threads = 0;
    std::string cpu_model;
    std::vector<std::string> cpu_flags;  // "avx2", "avx512", "rvv"
    
    // 内存信息
    size_t memory_total_mb = 0;
    size_t memory_available_mb = 0;
    
    // 加速器信息
    bool has_npu = false;           // SpacemiT NPU
    std::string npu_info;
    
    bool has_cuda = false;          // NVIDIA CUDA
    std::string cuda_arch;          // "sm_87" for Orin
    std::string cuda_version;
    int cuda_device_count = 0;
    
    bool has_tensorrt = false;      // TensorRT
    std::string tensorrt_version;
};

// ============================================================
// 平台配置
// ============================================================
struct PlatformConfig {
    Platform platform = Platform::UNKNOWN;
    std::string name;           // "x86", "riscv_b", "arm_gpu"
    std::string description;
    
    // 编译配置
    std::string toolchain_file;
    bool cross_compile = false;
    std::string sysroot;
    
    // YOLO 配置
    struct YOLOConfig {
        YOLOBackendType backend = YOLOBackendType::AUTO;
        std::string execution_provider;
        int workers = 2;
        int threads_per_worker = 4;
        std::string ort_lib_dir;
        std::string tensorrt_lib_dir;
    } yolo;
    
    // VLM 配置
    struct VLMConfig {
        VLMBackendType backend = VLMBackendType::AUTO;
        std::string cli_path;       // llama-mtmd-cli
        int threads = 4;
        std::string media_backend;  // "smt" for K3
        int n_gpu_layers = 0;       // -1 for full offload
        std::string extra_args;
    } vlm;
    
    // Benchmark 参数
    struct BenchmarkParams {
        int warmup_runs = 5;
        int main_runs = 100;
        std::vector<int> concurrency_levels = {1, 2, 4};
        int stress_duration_sec = 300;
    } benchmark;
};

// ============================================================
// 辅助函数
// ============================================================

// 平台枚举转字符串
inline std::string platform_to_string(Platform p) {
    switch (p) {
        case Platform::Plat_X86_64:    return "x86_64";
        case Platform::Plat_RISCV64:   return "riscv64";
        case Platform::Plat_ARM64_GPU: return "arm64_gpu";
        default:                       return "unknown";
    }
}

// 字符串转平台枚举
inline Platform string_to_platform(const std::string& s) {
    if (s == "x86_64" || s == "x86" || s == "amd64") return Platform::Plat_X86_64;
    if (s == "riscv64" || s == "riscv_b" || s == "k3") return Platform::Plat_RISCV64;
    if (s == "arm64_gpu" || s == "arm_gpu" || s == "jetson") return Platform::Plat_ARM64_GPU;
    return Platform::UNKNOWN;
}

// YOLO 后端类型转字符串
inline std::string yolo_backend_to_string(YOLOBackendType t) {
    switch (t) {
        case YOLOBackendType::AUTO:     return "auto";
        case YOLOBackendType::ORT_CPU:  return "ort_cpu";
        case YOLOBackendType::ORT_NPU:  return "ort_npu";
        case YOLOBackendType::TENSORRT: return "tensorrt";
        case YOLOBackendType::HTTP:     return "http";
        default:                        return "unknown";
    }
}

// 字符串转 YOLO 后端类型
inline YOLOBackendType string_to_yolo_backend(const std::string& s) {
    if (s == "auto") return YOLOBackendType::AUTO;
    if (s == "ort_cpu" || s == "cpu") return YOLOBackendType::ORT_CPU;
    if (s == "ort_npu" || s == "npu") return YOLOBackendType::ORT_NPU;
    if (s == "tensorrt" || s == "trt") return YOLOBackendType::TENSORRT;
    if (s == "http") return YOLOBackendType::HTTP;
    return YOLOBackendType::AUTO;
}

// VLM 后端类型转字符串
inline std::string vlm_backend_to_string(VLMBackendType t) {
    switch (t) {
        case VLMBackendType::AUTO:       return "auto";
        case VLMBackendType::LLAMA_CPU:  return "llama_cpu";
        case VLMBackendType::LLAMA_NPU:  return "llama_npu";
        case VLMBackendType::LLAMA_CUDA: return "llama_cuda";
        case VLMBackendType::HTTP:       return "http";
        default:                         return "unknown";
    }
}

// 字符串转 VLM 后端类型
inline VLMBackendType string_to_vlm_backend(const std::string& s) {
    if (s == "auto") return VLMBackendType::AUTO;
    if (s == "llama_cpu" || s == "cpu") return VLMBackendType::LLAMA_CPU;
    if (s == "llama_npu" || s == "npu" || s == "smt") return VLMBackendType::LLAMA_NPU;
    if (s == "llama_cuda" || s == "cuda") return VLMBackendType::LLAMA_CUDA;
    if (s == "http") return VLMBackendType::HTTP;
    return VLMBackendType::AUTO;
}

}  // namespace benchmark
}  // namespace rivision
