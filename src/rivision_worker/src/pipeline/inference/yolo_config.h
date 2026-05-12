#pragma once

#include "rivision/config.h"
#include <string>

namespace rivision::pipeline {

class YoloConfigLoader {
public:
    static YoloConfig load(const std::string& path);
    static YoloConfig loadDefault();
    
    static bool validate(const YoloConfig& config, std::string& error);
    
    static std::string resolveModelPath(const std::string& model_path, const std::string& config_dir);
};

} // namespace rivision::pipeline
