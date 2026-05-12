#include "config_handler.h"
#include <nlohmann/json.hpp>

namespace rivision::api {

using json = nlohmann::json;

ConfigHandler::ConfigHandler() = default;
ConfigHandler::~ConfigHandler() = default;

std::string ConfigHandler::handleGetConfig() {
    if (!get_callback_) {
        return R"({"error":"config handler not configured"})";
    }
    
    auto config = get_callback_();
    
    json result;
    result["node_id"] = config.node_id;
    
    result["server"] = {
        {"host", config.server.host},
        {"http_port", config.server.http_port}
    };
    
    result["hub"] = {
        {"url", config.hub.url},
        {"heartbeat_ms", config.hub.heartbeat_ms},
        {"reconnect_ms", config.hub.reconnect_ms}
    };
    
    result["inference"] = {
        {"yolo_config", config.inference.yolo_config},
        {"device", config.inference.device}
    };
    
    result["vlm"] = {
        {"enabled", config.vlm.enabled},
        {"endpoint", config.vlm.endpoint},
        {"timeout_ms", config.vlm.timeout_ms}
    };
    
    result["search"] = {
        {"max_vectors", config.search.max_vectors},
        {"embedding_dim", config.search.embedding_dim}
    };
    
    result["storage"] = {
        {"data_dir", config.storage.data_dir}
    };
    
    result["log"] = {
        {"level", config.log.level},
        {"console", config.log.console}
    };
    
    return result.dump();
}

std::string ConfigHandler::handleReload() {
    if (!reload_callback_) {
        return R"({"error":"reload handler not configured"})";
    }
    
    if (reload_callback_()) {
        return R"({"success":true,"message":"configuration reloaded"})";
    } else {
        return R"({"success":false,"error":"failed to reload configuration"})";
    }
}

} // namespace rivision::api
