/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *
 * @File      :    npu_scheduler.h
 * @Brief     :    NPU time-sharing scheduler for YOLO + VLM inference.
 *
 * Optimization: Layered triggering + async VLM scheduling for NPU sharing.
 * Architecture:
 *   YOLO (high-freq, real-time) → VLM (low-freq, async verification)
 *   NPU mutex ensures only one model runs at a time.
 *------------------------------------------------------------------------------
 */

#pragma once

#include "hal/interface/i_inference.h"
#include <atomic>
#include <condition_variable>
#include <functional>
#include <future>
#include <memory>
#include <mutex>
#include <queue>
#include <string>
#include <thread>
#include <vector>

namespace rivision::pipeline {

struct VlmTask {
    hal::Frame frame;
    std::string prompt;
    std::vector<float> detection_box;  // [x1, y1, x2, y2]
    std::string detection_class;
    int priority = 0;
    std::promise<std::string> result_promise;
};

struct SchedulerStats {
    uint64_t yolo_infer_count = 0;
    uint64_t vlm_infer_count = 0;
    float yolo_avg_ms = 0;
    float vlm_avg_ms = 0;
    float npu_utilization = 0;      // 0.0 - 1.0
    int vlm_queue_depth = 0;
    int vlm_dropped_tasks = 0;
};

class NpuScheduler {
public:
    struct Config {
        // YOLO settings
        float yolo_conf_threshold = 0.5f;
        int yolo_max_fps = 30;
        
        // VLM trigger settings
        bool enable_vlm = true;
        float vlm_trigger_conf = 0.7f;           // Trigger VLM if YOLO conf < this
        std::vector<std::string> vlm_classes;    // Classes that need VLM verify
        int vlm_cooldown_ms = 1000;              // Min interval between VLM calls
        
        // Queue settings
        int vlm_queue_size = 10;
        int vlm_priority_levels = 3;
        
        // Resource settings
        int vlm_timeout_ms = 5000;
        bool enable_npu_mutex = true;
    };
    
    using VlmResultCallback = std::function<void(const std::string& result, 
                                                   const VlmTask& task)>;
    
    NpuScheduler(std::shared_ptr<hal::IInference> yolo,
                 std::shared_ptr<hal::IInference> vlm,
                 Config config = {});
    ~NpuScheduler();
    
    void start();
    void stop();
    
    // Synchronous YOLO inference (real-time path)
    bool inferYolo(const hal::Frame& frame, std::vector<hal::Tensor>& outputs);
    
    // Async VLM verification (queued)
    std::future<std::string> submitVlmTask(VlmTask task);
    
    // Convenience: submit VLM for specific detection
    std::future<std::string> verifyDetection(const hal::Frame& frame,
                                              float x1, float y1, float x2, float y2,
                                              const std::string& class_name,
                                              const std::string& prompt);
    
    // Check if VLM should be triggered based on config
    bool shouldTriggerVlm(const std::string& class_name, float confidence) const;
    
    // Callbacks
    void setVlmResultCallback(VlmResultCallback cb) { vlm_callback_ = std::move(cb); }
    
    // Stats
    SchedulerStats getStats() const;
    
    // Dynamic config
    void setYoloThreshold(float conf) { config_.yolo_conf_threshold = conf; }
    void setVlmEnabled(bool enabled) { config_.enable_vlm = enabled; }

private:
    void vlmWorker();
    std::string cropAndInferVlm(const hal::Frame& frame, 
                                 const std::vector<float>& box,
                                 const std::string& prompt);
    
    std::shared_ptr<hal::IInference> yolo_;
    std::shared_ptr<hal::IInference> vlm_;
    Config config_;
    
    // NPU mutex for time-sharing
    std::mutex npu_mutex_;
    
    // VLM task queue (priority queue)
    struct VlmTaskWrapper {
        VlmTask task;
        int priority;
        std::chrono::steady_clock::time_point submit_time;
        
        bool operator<(const VlmTaskWrapper& other) const {
            return priority < other.priority;  // Higher priority first
        }
    };
    
    std::priority_queue<VlmTaskWrapper> vlm_queue_;
    std::mutex queue_mutex_;
    std::condition_variable queue_cv_;
    
    std::thread vlm_thread_;
    std::atomic<bool> running_{false};
    
    VlmResultCallback vlm_callback_;
    
    // Stats
    std::atomic<uint64_t> yolo_count_{0};
    std::atomic<uint64_t> vlm_count_{0};
    std::atomic<uint64_t> yolo_total_us_{0};
    std::atomic<uint64_t> vlm_total_us_{0};
    std::atomic<int> vlm_dropped_{0};
    
    std::chrono::steady_clock::time_point last_vlm_time_;
};

} // namespace rivision::pipeline
