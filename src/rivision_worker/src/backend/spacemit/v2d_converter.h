/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *
 * @File      :    v2d_converter.h
 * @Brief     :    V2D hardware accelerated color space converter.
 *
 * Optimization: Use V2D for NV12↔RGB conversion instead of CPU.
 *------------------------------------------------------------------------------
 */

#pragma once

#include "hal/interface/i_graphics.h"
#include <memory>
#include <string>

namespace rivision::spacemit {

class V2dConverter {
public:
    enum class Conversion {
        NV12_TO_RGB24,
        NV12_TO_BGR24,
        RGB24_TO_NV12,
        BGR24_TO_NV12,
        NV21_TO_NV12,
        YUYV_TO_NV12,
        UYVY_TO_NV12,
        NV12_TO_RGBA32,
        RGBA32_TO_NV12
    };
    
    V2dConverter();
    ~V2dConverter();
    
    bool convert(const hal::Frame& src, hal::Frame& dst, Conversion conv);
    
    bool convert(uint64_t src_phys, uint64_t dst_phys,
                 int width, int height, Conversion conv);
    
    // Batch conversion
    bool convertBatch(const std::vector<hal::Frame>& src,
                      std::vector<hal::Frame>& dst,
                      Conversion conv);
    
    const std::string& getLastError() const { return last_error_; }

private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    std::string last_error_;
    
    size_t getFrameSize(int width, int height, Conversion conv, bool is_src);
    hal::PixelFormat getOutputFormat(Conversion conv);
};

} // namespace rivision::spacemit
