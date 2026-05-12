#pragma once

#include "rivision/types.h"
#include <string>
#include <functional>
#include <memory>
#include <mutex>
#include <unordered_set>

namespace rivision::api {

class WebSocketServer {
public:
    using MessageHandler = std::function<void(const std::string& client_id, const std::string& message)>;
    using ConnectionHandler = std::function<void(const std::string& client_id)>;
    
    WebSocketServer();
    ~WebSocketServer();
    
    void setMessageHandler(MessageHandler handler) { message_handler_ = std::move(handler); }
    void setConnectHandler(ConnectionHandler handler) { connect_handler_ = std::move(handler); }
    void setDisconnectHandler(ConnectionHandler handler) { disconnect_handler_ = std::move(handler); }
    
    bool start(int port);
    void stop();
    
    void broadcast(const std::string& message);
    
    void sendTo(const std::string& client_id, const std::string& message);
    
    void broadcastDetection(const StreamId& stream_id, 
                            const std::vector<Detection>& detections,
                            const std::vector<Track>& tracks);
    
    void broadcastAlert(const AlertEvent& alert);
    
    int getClientCount() const;
    
    bool isRunning() const { return running_; }
    
private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    MessageHandler message_handler_;
    ConnectionHandler connect_handler_;
    ConnectionHandler disconnect_handler_;
    
    std::unordered_set<std::string> clients_;
    mutable std::mutex clients_mutex_;
    bool running_ = false;
};

} // namespace rivision::api
