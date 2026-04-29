#pragma once

#include "platform_types.h"
#include <memory>

namespace rivision {
namespace benchmark {

// ============================================================
// 平台检测器
// ============================================================
class PlatformDetector {
public:
    // 获取单例
    static PlatformDetector& instance();
    
    // 检测当前平台
    Platform detectPlatform() const;
    
    // 获取平台信息
    PlatformInfo getPlatformInfo() const;
    
    // 获取 CPU 信息
    std::string getCPUModel() const;
    int getCPUCores() const;
    int getCPUThreads() const;
    std::vector<std::string> getCPUFlags() const;
    
    // 获取内存信息
    size_t getTotalMemoryMB() const;
    size_t getAvailableMemoryMB() const;
    
    // 检查加速器
    bool hasAVX2() const;
    bool hasAVX512() const;
    bool hasRVV() const;          // RISC-V Vector Extension
    bool hasSpacemiTNPU() const;  // SpacemiT NPU
    bool hasCUDA() const;
    bool hasTensorRT() const;
    
    // 获取 CUDA 信息
    int getCUDADeviceCount() const;
    std::string getCUDAArch() const;
    std::string getCUDAVersion() const;
    
    // 获取最佳后端类型
    YOLOBackendType getBestYOLOBackend() const;
    VLMBackendType getBestVLMBackend() const;

private:
    PlatformDetector();
    ~PlatformDetector();
    
    struct Impl;
    std::unique_ptr<Impl> impl_;
};

}  // namespace benchmark
}  // namespace rivision
