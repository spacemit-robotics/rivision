#pragma once

#include "rivision/config.h"
#include <string>
#include <functional>
#include <shared_mutex>

namespace rivision::core {

class ConfigManager {
public:
    ConfigManager();
    ~ConfigManager();
    
    bool load(const std::string& worker_config_path);
    bool reload();
    
    const WorkerConfig& getWorkerConfig() const { return worker_config_; }
    const YoloConfig& getYoloConfig() const { return yolo_config_; }
    const NotifyConfig& getNotifyConfig() const { return notify_config_; }
    const VlmConfig& getVlmConfig() const { return vlm_config_; }
    std::vector<RuleConfig> getRules() const;
    
    void setRulesPath(const std::string& path) { rules_path_ = path; }
    void setNotifyPath(const std::string& path) { notify_path_ = path; }
    void setVlmPath(const std::string& path) { vlm_path_ = path; }
    
    using ConfigChangeCallback = std::function<void()>;
    void onConfigChange(ConfigChangeCallback cb) { on_change_ = std::move(cb); }
    
    std::string getLastError() const { return last_error_; }
    
private:
    bool loadWorkerConfig(const std::string& path);
    bool loadYoloConfig(const std::string& path);
    bool loadRules(const std::string& path);
    bool loadNotifyConfig(const std::string& path);
    bool loadVlmConfig(const std::string& path);
    bool loadEmbedConfig(const std::string& path);
    void extractModelNames();
    
    WorkerConfig worker_config_;
    YoloConfig yolo_config_;
    std::vector<RuleConfig> rules_;
    NotifyConfig notify_config_;
    VlmConfig vlm_config_;
    std::string embed_model_name_;  // Extracted from embed.yaml
    
    std::string worker_config_path_;
    std::string rules_path_;
    std::string notify_path_;
    std::string vlm_path_;
    
    mutable std::shared_mutex mutex_;
    ConfigChangeCallback on_change_;
    std::string last_error_;
};

} // namespace rivision::core
