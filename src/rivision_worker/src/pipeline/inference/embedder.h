#pragma once

#include "rivision/types.h"
#include <memory>
#include <string>
#include <vector>

namespace rivision::pipeline {

// =============================================================================
// Embedder - CLIP embedding generator
// =============================================================================

class Embedder {
public:
    struct Config {
        std::string model_path;
        int embedding_dim = 512;
        int input_size = 224;
        std::string device = "cpu";  // cpu | npu
        int num_threads = 4;
    };
    
    static std::unique_ptr<Embedder> create(const Config& cfg);
    
    virtual ~Embedder() = default;
    
    // Initialize (load model)
    virtual bool init() = 0;
    
    // Embed image
    virtual bool embed(const Frame& frame, std::vector<float>& embedding) = 0;
    
    // Embed text
    virtual bool embedText(const std::string& text, std::vector<float>& embedding) = 0;
    
    // Batch embed images
    virtual bool embedBatch(const std::vector<Frame>& frames,
                           std::vector<std::vector<float>>& embeddings) = 0;
    
    // Get embedding dimension
    virtual int embeddingDim() const = 0;
    
    // Check if ready
    virtual bool isReady() const = 0;
    
    // Warmup
    virtual void warmup(int iterations = 3) = 0;
    
    // Get last error
    virtual std::string getLastError() const = 0;
};

// =============================================================================
// ClipEmbedder - CLIP model implementation
// =============================================================================

class ClipEmbedder : public Embedder {
public:
    explicit ClipEmbedder(const Config& cfg);
    ~ClipEmbedder() override;
    
    bool init() override;
    bool embed(const Frame& frame, std::vector<float>& embedding) override;
    bool embedText(const std::string& text, std::vector<float>& embedding) override;
    bool embedBatch(const std::vector<Frame>& frames,
                   std::vector<std::vector<float>>& embeddings) override;
    int embeddingDim() const override { return config_.embedding_dim; }
    bool isReady() const override { return initialized_; }
    void warmup(int iterations = 3) override;
    std::string getLastError() const override { return last_error_; }
    
private:
    // Preprocess image for CLIP
    bool preprocess(const Frame& frame, std::vector<float>& input);
    
    // Normalize embedding
    void normalize(std::vector<float>& embedding);
    
    Config config_;
    bool initialized_ = false;
    std::string last_error_;
    
    // ONNX Runtime session (opaque)
    class Impl;
    std::unique_ptr<Impl> impl_;
};

} // namespace rivision::pipeline
