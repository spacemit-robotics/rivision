#pragma once

#include "rivision/types.h"
#include <string>
#include <memory>
#include <atomic>
#include <functional>
#include <thread>
#include <nlohmann/json.hpp>

namespace rivision::hub {

// =============================================================================
// HubClient - WebSocket client for Hub communication
// =============================================================================

class HubClient {
public:
    HubClient(const std::string& url, 
              const std::string& node_id,
              const std::string& token = "");
    ~HubClient();
    
    // Connection
    bool connect();
    void disconnect();
    bool isConnected() const { return connected_.load(); }
    
    // Configuration
    void setReconnectMs(int ms) { reconnect_ms_ = ms; }
    void setHeartbeatMs(int ms) { heartbeat_ms_ = ms; }
    void setHost(const std::string& host) { host_ = host; }
    void setHttpPort(int port) { http_port_ = port; }
    void setYoloEnabled(bool enabled) { yolo_enabled_ = enabled; }
    void setVlmEnabled(bool enabled) { vlm_enabled_ = enabled; }
    void setVersion(const std::string& ver) { version_ = ver; }
    void setVlmModel(const std::string& model) { 
        vlm_model_ = model;
        if (!capabilities_.empty()) {
            capabilities_["vlm_model"] = model;
        }
    }
    void setEmbedModel(const std::string& model) { 
        embed_model_ = model;
        if (!capabilities_.empty()) {
            capabilities_["embed_model"] = model;
        }
    }
    void setCapabilities(const std::string& yolo_model, int max_streams = 4) {
        // Extract model filename
        std::string model_name = yolo_model;
        auto pos = yolo_model.rfind('/');
        if (pos != std::string::npos) {
            model_name = yolo_model.substr(pos + 1);
        }
        yolo_model_ = model_name;
        
        // Build models list
        std::vector<std::string> models;
        if (!model_name.empty()) {
            models.push_back(model_name);
        }
        
        // Build capabilities matching Go Worker format
        capabilities_["max_streams"] = max_streams;
        capabilities_["models"] = models;
        capabilities_["yolo_model"] = model_name;
        capabilities_["vlm_model"] = vlm_model_;
        capabilities_["embed_model"] = embed_model_;
        capabilities_["platform"] = "riscv64-spacemit";
    }
    
    // Send message
    bool send(const std::string& message);
    
    // Get last heartbeat time
    std::string lastHeartbeat() const;
    
    // Message handlers
    using MessageHandler = std::function<void(const std::string&)>;
    void setMessageHandler(MessageHandler handler);
    
    // Callbacks
    using ConnectCallback = std::function<void()>;
    using DisconnectCallback = std::function<void()>;
    using PreHeartbeatCallback = std::function<void()>;
    void setConnectCallback(ConnectCallback cb) { on_connect_ = std::move(cb); }
    void setDisconnectCallback(DisconnectCallback cb) { on_disconnect_ = std::move(cb); }
    void setPreHeartbeatCallback(PreHeartbeatCallback cb) { on_pre_heartbeat_ = std::move(cb); }
    
private:
    void runLoop();
    void sendHeartbeat();
    bool registerWithHub();
    bool sendHeartbeatHttp();
    
    std::string url_;
    std::string node_id_;
    std::string token_;
    std::string host_;           // Our IP to report
    int http_port_ = 9181;       // Worker HTTP port
    bool yolo_enabled_ = true;   // YOLO service status
    bool vlm_enabled_ = false;   // VLM service status
    std::string version_ = "1.0.0";  // Worker version
    std::string yolo_model_;         // YOLO model name
    std::string vlm_model_;          // VLM model name
    std::string embed_model_;        // Embedding model name
    nlohmann::json capabilities_;    // Model capabilities
    
    int reconnect_ms_ = 3000;
    int heartbeat_ms_ = 10000;
    
    std::atomic<bool> connected_{false};
    std::atomic<bool> running_{false};
    
    std::thread loop_thread_;
    std::thread heartbeat_thread_;
    
    MessageHandler message_handler_;
    ConnectCallback on_connect_;
    DisconnectCallback on_disconnect_;
    PreHeartbeatCallback on_pre_heartbeat_;
    
    Timestamp last_heartbeat_ = 0;
    Timestamp start_time_ = 0;  // Worker start time for uptime calculation
    
    // WebSocket handle (implementation-specific)
    void* ws_handle_ = nullptr;
};

} // namespace rivision::hub
