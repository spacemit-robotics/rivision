#include "inference_backend/yolo_tensorrt.h"
#include <fstream>
#include <iostream>
#include <chrono>

namespace rivision {
namespace benchmark {

// ============================================================
// TensorRT YOLO Backend 实现
// ============================================================

TensorRTYOLOBackend::TensorRTYOLOBackend() = default;

TensorRTYOLOBackend::~TensorRTYOLOBackend() {
    shutdown();
}

bool TensorRTYOLOBackend::init(const BackendConfig& config) {
#ifndef USE_TENSORRT
    std::cerr << "[TensorRT] TensorRT support not compiled. "
              << "Rebuild with -DUSE_TENSORRT=ON" << std::endl;
    return false;
#else
    conf_threshold_ = config.conf_threshold;
    nms_threshold_ = config.nms_threshold;
    
    // 优先使用 .engine 文件
    std::string model_path = config.model_path;
    
    // 检查是否是 engine 文件
    if (model_path.size() > 7 && 
        model_path.substr(model_path.size() - 7) == ".engine") {
        engine_path_ = model_path;
        if (!loadEngine(engine_path_)) {
            std::cerr << "[TensorRT] Failed to load engine: " << engine_path_ << std::endl;
            return false;
        }
    } else if (model_path.size() > 5 && 
               model_path.substr(model_path.size() - 5) == ".onnx") {
        // ONNX 文件，需要构建 engine
        onnx_path_ = model_path;
        engine_path_ = model_path.substr(0, model_path.size() - 5) + ".engine";
        
        // 检查是否已有 engine
        std::ifstream engine_file(engine_path_, std::ios::binary);
        if (engine_file.good()) {
            engine_file.close();
            if (!loadEngine(engine_path_)) {
                std::cerr << "[TensorRT] Cached engine invalid, rebuilding..." << std::endl;
                if (!buildEngineFromONNX(onnx_path_, engine_path_, use_fp16_, use_int8_)) {
                    return false;
                }
            }
        } else {
            std::cout << "[TensorRT] Building engine from ONNX..." << std::endl;
            if (!buildEngineFromONNX(onnx_path_, engine_path_, use_fp16_, use_int8_)) {
                return false;
            }
        }
    } else {
        std::cerr << "[TensorRT] Unsupported model format: " << model_path << std::endl;
        return false;
    }
    
    // 分配缓冲区
    if (!allocateBuffers()) {
        std::cerr << "[TensorRT] Failed to allocate CUDA buffers" << std::endl;
        return false;
    }
    
    std::cout << "[TensorRT] Initialized successfully" << std::endl;
    std::cout << "  GPU: " << getGPUInfo() << std::endl;
    std::cout << "  Input: " << input_w_ << "x" << input_h_ << "x" << input_c_ << std::endl;
    
    return true;
#endif
}

InferenceResult TensorRTYOLOBackend::infer(const Frame& frame) {
    InferenceResult result;
    result.success = false;
    
#ifdef USE_TENSORRT
    if (!engine_ || !context_) {
        result.error_msg = "TensorRT engine not initialized";
        return result;
    }
    
    auto start = std::chrono::high_resolution_clock::now();
    
    // 预处理 (CPU -> GPU)
    auto preprocess_start = std::chrono::high_resolution_clock::now();
    if (!preprocessGPU(frame.image)) {
        result.error_msg = "Preprocessing failed";
        return result;
    }
    auto preprocess_end = std::chrono::high_resolution_clock::now();
    
    // 推理
    auto inference_start = std::chrono::high_resolution_clock::now();
    bool status = context_->enqueueV2(device_buffers_, stream_, nullptr);
    cudaStreamSynchronize(stream_);
    if (!status) {
        result.error_msg = "Inference execution failed";
        return result;
    }
    
    // 复制结果到主机
    cudaMemcpyAsync(host_output_, device_buffers_[1], 
                    output_size_ * sizeof(float),
                    cudaMemcpyDeviceToHost, stream_);
    cudaStreamSynchronize(stream_);
    auto inference_end = std::chrono::high_resolution_clock::now();
    
    // 后处理
    auto postprocess_start = std::chrono::high_resolution_clock::now();
    result.detections = postprocess(host_output_, output_size_, 
                                     conf_threshold_, nms_threshold_);
    auto postprocess_end = std::chrono::high_resolution_clock::now();
    
    auto end = std::chrono::high_resolution_clock::now();
    
    // 计算时间
    result.preprocess_time_ms = std::chrono::duration<double, std::milli>(
        preprocess_end - preprocess_start).count();
    result.inference_time_ms = std::chrono::duration<double, std::milli>(
        inference_end - inference_start).count();
    result.postprocess_time_ms = std::chrono::duration<double, std::milli>(
        postprocess_end - postprocess_start).count();
    result.total_time_ms = std::chrono::duration<double, std::milli>(
        end - start).count();
    
    result.success = true;
#else
    result.error_msg = "TensorRT not compiled";
#endif
    
    return result;
}

void TensorRTYOLOBackend::shutdown() {
#ifdef USE_TENSORRT
    freeBuffers();
    
    if (context_) {
        context_->destroy();
        context_ = nullptr;
    }
    if (engine_) {
        engine_->destroy();
        engine_ = nullptr;
    }
    if (runtime_) {
        runtime_->destroy();
        runtime_ = nullptr;
    }
#endif
}

bool TensorRTYOLOBackend::loadEngine(const std::string& engine_path) {
#ifdef USE_TENSORRT
    std::ifstream file(engine_path, std::ios::binary);
    if (!file.good()) {
        return false;
    }
    
    file.seekg(0, std::ios::end);
    size_t size = file.tellg();
    file.seekg(0, std::ios::beg);
    
    std::vector<char> buffer(size);
    file.read(buffer.data(), size);
    file.close();
    
    runtime_ = nvinfer1::createInferRuntime(logger_);
    if (!runtime_) {
        return false;
    }
    
    engine_ = runtime_->deserializeCudaEngine(buffer.data(), size);
    if (!engine_) {
        return false;
    }
    
    context_ = engine_->createExecutionContext();
    if (!context_) {
        return false;
    }
    
    // 获取输入/输出维度
    // 假设输入名为 "images", 输出名为 "output0"
    auto input_dims = engine_->getBindingDimensions(0);
    input_c_ = input_dims.d[1];
    input_h_ = input_dims.d[2];
    input_w_ = input_dims.d[3];
    
    auto output_dims = engine_->getBindingDimensions(1);
    output_size_ = 1;
    for (int i = 0; i < output_dims.nbDims; i++) {
        output_size_ *= output_dims.d[i];
    }
    
    return true;
#else
    return false;
#endif
}

bool TensorRTYOLOBackend::buildEngineFromONNX(const std::string& onnx_path,
                                               const std::string& engine_path,
                                               bool use_fp16, bool use_int8) {
#ifdef USE_TENSORRT
    auto builder = nvinfer1::createInferBuilder(logger_);
    if (!builder) return false;
    
    auto network = builder->createNetworkV2(
        1U << static_cast<uint32_t>(nvinfer1::NetworkDefinitionCreationFlag::kEXPLICIT_BATCH));
    if (!network) return false;
    
    auto parser = nvonnxparser::createParser(*network, logger_);
    if (!parser) return false;
    
    // 解析 ONNX
    if (!parser->parseFromFile(onnx_path.c_str(), 
                               static_cast<int>(nvinfer1::ILogger::Severity::kWARNING))) {
        std::cerr << "[TensorRT] Failed to parse ONNX file" << std::endl;
        return false;
    }
    
    // 构建配置
    auto config = builder->createBuilderConfig();
    config->setMaxWorkspaceSize(1 << 30);  // 1GB
    
    if (use_fp16 && builder->platformHasFastFp16()) {
        config->setFlag(nvinfer1::BuilderFlag::kFP16);
        std::cout << "[TensorRT] Using FP16 precision" << std::endl;
    }
    
    if (use_int8 && builder->platformHasFastInt8()) {
        config->setFlag(nvinfer1::BuilderFlag::kINT8);
        std::cout << "[TensorRT] Using INT8 precision" << std::endl;
        // TODO: 设置 INT8 校准器
    }
    
    // 构建 engine
    std::cout << "[TensorRT] Building engine (this may take a while)..." << std::endl;
    auto engine = builder->buildEngineWithConfig(*network, *config);
    if (!engine) {
        std::cerr << "[TensorRT] Failed to build engine" << std::endl;
        return false;
    }
    
    // 序列化保存
    auto serialized = engine->serialize();
    std::ofstream file(engine_path, std::ios::binary);
    file.write(static_cast<const char*>(serialized->data()), serialized->size());
    file.close();
    
    std::cout << "[TensorRT] Engine saved to: " << engine_path << std::endl;
    
    // 清理
    serialized->destroy();
    engine->destroy();
    parser->destroy();
    network->destroy();
    config->destroy();
    builder->destroy();
    
    // 重新加载
    return loadEngine(engine_path);
#else
    return false;
#endif
}

std::string TensorRTYOLOBackend::getGPUInfo() const {
#ifdef USE_TENSORRT
    int device;
    cudaGetDevice(&device);
    cudaDeviceProp prop;
    cudaGetDeviceProperties(&prop, device);
    return std::string(prop.name) + " (SM " + 
           std::to_string(prop.major) + "." + std::to_string(prop.minor) + ")";
#else
    return "N/A";
#endif
}

int TensorRTYOLOBackend::getCUDADeviceCount() const {
#ifdef USE_TENSORRT
    int count = 0;
    cudaGetDeviceCount(&count);
    return count;
#else
    return 0;
#endif
}

std::string TensorRTYOLOBackend::getCUDAArch() const {
#ifdef USE_TENSORRT
    int device;
    cudaGetDevice(&device);
    cudaDeviceProp prop;
    cudaGetDeviceProperties(&prop, device);
    return "sm_" + std::to_string(prop.major) + std::to_string(prop.minor);
#else
    return "N/A";
#endif
}

#ifdef USE_TENSORRT
void TensorRTYOLOBackend::Logger::log(Severity severity, const char* msg) noexcept {
    if (severity <= Severity::kWARNING) {
        std::cout << "[TensorRT] " << msg << std::endl;
    }
}

bool TensorRTYOLOBackend::allocateBuffers() {
    // 创建 CUDA 流
    cudaStreamCreate(&stream_);
    
    // 输入缓冲区
    size_t input_size = input_c_ * input_h_ * input_w_ * sizeof(float);
    cudaMalloc(&device_buffers_[0], input_size);
    
    // 输出缓冲区
    size_t output_bytes = output_size_ * sizeof(float);
    cudaMalloc(&device_buffers_[1], output_bytes);
    host_output_ = new float[output_size_];
    
    return true;
}

void TensorRTYOLOBackend::freeBuffers() {
    if (stream_) {
        cudaStreamDestroy(stream_);
        stream_ = nullptr;
    }
    if (device_buffers_[0]) {
        cudaFree(device_buffers_[0]);
        device_buffers_[0] = nullptr;
    }
    if (device_buffers_[1]) {
        cudaFree(device_buffers_[1]);
        device_buffers_[1] = nullptr;
    }
    if (host_output_) {
        delete[] host_output_;
        host_output_ = nullptr;
    }
}

bool TensorRTYOLOBackend::preprocessGPU(const cv::Mat& img) {
    // 调整大小
    cv::Mat resized;
    cv::resize(img, resized, cv::Size(input_w_, input_h_));
    
    // BGR -> RGB, 归一化
    cv::Mat rgb;
    cv::cvtColor(resized, rgb, cv::COLOR_BGR2RGB);
    rgb.convertTo(rgb, CV_32FC3, 1.0 / 255.0);
    
    // HWC -> CHW
    std::vector<cv::Mat> channels(3);
    cv::split(rgb, channels);
    
    size_t plane_size = input_h_ * input_w_ * sizeof(float);
    for (int c = 0; c < 3; c++) {
        cudaMemcpyAsync(static_cast<float*>(device_buffers_[0]) + c * input_h_ * input_w_,
                        channels[c].data, plane_size,
                        cudaMemcpyHostToDevice, stream_);
    }
    cudaStreamSynchronize(stream_);
    
    return true;
}

std::vector<Detection> TensorRTYOLOBackend::postprocess(
    float* output, int output_size,
    float conf_threshold, float nms_threshold) {
    
    std::vector<Detection> detections;
    
    // YOLOv8 output: [1, 84, 8400] -> transpose -> [8400, 84]
    // 84 = 4 (bbox) + 80 (classes)
    const int num_classes = 80;
    const int num_anchors = 8400;
    
    std::vector<cv::Rect> boxes;
    std::vector<float> confidences;
    std::vector<int> class_ids;
    
    for (int i = 0; i < num_anchors; i++) {
        // 获取类别分数
        float max_score = 0;
        int max_class = 0;
        for (int c = 0; c < num_classes; c++) {
            float score = output[(4 + c) * num_anchors + i];
            if (score > max_score) {
                max_score = score;
                max_class = c;
            }
        }
        
        if (max_score < conf_threshold) continue;
        
        // 解析边界框 (cx, cy, w, h)
        float cx = output[0 * num_anchors + i];
        float cy = output[1 * num_anchors + i];
        float w = output[2 * num_anchors + i];
        float h = output[3 * num_anchors + i];
        
        int x = static_cast<int>(cx - w / 2);
        int y = static_cast<int>(cy - h / 2);
        
        boxes.emplace_back(x, y, static_cast<int>(w), static_cast<int>(h));
        confidences.push_back(max_score);
        class_ids.push_back(max_class);
    }
    
    // NMS
    std::vector<int> indices;
    cv::dnn::NMSBoxes(boxes, confidences, conf_threshold, nms_threshold, indices);
    
    for (int idx : indices) {
        Detection det;
        det.bbox = boxes[idx];
        det.confidence = confidences[idx];
        det.class_id = class_ids[idx];
        detections.push_back(det);
    }
    
    return detections;
}
#endif

// ============================================================
// TensorRT Engine Builder
// ============================================================

bool TensorRTEngineBuilder::buildEngine(const std::string& onnx_path,
                                         const std::string& engine_path,
                                         const BuildOptions& options) {
#ifdef USE_TENSORRT
    TensorRTYOLOBackend backend;
    return backend.buildEngineFromONNX(onnx_path, engine_path, 
                                        options.use_fp16, options.use_int8);
#else
    std::cerr << "[TensorRT] Not compiled with TensorRT support" << std::endl;
    return false;
#endif
}

bool TensorRTEngineBuilder::isEngineValid(const std::string& engine_path,
                                           const std::string& cuda_arch) {
    // TODO: 检查 engine 文件头中的 CUDA 架构信息
    std::ifstream file(engine_path, std::ios::binary);
    return file.good();
}

}  // namespace benchmark
}  // namespace rivision
