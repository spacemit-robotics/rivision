#pragma once

#include "rivision/types.h"
#include <vector>
#include <cstdint>

namespace rivision::utils {

class ColorConverter {
public:
    static void nv12ToRgb(
        const uint8_t* y_plane,
        const uint8_t* uv_plane,
        int width,
        int height,
        int y_stride,
        int uv_stride,
        uint8_t* rgb_out
    );
    
    static void nv12ToBgr(
        const uint8_t* y_plane,
        const uint8_t* uv_plane,
        int width,
        int height,
        int y_stride,
        int uv_stride,
        uint8_t* bgr_out
    );
    
    static void rgbToBgr(
        const uint8_t* rgb,
        int width,
        int height,
        uint8_t* bgr_out
    );
    
    static void bgrToRgb(
        const uint8_t* bgr,
        int width,
        int height,
        uint8_t* rgb_out
    );
    
    static void rgbToNchw(
        const uint8_t* rgb,
        int width,
        int height,
        float* nchw_out,
        float scale = 1.0f / 255.0f
    );
    
    static void letterbox(
        const uint8_t* src,
        int src_width,
        int src_height,
        uint8_t* dst,
        int dst_width,
        int dst_height,
        int channels,
        uint8_t pad_value = 114
    );
    
    static std::pair<float, float> getLetterboxScale(
        int src_width,
        int src_height,
        int dst_width,
        int dst_height
    );
};

} // namespace rivision::utils
