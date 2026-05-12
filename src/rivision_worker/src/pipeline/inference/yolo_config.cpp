#include "yolo_config.h"
#include "utils/logger.h"
#include <yaml-cpp/yaml.h>
#include <filesystem>

namespace rivision::pipeline {

namespace fs = std::filesystem;

YoloConfig YoloConfigLoader::load(const std::string& path) {
    YoloConfig config;
    
    try {
        if (!fs::exists(path)) {
            LOG_WARN("YOLO config not found: {}, using defaults", path);
            return loadDefault();
        }
        
        YAML::Node yaml = YAML::LoadFile(path);
        std::string config_dir = fs::path(path).parent_path().string();
        
        if (auto model = yaml["model"]) {
            // Read detection model path (supports both model.path and model.detection.path)
            std::string model_path;
            if (auto detection = model["detection"]) {
                if (detection["path"]) {
                    model_path = detection["path"].as<std::string>();
                }
            } else if (model["path"]) {
                // Legacy: model.path directly
                model_path = model["path"].as<std::string>();
            }
            
            if (!model_path.empty()) {
                // Prepend base_dir if configured
                std::string base_dir = model["base_dir"] ? model["base_dir"].as<std::string>() : "";
                if (!base_dir.empty() && model_path[0] != '/') {
                    model_path = base_dir + "/" + model_path;
                }
                config.model.path = resolveModelPath(model_path, config_dir);
            }
            
            if (model["input_width"]) config.model.input_width = model["input_width"].as<int>();
            if (model["input_height"]) config.model.input_height = model["input_height"].as<int>();
        }
        
        if (auto detection = yaml["detection"]) {
            if (detection["confidence_threshold"]) {
                config.detection.confidence_threshold = detection["confidence_threshold"].as<float>();
            }
            if (detection["nms_threshold"]) {
                config.detection.nms_threshold = detection["nms_threshold"].as<float>();
            }
            if (detection["classes"]) {
                config.detection.classes = detection["classes"].as<std::vector<int>>();
            }
        }
        
        if (auto tracker = yaml["tracker"]) {
            if (tracker["enabled"]) config.tracker.enabled = tracker["enabled"].as<bool>();
            if (tracker["type"]) config.tracker.type = tracker["type"].as<std::string>();
            if (tracker["max_age"]) config.tracker.max_age = tracker["max_age"].as<int>();
            if (tracker["min_hits"]) config.tracker.min_hits = tracker["min_hits"].as<int>();
            if (tracker["iou_threshold"]) config.tracker.iou_threshold = tracker["iou_threshold"].as<float>();
        }
        
        if (auto emotion = yaml["emotion"]) {
            if (emotion["enabled"]) config.emotion.enabled = emotion["enabled"].as<bool>();
            if (emotion["sample_interval_sec"]) {
                config.emotion.sample_interval_sec = emotion["sample_interval_sec"].as<int>();
            }
        }
        
        if (yaml["class_names"]) {
            config.class_names = yaml["class_names"].as<std::vector<std::string>>();
        } else {
            config.initDefaultClasses();
        }
        
        LOG_INFO("Loaded YOLO config from {}", path);
        
    } catch (const YAML::Exception& e) {
        LOG_ERROR("Failed to parse YOLO config: {}", e.what());
        return loadDefault();
    }
    
    return config;
}

YoloConfig YoloConfigLoader::loadDefault() {
    YoloConfig config;
    config.model.path = "models/yolo/yolo11n.q.onnx";
    config.model.input_width = 640;
    config.model.input_height = 640;
    config.detection.confidence_threshold = 0.5f;
    config.detection.nms_threshold = 0.45f;
    config.tracker.enabled = true;
    config.tracker.type = "bytetrack";
    config.tracker.max_age = 30;
    config.tracker.min_hits = 3;
    config.initDefaultClasses();
    return config;
}

bool YoloConfigLoader::validate(const YoloConfig& config, std::string& error) {
    if (config.model.input_width <= 0 || config.model.input_height <= 0) {
        error = "Invalid model input dimensions";
        return false;
    }
    
    if (config.detection.confidence_threshold < 0.0f || 
        config.detection.confidence_threshold > 1.0f) {
        error = "Confidence threshold must be between 0 and 1";
        return false;
    }
    
    if (config.detection.nms_threshold < 0.0f || 
        config.detection.nms_threshold > 1.0f) {
        error = "NMS threshold must be between 0 and 1";
        return false;
    }
    
    return true;
}

std::string YoloConfigLoader::resolveModelPath(
    const std::string& model_path, 
    const std::string& config_dir
) {
    if (model_path.empty()) {
        return "";
    }
    
    if (model_path[0] == '/') {
        return model_path;
    }
    
    fs::path full_path = fs::path(config_dir) / model_path;
    if (fs::exists(full_path)) {
        return full_path.string();
    }
    
    return model_path;
}

} // namespace rivision::pipeline
