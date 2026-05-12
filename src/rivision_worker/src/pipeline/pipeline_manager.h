#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <memory>
#include <unordered_map>
#include <shared_mutex>

namespace rivision::pipeline {

class StreamContext;
class YoloService;

// =============================================================================
// PipelineManager - Manages stream processing pipelines
// =============================================================================

class PipelineManager {
public:
    PipelineManager();
    ~PipelineManager();
    
    // Set shared YoloService
    void setYoloService(YoloService* yolo_service);
    
    // Set detection callback
    void setDetectionCallback(DetectionCallback cb);
    
    // Create a new stream context
    std::shared_ptr<StreamContext> createContext(const StreamConfig& config);
    
    // Remove stream context
    void removeContext(const StreamId& id);
    
    // Get active context count
    int activeCount() const;
    
private:
    YoloService* yolo_service_ = nullptr;
    DetectionCallback detection_callback_;
    
    std::unordered_map<StreamId, std::shared_ptr<StreamContext>> contexts_;
    mutable std::shared_mutex mutex_;
};

} // namespace rivision::pipeline
