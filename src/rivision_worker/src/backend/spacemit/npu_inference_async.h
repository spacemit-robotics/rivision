/**
 * @file npu_inference_async.h
 * @brief Asynchronous NPU inference wrapper for SpacemiT K3
 * 
 * Provides non-blocking inference API with callback-based result delivery.
 * Uses separate inference thread to overlap inference with decode/encode.
 */

#ifndef RIVISION_BACKEND_SPACEMIT_NPU_INFERENCE_ASYNC_H
#define RIVISION_BACKEND_SPACEMIT_NPU_INFERENCE_ASYNC_H

#include "hal/interface/i_inference.h"
#include "pipeline/frame_queue.h"
#include <memory>
#include <thread>
#include <atomic>
#include <functional>
#include <queue>

namespace rivision {
namespace spacemit {

/**
 * @brief Inference task submitted to async inference
 */
struct InferenceTask {
    hal::Frame frame;
    uint64_t frame_id = 0;
    uint64_t timestamp_us = 0;
    void* user_data = nullptr;
};

/**
 * @brief Inference result from async inference
 */
struct InferenceResultAsync {
    std::vector<hal::Tensor> outputs;
    uint64_t frame_id = 0;
    uint64_t timestamp_us = 0;
    double infer_time_ms = 0.0;
    void* user_data = nullptr;
    bool success = false;
    hal::ErrorCode error = hal::ErrorCode::SUCCESS;
};

/**
 * @brief Async inference wrapping NpuInference with threaded execution
 */
class NpuInferenceAsync {
public:
    using ResultCallback = std::function<void(InferenceResultAsync&& result)>;
    using ErrorCallback = std::function<void(hal::ErrorCode code, const std::string& msg)>;

    struct Config {
        std::string model_path;
        std::string device = "NPU";     // "NPU" or "CPU"
        int input_queue_size = 4;       // Max frames waiting for inference
        int batch_size = 1;             // Batch size (1 for real-time)
        bool enable_fp16 = false;       // FP16 inference if supported
        int timeout_ms = 100;           // Inference timeout
    };

    struct Stats {
        uint64_t frames_inferred = 0;
        uint64_t frames_dropped = 0;
        uint64_t infer_errors = 0;
        double avg_infer_ms = 0.0;
        double max_infer_ms = 0.0;
        double min_infer_ms = 0.0;
        int queue_depth = 0;
        double npu_utilization = 0.0;   // 0.0 - 1.0
    };

public:
    NpuInferenceAsync();
    ~NpuInferenceAsync();

    /**
     * @brief Initialize async inference with model
     * @param config Inference configuration
     * @return ErrorCode::SUCCESS on success
     */
    hal::ErrorCode open(const Config& config);

    /**
     * @brief Shutdown inference and release resources
     */
    void close();

    /**
     * @brief Submit frame for async inference (non-blocking)
     * @param task Inference task containing frame and metadata
     * @return ErrorCode::SUCCESS if queued, QUEUE_FULL if buffer full
     */
    hal::ErrorCode submitTask(InferenceTask&& task);

    /**
     * @brief Submit frame for async inference (convenience)
     * @param frame Input frame
     * @param frame_id Optional frame identifier
     * @return ErrorCode::SUCCESS if queued
     */
    hal::ErrorCode submitFrame(const hal::Frame& frame, uint64_t frame_id = 0);

    /**
     * @brief Set callback for inference results
     * @param callback Function called when inference completes
     */
    void setResultCallback(ResultCallback callback);

    /**
     * @brief Set callback for inference errors
     * @param callback Function called on inference error
     */
    void setErrorCallback(ErrorCallback callback);

    /**
     * @brief Try to get next inference result (non-blocking)
     * @param result Output result
     * @return true if result available, false if queue empty
     */
    bool tryGetResult(InferenceResultAsync& result);

    /**
     * @brief Wait for next inference result (blocking)
     * @param result Output result
     * @param timeout_ms Max wait time
     * @return true if result received, false on timeout
     */
    bool waitResult(InferenceResultAsync& result, int timeout_ms = 100);

    /**
     * @brief Get inference statistics
     */
    Stats getStats() const;

    /**
     * @brief Check if inference is running
     */
    bool isRunning() const { return running_.load(); }

    /**
     * @brief Get current input queue depth
     */
    int getQueueDepth() const;

    /**
     * @brief Get model input shape
     */
    std::vector<int> getInputShape() const;

private:
    void inferThread();
    void updateStats(double infer_ms);

private:
    std::unique_ptr<hal::IInference> inference_;
    Config config_;
    
    // Input queue (tasks to infer)
    std::queue<InferenceTask> input_queue_;
    std::mutex input_mutex_;
    std::condition_variable input_cv_;
    
    // Output queue (inference results)
    std::queue<InferenceResultAsync> output_queue_;
    std::mutex output_mutex_;
    std::condition_variable output_cv_;
    
    // Inference thread
    std::thread infer_thread_;
    std::atomic<bool> running_{false};
    
    // Callbacks
    ResultCallback result_callback_;
    ErrorCallback error_callback_;
    std::mutex callback_mutex_;
    
    // Statistics
    mutable std::mutex stats_mutex_;
    Stats stats_;
    double total_infer_ms_ = 0.0;
    double min_infer_ms_ = 1e9;
};

} // namespace spacemit
} // namespace rivision

#endif // RIVISION_BACKEND_SPACEMIT_NPU_INFERENCE_ASYNC_H
