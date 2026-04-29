#pragma once

#include "platform_types.h"
#include "platform_detector.h"
#include "../models/model_types.h"
#include "../inference_backend.h"
#include <memory>
#include <string>

namespace rivision {
namespace benchmark {

// ============================================================
// 后端工厂
// ============================================================
class BackendFactory {
public:
    // 获取单例
    static BackendFactory& instance();
    
    // ========================================
    // YOLO 后端创建
    // ========================================
    
    // 创建 YOLO 后端 (使用模型配置)
    std::unique_ptr<InferenceBackend> createYOLOBackend(
        const YOLOModelConfig& model_config,
        YOLOBackendType type = YOLOBackendType::AUTO,
        Platform platform = Platform::UNKNOWN  // UNKNOWN = 自动检测
    );
    
    // 创建 YOLO 后端 (使用 BackendConfig)
    std::unique_ptr<InferenceBackend> createYOLOBackend(
        const BackendConfig& config,
        YOLOBackendType type = YOLOBackendType::AUTO,
        Platform platform = Platform::UNKNOWN
    );
    
    // ========================================
    // VLM 后端创建
    // ========================================
    
    // 创建 VLM 后端 (使用模型配置)
    std::unique_ptr<InferenceBackend> createVLMBackend(
        const VLMModelConfig& model_config,
        VLMBackendType type = VLMBackendType::AUTO,
        Platform platform = Platform::UNKNOWN
    );
    
    // 创建 VLM 后端 (使用 BackendConfig)
    std::unique_ptr<InferenceBackend> createVLMBackend(
        const BackendConfig& config,
        VLMBackendType type = VLMBackendType::AUTO,
        Platform platform = Platform::UNKNOWN
    );
    
    // ========================================
    // HTTP 后端创建 (平台无关)
    // ========================================
    
    // 创建 HTTP YOLO 后端
    std::unique_ptr<InferenceBackend> createHTTPYOLOBackend(
        const std::string& url,
        int pool_size = 8,
        int timeout_ms = 30000
    );
    
    // 创建 HTTP VLM 后端
    std::unique_ptr<InferenceBackend> createHTTPVLMBackend(
        const std::string& url,
        const std::string& prompt = "Describe this image.",
        int max_tokens = 512,
        int pool_size = 4,
        int timeout_ms = 60000
    );
    
    // ========================================
    // 辅助方法
    // ========================================
    
    // 获取平台最佳 YOLO 后端
    YOLOBackendType getBestYOLOBackend(Platform platform) const;
    
    // 获取平台最佳 VLM 后端
    VLMBackendType getBestVLMBackend(Platform platform) const;
    
    // 检查后端是否支持
    bool isYOLOBackendSupported(YOLOBackendType type, Platform platform) const;
    bool isVLMBackendSupported(VLMBackendType type, Platform platform) const;
    
    // 获取后端描述
    std::string getYOLOBackendDescription(YOLOBackendType type) const;
    std::string getVLMBackendDescription(VLMBackendType type) const;

private:
    BackendFactory();
    ~BackendFactory();
    
    // 内部创建方法
    std::unique_ptr<InferenceBackend> createORTCPUYOLOBackend(const BackendConfig& config);
    std::unique_ptr<InferenceBackend> createORTNPUYOLOBackend(const BackendConfig& config);
    std::unique_ptr<InferenceBackend> createTensorRTYOLOBackend(const BackendConfig& config);
    
    std::unique_ptr<InferenceBackend> createLlamaCPUBackend(const BackendConfig& config);
    std::unique_ptr<InferenceBackend> createLlamaNPUBackend(const BackendConfig& config);
    std::unique_ptr<InferenceBackend> createLlamaCUDABackend(const BackendConfig& config);
};

// ============================================================
// 便捷函数
// ============================================================

// 快速创建 YOLO 后端
inline std::unique_ptr<InferenceBackend> createYOLOBackend(
    const std::string& model_path,
    YOLOBackendType type = YOLOBackendType::AUTO) {
    
    BackendConfig config;
    config.type = "local_yolo";
    config.model_path = model_path;
    return BackendFactory::instance().createYOLOBackend(config, type);
}

// 快速创建 VLM 后端
inline std::unique_ptr<InferenceBackend> createVLMBackend(
    const std::string& model_path,
    const std::string& prompt = "Describe this image.",
    VLMBackendType type = VLMBackendType::AUTO) {
    
    BackendConfig config;
    config.type = "local_vlm";
    config.model_path = model_path;
    config.prompt = prompt;
    return BackendFactory::instance().createVLMBackend(config, type);
}

}  // namespace benchmark
}  // namespace rivision
