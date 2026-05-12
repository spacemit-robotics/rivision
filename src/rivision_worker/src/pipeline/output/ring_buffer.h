#pragma once

#include "rivision/types.h"
#include <vector>
#include <mutex>
#include <atomic>

namespace rivision::pipeline {

// =============================================================================
// RingBuffer - Lock-free ring buffer for frames
// =============================================================================

template<typename T>
class RingBuffer {
public:
    explicit RingBuffer(size_t capacity)
        : capacity_(capacity)
        , buffer_(capacity)
        , head_(0)
        , tail_(0) {
    }
    
    // Push item (overwrites oldest if full)
    bool push(const T& item) {
        std::lock_guard<std::mutex> lock(mutex_);
        
        buffer_[head_] = item;
        head_ = (head_ + 1) % capacity_;
        
        if (head_ == tail_) {
            // Buffer full, move tail
            tail_ = (tail_ + 1) % capacity_;
        }
        
        return true;
    }
    
    // Push with move semantics
    bool push(T&& item) {
        std::lock_guard<std::mutex> lock(mutex_);
        
        buffer_[head_] = std::move(item);
        head_ = (head_ + 1) % capacity_;
        
        if (head_ == tail_) {
            tail_ = (tail_ + 1) % capacity_;
        }
        
        return true;
    }
    
    // Pop oldest item
    bool pop(T& item) {
        std::lock_guard<std::mutex> lock(mutex_);
        
        if (head_ == tail_) {
            return false;  // Empty
        }
        
        item = std::move(buffer_[tail_]);
        tail_ = (tail_ + 1) % capacity_;
        
        return true;
    }
    
    // Peek at oldest item without removing
    bool peek(T& item) const {
        std::lock_guard<std::mutex> lock(mutex_);
        
        if (head_ == tail_) {
            return false;
        }
        
        item = buffer_[tail_];
        return true;
    }
    
    // Get all items (oldest first)
    std::vector<T> getAll() const {
        std::lock_guard<std::mutex> lock(mutex_);
        
        std::vector<T> result;
        size_t idx = tail_;
        
        while (idx != head_) {
            result.push_back(buffer_[idx]);
            idx = (idx + 1) % capacity_;
        }
        
        return result;
    }
    
    // Get last N items (newest first)
    std::vector<T> getLast(size_t n) const {
        std::lock_guard<std::mutex> lock(mutex_);
        
        std::vector<T> result;
        size_t count = size();
        n = std::min(n, count);
        
        size_t start = (head_ + capacity_ - n) % capacity_;
        
        for (size_t i = 0; i < n; ++i) {
            result.push_back(buffer_[(start + i) % capacity_]);
        }
        
        return result;
    }
    
    // Clear buffer
    void clear() {
        std::lock_guard<std::mutex> lock(mutex_);
        head_ = 0;
        tail_ = 0;
    }
    
    // Size and capacity
    size_t size() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return (head_ + capacity_ - tail_) % capacity_;
    }
    
    size_t capacity() const { return capacity_; }
    
    bool empty() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return head_ == tail_;
    }
    
    bool full() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return ((head_ + 1) % capacity_) == tail_;
    }
    
private:
    size_t capacity_;
    std::vector<T> buffer_;
    size_t head_;
    size_t tail_;
    mutable std::mutex mutex_;
};

// =============================================================================
// FrameRingBuffer - Specialized for frames with timestamp lookup
// =============================================================================

class FrameRingBuffer {
public:
    explicit FrameRingBuffer(size_t capacity, int fps = 30);
    
    // Push frame
    void push(const Frame& frame);
    
    // Get frames in time range [start_ts, end_ts]
    std::vector<Frame> getRange(Timestamp start_ts, Timestamp end_ts) const;
    
    // Get frames from N seconds ago
    std::vector<Frame> getLastSeconds(int seconds) const;
    
    // Get latest frame
    bool getLatest(Frame& frame) const;
    
    // Clear
    void clear();
    
    // Size
    size_t size() const;
    
private:
    RingBuffer<Frame> buffer_;
    int fps_;
};

} // namespace rivision::pipeline
