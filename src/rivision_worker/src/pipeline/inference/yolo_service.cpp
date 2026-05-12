#include "yolo_service.h"
#include "hal/interface/i_inference.h"
#include "hal/factory.h"
#include "utils/logger.h"

#include <chrono>
#include <algorithm>
#include <cmath>

namespace rivision::pipeline {

std::unique_ptr<YoloService> YoloService::create(const Options& opts) {
    return std::make_unique<YoloServiceImpl>(opts);
}

YoloServiceImpl::YoloServiceImpl(const Options& opts) {
    options_ = opts;
    class_names_ = opts.class_names;
    
    // Pre-allocate input buffer
    int input_size = opts.input_width * opts.input_height * 3;
    input_buffer_.resize(input_size);
}

YoloServiceImpl::~YoloServiceImpl() = default;

bool YoloServiceImpl::init() {
    LOG_INFO("Initializing YoloService: model={}, device={}", 
             options_.model_path, options_.device);
    std::cout.flush();
    
    // Create inference backend
    LOG_INFO("Creating inference backend...");
    std::cout.flush();
    inference_ = hal::createInference(options_.device);
    LOG_INFO("Inference backend created: {}", inference_ ? "OK" : "NULL");
    std::cout.flush();
    if (!inference_) {
        LOG_ERROR("Failed to create inference backend for device: {}", options_.device);
        return false;
    }
    
    // Load model
    LOG_INFO("Loading ONNX model...");
    std::cout.flush();
    hal::IInference::ModelConfig model_cfg;
    model_cfg.model_path = options_.model_path;
    model_cfg.input_width = options_.input_width;
    model_cfg.input_height = options_.input_height;
    model_cfg.input_format = "nchw";
    model_cfg.input_pixel_format = PixelFormat::RGB24;
    
    if (!inference_->loadModel(model_cfg)) {
        LOG_ERROR("Failed to load model: {}", options_.model_path);
        return false;
    }
    
    LOG_INFO("Model loaded successfully");
    return true;
}

std::vector<Detection> YoloServiceImpl::detect(const Frame& frame) {
    auto start = std::chrono::high_resolution_clock::now();
    
    // Pass original frame to inference backend for preprocessing
    // The backend handles resize/normalize internally
    std::vector<hal::IInference::Tensor> outputs;
    if (!inference_->infer(frame, outputs)) {
        LOG_WARN("Inference failed: {}", inference_->getLastError());
        return {};
    }
    
    // Postprocess
    auto detections = postprocess(outputs[0].data, frame.width, frame.height);
    
    // NMS
    detections = nms(detections, options_.nms_threshold);
    
    // Update stats
    auto end = std::chrono::high_resolution_clock::now();
    float latency = std::chrono::duration<float, std::milli>(end - start).count();
    
    inference_count_++;
    avg_latency_ms_ = avg_latency_ms_ * 0.9f + latency * 0.1f;
    
    return detections;
}

std::vector<std::vector<Detection>> YoloServiceImpl::detectBatch(
    const std::vector<Frame>& frames) {
    
    std::vector<std::vector<Detection>> results;
    results.reserve(frames.size());
    
    for (const auto& frame : frames) {
        results.push_back(detect(frame));
    }
    
    return results;
}

const std::vector<std::string>& YoloServiceImpl::getClassNames() const {
    return class_names_;
}

void YoloServiceImpl::warmup(int iterations) {
    LOG_DEBUG("Warming up YoloService ({} iterations)", iterations);
    
    // Create dummy frame
    Frame dummy;
    dummy.width = options_.input_width;
    dummy.height = options_.input_height;
    dummy.format = PixelFormat::RGB24;
    dummy.data = std::vector<uint8_t>(options_.input_width * options_.input_height * 3, 128);
    
    for (int i = 0; i < iterations; ++i) {
        detect(dummy);
    }
    
    // Reset stats after warmup
    inference_count_ = 0;
    
    LOG_DEBUG("Warmup complete");
}

float YoloServiceImpl::getAvgLatencyMs() const {
    return avg_latency_ms_.load();
}

int64_t YoloServiceImpl::getInferenceCount() const {
    return inference_count_.load();
}

bool YoloServiceImpl::preprocess(const Frame& frame, std::vector<float>& input) {
    // Resize and normalize
    // For simplicity, assume frame is already in RGB24 format
    // Real implementation would handle NV12 → RGB conversion
    
    if (frame.isEmpty()) {
        return false;
    }
    
    const uint8_t* src = frame.cpuData();
    if (!src) {
        return false;
    }
    
    int src_w = frame.width;
    int src_h = frame.height;
    int dst_w = options_.input_width;
    int dst_h = options_.input_height;
    
    // Calculate letterbox parameters
    float scale = std::min(static_cast<float>(dst_w) / src_w,
                          static_cast<float>(dst_h) / src_h);
    int new_w = static_cast<int>(src_w * scale);
    int new_h = static_cast<int>(src_h * scale);
    int pad_w = (dst_w - new_w) / 2;
    int pad_h = (dst_h - new_h) / 2;
    
    // Allocate output
    input.resize(3 * dst_w * dst_h);
    std::fill(input.begin(), input.end(), 0.5f);  // Gray padding
    
    // Simple bilinear resize + CHW conversion + normalization
    for (int y = 0; y < new_h; ++y) {
        for (int x = 0; x < new_w; ++x) {
            int src_x = static_cast<int>(x / scale);
            int src_y = static_cast<int>(y / scale);
            
            src_x = std::min(src_x, src_w - 1);
            src_y = std::min(src_y, src_h - 1);
            
            int dst_x = x + pad_w;
            int dst_y = y + pad_h;
            
            int src_idx = (src_y * src_w + src_x) * 3;
            
            // NCHW format, normalized to [0, 1]
            input[0 * dst_w * dst_h + dst_y * dst_w + dst_x] = src[src_idx + 0] / 255.0f;
            input[1 * dst_w * dst_h + dst_y * dst_w + dst_x] = src[src_idx + 1] / 255.0f;
            input[2 * dst_w * dst_h + dst_y * dst_w + dst_x] = src[src_idx + 2] / 255.0f;
        }
    }
    
    return true;
}

std::vector<Detection> YoloServiceImpl::postprocess(
    const std::vector<float>& output,
    int orig_width, int orig_height) {
    
    std::vector<Detection> detections;
    
    int num_classes = class_names_.size();
    int input_w = options_.input_width;
    int input_h = options_.input_height;
    
    // Calculate letterbox parameters
    float scale = std::min(
        static_cast<float>(input_w) / orig_width,
        static_cast<float>(input_h) / orig_height
    );
    int pad_w = static_cast<int>((input_w - orig_width * scale) / 2);
    int pad_h = static_cast<int>((input_h - orig_height * scale) / 2);
    
    // Check output format: 
    // YOLOv8/11 output is TRANSPOSED: [batch, 84, 8400] -> 84 rows, 8400 columns
    // Row 0-3: cx, cy, w, h
    // Row 4-83: class scores
    
    size_t expected_std = 8400 * (4 + num_classes);  // Standard YOLOv8/11
    
    if (output.size() >= expected_std * 0.8 && output.size() <= expected_std * 1.2) {
        // YOLOv8 transposed format: [84, 8400]
        int num_anchors = output.size() / (4 + num_classes);
        int stride = num_anchors;  // Data is transposed
        
        for (int i = 0; i < num_anchors; ++i) {
            // Find max class - data is transposed so class c for anchor i is at:
            // output[(4 + c) * stride + i]
            int best_class = -1;
            float best_conf = 0.0f;
            
            for (int c = 0; c < num_classes; ++c) {
                float conf = output[(4 + c) * stride + i];
                if (conf > best_conf) {
                    best_conf = conf;
                    best_class = c;
                }
            }
            
            if (best_conf < options_.conf_threshold) {
                continue;
            }
            
            // Extract bbox (cx, cy, w, h) - also transposed
            float cx = output[0 * stride + i];
            float cy = output[1 * stride + i];
            float w = output[2 * stride + i];
            float h = output[3 * stride + i];
            
            // Convert to original image coordinates
            float x1 = (cx - w / 2 - pad_w) / scale;
            float y1 = (cy - h / 2 - pad_h) / scale;
            float x2 = (cx + w / 2 - pad_w) / scale;
            float y2 = (cy + h / 2 - pad_h) / scale;
            
            // Normalize to [0, 1]
            Detection detection;
            detection.bbox.x1 = std::clamp(x1 / orig_width, 0.0f, 1.0f);
            detection.bbox.y1 = std::clamp(y1 / orig_height, 0.0f, 1.0f);
            detection.bbox.x2 = std::clamp(x2 / orig_width, 0.0f, 1.0f);
            detection.bbox.y2 = std::clamp(y2 / orig_height, 0.0f, 1.0f);
            
            detection.class_id = best_class;
            detection.confidence = best_conf;
            
            if (best_class >= 0 && best_class < static_cast<int>(class_names_.size())) {
                detection.class_name = class_names_[best_class];
            }
            
            detections.push_back(detection);
        }
    } else {
        // SpacemiT DFL format or unknown - log warning
        LOG_WARN("Unknown output size {}, expected ~{}", output.size(), expected_std);
    }
    
    return detections;
}

// DFL decode for SpacemiT multi-branch output (boxes, scores, score_sum)
std::vector<Detection> YoloServiceImpl::postprocessDFL(
    const std::vector<hal::IInference::Tensor>& outputs,
    int orig_width, int orig_height) {
    
    std::vector<Detection> detections;
    
    if (outputs.size() < 3) {
        return detections;
    }
    
    int num_classes = class_names_.size();
    int input_w = options_.input_width;
    int input_h = options_.input_height;
    
    float scale2orig = std::min(
        static_cast<float>(input_w) / orig_width,
        static_cast<float>(input_h) / orig_height
    );
    int pad_w = static_cast<int>((input_w - orig_width * scale2orig) / 2);
    int pad_h = static_cast<int>((input_h - orig_height * scale2orig) / 2);
    
    constexpr int kDflLen = 16;  // DFL distribution length
    
    // Process each branch (3 branches for YOLO11)
    int num_branches = outputs.size() / 3;
    
    for (int branch = 0; branch < num_branches; ++branch) {
        const auto& boxes_tensor = outputs[branch * 3];
        const auto& scores_tensor = outputs[branch * 3 + 1];
        const auto& scoresum_tensor = outputs[branch * 3 + 2];
        
        if (boxes_tensor.shape.size() < 4 || scores_tensor.shape.size() < 3) {
            continue;
        }
        
        int grid_h = boxes_tensor.shape[2];
        int grid_w = boxes_tensor.shape[3];
        int anchors = grid_h * grid_w;
        
        float scale_w = static_cast<float>(input_w) / grid_w;
        float scale_h = static_cast<float>(input_h) / grid_h;
        
        const float* boxes = boxes_tensor.data.data();
        const float* scores = scores_tensor.data.data();
        const float* scoresum = scoresum_tensor.data.data();
        
        for (int anchor_idx = 0; anchor_idx < anchors; ++anchor_idx) {
            // Quick filter using score_sum
            if (scoresum[anchor_idx] < options_.conf_threshold) {
                continue;
            }
            
            // Find best class
            float max_score = 0.0f;
            int class_id = -1;
            for (int c = 0; c < num_classes; ++c) {
                float score = scores[c * anchors + anchor_idx];
                if (score > max_score && score > options_.conf_threshold) {
                    max_score = score;
                    class_id = c;
                }
            }
            
            if (class_id < 0) continue;
            
            // DFL decode for bbox
            float xywh[4] = {0, 0, 0, 0};
            for (int i = 0; i < 4; ++i) {
                float exp_sum = 0.0f;
                float exp_dfl[kDflLen];
                
                for (int d = 0; d < kDflLen; ++d) {
                    size_t offset = i * kDflLen * anchors + d * anchors + anchor_idx;
                    exp_dfl[d] = std::exp(boxes[offset]);
                    exp_sum += exp_dfl[d];
                }
                
                for (int d = 0; d < kDflLen; ++d) {
                    xywh[i] += (exp_dfl[d] / exp_sum) * d;
                }
            }
            
            int h_idx = anchor_idx / grid_w;
            int w_idx = anchor_idx % grid_w;
            
            float x1 = ((w_idx - xywh[0] + 0.5f) * scale_w - pad_w) / scale2orig;
            float y1 = ((h_idx - xywh[1] + 0.5f) * scale_h - pad_h) / scale2orig;
            float x2 = ((w_idx + xywh[2] + 0.5f) * scale_w - pad_w) / scale2orig;
            float y2 = ((h_idx + xywh[3] + 0.5f) * scale_h - pad_h) / scale2orig;
            
            Detection det;
            det.bbox.x1 = std::clamp(x1 / orig_width, 0.0f, 1.0f);
            det.bbox.y1 = std::clamp(y1 / orig_height, 0.0f, 1.0f);
            det.bbox.x2 = std::clamp(x2 / orig_width, 0.0f, 1.0f);
            det.bbox.y2 = std::clamp(y2 / orig_height, 0.0f, 1.0f);
            det.class_id = class_id;
            det.confidence = max_score;
            
            if (class_id < static_cast<int>(class_names_.size())) {
                det.class_name = class_names_[class_id];
            }
            
            detections.push_back(det);
        }
    }
    
    return detections;
}

std::vector<Detection> YoloServiceImpl::nms(
    std::vector<Detection>& detections,
    float iou_threshold) {
    
    if (detections.empty()) {
        return detections;
    }
    
    // Sort by confidence
    std::sort(detections.begin(), detections.end(),
        [](const Detection& a, const Detection& b) {
            return a.confidence > b.confidence;
        });
    
    std::vector<Detection> result;
    std::vector<bool> suppressed(detections.size(), false);
    
    for (size_t i = 0; i < detections.size(); ++i) {
        if (suppressed[i]) continue;
        
        result.push_back(detections[i]);
        
        for (size_t j = i + 1; j < detections.size(); ++j) {
            if (suppressed[j]) continue;
            
            // Only suppress same class
            if (detections[i].class_id != detections[j].class_id) continue;
            
            float iou = detections[i].bbox.iou(detections[j].bbox);
            if (iou > iou_threshold) {
                suppressed[j] = true;
            }
        }
    }
    
    return result;
}

} // namespace rivision::pipeline
