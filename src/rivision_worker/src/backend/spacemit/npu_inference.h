#pragma once

#include "hal/interface/i_inference.h"
#include "mpp_common.h"
#include <memory>

namespace rivision::spacemit {

// =============================================================================
// NpuInference - SpacemiT NPU inference (ONNX Runtime)
// =============================================================================

class NpuInference : public hal::IInference {
public:
    NpuInference();
    ~NpuInference() override;
    
    bool loadModel(const ModelConfig& cfg) override;
    void unloadModel() override;
    bool isLoaded() const override;
    ModelConfig getModelConfig() const override;
    bool infer(const Frame& frame, std::vector<Tensor>& outputs) override;
    void warmup(int iterations = 5) override;
    float getAvgLatencyMs() const override;
    std::string getLastError() const override;
    
private:
    ModelConfig config_;
    bool initialized_ = false;
    std::string last_error_;
    
    // ONNX Runtime handles (opaque)
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    // NPU acceleration
    bool use_npu_ = true;
    
    // Latency tracking
    float avg_latency_ms_ = 0.0f;
    int infer_count_ = 0;
    
    // Preprocess frame
    bool preprocess(const Frame& frame, std::vector<float>& input);
};

} // namespace rivision::spacemit
