/*
 * Frame Pool Implementation: Zero-copy DMA-BUF frame management
*/

#include "frame_pool.h"
#include "utils/logger.h"

#include <cstring>
#include <algorithm>

#ifdef USE_SPACEMIT_MPP
extern "C" {
#include "sys_api.h"
#include "vb_api.h"
}
#endif

namespace rivision {

FramePool::FramePool(const FramePoolConfig& config)
    : config_(config) {
    
    if (!allocateFrames()) {
        LOG_ERROR("FramePool: Failed to allocate {} frames", config_.capacity);
        return;
    }
    
    initialized_ = true;
    stats_.total_frames = config_.capacity;
    
    LOG_INFO("FramePool: Initialized with {} frames ({}x{} {})",
             config_.capacity, config_.width, config_.height,
             config_.use_dma_buf ? "DMA-BUF" : "malloc");
}

FramePool::~FramePool() {
    freeFrames();
}

bool FramePool::allocateFrames() {
    frames_.reserve(config_.capacity);
    free_list_.reserve(config_.capacity);
    
#ifdef USE_SPACEMIT_MPP
    vb_pool_ids_.reserve(config_.capacity);
    vb_buffer_ids_.reserve(config_.capacity);
#endif
    
    for (size_t i = 0; i < config_.capacity; ++i) {
        auto frame = std::make_unique<DmaBufFrame>();
        frame->pool_index = static_cast<int>(i);
        
        if (!allocateFrame(*frame)) {
            LOG_ERROR("FramePool: Failed to allocate frame {}", i);
            freeFrames();
            return false;
        }
        
        free_list_.push_back(frame.get());
        frames_.push_back(std::move(frame));
    }
    
    return true;
}

void FramePool::freeFrames() {
    // Wait for all frames to be released
    {
        std::unique_lock<std::mutex> lock(mutex_);
        // Give warning if frames still in use
        if (free_list_.size() < frames_.size()) {
            LOG_WARN("FramePool: {} frames still in use during destruction",
                     frames_.size() - free_list_.size());
        }
    }
    
    for (auto& frame : frames_) {
        if (frame) {
            freeFrame(*frame);
        }
    }
    
    frames_.clear();
    free_list_.clear();
    
#ifdef USE_SPACEMIT_MPP
    // Destroy VB pools
    for (auto pool_id : vb_pool_ids_) {
        if (pool_id != 0) {
            VB_DestroyPool(pool_id);
        }
    }
    vb_pool_ids_.clear();
    vb_buffer_ids_.clear();
#endif
}

size_t FramePool::calculateFrameSize() const {
    return rivision::calculateFrameSize(config_.width, config_.height, config_.format);
}

bool FramePool::allocateFrame(DmaBufFrame& frame) {
    frame.width = config_.width;
    frame.height = config_.height;
    frame.format = config_.format;
    frame.stride = config_.width;  // Assume no padding for now
    frame.size = calculateFrameSize();
    
#ifdef USE_SPACEMIT_MPP
    if (config_.use_dma_buf) {
        // Use SpacemiT VB (Video Buffer) API for DMA-BUF allocation
        VbPoolCfg cfg = {};
        cfg.u32BufCnt = 1;
        cfg.u32BufSize = static_cast<uint32_t>(frame.size);
        cfg.eModId = MPP_ID_SYS;
        cfg.eRemapMode = config_.cache_coherent ? 
            VBUF_REMAP_MODE_CACHED : VBUF_REMAP_MODE_NOCACHE;
        
        uint64_t pool_id = VB_CreatePool(&cfg);
        if (pool_id == 0) {
            LOG_ERROR("FramePool: VB_CreatePool failed");
            return false;
        }
        
        uint64_t buffer = VB_GetBuffer(pool_id, 0);
        if (buffer == 0) {
            LOG_ERROR("FramePool: VB_GetBuffer failed");
            VB_DestroyPool(pool_id);
            return false;
        }
        
        // Get DMA-BUF fd
        int fd = -1;
        if (VB_GetDmaBufFd(buffer, &fd) != 0 || fd < 0) {
            LOG_ERROR("FramePool: VB_GetDmaBufFd failed");
            VB_ReleaseBuffer(buffer);
            VB_DestroyPool(pool_id);
            return false;
        }
        frame.fd = fd;
        
        // Get virtual address
        void* virt_addr = nullptr;
        if (VB_GetVirAddr(buffer, &virt_addr) != 0 || virt_addr == nullptr) {
            LOG_ERROR("FramePool: VB_GetVirAddr failed");
            VB_ReleaseBuffer(buffer);
            VB_DestroyPool(pool_id);
            return false;
        }
        frame.virt_addr = virt_addr;
        
        // Get physical address (for NPU/VPU direct access)
        uint64_t phys_addr = 0;
        if (VB_GetPhyAddr(buffer, &phys_addr) == 0) {
            frame.phys_addr = phys_addr;
        }
        
        vb_pool_ids_.push_back(pool_id);
        vb_buffer_ids_.push_back(buffer);
        
        return true;
    }
#endif
    
    // Fallback: use aligned malloc
    void* ptr = nullptr;
    int ret = posix_memalign(&ptr, 64, frame.size);  // 64-byte alignment for SIMD
    if (ret != 0 || ptr == nullptr) {
        LOG_ERROR("FramePool: posix_memalign failed");
        return false;
    }
    
    frame.virt_addr = ptr;
    frame.fd = -1;
    frame.phys_addr = 0;
    
    return true;
}

void FramePool::freeFrame(DmaBufFrame& frame) {
#ifdef USE_SPACEMIT_MPP
    if (config_.use_dma_buf && frame.pool_index >= 0) {
        size_t idx = static_cast<size_t>(frame.pool_index);
        if (idx < vb_buffer_ids_.size() && vb_buffer_ids_[idx] != 0) {
            VB_ReleaseBuffer(vb_buffer_ids_[idx]);
            vb_buffer_ids_[idx] = 0;
        }
        // Pool destruction is handled in freeFrames()
    } else
#endif
    {
        if (frame.virt_addr != nullptr) {
            free(frame.virt_addr);
        }
    }
    
    frame.fd = -1;
    frame.virt_addr = nullptr;
    frame.phys_addr = 0;
}

DmaBufFrame* FramePool::acquire(int timeout_ms) {
    std::unique_lock<std::mutex> lock(mutex_);
    
    if (free_list_.empty()) {
        // Need to wait
        {
            std::lock_guard<std::mutex> slock(stats_mutex_);
            stats_.wait_count++;
        }
        
        if (timeout_ms < 0) {
            // Infinite wait
            cv_.wait(lock, [this] { return !free_list_.empty(); });
        } else if (timeout_ms == 0) {
            // No wait
            std::lock_guard<std::mutex> slock(stats_mutex_);
            stats_.timeout_count++;
            return nullptr;
        } else {
            // Timed wait
            auto deadline = std::chrono::steady_clock::now() + 
                           std::chrono::milliseconds(timeout_ms);
            if (!cv_.wait_until(lock, deadline, [this] { return !free_list_.empty(); })) {
                std::lock_guard<std::mutex> slock(stats_mutex_);
                stats_.timeout_count++;
                return nullptr;
            }
        }
    }
    
    if (free_list_.empty()) {
        return nullptr;
    }
    
    DmaBufFrame* frame = free_list_.back();
    free_list_.pop_back();
    
    // Initialize reference count
    frame->ref_count.store(1, std::memory_order_relaxed);
    
    // Clear metadata
    frame->pts = 0;
    frame->dts = 0;
    frame->capture_time_us = 0;
    frame->stream_id.clear();
    frame->frame_id = 0;
    frame->keyframe = false;
    
    // Update stats
    {
        std::lock_guard<std::mutex> slock(stats_mutex_);
        stats_.total_acquires++;
        stats_.in_use = frames_.size() - free_list_.size();
        if (stats_.in_use > stats_.peak_usage) {
            stats_.peak_usage = stats_.in_use;
        }
    }
    
    return frame;
}

DmaBufFrame* FramePool::tryAcquire() {
    return acquire(0);
}

void FramePool::release(DmaBufFrame* frame) {
    if (frame == nullptr) {
        return;
    }
    
    // Decrement reference count
    int prev_count = frame->ref_count.fetch_sub(1, std::memory_order_acq_rel);
    
    if (prev_count > 1) {
        // Still has references, don't return to pool yet
        return;
    }
    
    if (prev_count <= 0) {
        LOG_WARN("FramePool: Double release detected for frame {}", frame->pool_index);
        return;
    }
    
    // Return to pool
    {
        std::lock_guard<std::mutex> lock(mutex_);
        free_list_.push_back(frame);
        
        std::lock_guard<std::mutex> slock(stats_mutex_);
        stats_.total_releases++;
        stats_.in_use = frames_.size() - free_list_.size();
    }
    
    cv_.notify_one();
}

void FramePool::addRef(DmaBufFrame* frame) {
    if (frame == nullptr) {
        return;
    }
    frame->ref_count.fetch_add(1, std::memory_order_relaxed);
}

FramePoolStats FramePool::getStats() const {
    std::lock_guard<std::mutex> lock(stats_mutex_);
    return stats_;
}

size_t FramePool::available() const {
    std::lock_guard<std::mutex> lock(mutex_);
    return free_list_.size();
}

} // namespace rivision
