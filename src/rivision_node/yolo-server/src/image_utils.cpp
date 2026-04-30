/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "image_utils.h"

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <stdexcept>
#include <string>
#include <vector>

#define STB_IMAGE_IMPLEMENTATION
#include "../components/thirdparty/stb/stb_image.h"

namespace image {

// ============ Base64 解码 ============
static const char BASE64_CHARS[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

static inline bool is_base64(unsigned char c) { return (isalnum(c) || (c == '+') || (c == '/')); }

std::string base64_decode(const std::string &encoded) {
    int in_len = encoded.size();
    int i = 0, j = 0, in_ = 0;
    unsigned char char_array_4[4], char_array_3[3];
    std::string ret;

    while (in_len-- && (encoded[in_] != '=') && is_base64(encoded[in_])) {
        char_array_4[i++] = encoded[in_];
        in_++;
        if (i == 4) {
            for (i = 0; i < 4; i++)
                char_array_4[i] = std::string(BASE64_CHARS).find(char_array_4[i]);
            char_array_3[0] = (char_array_4[0] << 2) + ((char_array_4[1] & 0x30) >> 4);
            char_array_3[1] = ((char_array_4[1] & 0xf) << 4) + ((char_array_4[2] & 0x3c) >> 2);
            char_array_3[2] = ((char_array_4[2] & 0x3) << 6) + char_array_4[3];
            for (i = 0; i < 3; i++)
                ret += char_array_3[i];
            i = 0;
        }
    }
    if (i) {
        for (j = i; j < 4; j++)
            char_array_4[j] = 0;
        for (j = 0; j < 4; j++)
            char_array_4[j] = std::string(BASE64_CHARS).find(char_array_4[j]);
        char_array_3[0] = (char_array_4[0] << 2) + ((char_array_4[1] & 0x30) >> 4);
        char_array_3[1] = ((char_array_4[1] & 0xf) << 4) + ((char_array_4[2] & 0x3c) >> 2);
        for (j = 0; j < i - 1; j++)
            ret += char_array_3[j];
    }
    return ret;
}

// ============ JPEG 解码（使用 stb_image）============
bool decode_jpeg(const std::vector<uint8_t> &jpeg_data, ImageData &out) {
    if (jpeg_data.empty())
        return false;

    int width, height, channels;
    // 强制 RGB 3 通道
    unsigned char *pixels =
        stbi_load_from_memory(jpeg_data.data(), static_cast<int>(jpeg_data.size()), &width, &height, &channels, 3);

    if (!pixels) {
        printf("[image_utils] JPEG decode failed: %s\n", stbi_failure_reason());
        return false;
    }

    out.width = width;
    out.height = height;
    out.channels = 3;
    out.data.assign(pixels, pixels + width * height * 3);

    stbi_image_free(pixels);
    return true;
}

// ============ Letterbox Resize + Normalize (YOLO 预处理) ============
std::vector<float> resize_and_normalize(const ImageData &img, int target_width, int target_height) {
    // Letterbox: 保持宽高比缩放，空白区域填充 114/255
    float scale =
        std::min(static_cast<float>(target_width) / img.width, static_cast<float>(target_height) / img.height);
    int new_w = static_cast<int>(img.width * scale);
    int new_h = static_cast<int>(img.height * scale);
    int pad_x = (target_width - new_w) / 2;
    int pad_y = (target_height - new_h) / 2;

    const float pad_value = 114.0f / 255.0f;

    // 输出: CHW 格式, 归一化到 [0, 1]
    std::vector<float> output(3 * target_width * target_height, pad_value);

    for (int y = 0; y < new_h; ++y) {
        for (int x = 0; x < new_w; ++x) {
            // 最近邻插值
            int src_x = static_cast<int>(x / scale);
            int src_y = static_cast<int>(y / scale);
            src_x = std::min(src_x, img.width - 1);
            src_y = std::min(src_y, img.height - 1);

            int src_idx = (src_y * img.width + src_x) * img.channels;
            int dst_x = x + pad_x;
            int dst_y = y + pad_y;

            for (int c = 0; c < 3; ++c) {
                int dst_idx = c * target_height * target_width + dst_y * target_width + dst_x;
                output[dst_idx] = static_cast<float>(img.data[src_idx + c]) / 255.0f;
            }
        }
    }

    return output;
}

} // namespace image
