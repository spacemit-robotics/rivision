/*
 * Batch Pipeline : Zero-copy frames + Batch inference integration
*/

#include <iostream>
#include <thread>

#include "batch_pipeline.h"
#include "utils/logger.h"

namespace rivision {

BatchPipeline::BatchPipeline(const BatchPipelineConfig& config)
    : config_(config) {
}

BatchPipeline::~BatchPipeline() {
    stop();
}

bool BatchPipeline::init() {
    // Initialize frame pool
    frame_pool_ = std::make_unique<FramePool>(config_.frame_pool);
    if (!frame_pool_->isInitialized()) {
        LOG_ERROR("BatchPipeline: Failed to initialize frame pool");
        return false;
    }
    
#ifdef USE_SPACEMIT_MPP
    // Initialize batch inference
    spacemit::BatchConfig batch_config;
    batch_config.batch_size = config_.batch_size;
    batch_config.max_wait_ms = config_.batch_wait_ms;
    batch_config.input_width = config_.input_width;
    batch_config.input_height = config_.input_height;
    batch_config.conf_threshold = config_.conf_threshold;
    batch_config.nms_threshold = config_.nms_threshold;
    batch_config.num_classes = config_.num_classes;
    batch_config.model_path = config_.model_path;
    batch_config.num_threads = config_.num_threads;
    
    batch_inference_ = std::make_unique<spacemit::NpuInferenceBatch>(batch_config);
    if (!batch_inference_->init()) {
        LOG_ERROR("BatchPipeline: Failed to initialize batch inference");
        return false;
    }
#endif
    
    initialized_ = true;
    LOG_INFO("BatchPipeline: Initialized with frame_pool_size={}, batch_size={}",
             config_.frame_pool.capacity, config_.batch_size);
    
    return true;
}

void BatchPipeline::start() {
    if (running_.load()) {
        return;
    }
    
    if (!initialized_.load()) {
        LOG_ERROR("BatchPipeline: Not initialized");
        return;
    }
    
#ifdef USE_SPACEMIT_MPP
    batch_inference_->start();
#endif
    
    running_ = true;
    LOG_INFO("BatchPipeline: Started");
}

void BatchPipeline::stop() {
    if (!running_.load()) {
        return;
    }
    
    running_ = false;
    
#ifdef USE_SPACEMIT_MPP
    if (batch_inference_) {
        batch_inference_->stop();
    }
#endif
    
    LOG_INFO("BatchPipeline: Stopped");
}

bool BatchPipeline::isRunning() const {
    return running_.load();
}

void BatchPipeline::registerStream(
    const std::string& stream_id,
    BatchDetectionCallback callback) {
    
    std::lock_guard<std::mutex> lock(streams_mutex_);
    
    StreamInfo info;
    info.stream_id = stream_id;
    info.callback = std::move(callback);
    info.frame_count = 0;
    info.detection_count = 0;
    
    streams_[stream_id] = std::move(info);
    
    LOG_DEBUG("BatchPipeline: Registered stream {}", stream_id);
}

void BatchPipeline::unregisterStream(const std::string& stream_id) {
    std::lock_guard<std::mutex> lock(streams_mutex_);
    
    auto it = streams_.find(stream_id);
    if (it != streams_.end()) {
        LOG_DEBUG("BatchPipeline: Unregistered stream {} (frames={}, detections={})",
                  stream_id, it->second.frame_count, it->second.detection_count);
        streams_.erase(it);
    }
}

bool BatchPipeline::submitFrame(
    const std::string& stream_id,
    const cv::Mat& frame,
    uint64_t frame_id) {
    
    if (!running_.load()) {
        return false;
    }
    
    // Find stream callback
    BatchDetectionCallback callback;
    {
        std::lock_guard<std::mutex> lock(streams_mutex_);
        auto it = streams_.find(stream_id);
        if (it == streams_.end()) {
            LOG_WARN("BatchPipeline: Unknown stream {}", stream_id);
            return false;
        }
        callback = it->second.callback;
        it->second.frame_count++;
    }
    
#ifdef USE_SPACEMIT_MPP
    // Submit to batch inference
    auto future = batch_inference_->submitFrame(frame, stream_id, frame_id);
    
    // Async result handling
    std::thread([this, stream_id, frame_id, callback, future = std::move(future)]() mutable {
        try {
            auto spacemit_dets = future.get();
            
            // Convert spacemit::Detection to rivision::Detection
            std::vector<Detection> detections;
            detections.reserve(spacemit_dets.size());
            for (const auto& d : spacemit_dets) {
                Detection det;
                det.bbox = BBox{d.x1, d.y1, d.x2, d.y2};
                det.confidence = d.confidence;
                det.class_id = d.class_id;
                det.class_name = d.class_name;
                detections.push_back(std::move(det));
            }
            
            // Update stats
            {
                std::lock_guard<std::mutex> lock(streams_mutex_);
                auto it = streams_.find(stream_id);
                if (it != streams_.end()) {
                    it->second.detection_count += detections.size();
                }
            }
            
            // Invoke callback
            if (callback) {
                callback(stream_id, frame_id, detections);
            }
        } catch (const std::exception& e) {
            LOG_ERROR("BatchPipeline: Inference error for {}: {}", stream_id, e.what());
        }
    }).detach();
    
    return true;
#else
    // Without SpacemiT, just call callback with empty results
    if (callback) {
        callback(stream_id, frame_id, {});
    }
    return true;
#endif
}

PooledFrame BatchPipeline::acquireFrame(int timeout_ms) {
    if (!frame_pool_) {
        return PooledFrame();
    }
    
    DmaBufFrame* frame = frame_pool_->acquire(timeout_ms);
    if (frame == nullptr) {
        return PooledFrame();
    }
    
    return PooledFrame(frame_pool_.get(), frame);
}

FramePoolStats BatchPipeline::getFramePoolStats() const {
    if (!frame_pool_) {
        return FramePoolStats{};
    }
    return frame_pool_->getStats();
}

#ifdef USE_SPACEMIT_MPP
spacemit::BatchStats BatchPipeline::getBatchStats() const {
    if (!batch_inference_) {
        return spacemit::BatchStats{};
    }
    return batch_inference_->getStats();
}
#endif

// Global Frame Pool singleton
GlobalFramePool& GlobalFramePool::instance() {
    static GlobalFramePool instance;
    return instance;
}

void GlobalFramePool::init(const FramePoolConfig& config) {
    std::lock_guard<std::mutex> lock(init_mutex_);
    
    if (pool_) {
        LOG_WARN("GlobalFramePool: Already initialized");
        return;
    }
    
    pool_ = std::make_unique<FramePool>(config);
    
    if (!pool_->isInitialized()) {
        LOG_ERROR("GlobalFramePool: Failed to initialize");
        pool_.reset();
        return;
    }
    
    LOG_INFO("GlobalFramePool: Initialized with {} frames", config.capacity);
}

PooledFrame GlobalFramePool::acquire(int timeout_ms) {
    if (!pool_) {
        return PooledFrame();
    }
    
    DmaBufFrame* frame = pool_->acquire(timeout_ms);
    if (frame == nullptr) {
        return PooledFrame();
    }
    
    return PooledFrame(pool_.get(), frame);
}

} // namespace rivision
