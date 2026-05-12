#include "pipeline_manager.h"
#include "stream_context.h"
#include "inference/yolo_service.h"
#include "utils/logger.h"

namespace rivision::pipeline {

PipelineManager::PipelineManager() = default;

PipelineManager::~PipelineManager() {
    std::unique_lock lock(mutex_);
    
    for (auto& [id, ctx] : contexts_) {
        if (ctx) {
            ctx->stop();
        }
    }
    contexts_.clear();
}

void PipelineManager::setYoloService(YoloService* yolo_service) {
    yolo_service_ = yolo_service;
}

void PipelineManager::setDetectionCallback(DetectionCallback cb) {
    detection_callback_ = std::move(cb);
}

std::shared_ptr<StreamContext> PipelineManager::createContext(const StreamConfig& config) {
    std::unique_lock lock(mutex_);
    
    // Check if already exists
    auto it = contexts_.find(config.id);
    if (it != contexts_.end()) {
        LOG_WARN("Context already exists: {}", config.id);
        return it->second;
    }
    
    // Create new context
    auto context = std::make_shared<StreamContext>(
        config, 
        yolo_service_,
        detection_callback_
    );
    
    contexts_[config.id] = context;
    
    LOG_DEBUG("Created pipeline context: {}", config.id);
    return context;
}

void PipelineManager::removeContext(const StreamId& id) {
    std::unique_lock lock(mutex_);
    
    auto it = contexts_.find(id);
    if (it != contexts_.end()) {
        if (it->second) {
            it->second->stop();
        }
        contexts_.erase(it);
        LOG_DEBUG("Removed pipeline context: {}", id);
    }
}

int PipelineManager::activeCount() const {
    std::shared_lock lock(mutex_);
    
    int count = 0;
    for (const auto& [id, ctx] : contexts_) {
        if (ctx && ctx->getStatus().state == StreamState::RUNNING) {
            count++;
        }
    }
    return count;
}

} // namespace rivision::pipeline
