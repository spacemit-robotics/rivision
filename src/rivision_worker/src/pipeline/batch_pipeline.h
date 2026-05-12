/*
 * Batch Pipeline: Combines FramePool + BatchInference for optimal throughput
*/

#pragma once

#include "frame_pool.h"

#ifdef USE_SPACEMIT_MPP
#include "backend/spacemit/npu_inference_batch.h"
#endif

#include <opencv2/core/mat.hpp>
#include <unordered_map>

namespace rivision {

// Use Detection from types.h

// Batch pipeline configuration
struct BatchPipelineConfig {
    // Frame pool config
    FramePoolConfig frame_pool;
    
    // Batch inference config (only for SpacemiT)
    int batch_size = 2;
    int batch_wait_ms = 5;
    
    // Model config
    std::string model_path;
    int input_width = 640;
    int input_height = 640;
    float conf_threshold = 0.5f;
    float nms_threshold = 0.45f;
    int num_classes = 80;
    
    // Threading
    int num_threads = 4;
};

// Batch detection callback type (different from types.h DetectionCallback)
using BatchDetectionCallback = std::function<void(
    const std::string& stream_id,
    uint64_t frame_id,
    const std::vector<Detection>& detections
)>;

/**
 * @brief Optimized batch processing pipeline
 * 
 * Integrates:
 * - FramePool for zero-copy frame management
 * - NpuInferenceBatch for batched inference
 * - Per-stream queues for frame collection
 * 
 * Usage:
 * 1. Create BatchPipeline with config
 * 2. Register streams with registerStream()
 * 3. Submit frames with submitFrame()
 * 4. Receive detections via callback
 */
class BatchPipeline {
public:
    explicit BatchPipeline(const BatchPipelineConfig& config);
    ~BatchPipeline();
    
    // Non-copyable
    BatchPipeline(const BatchPipeline&) = delete;
    BatchPipeline& operator=(const BatchPipeline&) = delete;
    
    /**
     * @brief Initialize the pipeline
     * @return true on success
     */
    bool init();
    
    /**
     * @brief Start the pipeline
     */
    void start();
    
    /**
     * @brief Stop the pipeline
     */
    void stop();
    
    /**
     * @brief Check if pipeline is running
     */
    bool isRunning() const;
    
    /**
     * @brief Register a stream for processing
     * @param stream_id Unique stream identifier
     * @param callback Detection callback for this stream
     */
    void registerStream(
        const std::string& stream_id,
        BatchDetectionCallback callback
    );
    
    /**
     * @brief Unregister a stream
     * @param stream_id Stream to unregister
     */
    void unregisterStream(const std::string& stream_id);
    
    /**
     * @brief Submit a decoded frame for inference
     * @param stream_id Stream identifier
     * @param frame Frame to process (will be cloned if needed)
     * @param frame_id Frame number
     * @return true if submitted successfully
     */
    bool submitFrame(
        const std::string& stream_id,
        const cv::Mat& frame,
        uint64_t frame_id = 0
    );
    
    /**
     * @brief Acquire a frame from the pool
     * @param timeout_ms Timeout in milliseconds
     * @return Pooled frame handle
     */
    PooledFrame acquireFrame(int timeout_ms = 100);
    
    /**
     * @brief Get frame pool statistics
     */
    FramePoolStats getFramePoolStats() const;
    
#ifdef USE_SPACEMIT_MPP
    /**
     * @brief Get batch inference statistics
     */
    spacemit::BatchStats getBatchStats() const;
#endif
    
    /**
     * @brief Get configuration
     */
    const BatchPipelineConfig& config() const { return config_; }
    
private:
    struct StreamInfo {
        std::string stream_id;
        BatchDetectionCallback callback;
        uint64_t frame_count = 0;
        uint64_t detection_count = 0;
    };
    
    void processResults();
    
    BatchPipelineConfig config_;
    
    std::unique_ptr<FramePool> frame_pool_;
    
#ifdef USE_SPACEMIT_MPP
    std::unique_ptr<spacemit::NpuInferenceBatch> batch_inference_;
#endif
    
    std::unordered_map<std::string, StreamInfo> streams_;
    mutable std::mutex streams_mutex_;
    
    std::atomic<bool> running_{false};
    std::atomic<bool> initialized_{false};
};

// Global frame pool instance (singleton for memory efficiency)
class GlobalFramePool {
public:
    static GlobalFramePool& instance();
    
    void init(const FramePoolConfig& config);
    bool isInitialized() const { return pool_ != nullptr; }
    
    FramePool* get() { return pool_.get(); }
    
    PooledFrame acquire(int timeout_ms = 100);
    
private:
    GlobalFramePool() = default;
    std::unique_ptr<FramePool> pool_;
    std::mutex init_mutex_;
};

} // namespace rivision
