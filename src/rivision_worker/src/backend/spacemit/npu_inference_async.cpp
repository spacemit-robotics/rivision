/**
 * @file npu_inference_async.cpp
 * @brief Asynchronous NPU inference implementation
 */

#include "npu_inference_async.h"
#include "npu_inference.h"
#include <chrono>

namespace rivision {
namespace spacemit {

NpuInferenceAsync::NpuInferenceAsync() = default;

NpuInferenceAsync::~NpuInferenceAsync() {
    close();
}

hal::ErrorCode NpuInferenceAsync::open(const Config& config) {
    if (running_.load()) {
        return hal::ErrorCode::ALREADY_INITIALIZED;
    }
    
    config_ = config;
    
    // Create underlying sync inference
    inference_ = std::make_unique<NpuInference>();
    
    hal::ModelConfig model_cfg;
    model_cfg.model_path = config.model_path;
    model_cfg.device = config.device;
    model_cfg.enable_fp16 = config.enable_fp16;
    
    auto ret = inference_->loadModel(model_cfg);
    if (ret != hal::ErrorCode::SUCCESS) {
        inference_.reset();
        return ret;
    }
    
    // Clear queues
    {
        std::lock_guard<std::mutex> lock(input_mutex_);
        while (!input_queue_.empty()) input_queue_.pop();
    }
    {
        std::lock_guard<std::mutex> lock(output_mutex_);
        while (!output_queue_.empty()) output_queue_.pop();
    }
    
    // Reset stats
    {
        std::lock_guard<std::mutex> lock(stats_mutex_);
        stats_ = Stats{};
        total_infer_ms_ = 0.0;
        min_infer_ms_ = 1e9;
    }
    
    // Start inference thread
    running_.store(true);
    infer_thread_ = std::thread(&NpuInferenceAsync::inferThread, this);
    
    return hal::ErrorCode::SUCCESS;
}

void NpuInferenceAsync::close() {
    if (!running_.load()) {
        return;
    }
    
    running_.store(false);
    
    // Wake up thread
    {
        std::lock_guard<std::mutex> lock(input_mutex_);
        input_cv_.notify_all();
    }
    
    if (infer_thread_.joinable()) {
        infer_thread_.join();
    }
    
    if (inference_) {
        inference_->unloadModel();
        inference_.reset();
    }
}

hal::ErrorCode NpuInferenceAsync::submitTask(InferenceTask&& task) {
    if (!running_.load()) {
        return hal::ErrorCode::NOT_INITIALIZED;
    }
    
    {
        std::lock_guard<std::mutex> lock(input_mutex_);
        
        if (static_cast<int>(input_queue_.size()) >= config_.input_queue_size) {
            // Drop oldest if queue full
            input_queue_.pop();
            
            std::lock_guard<std::mutex> slock(stats_mutex_);
            stats_.frames_dropped++;
        }
        
        input_queue_.push(std::move(task));
    }
    
    input_cv_.notify_one();
    return hal::ErrorCode::SUCCESS;
}

hal::ErrorCode NpuInferenceAsync::submitFrame(const hal::Frame& frame, uint64_t frame_id) {
    InferenceTask task;
    task.frame = frame;
    task.frame_id = frame_id;
    task.timestamp_us = std::chrono::duration_cast<std::chrono::microseconds>(
        std::chrono::steady_clock::now().time_since_epoch()).count();
    
    return submitTask(std::move(task));
}

void NpuInferenceAsync::setResultCallback(ResultCallback callback) {
    std::lock_guard<std::mutex> lock(callback_mutex_);
    result_callback_ = std::move(callback);
}

void NpuInferenceAsync::setErrorCallback(ErrorCallback callback) {
    std::lock_guard<std::mutex> lock(callback_mutex_);
    error_callback_ = std::move(callback);
}

bool NpuInferenceAsync::tryGetResult(InferenceResultAsync& result) {
    std::lock_guard<std::mutex> lock(output_mutex_);
    
    if (output_queue_.empty()) {
        return false;
    }
    
    result = std::move(output_queue_.front());
    output_queue_.pop();
    return true;
}

bool NpuInferenceAsync::waitResult(InferenceResultAsync& result, int timeout_ms) {
    std::unique_lock<std::mutex> lock(output_mutex_);
    
    auto pred = [this]() { return !output_queue_.empty() || !running_.load(); };
    
    if (!output_cv_.wait_for(lock, std::chrono::milliseconds(timeout_ms), pred)) {
        return false;
    }
    
    if (output_queue_.empty()) {
        return false;
    }
    
    result = std::move(output_queue_.front());
    output_queue_.pop();
    return true;
}

NpuInferenceAsync::Stats NpuInferenceAsync::getStats() const {
    std::lock_guard<std::mutex> lock(stats_mutex_);
    Stats s = stats_;
    
    // Get queue depth
    {
        std::lock_guard<std::mutex> ilock(const_cast<std::mutex&>(input_mutex_));
        s.queue_depth = static_cast<int>(input_queue_.size());
    }
    
    return s;
}

int NpuInferenceAsync::getQueueDepth() const {
    std::lock_guard<std::mutex> lock(const_cast<std::mutex&>(input_mutex_));
    return static_cast<int>(input_queue_.size());
}

std::vector<int> NpuInferenceAsync::getInputShape() const {
    if (!inference_) {
        return {};
    }
    return inference_->getInputShape(0);
}

void NpuInferenceAsync::inferThread() {
    while (running_.load()) {
        InferenceTask task;
        
        // Get task from input queue
        {
            std::unique_lock<std::mutex> lock(input_mutex_);
            
            input_cv_.wait(lock, [this]() {
                return !input_queue_.empty() || !running_.load();
            });
            
            if (!running_.load() && input_queue_.empty()) {
                break;
            }
            
            if (input_queue_.empty()) {
                continue;
            }
            
            task = std::move(input_queue_.front());
            input_queue_.pop();
        }
        
        auto start = std::chrono::steady_clock::now();
        
        // Run inference
        InferenceResultAsync result;
        result.frame_id = task.frame_id;
        result.timestamp_us = task.timestamp_us;
        result.user_data = task.user_data;
        
        auto ret = inference_->infer(task.frame, result.outputs);
        
        auto end = std::chrono::steady_clock::now();
        result.infer_time_ms = std::chrono::duration<double, std::milli>(end - start).count();
        
        if (ret == hal::ErrorCode::SUCCESS) {
            result.success = true;
            result.error = hal::ErrorCode::SUCCESS;
            
            // Update statistics
            updateStats(result.infer_time_ms);
            
        } else {
            result.success = false;
            result.error = ret;
            
            {
                std::lock_guard<std::mutex> lock(stats_mutex_);
                stats_.infer_errors++;
            }
            
            // Error callback
            std::lock_guard<std::mutex> lock(callback_mutex_);
            if (error_callback_) {
                error_callback_(ret, "Inference failed");
            }
        }
        
        // Deliver result via callback
        {
            std::lock_guard<std::mutex> lock(callback_mutex_);
            if (result_callback_) {
                InferenceResultAsync result_copy = result;
                result_callback_(std::move(result_copy));
            }
        }
        
        // Push to output queue
        {
            std::lock_guard<std::mutex> lock(output_mutex_);
            output_queue_.push(std::move(result));
        }
        output_cv_.notify_one();
    }
}

void NpuInferenceAsync::updateStats(double infer_ms) {
    std::lock_guard<std::mutex> lock(stats_mutex_);
    
    stats_.frames_inferred++;
    total_infer_ms_ += infer_ms;
    stats_.avg_infer_ms = total_infer_ms_ / stats_.frames_inferred;
    
    if (infer_ms > stats_.max_infer_ms) {
        stats_.max_infer_ms = infer_ms;
    }
    if (infer_ms < min_infer_ms_) {
        min_infer_ms_ = infer_ms;
        stats_.min_infer_ms = infer_ms;
    }
    
    // Estimate NPU utilization (rough estimate based on inference time)
    // Assuming 15ms is 100% utilization at 60+ FPS
    stats_.npu_utilization = std::min(1.0, stats_.avg_infer_ms / 15.0);
}

} // namespace spacemit
} // namespace rivision
