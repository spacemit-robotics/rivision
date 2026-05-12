#pragma once

#include "rivision/types.h"
#include <string>
#include <functional>
#include <memory>
#include <atomic>
#include <thread>

namespace rivision::api {

class WsHubClient {
public:
    using MessageHandler = std::function<void(const std::string& message)>;
    using ConnectHandler = std::function<void()>;
    using DisconnectHandler = std::function<void(const std::string& reason)>;
    
    WsHubClient();
    ~WsHubClient();
    
    void setUrl(const std::string& url) { url_ = url; }
    void setNodeId(const std::string& node_id) { node_id_ = node_id; }
    void setToken(const std::string& token) { token_ = token; }
    void setReconnectMs(int ms) { reconnect_ms_ = ms; }
    
    void setMessageHandler(MessageHandler handler) { message_handler_ = std::move(handler); }
    void setConnectHandler(ConnectHandler handler) { connect_handler_ = std::move(handler); }
    void setDisconnectHandler(DisconnectHandler handler) { disconnect_handler_ = std::move(handler); }
    
    bool connect();
    void disconnect();
    
    bool send(const std::string& message);
    
    bool sendDetection(const StreamId& stream_id,
                       const std::vector<Detection>& detections,
                       const std::vector<Track>& tracks);
    
    bool sendAlert(const AlertEvent& alert);
    
    bool sendHeartbeat(const std::string& heartbeat_json);
    
    bool isConnected() const { return connected_; }
    
private:
    void reconnectLoop();
    bool doConnect();
    
    std::string url_;
    std::string node_id_;
    std::string token_;
    int reconnect_ms_ = 3000;
    
    MessageHandler message_handler_;
    ConnectHandler connect_handler_;
    DisconnectHandler disconnect_handler_;
    
    std::atomic<bool> connected_{false};
    std::atomic<bool> should_reconnect_{false};
    std::thread reconnect_thread_;
    
    class Impl;
    std::unique_ptr<Impl> impl_;
};

} // namespace rivision::api
