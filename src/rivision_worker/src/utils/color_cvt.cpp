#include "color_cvt.h"
#include <algorithm>
#include <cmath>

namespace rivision::utils {

static inline uint8_t clamp_u8(int v) {
    return static_cast<uint8_t>(std::clamp(v, 0, 255));
}

void ColorConverter::nv12ToRgb(
    const uint8_t* y_plane,
    const uint8_t* uv_plane,
    int width,
    int height,
    int y_stride,
    int uv_stride,
    uint8_t* rgb_out
) {
    for (int y = 0; y < height; y++) {
        const uint8_t* y_row = y_plane + y * y_stride;
        const uint8_t* uv_row = uv_plane + (y / 2) * uv_stride;
        uint8_t* rgb_row = rgb_out + y * width * 3;
        
        for (int x = 0; x < width; x++) {
            int Y = y_row[x];
            int U = uv_row[(x / 2) * 2] - 128;
            int V = uv_row[(x / 2) * 2 + 1] - 128;
            
            int R = Y + ((359 * V) >> 8);
            int G = Y - ((88 * U + 183 * V) >> 8);
            int B = Y + ((454 * U) >> 8);
            
            rgb_row[x * 3 + 0] = clamp_u8(R);
            rgb_row[x * 3 + 1] = clamp_u8(G);
            rgb_row[x * 3 + 2] = clamp_u8(B);
        }
    }
}

void ColorConverter::nv12ToBgr(
    const uint8_t* y_plane,
    const uint8_t* uv_plane,
    int width,
    int height,
    int y_stride,
    int uv_stride,
    uint8_t* bgr_out
) {
    for (int y = 0; y < height; y++) {
        const uint8_t* y_row = y_plane + y * y_stride;
        const uint8_t* uv_row = uv_plane + (y / 2) * uv_stride;
        uint8_t* bgr_row = bgr_out + y * width * 3;
        
        for (int x = 0; x < width; x++) {
            int Y = y_row[x];
            int U = uv_row[(x / 2) * 2] - 128;
            int V = uv_row[(x / 2) * 2 + 1] - 128;
            
            int R = Y + ((359 * V) >> 8);
            int G = Y - ((88 * U + 183 * V) >> 8);
            int B = Y + ((454 * U) >> 8);
            
            bgr_row[x * 3 + 0] = clamp_u8(B);
            bgr_row[x * 3 + 1] = clamp_u8(G);
            bgr_row[x * 3 + 2] = clamp_u8(R);
        }
    }
}

void ColorConverter::rgbToBgr(
    const uint8_t* rgb,
    int width,
    int height,
    uint8_t* bgr_out
) {
    int total = width * height;
    for (int i = 0; i < total; i++) {
        bgr_out[i * 3 + 0] = rgb[i * 3 + 2];
        bgr_out[i * 3 + 1] = rgb[i * 3 + 1];
        bgr_out[i * 3 + 2] = rgb[i * 3 + 0];
    }
}

void ColorConverter::bgrToRgb(
    const uint8_t* bgr,
    int width,
    int height,
    uint8_t* rgb_out
) {
    rgbToBgr(bgr, width, height, rgb_out);  // Same operation
}

void ColorConverter::rgbToNchw(
    const uint8_t* rgb,
    int width,
    int height,
    float* nchw_out,
    float scale
) {
    int hw = width * height;
    float* r_plane = nchw_out;
    float* g_plane = nchw_out + hw;
    float* b_plane = nchw_out + hw * 2;
    
    for (int i = 0; i < hw; i++) {
        r_plane[i] = rgb[i * 3 + 0] * scale;
        g_plane[i] = rgb[i * 3 + 1] * scale;
        b_plane[i] = rgb[i * 3 + 2] * scale;
    }
}

void ColorConverter::letterbox(
    const uint8_t* src,
    int src_width,
    int src_height,
    uint8_t* dst,
    int dst_width,
    int dst_height,
    int channels,
    uint8_t pad_value
) {
    std::fill(dst, dst + dst_width * dst_height * channels, pad_value);
    
    auto [scale, _] = getLetterboxScale(src_width, src_height, dst_width, dst_height);
    
    int new_width = static_cast<int>(src_width * scale);
    int new_height = static_cast<int>(src_height * scale);
    int offset_x = (dst_width - new_width) / 2;
    int offset_y = (dst_height - new_height) / 2;
    
    for (int y = 0; y < new_height; y++) {
        int src_y = static_cast<int>(y / scale);
        src_y = std::min(src_y, src_height - 1);
        
        for (int x = 0; x < new_width; x++) {
            int src_x = static_cast<int>(x / scale);
            src_x = std::min(src_x, src_width - 1);
            
            int dst_idx = ((offset_y + y) * dst_width + (offset_x + x)) * channels;
            int src_idx = (src_y * src_width + src_x) * channels;
            
            for (int c = 0; c < channels; c++) {
                dst[dst_idx + c] = src[src_idx + c];
            }
        }
    }
}

std::pair<float, float> ColorConverter::getLetterboxScale(
    int src_width,
    int src_height,
    int dst_width,
    int dst_height
) {
    float scale_w = static_cast<float>(dst_width) / src_width;
    float scale_h = static_cast<float>(dst_height) / src_height;
    float scale = std::min(scale_w, scale_h);
    
    float pad_w = (dst_width - src_width * scale) / 2.0f;
    float pad_h = (dst_height - src_height * scale) / 2.0f;
    
    return {scale, std::max(pad_w, pad_h)};
}

} // namespace rivision::utils
