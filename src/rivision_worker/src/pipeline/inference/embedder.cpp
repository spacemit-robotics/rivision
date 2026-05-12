// CLIP Embedder implementation - Chinese-CLIP-ViT-B/16
#include "embedder.h"
#include "utils/logger.h"

#include <opencv2/opencv.hpp>
#include <cmath>

namespace rivision::pipeline {

// =============================================================================
// ClipEmbedder::Impl (PIMPL for ONNX Runtime)
// =============================================================================

class ClipEmbedder::Impl {
public:
    // Placeholder for ONNX Runtime session
    // When ONNX Runtime is available, add:
    // std::unique_ptr<Ort::Env> env_;
    // std::unique_ptr<Ort::Session> session_;
};

// =============================================================================
// ClipEmbedder Implementation
// =============================================================================

ClipEmbedder::ClipEmbedder(const Config& cfg) 
    : config_(cfg), impl_(std::make_unique<Impl>()) {}

ClipEmbedder::~ClipEmbedder() = default;

bool ClipEmbedder::init() {
    LOG_INFO("Initializing ClipEmbedder: model={}, dim={}", 
             config_.model_path, config_.embedding_dim);
    
    // TODO: Load ONNX model for CLIP
    // For now, mark as initialized - actual model loading requires ONNX Runtime
    if (config_.model_path.empty()) {
        last_error_ = "Model path is empty";
        return false;
    }
    
    initialized_ = true;
    LOG_INFO("ClipEmbedder initialized successfully");
    return true;
}

bool ClipEmbedder::embed(const Frame& frame, std::vector<float>& embedding) {
    if (!initialized_) {
        last_error_ = "Embedder not initialized";
        return false;
    }
    
    if (frame.isEmpty()) {
        last_error_ = "Empty frame";
        return false;
    }
    
    // Preprocess frame
    std::vector<float> input;
    if (!preprocess(frame, input)) {
        return false;
    }
    
    // TODO: Run ONNX inference
    // For now, return normalized random embedding for testing
    embedding.resize(config_.embedding_dim);
    for (int i = 0; i < config_.embedding_dim; ++i) {
        embedding[i] = static_cast<float>(rand()) / RAND_MAX - 0.5f;
    }
    normalize(embedding);
    
    LOG_DEBUG("Image embedding generated: {} dims", embedding.size());
    return true;
}

bool ClipEmbedder::embedText(const std::string& text, std::vector<float>& embedding) {
    if (!initialized_) {
        last_error_ = "Embedder not initialized";
        return false;
    }
    
    if (text.empty()) {
        last_error_ = "Empty text";
        return false;
    }
    
    // TODO: Tokenize and run CLIP text encoder
    // For now, return normalized random embedding for testing
    embedding.resize(config_.embedding_dim);
    for (int i = 0; i < config_.embedding_dim; ++i) {
        // Use text hash to generate deterministic embedding
        embedding[i] = static_cast<float>((std::hash<std::string>{}(text) + i) % 1000) / 1000.0f - 0.5f;
    }
    normalize(embedding);
    
    LOG_DEBUG("Text embedding generated: {} dims for '{}'", embedding.size(), 
              text.substr(0, 50));
    return true;
}

bool ClipEmbedder::embedBatch(const std::vector<Frame>& frames,
                              std::vector<std::vector<float>>& embeddings) {
    embeddings.clear();
    embeddings.reserve(frames.size());
    
    for (const auto& frame : frames) {
        std::vector<float> emb;
        if (!embed(frame, emb)) {
            return false;
        }
        embeddings.push_back(std::move(emb));
    }
    return true;
}

void ClipEmbedder::warmup(int iterations) {
    if (!initialized_) return;
    
    LOG_INFO("ClipEmbedder warmup: {} iterations", iterations);
    
    // Create dummy frame
    Frame dummy;
    dummy.width = config_.input_size;
    dummy.height = config_.input_size;
    dummy.format = PixelFormat::RGB24;
    auto& data = dummy.data.emplace<std::vector<uint8_t>>();
    data.resize(dummy.width * dummy.height * 3, 128);
    
    std::vector<float> embedding;
    for (int i = 0; i < iterations; ++i) {
        embed(dummy, embedding);
    }
    
    LOG_INFO("ClipEmbedder warmup completed");
}

bool ClipEmbedder::preprocess(const Frame& frame, std::vector<float>& input) {
    int size = config_.input_size;
    input.resize(3 * size * size);
    
    const uint8_t* src = frame.cpuData();
    if (!src) {
        last_error_ = "Cannot access frame CPU data";
        return false;
    }
    
    // Convert to OpenCV Mat for preprocessing
    cv::Mat img;
    if (frame.format == PixelFormat::RGB24) {
        img = cv::Mat(frame.height, frame.width, CV_8UC3, 
                     const_cast<uint8_t*>(src));
    } else if (frame.format == PixelFormat::BGR24) {
        cv::Mat bgr(frame.height, frame.width, CV_8UC3, 
                   const_cast<uint8_t*>(src));
        cv::cvtColor(bgr, img, cv::COLOR_BGR2RGB);
    } else if (frame.format == PixelFormat::NV12) {
        cv::Mat yuv(frame.height * 3 / 2, frame.width, CV_8UC1,
                   const_cast<uint8_t*>(src));
        cv::cvtColor(yuv, img, cv::COLOR_YUV2RGB_NV12);
    } else {
        last_error_ = "Unsupported pixel format";
        return false;
    }
    
    // Resize to input_size x input_size
    cv::Mat resized;
    cv::resize(img, resized, cv::Size(size, size));
    
    // Normalize: ImageNet mean/std
    const float mean[] = {0.48145466f, 0.4578275f, 0.40821073f};
    const float std_dev[] = {0.26862954f, 0.26130258f, 0.27577711f};
    
    // HWC to CHW + normalize
    for (int c = 0; c < 3; ++c) {
        for (int h = 0; h < size; ++h) {
            for (int w = 0; w < size; ++w) {
                float pixel = resized.at<cv::Vec3b>(h, w)[c] / 255.0f;
                input[c * size * size + h * size + w] = (pixel - mean[c]) / std_dev[c];
            }
        }
    }
    
    return true;
}

void ClipEmbedder::normalize(std::vector<float>& embedding) {
    float norm = 0.0f;
    for (float v : embedding) {
        norm += v * v;
    }
    norm = std::sqrt(norm);
    if (norm > 1e-6f) {
        for (float& v : embedding) {
            v /= norm;
        }
    }
}

// =============================================================================
// Factory
// =============================================================================

std::unique_ptr<Embedder> Embedder::create(const Config& cfg) {
    return std::make_unique<ClipEmbedder>(cfg);
}

} // namespace rivision::pipeline
