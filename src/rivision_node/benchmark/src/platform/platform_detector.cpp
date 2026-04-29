#include "platform/platform_detector.h"
#include <fstream>
#include <sstream>
#include <algorithm>
#include <cstring>
#include <sys/utsname.h>
#include <unistd.h>

#ifdef __x86_64__
#include <cpuid.h>
#endif

namespace rivision {
namespace benchmark {

// ============================================================
// 实现细节
// ============================================================
struct PlatformDetector::Impl {
    Platform detected_platform = Platform::UNKNOWN;
    PlatformInfo info;
    bool initialized = false;
    
    void initialize() {
        if (initialized) return;
        
        detectArchitecture();
        detectCPUInfo();
        detectMemoryInfo();
        detectAccelerators();
        
        initialized = true;
    }
    
    void detectArchitecture() {
        struct utsname uts;
        if (uname(&uts) == 0) {
            std::string machine = uts.machine;
            
            if (machine == "x86_64" || machine == "amd64") {
                detected_platform = Platform::Plat_X86_64;
                info.platform = Platform::Plat_X86_64;
                info.name = "x86_64";
                info.arch = "amd64";
            } else if (machine == "riscv64") {
                detected_platform = Platform::Plat_RISCV64;
                info.platform = Platform::Plat_RISCV64;
                info.name = "riscv64";
                info.arch = "riscv64";
            } else if (machine == "aarch64" || machine == "arm64") {
                // ARM64 可能是带 GPU 或不带 GPU
                detected_platform = Platform::Plat_ARM64_GPU;  // 默认假设有 GPU
                info.platform = Platform::Plat_ARM64_GPU;
                info.name = "arm64";
                info.arch = "arm64";
            }
        }
    }
    
    void detectCPUInfo() {
        // 读取 /proc/cpuinfo
        std::ifstream cpuinfo("/proc/cpuinfo");
        if (!cpuinfo.is_open()) return;
        
        std::string line;
        int processor_count = 0;
        
        while (std::getline(cpuinfo, line)) {
            if (line.find("processor") == 0) {
                processor_count++;
            } else if (line.find("model name") == 0 && info.cpu_model.empty()) {
                size_t pos = line.find(':');
                if (pos != std::string::npos) {
                    info.cpu_model = line.substr(pos + 2);
                }
            } else if (line.find("flags") == 0 || line.find("Features") == 0) {
                size_t pos = line.find(':');
                if (pos != std::string::npos) {
                    std::istringstream iss(line.substr(pos + 2));
                    std::string flag;
                    while (iss >> flag) {
                        info.cpu_flags.push_back(flag);
                    }
                }
            }
        }
        
        info.cpu_threads = processor_count;
        info.cpu_cores = sysconf(_SC_NPROCESSORS_CONF);
        
        // 生成描述
        if (!info.cpu_model.empty()) {
            info.description = info.cpu_model;
        } else {
            info.description = info.name + " (" + std::to_string(info.cpu_cores) + " cores)";
        }
    }
    
    void detectMemoryInfo() {
        std::ifstream meminfo("/proc/meminfo");
        if (!meminfo.is_open()) return;
        
        std::string line;
        while (std::getline(meminfo, line)) {
            if (line.find("MemTotal:") == 0) {
                size_t kb = 0;
                sscanf(line.c_str(), "MemTotal: %zu kB", &kb);
                info.memory_total_mb = kb / 1024;
            } else if (line.find("MemAvailable:") == 0) {
                size_t kb = 0;
                sscanf(line.c_str(), "MemAvailable: %zu kB", &kb);
                info.memory_available_mb = kb / 1024;
            }
        }
    }
    
    void detectAccelerators() {
        // 检查 CPU 特性
        auto hasFlag = [this](const std::string& flag) {
            return std::find(info.cpu_flags.begin(), info.cpu_flags.end(), flag) 
                   != info.cpu_flags.end();
        };
        
#ifdef __x86_64__
        // x86 检查 AVX2/AVX512
        if (hasFlag("avx2")) {
            info.cpu_flags.push_back("avx2_verified");
        }
        if (hasFlag("avx512f")) {
            info.cpu_flags.push_back("avx512_verified");
        }
#endif
        
#ifdef __riscv
        // RISC-V 检查向量扩展
        if (hasFlag("v") || hasFlag("rvv")) {
            info.cpu_flags.push_back("rvv_verified");
        }
#endif
        
        // 检查 SpacemiT NPU (K3)
        if (detected_platform == Platform::Plat_RISCV64) {
            // 检查 NPU 设备或驱动
            std::ifstream npu_check("/dev/spacemit_npu");
            if (npu_check.good() || std::ifstream("/sys/class/misc/spacemit-npu").good()) {
                info.has_npu = true;
                info.npu_info = "SpacemiT NPU";
            } else {
                // 尝试检查库文件
                std::ifstream ort_npu("/usr/lib/libonnxruntime_providers_spacemit.so");
                if (ort_npu.good()) {
                    info.has_npu = true;
                    info.npu_info = "SpacemiT NPU (via ORT)";
                }
            }
        }
        
        // 检查 CUDA
        detectCUDA();
        
        // 检查 TensorRT
        if (info.has_cuda) {
            std::ifstream trt_lib("/usr/lib/aarch64-linux-gnu/libnvinfer.so");
            if (trt_lib.good()) {
                info.has_tensorrt = true;
                info.tensorrt_version = "detected";
            }
        }
    }
    
    void detectCUDA() {
#ifdef USE_CUDA
        info.has_cuda = true;
        // 实际需要调用 CUDA API
#else
        // 检查 nvidia-smi 或 CUDA 库
        std::ifstream nvidia_smi("/usr/bin/nvidia-smi");
        std::ifstream cuda_lib("/usr/local/cuda/lib64/libcudart.so");
        
        if (nvidia_smi.good() || cuda_lib.good()) {
            info.has_cuda = true;
            
            // 尝试获取 CUDA 版本
            FILE* pipe = popen("nvidia-smi --query-gpu=driver_version --format=csv,noheader 2>/dev/null", "r");
            if (pipe) {
                char buffer[128];
                if (fgets(buffer, sizeof(buffer), pipe)) {
                    info.cuda_version = buffer;
                    // 去除换行符
                    info.cuda_version.erase(
                        std::remove(info.cuda_version.begin(), info.cuda_version.end(), '\n'),
                        info.cuda_version.end()
                    );
                }
                pclose(pipe);
            }
            
            // 检查 GPU 数量
            pipe = popen("nvidia-smi --query-gpu=count --format=csv,noheader 2>/dev/null | wc -l", "r");
            if (pipe) {
                char buffer[16];
                if (fgets(buffer, sizeof(buffer), pipe)) {
                    info.cuda_device_count = atoi(buffer);
                }
                pclose(pipe);
            }
            
            // Jetson Orin 架构
            if (detected_platform == Platform::Plat_ARM64_GPU) {
                info.cuda_arch = "sm_87";
            }
        }
#endif
    }
};

// ============================================================
// PlatformDetector 实现
// ============================================================

PlatformDetector& PlatformDetector::instance() {
    static PlatformDetector instance;
    return instance;
}

PlatformDetector::PlatformDetector() : impl_(std::make_unique<Impl>()) {
    impl_->initialize();
}

PlatformDetector::~PlatformDetector() = default;

Platform PlatformDetector::detectPlatform() const {
    return impl_->detected_platform;
}

PlatformInfo PlatformDetector::getPlatformInfo() const {
    return impl_->info;
}

std::string PlatformDetector::getCPUModel() const {
    return impl_->info.cpu_model;
}

int PlatformDetector::getCPUCores() const {
    return impl_->info.cpu_cores;
}

int PlatformDetector::getCPUThreads() const {
    return impl_->info.cpu_threads;
}

std::vector<std::string> PlatformDetector::getCPUFlags() const {
    return impl_->info.cpu_flags;
}

size_t PlatformDetector::getTotalMemoryMB() const {
    return impl_->info.memory_total_mb;
}

size_t PlatformDetector::getAvailableMemoryMB() const {
    return impl_->info.memory_available_mb;
}

bool PlatformDetector::hasAVX2() const {
    auto& flags = impl_->info.cpu_flags;
    return std::find(flags.begin(), flags.end(), "avx2") != flags.end();
}

bool PlatformDetector::hasAVX512() const {
    auto& flags = impl_->info.cpu_flags;
    return std::find(flags.begin(), flags.end(), "avx512f") != flags.end();
}

bool PlatformDetector::hasRVV() const {
    auto& flags = impl_->info.cpu_flags;
    return std::find(flags.begin(), flags.end(), "v") != flags.end() ||
           std::find(flags.begin(), flags.end(), "rvv") != flags.end();
}

bool PlatformDetector::hasSpacemiTNPU() const {
    return impl_->info.has_npu;
}

bool PlatformDetector::hasCUDA() const {
    return impl_->info.has_cuda;
}

bool PlatformDetector::hasTensorRT() const {
    return impl_->info.has_tensorrt;
}

int PlatformDetector::getCUDADeviceCount() const {
    return impl_->info.cuda_device_count;
}

std::string PlatformDetector::getCUDAArch() const {
    return impl_->info.cuda_arch;
}

std::string PlatformDetector::getCUDAVersion() const {
    return impl_->info.cuda_version;
}

YOLOBackendType PlatformDetector::getBestYOLOBackend() const {
    switch (impl_->detected_platform) {
        case Platform::Plat_X86_64:
            return YOLOBackendType::ORT_CPU;
        case Platform::Plat_RISCV64:
            return impl_->info.has_npu ? YOLOBackendType::ORT_NPU : YOLOBackendType::ORT_CPU;
        case Platform::Plat_ARM64_GPU:
            return impl_->info.has_tensorrt ? YOLOBackendType::TENSORRT : YOLOBackendType::ORT_CPU;
        default:
            return YOLOBackendType::ORT_CPU;
    }
}

VLMBackendType PlatformDetector::getBestVLMBackend() const {
    switch (impl_->detected_platform) {
        case Platform::Plat_X86_64:
            return VLMBackendType::LLAMA_CPU;
        case Platform::Plat_RISCV64:
            return impl_->info.has_npu ? VLMBackendType::LLAMA_NPU : VLMBackendType::LLAMA_CPU;
        case Platform::Plat_ARM64_GPU:
            return impl_->info.has_cuda ? VLMBackendType::LLAMA_CUDA : VLMBackendType::LLAMA_CPU;
        default:
            return VLMBackendType::LLAMA_CPU;
    }
}

}  // namespace benchmark
}  // namespace rivision
