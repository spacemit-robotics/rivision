#pragma once

#include <cstdint>
#include <string>
#include <vector>
#include <memory>

namespace rivision::vlm {

class VisionEncoder {
public:
    VisionEncoder();
    ~VisionEncoder();
    
    bool init(const std::string& model_path);
    
    std::vector<float> encode(
        const uint8_t* image_data,
        int width,
        int height,
        int channels = 3
    );
    
    std::vector<float> encodeJpeg(
        const uint8_t* jpeg_data,
        size_t jpeg_size
    );
    
    int getEmbeddingDim() const { return embedding_dim_; }
    int getInputWidth() const { return input_width_; }
    int getInputHeight() const { return input_height_; }
    
    bool isReady() const { return ready_; }
    
private:
    std::vector<float> preprocess(
        const uint8_t* rgb,
        int width,
        int height
    );
    
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    bool ready_ = false;
    int embedding_dim_ = 768;
    int input_width_ = 224;
    int input_height_ = 224;
};

} // namespace rivision::vlm
