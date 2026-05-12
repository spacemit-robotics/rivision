/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *------------------------------------------------------------------------------
 */

#include "npu_scheduler.h"
#include <algorithm>
#include <chrono>

namespace rivision::pipeline {

NpuScheduler::NpuScheduler(std::shared_ptr<hal::IInference> yolo,
                           std::shared_ptr<hal::IInference> vlm,
                           Config config)
    : yolo_(std::move(yolo))
    , vlm_(std::move(vlm))
    , config_(std::move(config))
{
    last_vlm_time_ = std::chrono::steady_clock::now() - 
                     std::chrono::milliseconds(config_.vlm_cooldown_ms);
}

NpuScheduler::~NpuScheduler() {
    stop();
}

void NpuScheduler::start() {
    if (running_.exchange(true)) return;
    
    if (config_.enable_vlm && vlm_) {
        vlm_thread_ = std::thread(&NpuScheduler::vlmWorker, this);
    }
}

void NpuScheduler::stop() {
    if (!running_.exchange(false)) return;
    
    queue_cv_.notify_all();
    
    if (vlm_thread_.joinable()) {
        vlm_thread_.join();
    }
}

bool NpuScheduler::inferYolo(const hal::Frame& frame, std::vector<hal::Tensor>& outputs) {
    if (!yolo_) return false;
    
    auto start = std::chrono::steady_clock::now();
    bool result = false;
    
    if (config_.enable_npu_mutex) {
        std::lock_guard<std::mutex> lock(npu_mutex_);
        result = yolo_->infer(frame, outputs);
    } else {
        result = yolo_->infer(frame, outputs);
    }
    
    auto end = std::chrono::steady_clock::now();
    auto us = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
    
    yolo_count_++;
    yolo_total_us_ += us;
    
    return result;
}

std::future<std::string> NpuScheduler::submitVlmTask(VlmTask task) {
    auto future = task.result_promise.get_future();
    
    // Check cooldown
    auto now = std::chrono::steady_clock::now();
    auto elapsed = std::chrono::duration_cast<std::chrono::milliseconds>(
        now - last_vlm_time_).count();
    
    if (elapsed < config_.vlm_cooldown_ms) {
        task.result_promise.set_value("");
        return future;
    }
    
    {
        std::lock_guard<std::mutex> lock(queue_mutex_);
        
        // Drop if queue is full
        if (static_cast<int>(vlm_queue_.size()) >= config_.vlm_queue_size) {
            vlm_dropped_++;
            task.result_promise.set_value("");
            return future;
        }
        
        VlmTaskWrapper wrapper;
        wrapper.task = std::move(task);
        wrapper.priority = wrapper.task.priority;
        wrapper.submit_time = now;
        
        vlm_queue_.push(std::move(wrapper));
    }
    
    queue_cv_.notify_one();
    return future;
}

std::future<std::string> NpuScheduler::verifyDetection(const hal::Frame& frame,
                                                        float x1, float y1, float x2, float y2,
                                                        const std::string& class_name,
                                                        const std::string& prompt) {
    VlmTask task;
    task.frame = frame;
    task.detection_box = {x1, y1, x2, y2};
    task.detection_class = class_name;
    task.prompt = prompt.empty() ? 
        "Is there a " + class_name + " in this image? Answer yes or no." : prompt;
    task.priority = 1;
    
    return submitVlmTask(std::move(task));
}

bool NpuScheduler::shouldTriggerVlm(const std::string& class_name, float confidence) const {
    if (!config_.enable_vlm) return false;
    if (confidence >= config_.vlm_trigger_conf) return false;
    
    // Check if class is in VLM trigger list
    if (!config_.vlm_classes.empty()) {
        auto it = std::find(config_.vlm_classes.begin(), 
                            config_.vlm_classes.end(), 
                            class_name);
        if (it == config_.vlm_classes.end()) {
            return false;
        }
    }
    
    return true;
}

void NpuScheduler::vlmWorker() {
    while (running_) {
        VlmTaskWrapper wrapper;
        
        {
            std::unique_lock<std::mutex> lock(queue_mutex_);
            queue_cv_.wait(lock, [this] {
                return !running_ || !vlm_queue_.empty();
            });
            
            if (!running_ && vlm_queue_.empty()) break;
            if (vlm_queue_.empty()) continue;
            
            wrapper = std::move(const_cast<VlmTaskWrapper&>(vlm_queue_.top()));
            vlm_queue_.pop();
        }
        
        // Process task
        auto start = std::chrono::steady_clock::now();
        
        std::string result = cropAndInferVlm(wrapper.task.frame,
                                              wrapper.task.detection_box,
                                              wrapper.task.prompt);
        
        auto end = std::chrono::steady_clock::now();
        auto us = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
        
        vlm_count_++;
        vlm_total_us_ += us;
        last_vlm_time_ = end;
        
        // Fulfill promise
        wrapper.task.result_promise.set_value(result);
        
        // Callback
        if (vlm_callback_) {
            vlm_callback_(result, wrapper.task);
        }
    }
}

std::string NpuScheduler::cropAndInferVlm(const hal::Frame& frame,
                                           const std::vector<float>& box,
                                           const std::string& prompt) {
    if (!vlm_ || box.size() < 4) return "";
    
    // Crop region
    int x1 = static_cast<int>(box[0] * frame.width);
    int y1 = static_cast<int>(box[1] * frame.height);
    int x2 = static_cast<int>(box[2] * frame.width);
    int y2 = static_cast<int>(box[3] * frame.height);
    
    // Clamp
    x1 = std::max(0, std::min(x1, frame.width - 1));
    y1 = std::max(0, std::min(y1, frame.height - 1));
    x2 = std::max(x1 + 1, std::min(x2, frame.width));
    y2 = std::max(y1 + 1, std::min(y2, frame.height));
    
    // Create cropped frame
    hal::Frame cropped;
    cropped.width = x2 - x1;
    cropped.height = y2 - y1;
    cropped.format = frame.format;
    
    // Simple crop for NV12 (Y plane only for simplicity)
    if (frame.format == hal::PixelFormat::NV12) {
        cropped.data.resize(cropped.width * cropped.height * 3 / 2);
        
        // Copy Y plane
        for (int y = 0; y < cropped.height; y++) {
            const uint8_t* src = frame.data.data() + (y1 + y) * frame.width + x1;
            uint8_t* dst = cropped.data.data() + y * cropped.width;
            std::copy(src, src + cropped.width, dst);
        }
        
        // Copy UV plane (simplified)
        int src_uv_offset = frame.width * frame.height;
        int dst_uv_offset = cropped.width * cropped.height;
        
        for (int y = 0; y < cropped.height / 2; y++) {
            const uint8_t* src = frame.data.data() + src_uv_offset + 
                                 ((y1 / 2) + y) * frame.width + (x1 & ~1);
            uint8_t* dst = cropped.data.data() + dst_uv_offset + y * cropped.width;
            std::copy(src, src + cropped.width, dst);
        }
    }
    
    // Infer with VLM
    std::vector<hal::Tensor> outputs;
    
    {
        std::lock_guard<std::mutex> lock(npu_mutex_);
        
        // Set prompt if VLM supports it
        // vlm_->setPrompt(prompt);
        
        if (!vlm_->infer(cropped, outputs)) {
            return "";
        }
    }
    
    // Parse VLM output (model-specific)
    if (!outputs.empty() && !outputs[0].data.empty()) {
        // Simplified: assume output is text tokens
        return "VLM result for: " + prompt;
    }
    
    return "";
}

SchedulerStats NpuScheduler::getStats() const {
    SchedulerStats stats;
    
    stats.yolo_infer_count = yolo_count_.load();
    stats.vlm_infer_count = vlm_count_.load();
    
    if (stats.yolo_infer_count > 0) {
        stats.yolo_avg_ms = static_cast<float>(yolo_total_us_) / 
                            stats.yolo_infer_count / 1000.0f;
    }
    
    if (stats.vlm_infer_count > 0) {
        stats.vlm_avg_ms = static_cast<float>(vlm_total_us_) / 
                           stats.vlm_infer_count / 1000.0f;
    }
    
    // Estimate NPU utilization
    uint64_t total_us = yolo_total_us_ + vlm_total_us_;
    auto elapsed = std::chrono::steady_clock::now() - last_vlm_time_;
    auto elapsed_us = std::chrono::duration_cast<std::chrono::microseconds>(elapsed).count();
    
    if (elapsed_us > 0) {
        stats.npu_utilization = std::min(1.0f, 
            static_cast<float>(total_us) / (elapsed_us * 2));  // Rough estimate
    }
    
    {
        std::lock_guard<std::mutex> lock(const_cast<std::mutex&>(queue_mutex_));
        stats.vlm_queue_depth = static_cast<int>(vlm_queue_.size());
    }
    
    stats.vlm_dropped_tasks = vlm_dropped_.load();
    
    return stats;
}

} // namespace rivision::pipeline
