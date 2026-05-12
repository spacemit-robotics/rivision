#pragma once

#include <string>
#include <vector>
#include <memory>

namespace rivision::vlm {

class LlmEngine {
public:
    struct Config {
        std::string model_path;
        int context_length = 2048;
        int n_gpu_layers = -1;  // -1 = auto
        int threads = 4;
        int batch_size = 512;
    };
    
    struct GenerateRequest {
        std::string prompt;
        std::vector<float> vision_features;  // From vision encoder
        int max_tokens = 256;
        float temperature = 0.7f;
        float top_p = 0.9f;
        std::vector<std::string> stop_words;
    };
    
    struct GenerateResult {
        bool success = false;
        std::string text;
        int tokens_generated = 0;
        int prompt_tokens = 0;
        float tokens_per_second = 0.0f;
        std::string error;
    };
    
    LlmEngine();
    ~LlmEngine();
    
    bool init(const Config& config);
    
    GenerateResult generate(const GenerateRequest& request);
    
    bool isReady() const { return ready_; }
    
    int getContextLength() const { return context_length_; }
    
    void reset();
    
private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    bool ready_ = false;
    int context_length_ = 2048;
};

} // namespace rivision::vlm
