/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *------------------------------------------------------------------------------
 */

#include "v2d_converter.h"
#include "mpp_common.h"
#include <cstring>

#ifdef USE_SPACEMIT_MPP
#include "v2d_api.h"
#endif

namespace rivision::spacemit {

class V2dConverter::Impl {
public:
#ifdef USE_SPACEMIT_MPP
    V2D_HANDLE v2d_handle_ = nullptr;
#endif
};

V2dConverter::V2dConverter()
    : impl_(std::make_unique<Impl>())
{
}

V2dConverter::~V2dConverter() {
#ifdef USE_SPACEMIT_MPP
    if (impl_->v2d_handle_) {
        V2D_Destroy(impl_->v2d_handle_);
    }
#endif
}

size_t V2dConverter::getFrameSize(int width, int height, Conversion conv, bool is_src) {
    size_t pixels = width * height;
    
    if (is_src) {
        switch (conv) {
            case Conversion::NV12_TO_RGB24:
            case Conversion::NV12_TO_BGR24:
            case Conversion::NV12_TO_RGBA32:
            case Conversion::NV21_TO_NV12:
                return pixels * 3 / 2;  // NV12/NV21
            case Conversion::RGB24_TO_NV12:
            case Conversion::BGR24_TO_NV12:
                return pixels * 3;      // RGB24/BGR24
            case Conversion::RGBA32_TO_NV12:
                return pixels * 4;      // RGBA32
            case Conversion::YUYV_TO_NV12:
            case Conversion::UYVY_TO_NV12:
                return pixels * 2;      // YUYV/UYVY
            default:
                return pixels * 3 / 2;
        }
    } else {
        switch (conv) {
            case Conversion::NV12_TO_RGB24:
            case Conversion::NV12_TO_BGR24:
                return pixels * 3;      // RGB24/BGR24
            case Conversion::NV12_TO_RGBA32:
                return pixels * 4;      // RGBA32
            case Conversion::RGB24_TO_NV12:
            case Conversion::BGR24_TO_NV12:
            case Conversion::NV21_TO_NV12:
            case Conversion::YUYV_TO_NV12:
            case Conversion::UYVY_TO_NV12:
            case Conversion::RGBA32_TO_NV12:
                return pixels * 3 / 2;  // NV12
            default:
                return pixels * 3 / 2;
        }
    }
}

hal::PixelFormat V2dConverter::getOutputFormat(Conversion conv) {
    switch (conv) {
        case Conversion::NV12_TO_RGB24:
        case Conversion::NV12_TO_BGR24:
            return hal::PixelFormat::RGB24;
        case Conversion::NV12_TO_RGBA32:
            return hal::PixelFormat::RGBA32;
        default:
            return hal::PixelFormat::NV12;
    }
}

bool V2dConverter::convert(const hal::Frame& src, hal::Frame& dst, Conversion conv) {
    if (src.data.empty()) {
        last_error_ = "Empty source frame";
        return false;
    }
    
    dst.width = src.width;
    dst.height = src.height;
    dst.format = getOutputFormat(conv);
    dst.timestamp = src.timestamp;
    
    size_t dst_size = getFrameSize(src.width, src.height, conv, false);
    dst.data.resize(dst_size);
    
#ifdef USE_SPACEMIT_MPP
    VbBuffer src_vb = allocVbBuffer(src.data.size());
    VbBuffer dst_vb = allocVbBuffer(dst_size);
    
    std::memcpy(src_vb.virt_addr, src.data.data(), src.data.size());
    
    V2D_CSC_CONFIG csc_cfg = {};
    csc_cfg.src_width = src.width;
    csc_cfg.src_height = src.height;
    csc_cfg.src_phys_addr = src_vb.phys_addr;
    csc_cfg.dst_phys_addr = dst_vb.phys_addr;
    
    switch (conv) {
        case Conversion::NV12_TO_RGB24:
            csc_cfg.src_format = V2D_FORMAT_NV12;
            csc_cfg.dst_format = V2D_FORMAT_RGB24;
            break;
        case Conversion::NV12_TO_BGR24:
            csc_cfg.src_format = V2D_FORMAT_NV12;
            csc_cfg.dst_format = V2D_FORMAT_BGR24;
            break;
        case Conversion::RGB24_TO_NV12:
            csc_cfg.src_format = V2D_FORMAT_RGB24;
            csc_cfg.dst_format = V2D_FORMAT_NV12;
            break;
        default:
            freeVbBuffer(src_vb);
            freeVbBuffer(dst_vb);
            last_error_ = "Unsupported conversion";
            return false;
    }
    
    int ret = V2D_ColorSpaceConvert(&csc_cfg);
    if (ret != 0) {
        freeVbBuffer(src_vb);
        freeVbBuffer(dst_vb);
        last_error_ = "V2D_ColorSpaceConvert failed: " + std::to_string(ret);
        return false;
    }
    
    std::memcpy(dst.data.data(), dst_vb.virt_addr, dst_size);
    
    freeVbBuffer(src_vb);
    freeVbBuffer(dst_vb);
    
    return true;
#else
    // CPU fallback for NV12 to RGB24
    if (conv == Conversion::NV12_TO_RGB24 || conv == Conversion::NV12_TO_BGR24) {
        const uint8_t* y_plane = src.data.data();
        const uint8_t* uv_plane = src.data.data() + src.width * src.height;
        uint8_t* rgb = dst.data.data();
        
        bool bgr = (conv == Conversion::NV12_TO_BGR24);
        
        for (int j = 0; j < src.height; j++) {
            for (int i = 0; i < src.width; i++) {
                int y = y_plane[j * src.width + i];
                int uv_idx = (j / 2) * src.width + (i & ~1);
                int u = uv_plane[uv_idx] - 128;
                int v = uv_plane[uv_idx + 1] - 128;
                
                int r = y + ((359 * v) >> 8);
                int g = y - ((88 * u + 183 * v) >> 8);
                int b = y + ((454 * u) >> 8);
                
                r = std::max(0, std::min(255, r));
                g = std::max(0, std::min(255, g));
                b = std::max(0, std::min(255, b));
                
                int idx = (j * src.width + i) * 3;
                if (bgr) {
                    rgb[idx] = b;
                    rgb[idx + 1] = g;
                    rgb[idx + 2] = r;
                } else {
                    rgb[idx] = r;
                    rgb[idx + 1] = g;
                    rgb[idx + 2] = b;
                }
            }
        }
        return true;
    }
    
    last_error_ = "Conversion not implemented in CPU fallback";
    return false;
#endif
}

bool V2dConverter::convert(uint64_t src_phys, uint64_t dst_phys,
                           int width, int height, Conversion conv) {
#ifdef USE_SPACEMIT_MPP
    V2D_CSC_CONFIG csc_cfg = {};
    csc_cfg.src_width = width;
    csc_cfg.src_height = height;
    csc_cfg.src_phys_addr = src_phys;
    csc_cfg.dst_phys_addr = dst_phys;
    
    // Set formats based on conversion type
    switch (conv) {
        case Conversion::NV12_TO_RGB24:
            csc_cfg.src_format = V2D_FORMAT_NV12;
            csc_cfg.dst_format = V2D_FORMAT_RGB24;
            break;
        // ... other cases
        default:
            last_error_ = "Unsupported conversion";
            return false;
    }
    
    int ret = V2D_ColorSpaceConvert(&csc_cfg);
    if (ret != 0) {
        last_error_ = "V2D_ColorSpaceConvert failed";
        return false;
    }
    return true;
#else
    (void)src_phys; (void)dst_phys; (void)width; (void)height; (void)conv;
    last_error_ = "V2D not available";
    return false;
#endif
}

bool V2dConverter::convertBatch(const std::vector<hal::Frame>& src,
                                std::vector<hal::Frame>& dst,
                                Conversion conv) {
    dst.resize(src.size());
    for (size_t i = 0; i < src.size(); i++) {
        if (!convert(src[i], dst[i], conv)) {
            return false;
        }
    }
    return true;
}

} // namespace rivision::spacemit
