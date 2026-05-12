#pragma once

#include "rivision/config.h"
#include <string>
#include <memory>
#include <functional>
#include <atomic>

namespace rivision::vlm {

class LlmEngine;
class VisionEncoder;

class VlmServer {
public:
    struct Request {
        std::string prompt;
        std::vector<uint8_t> image_data;  // JPEG or RGB
        int image_width = 0;
        int image_height = 0;
        int max_tokens = 256;
        float temperature = 0.7f;
    };
    
    struct Response {
        bool success = false;
        std::string text;
        float confidence = 0.0f;
        int tokens_generated = 0;
        int64_t latency_ms = 0;
        std::string error;
    };
    
    VlmServer();
    ~VlmServer();
    
    bool init(const VlmConfig& config);
    
    bool start(int port);
    void stop();
    
    Response process(const Request& request);
    
    bool isReady() const;
    
    std::string getModelInfo() const;
    
private:
    std::unique_ptr<LlmEngine> llm_engine_;
    std::unique_ptr<VisionEncoder> vision_encoder_;
    
    VlmConfig config_;
    std::atomic<bool> running_{false};
    std::atomic<bool> ready_{false};
    
    class HttpServer;
    std::unique_ptr<HttpServer> http_server_;
};

} // namespace rivision::vlm
