#pragma once

#include <string>
#include <memory>
#include <thread>
#include <atomic>

namespace rivision::core {
class Worker;
}

namespace rivision::api {

// =============================================================================
// HttpServer - oatpp-based HTTP server
// =============================================================================

class HttpServer {
public:
    struct Config {
        std::string host = "0.0.0.0";
        int port = 8080;
        int max_connections = 100;
        int request_timeout_ms = 30000;
    };
    
    HttpServer(const Config& config, core::Worker* worker);
    ~HttpServer();
    
    // Start server (non-blocking)
    bool start();
    
    // Stop server
    void stop();
    
    // Check if running
    bool isRunning() const { return running_.load(); }
    
private:
    void runLoop();
    
    Config config_;
    core::Worker* worker_;
    
    std::atomic<bool> running_{false};
    std::thread server_thread_;
    
    // oatpp components (opaque)
    class Impl;
    std::unique_ptr<Impl> impl_;
};

} // namespace rivision::api
