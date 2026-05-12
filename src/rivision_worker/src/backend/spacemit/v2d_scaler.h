/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *
 * @File      :    v2d_scaler.h
 * @Brief     :    V2D hardware accelerated scaler for SpacemiT K3.
 *
 * Optimization: Use V2D 2D engine for zero-copy scaling instead of CPU.
 * Benefit: ~90% CPU reduction for transcoding workloads.
 *------------------------------------------------------------------------------
 */

#pragma once

#include "hal/interface/i_graphics.h"
#include "mpp_common.h"
#include <memory>

namespace rivision::spacemit {

class V2dScaler {
public:
    struct Config {
        int src_width = 0;
        int src_height = 0;
        int dst_width = 0;
        int dst_height = 0;
        hal::PixelFormat src_format = hal::PixelFormat::NV12;
        hal::PixelFormat dst_format = hal::PixelFormat::NV12;
        
        enum class Interpolation {
            NEAREST,    // Fastest, lowest quality
            BILINEAR,   // Good balance (default)
            BICUBIC     // Best quality, slower
        };
        Interpolation interp = Interpolation::BILINEAR;
        
        bool letterbox = false;     // Preserve aspect ratio with padding
        uint32_t pad_color = 0;     // Padding color (YUV or RGB)
    };
    
    V2dScaler();
    ~V2dScaler();
    
    bool configure(const Config& cfg);
    
    // Scale using physical addresses (zero-copy path)
    bool scale(uint64_t src_phys_addr, uint64_t dst_phys_addr);
    
    // Scale using Frame objects
    bool scale(const hal::Frame& src, hal::Frame& dst);
    
    // Batch scaling for pipeline optimization
    bool scaleBatch(const std::vector<uint64_t>& src_addrs,
                    const std::vector<uint64_t>& dst_addrs);
    
    // Letterbox scaling (for YOLO input preprocessing)
    bool scaleLetterbox(const hal::Frame& src, hal::Frame& dst,
                        int target_width, int target_height);
    
    // Get scale ratios for coordinate conversion
    void getScaleRatio(float& ratio_x, float& ratio_y) const;
    void getLetterboxOffset(int& offset_x, int& offset_y) const;
    
    const std::string& getLastError() const { return last_error_; }

private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    Config config_;
    std::string last_error_;
    
    float ratio_x_ = 1.0f;
    float ratio_y_ = 1.0f;
    int letterbox_offset_x_ = 0;
    int letterbox_offset_y_ = 0;
};

} // namespace rivision::spacemit
