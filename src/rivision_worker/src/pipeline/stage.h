/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *
 * @File      :    stage.h
 * @Brief     :    Pipeline stage abstraction for async processing.
 *------------------------------------------------------------------------------
 */

#pragma once

#include "frame_queue.h"
#include <atomic>
#include <chrono>
#include <climits>
#include <cstdint>
#include <functional>
#include <future>
#include <string>
#include <thread>

namespace rivision::pipeline {

// Stage statistics structure
struct StageStats {
    uint64_t frames_processed = 0;
    uint64_t frames_dropped = 0;
    uint64_t total_latency_us = 0;
    uint64_t max_latency_us = 0;
    uint64_t min_latency_us = UINT64_MAX;
    uint64_t errors = 0;
    
    void reset();
    void recordLatency(uint64_t latency_us);
    double avgLatencyMs() const;
    double maxLatencyMs() const;
    double minLatencyMs() const;
};

template<typename Input, typename Output>
class Stage {
public:
    using ProcessFunc = std::function<Output(Input)>;
    using ErrorCallback = std::function<void(const std::string&)>;
    
    Stage(std::string name, ProcessFunc func, size_t queue_size = 4)
        : name_(std::move(name))
        , process_func_(std::move(func))
        , input_queue_(queue_size)
        , running_(false) {}
    
    ~Stage() { stop(); }
    
    void start() {
        if (running_.exchange(true)) return;
        
        worker_ = std::thread([this] {
            while (running_) {
                auto input_opt = input_queue_.pop(true);
                if (!input_opt) continue;
                
                auto start = std::chrono::steady_clock::now();
                
                try {
                    Output output = process_func_(std::move(*input_opt));
                    
                    auto end = std::chrono::steady_clock::now();
                    last_latency_us_ = std::chrono::duration_cast<
                        std::chrono::microseconds>(end - start).count();
                    total_latency_us_ += last_latency_us_;
                    processed_count_++;
                    
                    if (output_queue_) {
                        output_queue_->push(std::move(output));
                    }
                    if (output_callback_) {
                        output_callback_(std::move(output));
                    }
                } catch (const std::exception& e) {
                    if (error_callback_) {
                        error_callback_(name_ + ": " + e.what());
                    }
                }
            }
        });
    }
    
    void stop() {
        if (!running_.exchange(false)) return;
        input_queue_.stop();
        if (worker_.joinable()) {
            worker_.join();
        }
    }
    
    bool submit(Input input) {
        return input_queue_.push(std::move(input));
    }
    
    std::future<Output> submitAsync(Input input) {
        auto promise = std::make_shared<std::promise<Output>>();
        auto future = promise->get_future();
        
        // Wrap with promise fulfillment
        auto wrapped_func = [this, promise](Input in) {
            try {
                Output out = process_func_(std::move(in));
                promise->set_value(std::move(out));
                return out;
            } catch (...) {
                promise->set_exception(std::current_exception());
                throw;
            }
        };
        
        // This is simplified - actual async needs task queue
        submit(std::move(input));
        return future;
    }
    
    void setOutputQueue(FrameQueue<Output>* queue) {
        output_queue_ = queue;
    }
    
    void setOutputCallback(std::function<void(Output)> cb) {
        output_callback_ = std::move(cb);
    }
    
    void setErrorCallback(ErrorCallback cb) {
        error_callback_ = std::move(cb);
    }
    
    const std::string& name() const { return name_; }
    
    float getAvgLatencyMs() const {
        if (processed_count_ == 0) return 0.0f;
        return static_cast<float>(total_latency_us_) / processed_count_ / 1000.0f;
    }
    
    float getLastLatencyMs() const {
        return static_cast<float>(last_latency_us_) / 1000.0f;
    }
    
    uint64_t getProcessedCount() const { return processed_count_; }
    
    size_t queueSize() const { return input_queue_.size(); }

private:
    std::string name_;
    ProcessFunc process_func_;
    FrameQueue<Input> input_queue_;
    FrameQueue<Output>* output_queue_ = nullptr;
    std::function<void(Output)> output_callback_;
    ErrorCallback error_callback_;
    
    std::atomic<bool> running_;
    std::thread worker_;
    
    std::atomic<uint64_t> processed_count_{0};
    std::atomic<uint64_t> total_latency_us_{0};
    std::atomic<uint64_t> last_latency_us_{0};
};

} // namespace rivision::pipeline
