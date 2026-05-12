#pragma once

// SpacemiT MPP Channel Manager
// 管理多路解码/编码通道的分配和释放

#include <mutex>
#include <set>
#include <cstdint>
#include "mpp_common.h"

namespace rivision::spacemit {

// =============================================================================
// ChannelManager - 管理 MPP 通道分配
// =============================================================================
// SpacemiT K3 VPU 资源限制:
// - VDEC: 最多 8 个解码通道 (chn 0-7)
// - VENC: 最多 8 个编码通道 (chn 0-7)
// - 总带宽: 4K@30fps 或 1080p@120fps

class ChannelManager {
public:
    enum class ChannelType {
        DECODER,    // VDEC 通道
        ENCODER,    // VENC 通道
        DEMUX,      // Demux 通道 (使用 VDEC 通道)
        MUX         // Mux 通道 (使用 VENC 通道)
    };
    
    static constexpr int MAX_DECODER_CHANNELS = 8;
    static constexpr int MAX_ENCODER_CHANNELS = 8;
    
    // 单例模式
    static ChannelManager& instance() {
        static ChannelManager inst;
        return inst;
    }
    
    // 分配通道，返回 -1 表示失败
    int32_t allocate(ChannelType type) {
        std::lock_guard<std::mutex> lock(mutex_);
        
        std::set<int32_t>* pool = nullptr;
        int max_channels = 0;
        
        switch (type) {
            case ChannelType::DECODER:
            case ChannelType::DEMUX:
                pool = &decoder_channels_;
                max_channels = MAX_DECODER_CHANNELS;
                break;
            case ChannelType::ENCODER:
            case ChannelType::MUX:
                pool = &encoder_channels_;
                max_channels = MAX_ENCODER_CHANNELS;
                break;
        }
        
        // 找到第一个未使用的通道
        for (int32_t i = 0; i < max_channels; ++i) {
            if (pool->find(i) == pool->end()) {
                pool->insert(i);
                return i;
            }
        }
        
        return -1;  // 无可用通道
    }
    
    // 释放通道
    void release(ChannelType type, int32_t chn_id) {
        if (chn_id < 0) return;
        
        std::lock_guard<std::mutex> lock(mutex_);
        
        switch (type) {
            case ChannelType::DECODER:
            case ChannelType::DEMUX:
                decoder_channels_.erase(chn_id);
                break;
            case ChannelType::ENCODER:
            case ChannelType::MUX:
                encoder_channels_.erase(chn_id);
                break;
        }
    }
    
    // 获取使用中的通道数
    int getActiveCount(ChannelType type) const {
        std::lock_guard<std::mutex> lock(mutex_);
        
        switch (type) {
            case ChannelType::DECODER:
            case ChannelType::DEMUX:
                return static_cast<int>(decoder_channels_.size());
            case ChannelType::ENCODER:
            case ChannelType::MUX:
                return static_cast<int>(encoder_channels_.size());
        }
        return 0;
    }
    
    // 获取可用通道数
    int getAvailableCount(ChannelType type) const {
        std::lock_guard<std::mutex> lock(mutex_);
        
        switch (type) {
            case ChannelType::DECODER:
            case ChannelType::DEMUX:
                return MAX_DECODER_CHANNELS - static_cast<int>(decoder_channels_.size());
            case ChannelType::ENCODER:
            case ChannelType::MUX:
                return MAX_ENCODER_CHANNELS - static_cast<int>(encoder_channels_.size());
        }
        return 0;
    }
    
    // 获取资源使用状态
    struct ResourceStats {
        int decoder_active;
        int decoder_available;
        int encoder_active;
        int encoder_available;
    };
    
    ResourceStats getStats() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return {
            static_cast<int>(decoder_channels_.size()),
            MAX_DECODER_CHANNELS - static_cast<int>(decoder_channels_.size()),
            static_cast<int>(encoder_channels_.size()),
            MAX_ENCODER_CHANNELS - static_cast<int>(encoder_channels_.size())
        };
    }

private:
    ChannelManager() = default;
    ~ChannelManager() = default;
    ChannelManager(const ChannelManager&) = delete;
    ChannelManager& operator=(const ChannelManager&) = delete;
    
    mutable std::mutex mutex_;
    std::set<int32_t> decoder_channels_;
    std::set<int32_t> encoder_channels_;
};

// =============================================================================
// RAII 通道持有者
// =============================================================================

class ChannelHolder {
public:
    ChannelHolder(ChannelManager::ChannelType type)
        : type_(type), chn_id_(ChannelManager::instance().allocate(type)) {}
    
    ~ChannelHolder() {
        release();
    }
    
    // 移动语义
    ChannelHolder(ChannelHolder&& other) noexcept
        : type_(other.type_), chn_id_(other.chn_id_) {
        other.chn_id_ = -1;
    }
    
    ChannelHolder& operator=(ChannelHolder&& other) noexcept {
        if (this != &other) {
            release();
            type_ = other.type_;
            chn_id_ = other.chn_id_;
            other.chn_id_ = -1;
        }
        return *this;
    }
    
    // 禁止拷贝
    ChannelHolder(const ChannelHolder&) = delete;
    ChannelHolder& operator=(const ChannelHolder&) = delete;
    
    int32_t id() const { return chn_id_; }
    bool valid() const { return chn_id_ >= 0; }
    explicit operator bool() const { return valid(); }
    
    void release() {
        if (chn_id_ >= 0) {
            ChannelManager::instance().release(type_, chn_id_);
            chn_id_ = -1;
        }
    }
    
private:
    ChannelManager::ChannelType type_;
    int32_t chn_id_;
};

} // namespace rivision::spacemit
