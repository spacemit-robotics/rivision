#pragma once

#include "rivision/types.h"
#include <string>
#include <vector>
#include <memory>
#include <optional>

namespace rivision::core {

// =============================================================================
// VlmClient - HTTP client for VLM service
// =============================================================================

class VlmClient {
public:
    explicit VlmClient(const std::string& endpoint);
    ~VlmClient();
    
    // Set timeout
    void setTimeout(int timeout_ms) { timeout_ms_ = timeout_ms; }
    
    // Health check
    bool checkHealth();
    
    // Verify scene with prompt
    struct VerifyResult {
        bool verified = false;
        float confidence = 0.0f;
        std::string description;
        bool timeout = false;
    };
    VerifyResult verify(const Frame& frame, const std::string& prompt);
    VerifyResult verify(const std::string& image_base64, const std::string& prompt);
    
    // Analyze scene (open-ended)
    struct AnalyzeResult {
        std::string description;
        std::vector<std::string> objects;
        std::optional<std::string> scene_type;
        bool timeout = false;
    };
    AnalyzeResult analyze(const Frame& frame, const std::string& prompt = "");
    
    // Emotion analysis (G1 feature)
    struct EmotionResult {
        std::string emotion;  // happy, sad, angry, neutral, etc.
        float valence = 0.0f; // -1 to 1
        float confidence = 0.0f;
        bool timeout = false;
    };
    EmotionResult analyzeEmotion(const Frame& face_crop);
    
    // Check connection status
    bool isConnected() const { return connected_; }
    
private:
    // HTTP POST request
    std::string httpPost(const std::string& path, const std::string& body);
    
    // Encode frame to base64 JPEG
    std::string encodeFrame(const Frame& frame);
    
    std::string endpoint_;
    int timeout_ms_ = 5000;
    bool connected_ = false;
    
    class Impl;
    std::unique_ptr<Impl> impl_;
};

} // namespace rivision::core
