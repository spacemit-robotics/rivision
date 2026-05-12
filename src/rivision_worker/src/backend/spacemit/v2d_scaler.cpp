/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *------------------------------------------------------------------------------
 */

#include "v2d_scaler.h"
#include <algorithm>
#include <cstring>

#ifdef USE_SPACEMIT_MPP
#include "v2d_api.h"
#endif

namespace rivision::spacemit {

class V2dScaler::Impl {
public:
#ifdef USE_SPACEMIT_MPP
    V2D_HANDLE v2d_handle_ = nullptr;
    V2D_CONFIG v2d_config_ = {};
#endif
    VbBuffer src_vb_;
    VbBuffer dst_vb_;
    bool configured_ = false;
};

V2dScaler::V2dScaler()
    : impl_(std::make_unique<Impl>())
{
}

V2dScaler::~V2dScaler() {
#ifdef USE_SPACEMIT_MPP
    if (impl_->v2d_handle_) {
        V2D_Destroy(impl_->v2d_handle_);
    }
#endif
}

bool V2dScaler::configure(const Config& cfg) {
    config_ = cfg;
    
    if (cfg.src_width <= 0 || cfg.src_height <= 0 ||
        cfg.dst_width <= 0 || cfg.dst_height <= 0) {
        last_error_ = "Invalid dimensions";
        return false;
    }
    
    ratio_x_ = static_cast<float>(cfg.dst_width) / cfg.src_width;
    ratio_y_ = static_cast<float>(cfg.dst_height) / cfg.src_height;
    
#ifdef USE_SPACEMIT_MPP
    V2D_CONFIG v2d_cfg = {};
    v2d_cfg.src_width = cfg.src_width;
    v2d_cfg.src_height = cfg.src_height;
    v2d_cfg.src_format = (cfg.src_format == hal::PixelFormat::NV12) ? 
                          V2D_FORMAT_NV12 : V2D_FORMAT_RGB24;
    v2d_cfg.dst_width = cfg.dst_width;
    v2d_cfg.dst_height = cfg.dst_height;
    v2d_cfg.dst_format = (cfg.dst_format == hal::PixelFormat::NV12) ?
                          V2D_FORMAT_NV12 : V2D_FORMAT_RGB24;
    
    switch (cfg.interp) {
        case Config::Interpolation::NEAREST:
            v2d_cfg.interp_mode = V2D_INTERP_NEAREST;
            break;
        case Config::Interpolation::BILINEAR:
            v2d_cfg.interp_mode = V2D_INTERP_BILINEAR;
            break;
        case Config::Interpolation::BICUBIC:
            v2d_cfg.interp_mode = V2D_INTERP_BICUBIC;
            break;
    }
    
    impl_->v2d_config_ = v2d_cfg;
    
    int ret = V2D_Create(&impl_->v2d_handle_, &v2d_cfg);
    if (ret != 0) {
        last_error_ = "V2D_Create failed: " + std::to_string(ret);
        return false;
    }
    
    impl_->configured_ = true;
#else
    impl_->configured_ = true;
#endif
    
    return true;
}

bool V2dScaler::scale(uint64_t src_phys_addr, uint64_t dst_phys_addr) {
    if (!impl_->configured_) {
        last_error_ = "Not configured";
        return false;
    }
    
#ifdef USE_SPACEMIT_MPP
    V2D_TASK task = {};
    task.src_phys_addr = src_phys_addr;
    task.dst_phys_addr = dst_phys_addr;
    
    int ret = V2D_Submit(impl_->v2d_handle_, &task);
    if (ret != 0) {
        last_error_ = "V2D_Submit failed: " + std::to_string(ret);
        return false;
    }
    
    ret = V2D_Wait(impl_->v2d_handle_, 1000);  // 1s timeout
    if (ret != 0) {
        last_error_ = "V2D_Wait failed: " + std::to_string(ret);
        return false;
    }
    
    return true;
#else
    (void)src_phys_addr;
    (void)dst_phys_addr;
    last_error_ = "V2D not supported (USE_SPACEMIT_MPP not defined)";
    return false;
#endif
}

bool V2dScaler::scale(const hal::Frame& src, hal::Frame& dst) {
    if (!impl_->configured_) {
        last_error_ = "Not configured";
        return false;
    }
    
    // Allocate destination if needed
    dst.width = config_.dst_width;
    dst.height = config_.dst_height;
    dst.format = config_.dst_format;
    
    size_t dst_size = dst.width * dst.height;
    if (dst.format == hal::PixelFormat::NV12) {
        dst_size = dst_size * 3 / 2;
    } else if (dst.format == hal::PixelFormat::RGB24) {
        dst_size = dst_size * 3;
    }
    
#ifdef USE_SPACEMIT_MPP
    // Allocate VB buffers
    VbBuffer src_vb = allocVbBuffer(src.data.size());
    VbBuffer dst_vb = allocVbBuffer(dst_size);
    
    // Copy source to VB
    std::memcpy(src_vb.virt_addr, src.data.data(), src.data.size());
    
    // Scale
    bool result = scale(src_vb.phys_addr, dst_vb.phys_addr);
    
    if (result) {
        dst.data.resize(dst_size);
        std::memcpy(dst.data.data(), dst_vb.virt_addr, dst_size);
    }
    
    freeVbBuffer(src_vb);
    freeVbBuffer(dst_vb);
    
    return result;
#else
    // CPU fallback using simple bilinear
    dst.data.resize(dst_size);
    
    // Simplified NV12 scaling (Y plane only for demo)
    for (int y = 0; y < dst.height; y++) {
        for (int x = 0; x < dst.width; x++) {
            float src_x = x / ratio_x_;
            float src_y = y / ratio_y_;
            
            int sx = std::min(static_cast<int>(src_x), src.width - 1);
            int sy = std::min(static_cast<int>(src_y), src.height - 1);
            
            dst.data[y * dst.width + x] = src.data[sy * src.width + sx];
        }
    }
    
    // UV plane
    int src_uv_offset = src.width * src.height;
    int dst_uv_offset = dst.width * dst.height;
    
    for (int y = 0; y < dst.height / 2; y++) {
        for (int x = 0; x < dst.width; x += 2) {
            float src_x = x / ratio_x_;
            float src_y = y / ratio_y_;
            
            int sx = std::min(static_cast<int>(src_x) & ~1, src.width - 2);
            int sy = std::min(static_cast<int>(src_y), src.height / 2 - 1);
            
            int src_idx = src_uv_offset + sy * src.width + sx;
            int dst_idx = dst_uv_offset + y * dst.width + x;
            
            if (src_idx + 1 < static_cast<int>(src.data.size()) && 
                dst_idx + 1 < static_cast<int>(dst.data.size())) {
                dst.data[dst_idx] = src.data[src_idx];
                dst.data[dst_idx + 1] = src.data[src_idx + 1];
            }
        }
    }
    
    return true;
#endif
}

bool V2dScaler::scaleBatch(const std::vector<uint64_t>& src_addrs,
                           const std::vector<uint64_t>& dst_addrs) {
    if (src_addrs.size() != dst_addrs.size()) {
        last_error_ = "Batch size mismatch";
        return false;
    }
    
    for (size_t i = 0; i < src_addrs.size(); i++) {
        if (!scale(src_addrs[i], dst_addrs[i])) {
            return false;
        }
    }
    
    return true;
}

bool V2dScaler::scaleLetterbox(const hal::Frame& src, hal::Frame& dst,
                                int target_width, int target_height) {
    // Calculate letterbox dimensions
    float scale = std::min(
        static_cast<float>(target_width) / src.width,
        static_cast<float>(target_height) / src.height
    );
    
    int scaled_width = static_cast<int>(src.width * scale);
    int scaled_height = static_cast<int>(src.height * scale);
    
    letterbox_offset_x_ = (target_width - scaled_width) / 2;
    letterbox_offset_y_ = (target_height - scaled_height) / 2;
    
    ratio_x_ = scale;
    ratio_y_ = scale;
    
    // Configure for scaled dimensions
    Config cfg = config_;
    cfg.dst_width = scaled_width;
    cfg.dst_height = scaled_height;
    cfg.letterbox = true;
    cfg.pad_color = 114;  // YOLO gray padding
    
    hal::Frame scaled;
    if (!configure(cfg) || !scale(src, scaled)) {
        return false;
    }
    
    // Create target with padding
    dst.width = target_width;
    dst.height = target_height;
    dst.format = config_.dst_format;
    
    size_t dst_size = target_width * target_height;
    if (dst.format == hal::PixelFormat::NV12) {
        dst_size = dst_size * 3 / 2;
    }
    
    dst.data.resize(dst_size);
    std::fill(dst.data.begin(), dst.data.end(), 114);  // Gray padding
    
    // Copy scaled frame to center
    for (int y = 0; y < scaled_height; y++) {
        int src_row = y * scaled_width;
        int dst_row = (y + letterbox_offset_y_) * target_width + letterbox_offset_x_;
        std::memcpy(&dst.data[dst_row], &scaled.data[src_row], scaled_width);
    }
    
    return true;
}

void V2dScaler::getScaleRatio(float& ratio_x, float& ratio_y) const {
    ratio_x = ratio_x_;
    ratio_y = ratio_y_;
}

void V2dScaler::getLetterboxOffset(int& offset_x, int& offset_y) const {
    offset_x = letterbox_offset_x_;
    offset_y = letterbox_offset_y_;
}

} // namespace rivision::spacemit
