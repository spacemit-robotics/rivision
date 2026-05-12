/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *
 * @File      :    frame_queue.h
 * @Brief     :    Lock-free frame queue for async pipeline.
 *------------------------------------------------------------------------------
 */

#pragma once

#include "hal/interface/i_decoder.h"
#include "rivision/types.h"
#include <atomic>
#include <condition_variable>
#include <mutex>
#include <optional>
#include <queue>

namespace rivision::pipeline {

template<typename T>
class FrameQueue {
public:
    explicit FrameQueue(size_t max_size = 8)
        : max_size_(max_size), running_(true) {}
    
    ~FrameQueue() { stop(); }
    
    bool push(T item, bool block = true) {
        std::unique_lock<std::mutex> lock(mutex_);
        
        if (block) {
            not_full_.wait(lock, [this] { 
                return !running_ || queue_.size() < max_size_; 
            });
        } else {
            if (queue_.size() >= max_size_) return false;
        }
        
        if (!running_) return false;
        
        queue_.push(std::move(item));
        not_empty_.notify_one();
        return true;
    }
    
    std::optional<T> pop(bool block = true) {
        std::unique_lock<std::mutex> lock(mutex_);
        
        if (block) {
            not_empty_.wait(lock, [this] { 
                return !running_ || !queue_.empty(); 
            });
        }
        
        if (queue_.empty()) return std::nullopt;
        
        T item = std::move(queue_.front());
        queue_.pop();
        not_full_.notify_one();
        return item;
    }
    
    bool tryPop(T& out) {
        std::lock_guard<std::mutex> lock(mutex_);
        if (queue_.empty()) return false;
        out = std::move(queue_.front());
        queue_.pop();
        not_full_.notify_one();
        return true;
    }
    
    void stop() {
        {
            std::lock_guard<std::mutex> lock(mutex_);
            running_ = false;
        }
        not_empty_.notify_all();
        not_full_.notify_all();
    }
    
    void start() {
        std::lock_guard<std::mutex> lock(mutex_);
        running_ = true;
    }
    
    void clear() {
        std::lock_guard<std::mutex> lock(mutex_);
        while (!queue_.empty()) queue_.pop();
        not_full_.notify_all();
    }
    
    size_t size() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return queue_.size();
    }
    
    bool empty() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return queue_.empty();
    }
    
    bool full() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return queue_.size() >= max_size_;
    }

private:
    size_t max_size_;
    std::atomic<bool> running_;
    mutable std::mutex mutex_;
    std::condition_variable not_empty_;
    std::condition_variable not_full_;
    std::queue<T> queue_;
};

using PacketQueue = FrameQueue<hal::StreamPacket>;
using DecodedFrameQueue = FrameQueue<rivision::Frame>;

} // namespace rivision::pipeline
