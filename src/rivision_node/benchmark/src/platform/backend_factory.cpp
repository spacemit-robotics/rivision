#include "platform/backend_factory.h"
#ifdef USE_TENSORRT
#include "inference_backend/yolo_tensorrt.h"
#endif
#include <stdexcept>
#include <iostream>

namespace rivision {
namespace benchmark {

// ============================================================
// BackendFactory 实现
// ============================================================

BackendFactory& BackendFactory::instance() {
    static BackendFactory instance;
    return instance;
}

BackendFactory::BackendFactory() = default;
BackendFactory::~BackendFactory() = default;

// ============================================================
// YOLO 后端创建
// ============================================================

std::unique_ptr<InferenceBackend> BackendFactory::createYOLOBackend(
    const YOLOModelConfig& model_config,
    YOLOBackendType type,
    Platform platform) {
    
    // 自动检测平台
    if (platform == Platform::UNKNOWN) {
        platform = PlatformDetector::instance().detectPlatform();
    }
    
    // 自动选择最佳后端
    if (type == YOLOBackendType::AUTO) {
        type = getBestYOLOBackend(platform);
    }
    
    // 构建 BackendConfig
    BackendConfig config;
    config.type = "local_yolo";
    config.model_path = model_config.getModelFile(platform);
    config.conf_threshold = model_config.conf_threshold;
    config.nms_threshold = model_config.nms_threshold;
    
    return createYOLOBackend(config, type, platform);
}

std::unique_ptr<InferenceBackend> BackendFactory::createYOLOBackend(
    const BackendConfig& config,
    YOLOBackendType type,
    Platform platform) {
    
    // 自动检测平台
    if (platform == Platform::UNKNOWN) {
        platform = PlatformDetector::instance().detectPlatform();
    }
    
    // 自动选择最佳后端
    if (type == YOLOBackendType::AUTO) {
        type = getBestYOLOBackend(platform);
    }
    
    // 创建对应后端
    switch (type) {
        case YOLOBackendType::ORT_CPU:
            return createORTCPUYOLOBackend(config);
            
        case YOLOBackendType::ORT_NPU:
            return createORTNPUYOLOBackend(config);
            
        case YOLOBackendType::TENSORRT:
            return createTensorRTYOLOBackend(config);
            
        case YOLOBackendType::HTTP:
            return createHTTPYOLOBackend(config.http_url, config.http_pool_size, config.http_timeout_ms);
            
        default:
            throw std::runtime_error("Unsupported YOLO backend type");
    }
}

// ============================================================
// VLM 后端创建
// ============================================================

std::unique_ptr<InferenceBackend> BackendFactory::createVLMBackend(
    const VLMModelConfig& model_config,
    VLMBackendType type,
    Platform platform) {
    
    // 自动检测平台
    if (platform == Platform::UNKNOWN) {
        platform = PlatformDetector::instance().detectPlatform();
    }
    
    // 自动选择最佳后端
    if (type == VLMBackendType::AUTO) {
        type = getBestVLMBackend(platform);
    }
    
    // 检查平台兼容性
    if (!model_config.isPlatformSupported(platform)) {
        throw std::runtime_error("Model " + model_config.name + 
                                 " is not supported on platform " + 
                                 platform_to_string(platform));
    }
    
    // 构建 BackendConfig
    BackendConfig config;
    config.type = "local_vlm";
    config.model_path = model_config.model_file;
    config.mmproj_path = model_config.mmproj_file;
    config.prompt = model_config.default_prompt;
    config.max_tokens = model_config.max_tokens;
    config.temperature = model_config.temperature;
    
    // 平台特定参数
    auto params = model_config.getPlatformParams(platform);
    config.cpu_threads = params.threads;
    
    return createVLMBackend(config, type, platform);
}

std::unique_ptr<InferenceBackend> BackendFactory::createVLMBackend(
    const BackendConfig& config,
    VLMBackendType type,
    Platform platform) {
    
    // 自动检测平台
    if (platform == Platform::UNKNOWN) {
        platform = PlatformDetector::instance().detectPlatform();
    }
    
    // 自动选择最佳后端
    if (type == VLMBackendType::AUTO) {
        type = getBestVLMBackend(platform);
    }
    
    // 创建对应后端
    switch (type) {
        case VLMBackendType::LLAMA_CPU:
            return createLlamaCPUBackend(config);
            
        case VLMBackendType::LLAMA_NPU:
            return createLlamaNPUBackend(config);
            
        case VLMBackendType::LLAMA_CUDA:
            return createLlamaCUDABackend(config);
            
        case VLMBackendType::HTTP:
            return createHTTPVLMBackend(config.http_url, config.prompt, config.max_tokens);
            
        default:
            throw std::runtime_error("Unsupported VLM backend type");
    }
}

// ============================================================
// HTTP 后端创建
// ============================================================

std::unique_ptr<InferenceBackend> BackendFactory::createHTTPYOLOBackend(
    const std::string& url,
    int pool_size,
    int timeout_ms) {
    
    BackendConfig config;
    config.type = "http_yolo";
    config.http_url = url;
    config.http_pool_size = pool_size;
    config.http_timeout_ms = timeout_ms;
    
    auto backend = std::make_unique<HTTPYOLOBackend>();
    if (!backend->init(config)) {
        throw std::runtime_error("Failed to initialize HTTP YOLO backend");
    }
    return backend;
}

std::unique_ptr<InferenceBackend> BackendFactory::createHTTPVLMBackend(
    const std::string& url,
    const std::string& prompt,
    int max_tokens,
    int pool_size,
    int timeout_ms) {
    
    BackendConfig config;
    config.type = "http_vlm";
    config.http_url = url;
    config.prompt = prompt;
    config.max_tokens = max_tokens;
    config.http_pool_size = pool_size;
    config.http_timeout_ms = timeout_ms;
    
    auto backend = std::make_unique<HTTPVLMBackend>();
    if (!backend->init(config)) {
        throw std::runtime_error("Failed to initialize HTTP VLM backend");
    }
    return backend;
}

// ============================================================
// 辅助方法
// ============================================================

YOLOBackendType BackendFactory::getBestYOLOBackend(Platform platform) const {
    return PlatformDetector::instance().getBestYOLOBackend();
}

VLMBackendType BackendFactory::getBestVLMBackend(Platform platform) const {
    return PlatformDetector::instance().getBestVLMBackend();
}

bool BackendFactory::isYOLOBackendSupported(YOLOBackendType type, Platform platform) const {
    switch (type) {
        case YOLOBackendType::ORT_CPU:
            return true;  // 所有平台支持
        case YOLOBackendType::ORT_NPU: {
            bool supported = platform == Platform::Plat_RISCV64 && 
                   PlatformDetector::instance().hasSpacemiTNPU();
            return supported;
        }
        case YOLOBackendType::TENSORRT:
            return platform == Platform::Plat_ARM64_GPU && 
                   PlatformDetector::instance().hasTensorRT();
        case YOLOBackendType::HTTP:
            return true;  // 所有平台支持
        default:
            return false;
    }
}

bool BackendFactory::isVLMBackendSupported(VLMBackendType type, Platform platform) const {
    switch (type) {
        case VLMBackendType::LLAMA_CPU:
            return true;  // 所有平台支持
        case VLMBackendType::LLAMA_NPU: {
            bool supported = platform == Platform::Plat_RISCV64 && 
                   PlatformDetector::instance().hasSpacemiTNPU();
            return supported;
        }
        case VLMBackendType::LLAMA_CUDA:
            return platform == Platform::Plat_ARM64_GPU && 
                   PlatformDetector::instance().hasCUDA();
        case VLMBackendType::HTTP:
            return true;  // 所有平台支持
        default:
            return false;
    }
}

std::string BackendFactory::getYOLOBackendDescription(YOLOBackendType type) const {
    switch (type) {
        case YOLOBackendType::ORT_CPU:  return "ONNX Runtime CPU ExecutionProvider";
        case YOLOBackendType::ORT_NPU:  return "ONNX Runtime SpacemiT NPU ExecutionProvider";
        case YOLOBackendType::TENSORRT: return "TensorRT (CUDA)";
        case YOLOBackendType::HTTP:     return "HTTP Service (yolo-server)";
        default:                        return "Unknown";
    }
}

std::string BackendFactory::getVLMBackendDescription(VLMBackendType type) const {
    switch (type) {
        case VLMBackendType::LLAMA_CPU:  return "llama.cpp CPU";
        case VLMBackendType::LLAMA_NPU:  return "llama.cpp SpacemiT smt (NPU)";
        case VLMBackendType::LLAMA_CUDA: return "llama.cpp CUDA";
        case VLMBackendType::HTTP:       return "HTTP Service (llama-server)";
        default:                         return "Unknown";
    }
}

// ============================================================
// 内部创建方法
// ============================================================

std::unique_ptr<InferenceBackend> BackendFactory::createORTCPUYOLOBackend(
    const BackendConfig& config) {
    
    // 使用现有的 LocalYOLOBackend
    auto backend = std::make_unique<LocalYOLOBackend>();
    if (!backend->init(config)) {
        throw std::runtime_error("Failed to initialize ORT CPU YOLO backend");
    }
    return backend;
}

std::unique_ptr<InferenceBackend> BackendFactory::createORTNPUYOLOBackend(
    const BackendConfig& config) {
    
#ifdef USE_SPACEMIT_NPU
    // 使用 SpacemiT NPU ExecutionProvider
    auto backend = std::make_unique<LocalYOLOBackend>();
    // 设置 NPU 特定选项
    BackendConfig npu_config = config;
    // TODO: 添加 NPU 特定配置
    if (!backend->init(npu_config)) {
        throw std::runtime_error("Failed to initialize ORT NPU YOLO backend");
    }
    return backend;
#else
    throw std::runtime_error("SpacemiT NPU support not compiled");
#endif
}

std::unique_ptr<InferenceBackend> BackendFactory::createTensorRTYOLOBackend(
    const BackendConfig& config) {
    
#ifdef USE_TENSORRT
    auto backend = std::make_unique<TensorRTYOLOBackend>();
    if (!backend->init(config)) {
        throw std::runtime_error("Failed to initialize TensorRT YOLO backend");
    }
    return backend;
#else
    throw std::runtime_error("TensorRT support not compiled. Rebuild with -DUSE_TENSORRT=ON");
#endif
}

std::unique_ptr<InferenceBackend> BackendFactory::createLlamaCPUBackend(
    const BackendConfig& config) {
    
    // 使用现有的 LocalVLMBackend
    auto backend = std::make_unique<LocalVLMBackend>();
    if (!backend->init(config)) {
        throw std::runtime_error("Failed to initialize llama CPU backend");
    }
    return backend;
}

std::unique_ptr<InferenceBackend> BackendFactory::createLlamaNPUBackend(
    const BackendConfig& config) {
    
#ifdef USE_SPACEMIT_NPU
    // 使用 smt media backend
    auto backend = std::make_unique<LocalVLMBackend>();
    BackendConfig npu_config = config;
    // TODO: 添加 --media-backend smt 参数
    if (!backend->init(npu_config)) {
        throw std::runtime_error("Failed to initialize llama NPU backend");
    }
    return backend;
#else
    throw std::runtime_error("SpacemiT NPU support not compiled");
#endif
}

std::unique_ptr<InferenceBackend> BackendFactory::createLlamaCUDABackend(
    const BackendConfig& config) {
    
#ifdef USE_CUDA
    // 使用 CUDA offload
    auto backend = std::make_unique<LocalVLMBackend>();
    BackendConfig cuda_config = config;
    // TODO: 添加 --n-gpu-layers -1 参数
    if (!backend->init(cuda_config)) {
        throw std::runtime_error("Failed to initialize llama CUDA backend");
    }
    return backend;
#else
    throw std::runtime_error("CUDA support not compiled");
#endif
}

}  // namespace benchmark
}  // namespace rivision
