#include "config_manager.h"
#include "pipeline/inference/yolo_config.h"
#include "utils/logger.h"
#include <yaml-cpp/yaml.h>
#include <filesystem>
#include <fstream>

namespace rivision::core {

namespace fs = std::filesystem;

ConfigManager::ConfigManager() = default;
ConfigManager::~ConfigManager() = default;

bool ConfigManager::load(const std::string& worker_config_path) {
    std::unique_lock lock(mutex_);
    
    worker_config_path_ = worker_config_path;
    
    if (!loadWorkerConfig(worker_config_path)) {
        return false;
    }
    
    worker_config_.initPaths();
    
    if (!worker_config_.inference.yolo_config.empty()) {
        loadYoloConfig(worker_config_.inference.yolo_config);
    }
    
    // Load VLM config and extract model name
    if (!worker_config_.inference.vlm_config.empty()) {
        loadVlmConfig(worker_config_.inference.vlm_config);
    } else if (!vlm_path_.empty()) {
        loadVlmConfig(vlm_path_);
    }
    
    // Load Embed config and extract model name
    if (!worker_config_.inference.embed_config.empty()) {
        loadEmbedConfig(worker_config_.inference.embed_config);
    }
    
    if (!rules_path_.empty()) {
        loadRules(rules_path_);
    }
    
    if (!notify_path_.empty()) {
        loadNotifyConfig(notify_path_);
    }
    
    // Extract model names for HubClient
    extractModelNames();
    
    return true;
}

bool ConfigManager::reload() {
    if (worker_config_path_.empty()) {
        last_error_ = "No config path set";
        return false;
    }
    
    if (!load(worker_config_path_)) {
        return false;
    }
    
    if (on_change_) {
        on_change_();
    }
    
    return true;
}

std::vector<RuleConfig> ConfigManager::getRules() const {
    std::shared_lock lock(mutex_);
    return rules_;
}

bool ConfigManager::loadWorkerConfig(const std::string& path) {
    try {
        if (!fs::exists(path)) {
            last_error_ = "Config file not found: " + path;
            return false;
        }
        
        YAML::Node config = YAML::LoadFile(path);
        
        // Support both C++ Worker format (node_id) and Go Worker format (node.id)
        if (config["node_id"]) {
            worker_config_.node_id = config["node_id"].as<std::string>();
        } else if (config["node"] && config["node"]["id"]) {
            worker_config_.node_id = config["node"]["id"].as<std::string>();
        }
        
        // Version
        if (config["version"]) {
            worker_config_.version = config["version"].as<std::string>();
        }
        
        if (auto server = config["server"]) {
            if (server["host"]) worker_config_.server.host = server["host"].as<std::string>();
            // Support both http_port and port
            if (server["http_port"]) worker_config_.server.http_port = server["http_port"].as<int>();
            else if (server["port"]) worker_config_.server.http_port = server["port"].as<int>();
            if (server["advertise_host"]) worker_config_.server.advertise_host = server["advertise_host"].as<std::string>();
            if (server["max_connections"]) worker_config_.server.max_connections = server["max_connections"].as<int>();
        }
        
        if (auto hub = config["hub"]) {
            if (hub["url"]) worker_config_.hub.url = hub["url"].as<std::string>();
            if (hub["token"]) worker_config_.hub.token = hub["token"].as<std::string>();
            if (hub["reconnect_ms"]) worker_config_.hub.reconnect_ms = hub["reconnect_ms"].as<int>();
            if (hub["heartbeat_ms"]) worker_config_.hub.heartbeat_ms = hub["heartbeat_ms"].as<int>();
            // Support Go Worker format: heartbeat_interval (e.g., "30s")
            if (hub["heartbeat_interval"]) {
                std::string interval = hub["heartbeat_interval"].as<std::string>();
                // Parse "30s" format
                if (interval.back() == 's') {
                    worker_config_.hub.heartbeat_ms = std::stoi(interval.substr(0, interval.size()-1)) * 1000;
                }
            }
        }
        
        if (auto inference = config["inference"]) {
            if (inference["yolo_config"]) worker_config_.inference.yolo_config = inference["yolo_config"].as<std::string>();
            if (inference["vlm_config"]) worker_config_.inference.vlm_config = inference["vlm_config"].as<std::string>();
            if (inference["embed_config"]) worker_config_.inference.embed_config = inference["embed_config"].as<std::string>();
            if (inference["device"]) worker_config_.inference.device = inference["device"].as<std::string>();
        }
        
        if (auto vlm = config["vlm"]) {
            if (vlm["enabled"]) worker_config_.vlm.enabled = vlm["enabled"].as<bool>();
            if (vlm["endpoint"]) worker_config_.vlm.endpoint = vlm["endpoint"].as<std::string>();
            if (vlm["timeout_ms"]) worker_config_.vlm.timeout_ms = vlm["timeout_ms"].as<int>();
        }
        
        if (auto embed = config["embed"]) {
            if (embed["url"]) worker_config_.embed.url = embed["url"].as<std::string>();
        }
        
        if (auto search = config["search"]) {
            if (search["max_vectors"]) worker_config_.search.max_vectors = search["max_vectors"].as<int>();
            if (search["embedding_dim"]) worker_config_.search.embedding_dim = search["embedding_dim"].as<int>();
        }
        
        if (auto storage = config["storage"]) {
            if (storage["data_dir"]) worker_config_.storage.data_dir = storage["data_dir"].as<std::string>();
        }
        
        if (auto cleanup = config["cleanup"]) {
            if (cleanup["detection_retain_days"]) worker_config_.cleanup.detection_retain_days = cleanup["detection_retain_days"].as<int>();
            if (cleanup["recording_retain_days"]) worker_config_.cleanup.recording_retain_days = cleanup["recording_retain_days"].as<int>();
            if (cleanup["disk_high_watermark_pct"]) worker_config_.cleanup.disk_high_watermark_pct = cleanup["disk_high_watermark_pct"].as<int>();
        }
        
        if (auto log = config["log"]) {
            if (log["level"]) worker_config_.log.level = log["level"].as<std::string>();
            if (log["file"]) worker_config_.log.file = log["file"].as<std::string>();
            if (log["console"]) worker_config_.log.console = log["console"].as<bool>();
        }
        
        LOG_INFO("Loaded worker config from {}", path);
        return true;
        
    } catch (const YAML::Exception& e) {
        last_error_ = "YAML parse error: " + std::string(e.what());
        LOG_ERROR("Failed to load config: {}", last_error_);
        return false;
    }
}

bool ConfigManager::loadYoloConfig(const std::string& path) {
    // Delegate to YoloConfigLoader to avoid code duplication
    try {
        yolo_config_ = pipeline::YoloConfigLoader::load(path);
        LOG_INFO("Loaded YOLO config from {}, model={}", path, yolo_config_.model.path);
        return true;
    } catch (const std::exception& e) {
        LOG_WARN("Failed to load YOLO config: {}, using defaults", e.what());
        yolo_config_ = pipeline::YoloConfigLoader::loadDefault();
        return false;
    }
}

bool ConfigManager::loadRules(const std::string& path) {
    try {
        if (!fs::exists(path)) {
            LOG_WARN("Rules config not found: {}", path);
            return true;
        }
        
        YAML::Node config = YAML::LoadFile(path);
        rules_.clear();
        
        if (config["rules"]) {
            for (const auto& rule_node : config["rules"]) {
                RuleConfig rule;
                rule.id = rule_node["id"].as<std::string>();
                rule.name = rule_node["name"].as<std::string>();
                rule.enabled = rule_node["enabled"].as<bool>(true);
                
                if (rule_node["streams"]) {
                    rule.streams = rule_node["streams"].as<std::vector<std::string>>();
                }
                
                if (auto trigger = rule_node["trigger"]) {
                    rule.trigger.type = trigger["type"].as<std::string>();
                    if (trigger["classes"]) {
                        rule.trigger.classes = trigger["classes"].as<std::vector<int>>();
                    }
                    if (trigger["dwell_sec"]) {
                        rule.trigger.dwell_sec = trigger["dwell_sec"].as<int>();
                    }
                }
                
                if (rule_node["actions"]) {
                    for (const auto& action_node : rule_node["actions"]) {
                        RuleConfig::Action action;
                        action.type = action_node["type"].as<std::string>();
                        action.level = action_node["level"].as<std::string>("warning");
                        if (action_node["prompt"]) {
                            action.prompt = action_node["prompt"].as<std::string>();
                        }
                        rule.actions.push_back(action);
                    }
                }
                
                rules_.push_back(rule);
            }
        }
        
        LOG_INFO("Loaded {} rules from {}", rules_.size(), path);
        return true;
        
    } catch (const YAML::Exception& e) {
        LOG_WARN("Failed to load rules: {}", e.what());
        return false;
    }
}

bool ConfigManager::loadNotifyConfig(const std::string& path) {
    try {
        if (!fs::exists(path)) {
            LOG_WARN("Notify config not found: {}", path);
            return true;
        }
        
        YAML::Node config = YAML::LoadFile(path);
        
        if (config["enabled"]) {
            notify_config_.enabled = config["enabled"].as<bool>();
        }
        
        if (auto wechat = config["wechat"]) {
            notify_config_.wechat.enabled = wechat["enabled"].as<bool>(false);
            if (wechat["webhook_url"]) {
                notify_config_.wechat.webhook_url = wechat["webhook_url"].as<std::string>();
            }
        }
        
        if (auto dingtalk = config["dingtalk"]) {
            notify_config_.dingtalk.enabled = dingtalk["enabled"].as<bool>(false);
            if (dingtalk["webhook_url"]) {
                notify_config_.dingtalk.webhook_url = dingtalk["webhook_url"].as<std::string>();
            }
        }
        
        if (auto webhook = config["webhook"]) {
            notify_config_.webhook.enabled = webhook["enabled"].as<bool>(false);
            if (webhook["url"]) {
                notify_config_.webhook.url = webhook["url"].as<std::string>();
            }
        }
        
        LOG_INFO("Loaded notify config from {}", path);
        return true;
        
    } catch (const YAML::Exception& e) {
        LOG_WARN("Failed to load notify config: {}", e.what());
        return false;
    }
}

bool ConfigManager::loadVlmConfig(const std::string& path) {
    try {
        if (!fs::exists(path)) {
            LOG_WARN("VLM config not found: {}", path);
            return true;
        }
        
        YAML::Node config = YAML::LoadFile(path);
        
        if (auto server = config["server"]) {
            if (server["host"]) vlm_config_.server.host = server["host"].as<std::string>();
            if (server["port"]) vlm_config_.server.port = server["port"].as<int>();
        }
        
        if (auto model = config["model"]) {
            if (model["path"]) vlm_config_.model.path = model["path"].as<std::string>();
            if (model["type"]) vlm_config_.model.type = model["type"].as<std::string>();
        }
        
        if (auto inference = config["inference"]) {
            if (inference["max_tokens"]) vlm_config_.inference.max_tokens = inference["max_tokens"].as<int>();
            if (inference["temperature"]) vlm_config_.inference.temperature = inference["temperature"].as<float>();
        }
        
        LOG_INFO("Loaded VLM config from {}", path);
        return true;
        
    } catch (const YAML::Exception& e) {
        LOG_WARN("Failed to load VLM config: {}", e.what());
        return false;
    }
}

bool ConfigManager::loadEmbedConfig(const std::string& path) {
    try {
        if (!fs::exists(path)) {
            LOG_WARN("Embed config not found: {}", path);
            return true;
        }
        
        YAML::Node config = YAML::LoadFile(path);
        
        // Extract embed model name
        if (auto model = config["model"]) {
            if (model["name"]) {
                embed_model_name_ = model["name"].as<std::string>();
            }
        }
        
        LOG_INFO("Loaded Embed config from {}, model={}", path, embed_model_name_);
        return true;
        
    } catch (const YAML::Exception& e) {
        LOG_WARN("Failed to load Embed config: {}", e.what());
        return false;
    }
}

void ConfigManager::extractModelNames() {
    // Extract YOLO model name from path
    if (!yolo_config_.model.path.empty()) {
        std::string model_path = yolo_config_.model.path;
        auto pos = model_path.rfind('/');
        if (pos != std::string::npos) {
            worker_config_.models.yolo = model_path.substr(pos + 1);
        } else {
            worker_config_.models.yolo = model_path;
        }
        // Remove extension
        auto dot_pos = worker_config_.models.yolo.rfind('.');
        if (dot_pos != std::string::npos) {
            worker_config_.models.yolo = worker_config_.models.yolo.substr(0, dot_pos);
        }
    }
    
    // VLM model name from vlm_config
    if (!vlm_config_.model.path.empty() && vlm_config_.model.path != "auto") {
        std::string model_path = vlm_config_.model.path;
        auto pos = model_path.rfind('/');
        if (pos != std::string::npos) {
            worker_config_.models.vlm = model_path.substr(pos + 1);
        } else {
            worker_config_.models.vlm = model_path;
        }
    } else if (!vlm_config_.model.type.empty()) {
        worker_config_.models.vlm = vlm_config_.model.type;
    }
    
    // Embed model name
    if (!embed_model_name_.empty()) {
        worker_config_.models.embed = embed_model_name_;
    }
    
    LOG_INFO("Model names: yolo={}, vlm={}, embed={}", 
             worker_config_.models.yolo, 
             worker_config_.models.vlm, 
             worker_config_.models.embed);
}

} // namespace rivision::core

// =============================================================================
// Static load method implementations for config structs
// =============================================================================

namespace rivision {

WorkerConfig WorkerConfig::load(const std::string& path) {
    WorkerConfig config;
    namespace fs = std::filesystem;
    
    try {
        if (!fs::exists(path)) {
            LOG_WARN("Config file not found: {}, using defaults", path);
            return config;
        }
        
        YAML::Node yaml = YAML::LoadFile(path);
        
        // Support both C++ Worker format (node_id) and Go Worker format (node.id)
        if (yaml["node_id"]) {
            config.node_id = yaml["node_id"].as<std::string>();
        } else if (yaml["node"] && yaml["node"]["id"]) {
            config.node_id = yaml["node"]["id"].as<std::string>();
        }
        
        if (auto server = yaml["server"]) {
            if (server["host"]) config.server.host = server["host"].as<std::string>();
            if (server["http_port"]) config.server.http_port = server["http_port"].as<int>();
            else if (server["port"]) config.server.http_port = server["port"].as<int>();
            if (server["advertise_host"]) config.server.advertise_host = server["advertise_host"].as<std::string>();
            if (server["max_connections"]) config.server.max_connections = server["max_connections"].as<int>();
        }
        
        if (auto hub = yaml["hub"]) {
            if (hub["url"]) config.hub.url = hub["url"].as<std::string>();
            if (hub["token"]) config.hub.token = hub["token"].as<std::string>();
            if (hub["reconnect_ms"]) config.hub.reconnect_ms = hub["reconnect_ms"].as<int>();
            if (hub["heartbeat_ms"]) config.hub.heartbeat_ms = hub["heartbeat_ms"].as<int>();
            // Support Go Worker format: heartbeat_interval (e.g., "30s")
            if (hub["heartbeat_interval"]) {
                std::string interval = hub["heartbeat_interval"].as<std::string>();
                if (!interval.empty() && interval.back() == 's') {
                    config.hub.heartbeat_ms = std::stoi(interval.substr(0, interval.size()-1)) * 1000;
                }
            }
        }
        
        if (auto inference = yaml["inference"]) {
            if (inference["yolo_config"]) config.inference.yolo_config = inference["yolo_config"].as<std::string>();
            if (inference["device"]) config.inference.device = inference["device"].as<std::string>();
        }
        
        if (auto vlm = yaml["vlm"]) {
            if (vlm["enabled"]) config.vlm.enabled = vlm["enabled"].as<bool>();
            if (vlm["endpoint"]) config.vlm.endpoint = vlm["endpoint"].as<std::string>();
            if (vlm["timeout_ms"]) config.vlm.timeout_ms = vlm["timeout_ms"].as<int>();
        }
        
        if (auto embed = yaml["embed"]) {
            if (embed["url"]) config.embed.url = embed["url"].as<std::string>();
        }
        
        if (auto search = yaml["search"]) {
            if (search["max_vectors"]) config.search.max_vectors = search["max_vectors"].as<int>();
            if (search["embedding_dim"]) config.search.embedding_dim = search["embedding_dim"].as<int>();
        }
        
        if (auto storage = yaml["storage"]) {
            if (storage["data_dir"]) config.storage.data_dir = storage["data_dir"].as<std::string>();
        }
        
        if (auto cleanup = yaml["cleanup"]) {
            if (cleanup["detection_retain_days"]) config.cleanup.detection_retain_days = cleanup["detection_retain_days"].as<int>();
            if (cleanup["recording_retain_days"]) config.cleanup.recording_retain_days = cleanup["recording_retain_days"].as<int>();
            if (cleanup["disk_high_watermark_pct"]) config.cleanup.disk_high_watermark_pct = cleanup["disk_high_watermark_pct"].as<int>();
        }
        
        if (auto log = yaml["log"]) {
            if (log["level"]) config.log.level = log["level"].as<std::string>();
            if (log["file"]) config.log.file = log["file"].as<std::string>();
            if (log["console"]) config.log.console = log["console"].as<bool>();
        }
        
        config.initPaths();
        LOG_INFO("Loaded worker config from {}", path);
        
    } catch (const YAML::Exception& e) {
        LOG_ERROR("Failed to load config {}: {}", path, e.what());
    }
    
    return config;
}

YoloConfig YoloConfig::load(const std::string& path) {
    // Delegate to YoloConfigLoader to avoid code duplication
    return pipeline::YoloConfigLoader::load(path);
}

std::vector<RuleConfig> RuleConfig::loadAll(const std::string& path) {
    std::vector<RuleConfig> rules;
    namespace fs = std::filesystem;
    
    try {
        if (!fs::exists(path)) {
            LOG_WARN("Rules config not found: {}", path);
            return rules;
        }
        
        YAML::Node yaml = YAML::LoadFile(path);
        
        if (yaml["rules"]) {
            for (const auto& rule_node : yaml["rules"]) {
                RuleConfig rule;
                rule.id = rule_node["id"].as<std::string>("");
                rule.name = rule_node["name"].as<std::string>("");
                rule.enabled = rule_node["enabled"].as<bool>(true);
                
                if (rule_node["streams"]) {
                    rule.streams = rule_node["streams"].as<std::vector<std::string>>();
                }
                
                if (auto trigger = rule_node["trigger"]) {
                    rule.trigger.type = trigger["type"].as<std::string>("");
                    if (trigger["classes"]) rule.trigger.classes = trigger["classes"].as<std::vector<int>>();
                    if (trigger["dwell_sec"]) rule.trigger.dwell_sec = trigger["dwell_sec"].as<int>();
                }
                
                if (rule_node["actions"]) {
                    for (const auto& action_node : rule_node["actions"]) {
                        RuleConfig::Action action;
                        action.type = action_node["type"].as<std::string>("");
                        action.level = action_node["level"].as<std::string>("warning");
                        if (action_node["prompt"]) action.prompt = action_node["prompt"].as<std::string>();
                        if (action_node["vlm_timeout_ms"]) action.vlm_timeout_ms = action_node["vlm_timeout_ms"].as<int>();
                        if (action_node["default_confidence"]) action.default_confidence = action_node["default_confidence"].as<float>();
                        rule.actions.push_back(action);
                    }
                }
                
                rules.push_back(rule);
            }
        }
        
        LOG_INFO("Loaded {} rules from {}", rules.size(), path);
        
    } catch (const YAML::Exception& e) {
        LOG_ERROR("Failed to load rules {}: {}", path, e.what());
    }
    
    return rules;
}

NotifyConfig NotifyConfig::load(const std::string& path) {
    NotifyConfig config;
    namespace fs = std::filesystem;
    
    try {
        if (!fs::exists(path)) {
            LOG_WARN("Notify config not found: {}", path);
            return config;
        }
        
        YAML::Node yaml = YAML::LoadFile(path);
        
        if (yaml["enabled"]) config.enabled = yaml["enabled"].as<bool>();
        
        if (auto wechat = yaml["wechat"]) {
            config.wechat.enabled = wechat["enabled"].as<bool>(false);
            if (wechat["webhook_url"]) config.wechat.webhook_url = wechat["webhook_url"].as<std::string>();
        }
        
        if (auto dingtalk = yaml["dingtalk"]) {
            config.dingtalk.enabled = dingtalk["enabled"].as<bool>(false);
            if (dingtalk["webhook_url"]) config.dingtalk.webhook_url = dingtalk["webhook_url"].as<std::string>();
        }
        
        if (auto webhook = yaml["webhook"]) {
            config.webhook.enabled = webhook["enabled"].as<bool>(false);
            if (webhook["url"]) config.webhook.url = webhook["url"].as<std::string>();
        }
        
        LOG_INFO("Loaded notify config from {}", path);
        
    } catch (const YAML::Exception& e) {
        LOG_ERROR("Failed to load notify config {}: {}", path, e.what());
    }
    
    return config;
}

VlmConfig VlmConfig::load(const std::string& path) {
    VlmConfig config;
    namespace fs = std::filesystem;
    
    try {
        if (!fs::exists(path)) {
            LOG_WARN("VLM config not found: {}", path);
            return config;
        }
        
        YAML::Node yaml = YAML::LoadFile(path);
        
        if (auto server = yaml["server"]) {
            if (server["host"]) config.server.host = server["host"].as<std::string>();
            if (server["port"]) config.server.port = server["port"].as<int>();
        }
        
        if (auto model = yaml["model"]) {
            if (model["path"]) config.model.path = model["path"].as<std::string>();
            if (model["type"]) config.model.type = model["type"].as<std::string>();
        }
        
        if (auto inference = yaml["inference"]) {
            if (inference["max_tokens"]) config.inference.max_tokens = inference["max_tokens"].as<int>();
            if (inference["temperature"]) config.inference.temperature = inference["temperature"].as<float>();
        }
        
        LOG_INFO("Loaded VLM config from {}", path);
        
    } catch (const YAML::Exception& e) {
        LOG_ERROR("Failed to load VLM config {}: {}", path, e.what());
    }
    
    return config;
}

} // namespace rivision
