#pragma once

#include "rivision/types.h"
#include "hal/interface/i_inference.h"
#include <memory>
#include <vector>
#include <string>
#include <atomic>

namespace rivision::hal {
class IInference;
}

namespace rivision::pipeline {

// =============================================================================
// YoloService - YOLO inference service
// =============================================================================

class YoloService {
public:
    struct Options {
        std::string model_path;
        int input_width = 640;
        int input_height = 640;
        float conf_threshold = 0.5f;
        float nms_threshold = 0.45f;
        std::string device = "npu";  // npu | cpu
        std::vector<std::string> class_names;
    };
    
    static std::unique_ptr<YoloService> create(const Options& opts);
    
    virtual ~YoloService() = default;
    
    // Initialize (load model, warmup)
    virtual bool init() = 0;
    
    // Detect objects in frame
    virtual std::vector<Detection> detect(const Frame& frame) = 0;
    
    // Batch detection
    virtual std::vector<std::vector<Detection>> detectBatch(
        const std::vector<Frame>& frames) = 0;
    
    // Get class names
    virtual const std::vector<std::string>& getClassNames() const = 0;
    
    // Warmup (N iterations)
    virtual void warmup(int iterations = 5) = 0;
    
    // Get average latency in ms
    virtual float getAvgLatencyMs() const = 0;
    
    // Get total inference count
    virtual int64_t getInferenceCount() const = 0;
    
protected:
    Options options_;
    std::atomic<int64_t> inference_count_{0};
    std::atomic<float> avg_latency_ms_{0.0f};
};

// =============================================================================
// YoloServiceImpl - Implementation
// =============================================================================

class YoloServiceImpl : public YoloService {
public:
    explicit YoloServiceImpl(const Options& opts);
    ~YoloServiceImpl() override;
    
    bool init() override;
    std::vector<Detection> detect(const Frame& frame) override;
    std::vector<std::vector<Detection>> detectBatch(
        const std::vector<Frame>& frames) override;
    const std::vector<std::string>& getClassNames() const override;
    void warmup(int iterations = 5) override;
    float getAvgLatencyMs() const override;
    int64_t getInferenceCount() const override;
    
private:
    // Preprocess frame to model input
    bool preprocess(const Frame& frame, std::vector<float>& input);
    
    // Postprocess model output to detections (standard format)
    std::vector<Detection> postprocess(
        const std::vector<float>& output,
        int orig_width, int orig_height);
    
    // Postprocess SpacemiT DFL multi-branch output (boxes, scores, score_sum)
    std::vector<Detection> postprocessDFL(
        const std::vector<hal::IInference::Tensor>& outputs,
        int orig_width, int orig_height);
    
    // Non-maximum suppression
    std::vector<Detection> nms(
        std::vector<Detection>& detections,
        float iou_threshold);
    
    std::unique_ptr<hal::IInference> inference_;
    std::vector<std::string> class_names_;
    
    // Preprocessing buffer
    std::vector<float> input_buffer_;
};

} // namespace rivision::pipeline
