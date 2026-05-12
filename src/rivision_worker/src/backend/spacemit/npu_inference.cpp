// SpacemiT NPU Inference using ONNX Runtime + SpaceMITExecutionProvider
// Reference: model-zoo-vision/src/core/cpp/vision_model_base.cpp

#include "npu_inference.h"
#include "utils/logger.h"

#include <onnxruntime_cxx_api.h>
#include <spacemit_ort_env.h>
#include <opencv2/opencv.hpp>

#include <chrono>
#include <algorithm>
#include <cmath>

namespace rivision::spacemit {

// =============================================================================
// NpuInference::Impl
// =============================================================================

class NpuInference::Impl {
public:
    Impl() = default;
    ~Impl() { release(); }
    
    bool init(const hal::IInference::ModelConfig& cfg) {
        try {
            model_path_ = cfg.model_path;
            num_threads_ = cfg.threads > 0 ? cfg.threads : 4;
            
            // Create environment (SpacemiT optimized)
            env_ = std::make_unique<Ort::Env>(ORT_LOGGING_LEVEL_WARNING, "rivision_npu");
            
            // Session options
            session_options_.SetIntraOpNumThreads(num_threads_);
            session_options_.SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_ALL);
            
            // Enable SpaceMIT EP (NPU acceleration)
            try {
                Ort::SessionOptionsSpaceMITEnvInit(session_options_);
                LOG_INFO("SpaceMIT EP enabled for NPU acceleration");
                use_npu_ = true;
            } catch (...) {
                LOG_WARN("SpaceMIT EP not available, falling back to CPU");
                use_npu_ = false;
            }
            
            // Create session
            session_ = std::make_unique<Ort::Session>(*env_, model_path_.c_str(), session_options_);
            
            // Get input info
            Ort::AllocatorWithDefaultOptions allocator;
            size_t num_inputs = session_->GetInputCount();
            
            for (size_t i = 0; i < num_inputs; ++i) {
                auto name = session_->GetInputNameAllocated(i, allocator);
                input_names_str_.push_back(name.get());
                
                auto info = session_->GetInputTypeInfo(i);
                auto shape = info.GetTensorTypeAndShapeInfo().GetShape();
                if (i == 0) {
                    input_shape_ = shape;
                }
            }
            
            for (const auto& n : input_names_str_) {
                input_names_.push_back(n.c_str());
            }
            
            // Get output info
            size_t num_outputs = session_->GetOutputCount();
            for (size_t i = 0; i < num_outputs; ++i) {
                auto name = session_->GetOutputNameAllocated(i, allocator);
                output_names_str_.push_back(name.get());
            }
            
            for (const auto& n : output_names_str_) {
                output_names_.push_back(n.c_str());
            }
            
            output_num_ = num_outputs;
            
            // Memory info for tensor creation
            memory_info_ = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);
            
            model_loaded_ = true;
            
            LOG_INFO("Model loaded: {} ({}x{}, {} outputs, NPU={})",
                     model_path_,
                     input_shape_.size() >= 4 ? input_shape_[3] : 0,
                     input_shape_.size() >= 3 ? input_shape_[2] : 0,
                     output_num_,
                     use_npu_);
            
            return true;
            
        } catch (const Ort::Exception& e) {
            last_error_ = std::string("ORT error: ") + e.what();
            LOG_ERROR("{}", last_error_);
            return false;
        } catch (const std::exception& e) {
            last_error_ = std::string("Error: ") + e.what();
            LOG_ERROR("{}", last_error_);
            return false;
        }
    }
    
    void release() {
        session_.reset();
        env_.reset();
        input_names_.clear();
        output_names_.clear();
        input_names_str_.clear();
        output_names_str_.clear();
        model_loaded_ = false;
    }
    
    std::vector<Ort::Value> runSession(const cv::Mat& input_blob) {
        if (!model_loaded_) {
            throw std::runtime_error("Model not loaded");
        }
        
        // Create input tensor
        std::vector<int64_t> shape = input_shape_;
        if (shape[0] < 0) shape[0] = 1;  // Dynamic batch
        
        size_t tensor_size = 1;
        for (auto s : shape) tensor_size *= s;
        
        Ort::Value input_tensor = Ort::Value::CreateTensor<float>(
            memory_info_,
            const_cast<float*>(reinterpret_cast<const float*>(input_blob.data)),
            tensor_size,
            shape.data(),
            shape.size()
        );
        
        // Run inference
        return session_->Run(
            Ort::RunOptions{nullptr},
            input_names_.data(),
            &input_tensor,
            1,
            output_names_.data(),
            output_num_
        );
    }
    
    std::vector<int64_t> input_shape_;
    size_t output_num_ = 0;
    bool model_loaded_ = false;
    bool use_npu_ = true;
    std::string last_error_;
    
private:
    std::string model_path_;
    int num_threads_ = 4;
    
    std::unique_ptr<Ort::Env> env_;
    Ort::SessionOptions session_options_;
    std::unique_ptr<Ort::Session> session_;
    Ort::MemoryInfo memory_info_{nullptr};
    
    std::vector<const char*> input_names_;
    std::vector<const char*> output_names_;
    std::vector<std::string> input_names_str_;
    std::vector<std::string> output_names_str_;
};

// =============================================================================
// NpuInference
// =============================================================================

NpuInference::NpuInference()
    : impl_(std::make_unique<Impl>()) {
}

NpuInference::~NpuInference() = default;

bool NpuInference::loadModel(const ModelConfig& cfg) {
    config_ = cfg;
    initialized_ = impl_->init(cfg);
    if (!initialized_) {
        last_error_ = impl_->last_error_;
    }
    return initialized_;
}

void NpuInference::unloadModel() {
    impl_->release();
    initialized_ = false;
}

bool NpuInference::isLoaded() const {
    return initialized_;
}

bool NpuInference::infer(const Frame& frame, std::vector<Tensor>& outputs) {
    if (!initialized_) {
        last_error_ = "Model not loaded";
        return false;
    }
    
    try {
        // Preprocess frame
        std::vector<float> input_data;
        if (!preprocess(frame, input_data)) {
            return false;
        }
        
        // Create input blob (NCHW format)
        int input_w = impl_->input_shape_.size() >= 4 ? static_cast<int>(impl_->input_shape_[3]) : config_.input_width;
        int input_h = impl_->input_shape_.size() >= 3 ? static_cast<int>(impl_->input_shape_[2]) : config_.input_height;
        
        cv::Mat input_blob(1, input_data.size(), CV_32F, input_data.data());
        
        // Run inference
        auto ort_outputs = impl_->runSession(input_blob);
        
        // Convert outputs
        outputs.clear();
        for (size_t i = 0; i < ort_outputs.size(); ++i) {
            auto& ot = ort_outputs[i];
            auto info = ot.GetTensorTypeAndShapeInfo();
            auto shape = info.GetShape();
            
            Tensor tensor;
            tensor.shape.assign(shape.begin(), shape.end());
            
            size_t num_elements = info.GetElementCount();
            const float* data = ot.GetTensorData<float>();
            tensor.data.assign(data, data + num_elements);
            
            outputs.push_back(std::move(tensor));
        }
        
        return true;
        
    } catch (const Ort::Exception& e) {
        last_error_ = std::string("Inference error: ") + e.what();
        LOG_ERROR("{}", last_error_);
        return false;
    }
}

std::string NpuInference::getLastError() const {
    return last_error_;
}

hal::IInference::ModelConfig NpuInference::getModelConfig() const {
    return config_;
}

void NpuInference::warmup(int iterations) {
    if (!initialized_) return;
    
    // Create dummy frame for warmup
    int input_w = impl_->input_shape_.size() >= 4 ? static_cast<int>(impl_->input_shape_[3]) : config_.input_width;
    int input_h = impl_->input_shape_.size() >= 3 ? static_cast<int>(impl_->input_shape_[2]) : config_.input_height;
    
    Frame dummy;
    dummy.width = input_w;
    dummy.height = input_h;
    dummy.format = PixelFormat::BGR24;
    std::vector<uint8_t> data(input_w * input_h * 3, 128);
    dummy.data = data.data();
    
    std::vector<Tensor> outputs;
    for (int i = 0; i < iterations; ++i) {
        infer(dummy, outputs);
    }
    
    LOG_INFO("Warmup completed: {} iterations", iterations);
}

float NpuInference::getAvgLatencyMs() const {
    return avg_latency_ms_;
}

bool NpuInference::preprocess(const Frame& frame, std::vector<float>& input) {
    // Get input dimensions
    int input_w = impl_->input_shape_.size() >= 4 ? static_cast<int>(impl_->input_shape_[3]) : config_.input_width;
    int input_h = impl_->input_shape_.size() >= 3 ? static_cast<int>(impl_->input_shape_[2]) : config_.input_height;
    
    // Convert Frame to cv::Mat
    cv::Mat img;
    if (frame.format == PixelFormat::RGB24) {
        img = cv::Mat(frame.height, frame.width, CV_8UC3, const_cast<uint8_t*>(frame.cpuData()));
        cv::cvtColor(img, img, cv::COLOR_RGB2BGR);
    } else if (frame.format == PixelFormat::BGR24) {
        img = cv::Mat(frame.height, frame.width, CV_8UC3, const_cast<uint8_t*>(frame.cpuData()));
    } else {
        last_error_ = "Unsupported pixel format";
        return false;
    }
    
    // Letterbox resize (same as model-zoo-vision)
    float scale = std::min(
        static_cast<float>(input_w) / img.cols,
        static_cast<float>(input_h) / img.rows
    );
    
    int new_w = static_cast<int>(img.cols * scale);
    int new_h = static_cast<int>(img.rows * scale);
    int pad_w = (input_w - new_w) / 2;
    int pad_h = (input_h - new_h) / 2;
    
    cv::Mat resized;
    cv::resize(img, resized, cv::Size(new_w, new_h));
    
    cv::Mat padded(input_h, input_w, CV_8UC3, cv::Scalar(114, 114, 114));
    resized.copyTo(padded(cv::Rect(pad_w, pad_h, new_w, new_h)));
    
    // Use OpenCV dnn::blobFromImage for NCHW conversion + normalization
    cv::Mat blob = cv::dnn::blobFromImage(
        padded,
        1.0 / 255.0,
        cv::Size(input_w, input_h),
        cv::Scalar(0, 0, 0),
        true,   // swapRB
        false,  // crop
        CV_32F
    );
    
    // Copy to output
    input.assign(
        reinterpret_cast<float*>(blob.data),
        reinterpret_cast<float*>(blob.data) + blob.total()
    );
    
    return true;
}

} // namespace rivision::spacemit
