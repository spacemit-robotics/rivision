#pragma once

#include "rivision/types.h"
#include <memory>
#include <string>
#include <vector>

namespace rivision::hal {

// =============================================================================
// IInference - Inference backend interface
// =============================================================================

class IInference {
public:
    virtual ~IInference() = default;
    
    struct ModelConfig {
        std::string model_path;
        int input_width = 640;
        int input_height = 640;
        std::string input_format = "nchw";  // "nchw" | "nhwc"
        PixelFormat input_pixel_format = PixelFormat::RGB24;
        
        // Performance options
        int threads = 4;
        bool enable_profiling = false;
    };
    
    // Load model
    virtual bool loadModel(const ModelConfig& cfg) = 0;
    
    // Unload model
    virtual void unloadModel() = 0;
    
    // Check if loaded
    virtual bool isLoaded() const = 0;
    
    // Get model info
    virtual ModelConfig getModelConfig() const = 0;
    
    // Inference: Returns raw output tensors
    struct Tensor {
        std::vector<float> data;
        std::vector<int64_t> shape;
    };
    virtual bool infer(const Frame& frame, std::vector<Tensor>& outputs) = 0;
    
    // Batch inference
    virtual bool inferBatch(const std::vector<Frame>& frames, 
                           std::vector<std::vector<Tensor>>& outputs) {
        // Default: sequential inference
        outputs.resize(frames.size());
        for (size_t i = 0; i < frames.size(); ++i) {
            if (!infer(frames[i], outputs[i])) {
                return false;
            }
        }
        return true;
    }
    
    // Warmup
    virtual void warmup(int iterations = 5) = 0;
    
    // Get average latency
    virtual float getAvgLatencyMs() const = 0;
    
    // Get last error
    virtual std::string getLastError() const = 0;
    
protected:
    ModelConfig config_;
    float avg_latency_ms_ = 0.0f;
};

// =============================================================================
// IEmbedder - CLIP embedding interface
// =============================================================================

class IEmbedder {
public:
    virtual ~IEmbedder() = default;
    
    struct Config {
        std::string model_path;
        int embedding_dim = 512;
        int input_size = 224;
    };
    
    // Load model
    virtual bool loadModel(const Config& cfg) = 0;
    
    // Unload model
    virtual void unloadModel() = 0;
    
    // Check if loaded
    virtual bool isLoaded() const = 0;
    
    // Embed image (full frame or ROI)
    virtual bool embed(const Frame& frame, std::vector<float>& embedding) = 0;
    
    // Embed image ROI
    virtual bool embed(const Frame& frame, const BBox& roi, 
                      std::vector<float>& embedding) = 0;
    
    // Embed text
    virtual bool embedText(const std::string& text, 
                          std::vector<float>& embedding) = 0;
    
    // Get average latency
    virtual float getAvgLatencyMs() const = 0;
    
protected:
    Config config_;
};

// Factory functions
std::unique_ptr<IInference> createInference(const std::string& device);
std::unique_ptr<IEmbedder> createEmbedder(const std::string& device);

} // namespace rivision::hal
