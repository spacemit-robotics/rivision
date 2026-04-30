/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

#ifndef IMAGE_UTILS_H
#define IMAGE_UTILS_H

#include <cstdint>
#include <string>
#include <vector>

namespace image {

struct ImageData {
    std::vector<uint8_t> data;
    int width;
    int height;
    int channels;
};

bool decode_jpeg(const std::vector<uint8_t> &jpeg_data, ImageData &out);

std::vector<float> resize_and_normalize(const ImageData &img, int target_width, int target_height);

std::string base64_decode(const std::string &encoded);

}  // namespace image

#endif  // IMAGE_UTILS_H
