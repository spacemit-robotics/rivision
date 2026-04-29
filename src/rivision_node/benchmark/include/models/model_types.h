#pragma once

#include <string>
#include <map>
#include <vector>
#include "../platform/platform_types.h"

namespace rivision {
namespace benchmark {

// ============================================================
// YOLO 模型系列
// ============================================================
enum class YOLOFamily {
    UNKNOWN = 0,
    YOLOv5,
    YOLOv8,
    YOLOv11
};

// ============================================================
// YOLO 模型尺寸
// ============================================================
enum class YOLOSize {
    UNKNOWN = 0,
    NANO,       // n
    SMALL,      // s
    MEDIUM,     // m
    LARGE,      // l
    XLARGE      // x
};

// ============================================================
// YOLO 平台模型配置
// ============================================================
struct YOLOPlatformModel {
    std::string file;               // 模型文件路径
    std::string provider;           // ExecutionProvider
    std::string precision;          // "fp32", "fp16", "int8"
    std::map<std::string, std::string> options;
};

// ============================================================
// YOLO 模型配置
// ============================================================
struct YOLOModelConfig {
    std::string name;               // "yolov8n"
    YOLOFamily family = YOLOFamily::UNKNOWN;
    YOLOSize size = YOLOSize::UNKNOWN;
    
    // 输入规格
    int input_width = 640;
    int input_height = 640;
    int input_channels = 3;
    std::string input_format = "NCHW";  // "NCHW" or "NHWC"
    
    // 推理参数
    float conf_threshold = 0.25f;
    float nms_threshold = 0.45f;
    int max_detections = 100;
    
    // 类别信息
    int num_classes = 80;
    std::vector<std::string> class_names;
    
    // 平台模型文件映射
    std::map<Platform, YOLOPlatformModel> platform_models;
    
    // 预期性能 (用于验证)
    struct ExpectedPerf {
        float fps_min = 0;
        float fps_max = 0;
        float latency_ms = 0;
    };
    std::map<Platform, ExpectedPerf> expected_performance;
    
    // 获取平台模型文件
    std::string getModelFile(Platform platform) const {
        auto it = platform_models.find(platform);
        if (it != platform_models.end()) {
            return it->second.file;
        }
        return "";
    }
    
    // 获取平台 ExecutionProvider
    std::string getProvider(Platform platform) const {
        auto it = platform_models.find(platform);
        if (it != platform_models.end()) {
            return it->second.provider;
        }
        return "CPUExecutionProvider";
    }
};

// ============================================================
// VLM 模型系列
// ============================================================
enum class VLMFamily {
    UNKNOWN = 0,
    QWEN3VL,
    FASTVLM,
    MINICPM
};

// ============================================================
// VLM 平台参数
// ============================================================
struct VLMPlatformParams {
    int threads = 4;
    int batch_size = 1;
    std::string extra_args;
    
    // K3 NPU 专用
    std::string media_backend;      // "smt"
    std::string smt_config_dir;
    
    // Jetson CUDA 专用
    int n_gpu_layers = 0;           // -1 = 全部 offload
    int cuda_device = 0;
    bool flash_attention = false;
    
    // KV Cache 配置
    std::string cache_type_k;       // "q8_0"
    std::string cache_type_v;       // "q8_0"
};

// ============================================================
// VLM 模型配置
// ============================================================
struct VLMModelConfig {
    std::string name;               // "qwen3vl-4b"
    VLMFamily family = VLMFamily::UNKNOWN;
    std::string size;               // "4B", "30B"
    std::string quantization;       // "Q4_K_M", "Q8_0", "F16"
    
    // 模型文件
    std::string model_file;         // "xxx.gguf"
    std::string mmproj_file;        // "mmproj-xxx.gguf" (可选)
    
    // 推理参数
    int context_length = 4096;
    int max_tokens = 512;
    float temperature = 0.3f;
    float top_p = 0.9f;
    int top_k = 40;
    int seed = 42;
    std::string default_prompt = "Describe this image.";
    
    // 平台参数
    std::map<Platform, VLMPlatformParams> platform_params;
    
    // 平台兼容性
    std::vector<Platform> supported_platforms;
    std::vector<Platform> excluded_platforms;
    
    // 预期性能
    struct ExpectedPerf {
        float tokens_per_sec = 0;
        float vision_encode_ms = 0;
        float ttft_ms = 0;
        float total_time_sec = 0;
    };
    std::map<Platform, ExpectedPerf> expected_performance;
    
    // 获取平台参数
    VLMPlatformParams getPlatformParams(Platform platform) const {
        auto it = platform_params.find(platform);
        if (it != platform_params.end()) {
            return it->second;
        }
        return VLMPlatformParams{};
    }
    
    // 检查平台是否支持
    bool isPlatformSupported(Platform platform) const {
        // 检查排除列表
        for (auto p : excluded_platforms) {
            if (p == platform) return false;
        }
        // 如果有支持列表，检查是否在列表中
        if (!supported_platforms.empty()) {
            for (auto p : supported_platforms) {
                if (p == platform) return true;
            }
            return false;
        }
        return true;
    }
};

// ============================================================
// 辅助函数
// ============================================================

// YOLO 系列转字符串
inline std::string yolo_family_to_string(YOLOFamily f) {
    switch (f) {
        case YOLOFamily::YOLOv5:  return "yolov5";
        case YOLOFamily::YOLOv8:  return "yolov8";
        case YOLOFamily::YOLOv11: return "yolov11";
        default:                  return "unknown";
    }
}

inline YOLOFamily string_to_yolo_family(const std::string& s) {
    if (s == "yolov5" || s == "v5") return YOLOFamily::YOLOv5;
    if (s == "yolov8" || s == "v8") return YOLOFamily::YOLOv8;
    if (s == "yolov11" || s == "v11") return YOLOFamily::YOLOv11;
    return YOLOFamily::UNKNOWN;
}

// YOLO 尺寸转字符串
inline std::string yolo_size_to_string(YOLOSize s) {
    switch (s) {
        case YOLOSize::NANO:   return "n";
        case YOLOSize::SMALL:  return "s";
        case YOLOSize::MEDIUM: return "m";
        case YOLOSize::LARGE:  return "l";
        case YOLOSize::XLARGE: return "x";
        default:               return "?";
    }
}

inline YOLOSize string_to_yolo_size(const std::string& s) {
    if (s == "n" || s == "nano") return YOLOSize::NANO;
    if (s == "s" || s == "small") return YOLOSize::SMALL;
    if (s == "m" || s == "medium") return YOLOSize::MEDIUM;
    if (s == "l" || s == "large") return YOLOSize::LARGE;
    if (s == "x" || s == "xlarge") return YOLOSize::XLARGE;
    return YOLOSize::UNKNOWN;
}

// VLM 系列转字符串
inline std::string vlm_family_to_string(VLMFamily f) {
    switch (f) {
        case VLMFamily::QWEN3VL: return "qwen3vl";
        case VLMFamily::FASTVLM: return "fastvlm";
        case VLMFamily::MINICPM: return "minicpm";
        default:                 return "unknown";
    }
}

inline VLMFamily string_to_vlm_family(const std::string& s) {
    if (s == "qwen3vl" || s == "qwen") return VLMFamily::QWEN3VL;
    if (s == "fastvlm" || s == "fast") return VLMFamily::FASTVLM;
    if (s == "minicpm" || s == "cpm") return VLMFamily::MINICPM;
    return VLMFamily::UNKNOWN;
}

}  // namespace benchmark
}  // namespace rivision
