/*
 * Frame Pool - Zero-copy DMA-BUF frame management
 * P1 Optimization: Eliminates per-frame malloc/free, reduces memory fragmentation
*/

#pragma once

#include "rivision/types.h"

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <cstdint>
#include <memory>
#include <mutex>
#include <string>
#include <vector>

namespace rivision {

// Use PixelFormat from types.h

// DMA-BUF frame structure (zero-copy between VPU/NPU/V2D)
struct DmaBufFrame {
    int fd = -1;                    // DMA-BUF file descriptor
    void* virt_addr = nullptr;      // Virtual address (mmap'd)
    uint64_t phys_addr = 0;         // Physical address (for NPU/VPU direct access)
    size_t size = 0;                // Buffer size in bytes
    
    int width = 0;
    int height = 0;
    int stride = 0;                 // Bytes per row
    PixelFormat format = PixelFormat::NV12;
    
    // Reference counting for multi-consumer
    std::atomic<int> ref_count{0};
    
    // Timestamps
    int64_t pts = 0;                // Presentation timestamp
    int64_t dts = 0;                // Decode timestamp
    uint64_t capture_time_us = 0;   // Capture time (microseconds since epoch)
    
    // Metadata
    std::string stream_id;
    uint64_t frame_id = 0;
    bool keyframe = false;
    
    // Pool management
    int pool_index = -1;            // Index in pool
    
    // Helper methods
    size_t yPlaneSize() const {
        return stride * height;
    }
    
    size_t uvPlaneSize() const {
        if (format == PixelFormat::NV12) {
            return stride * height / 2;
        }
        return 0;
    }
    
    uint8_t* yPlane() const {
        return static_cast<uint8_t*>(virt_addr);
    }
    
    uint8_t* uvPlane() const {
        if (format == PixelFormat::NV12) {
            return static_cast<uint8_t*>(virt_addr) + yPlaneSize();
        }
        return nullptr;
    }
};

// Frame pool configuration
struct FramePoolConfig {
    size_t capacity = 32;           // Number of frames in pool
    int width = 1920;               // Frame width
    int height = 1080;              // Frame height
    PixelFormat format = PixelFormat::NV12;
    bool use_dma_buf = true;        // K3: DMA-BUF for zero-copy
    bool cache_coherent = true;     // Use cached memory mapping
};

// Frame pool statistics
struct FramePoolStats {
    size_t total_frames = 0;
    size_t in_use = 0;
    size_t peak_usage = 0;
    uint64_t total_acquires = 0;
    uint64_t total_releases = 0;
    uint64_t wait_count = 0;        // Times we had to wait for a free frame
    uint64_t timeout_count = 0;     // Times acquire timed out
};

/**
 * @brief High-performance frame pool for zero-copy video processing
 * 
 * Features:
 * - Pre-allocated DMA-BUF frames (no runtime malloc)
 * - Reference counting for multi-consumer pipelines
 * - Thread-safe acquire/release
 * - RAII wrapper (PooledFrame) for automatic release
 */
class FramePool {
public:
    explicit FramePool(const FramePoolConfig& config);
    ~FramePool();
    
    // Non-copyable
    FramePool(const FramePool&) = delete;
    FramePool& operator=(const FramePool&) = delete;
    
    /**
     * @brief Acquire a free frame from the pool
     * @param timeout_ms Maximum wait time in milliseconds (-1 = infinite)
     * @return Pointer to frame, or nullptr on timeout
     * @thread_safety Thread-safe
     */
    DmaBufFrame* acquire(int timeout_ms = 100);
    
    /**
     * @brief Try to acquire a frame without waiting
     * @return Pointer to frame, or nullptr if none available
     * @thread_safety Thread-safe
     */
    DmaBufFrame* tryAcquire();
    
    /**
     * @brief Release a frame back to the pool
     * @param frame Frame to release (must be from this pool)
     * @thread_safety Thread-safe
     */
    void release(DmaBufFrame* frame);
    
    /**
     * @brief Increment reference count (for multi-consumer)
     * @param frame Frame to add reference to
     * @thread_safety Thread-safe
     */
    void addRef(DmaBufFrame* frame);
    
    /**
     * @brief Get pool statistics
     * @thread_safety Thread-safe
     */
    FramePoolStats getStats() const;
    
    /**
     * @brief Get pool configuration
     */
    const FramePoolConfig& config() const { return config_; }
    
    /**
     * @brief Check if pool is initialized
     */
    bool isInitialized() const { return initialized_; }
    
    /**
     * @brief Get number of available frames
     */
    size_t available() const;
    
private:
    bool allocateFrames();
    void freeFrames();
    bool allocateFrame(DmaBufFrame& frame);
    void freeFrame(DmaBufFrame& frame);
    size_t calculateFrameSize() const;
    
    FramePoolConfig config_;
    std::vector<std::unique_ptr<DmaBufFrame>> frames_;
    std::vector<DmaBufFrame*> free_list_;
    
    mutable std::mutex mutex_;
    std::condition_variable cv_;
    
    mutable std::mutex stats_mutex_;
    FramePoolStats stats_;
    
    bool initialized_ = false;
    
#ifdef USE_SPACEMIT_MPP
    // VB pool IDs for SpacemiT
    std::vector<uint64_t> vb_pool_ids_;
    std::vector<uint64_t> vb_buffer_ids_;
#endif
};

/**
 * @brief RAII wrapper for pooled frames
 * 
 * Automatically releases frame back to pool on destruction.
 * Move-only semantics prevent double-release.
 */
class PooledFrame {
public:
    PooledFrame() = default;
    
    PooledFrame(FramePool* pool, DmaBufFrame* frame)
        : pool_(pool), frame_(frame) {}
    
    ~PooledFrame() {
        reset();
    }
    
    // Move-only
    PooledFrame(PooledFrame&& other) noexcept
        : pool_(other.pool_), frame_(other.frame_) {
        other.pool_ = nullptr;
        other.frame_ = nullptr;
    }
    
    PooledFrame& operator=(PooledFrame&& other) noexcept {
        if (this != &other) {
            reset();
            pool_ = other.pool_;
            frame_ = other.frame_;
            other.pool_ = nullptr;
            other.frame_ = nullptr;
        }
        return *this;
    }
    
    // Non-copyable
    PooledFrame(const PooledFrame&) = delete;
    PooledFrame& operator=(const PooledFrame&) = delete;
    
    // Access
    DmaBufFrame* get() const { return frame_; }
    DmaBufFrame* operator->() const { return frame_; }
    DmaBufFrame& operator*() const { return *frame_; }
    explicit operator bool() const { return frame_ != nullptr; }
    
    // Release ownership without returning to pool
    DmaBufFrame* release() {
        DmaBufFrame* f = frame_;
        pool_ = nullptr;
        frame_ = nullptr;
        return f;
    }
    
    // Return to pool and clear
    void reset() {
        if (pool_ && frame_) {
            pool_->release(frame_);
        }
        pool_ = nullptr;
        frame_ = nullptr;
    }
    
private:
    FramePool* pool_ = nullptr;
    DmaBufFrame* frame_ = nullptr;
};

// Helper function to calculate frame size
inline size_t calculateFrameSize(int width, int height, PixelFormat format) {
    switch (format) {
        case PixelFormat::NV12:
        case PixelFormat::YUV420P:
            return width * height * 3 / 2;
        case PixelFormat::RGB24:
        case PixelFormat::BGR24:
            return width * height * 3;
        case PixelFormat::RGBA:
        case PixelFormat::BGRA:
            return width * height * 4;
        case PixelFormat::GRAY:
            return width * height;
        default:
            return width * height * 3 / 2;
    }
}

} // namespace rivision
