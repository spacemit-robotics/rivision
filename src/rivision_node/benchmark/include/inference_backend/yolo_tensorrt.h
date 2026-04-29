#pragma once

#include "inference_backend.h"
#include "platform/platform_types.h"

#ifdef USE_TENSORRT
#include <NvInfer.h>
#include <NvOnnxParser.h>
#include <cuda_runtime_api.h>
#endif

namespace rivision {
namespace benchmark {

/**
 * @brief TensorRT YOLO 后端 (Jetson Orin / NVIDIA GPU)
 * 
 * 支持:
 * - TensorRT .engine 文件直接加载
 * - ONNX 模型动态转换为 TensorRT
 * - FP16/INT8 精度优化
 * - 批量推理
 * 
 * 交叉编译注意:
 * - 需要在 Jetson 上运行或使用 NVIDIA Cross-Compilation Toolkit
 * - engine 文件与 GPU 架构绑定 (sm_87 for Orin)
 */
class TensorRTYOLOBackend : public LocalYOLOBackend {
public:
    TensorRTYOLOBackend();
    ~TensorRTYOLOBackend() override;
    
    bool init(const BackendConfig& config) override;
    InferenceResult infer(const Frame& frame) override;
    void shutdown() override;
    
    std::string getName() const override { return "TensorRT YOLO"; }
    std::string getBackendType() const override { return "tensorrt"; }
    
    // TensorRT 特定方法
    bool loadEngine(const std::string& engine_path);
    bool buildEngineFromONNX(const std::string& onnx_path, 
                              const std::string& engine_path,
                              bool use_fp16 = true,
                              bool use_int8 = false);
    
    // 获取 GPU 信息
    std::string getGPUInfo() const;
    int getCUDADeviceCount() const;
    std::string getCUDAArch() const;
    
private:
#ifdef USE_TENSORRT
    // TensorRT 运行时对象
    nvinfer1::IRuntime* runtime_ = nullptr;
    nvinfer1::ICudaEngine* engine_ = nullptr;
    nvinfer1::IExecutionContext* context_ = nullptr;
    
    // CUDA 流和缓冲区
    cudaStream_t stream_ = nullptr;
    void* device_buffers_[2] = {nullptr, nullptr};  // input, output
    float* host_output_ = nullptr;
    
    // 输入/输出维度
    int input_h_ = 640;
    int input_w_ = 640;
    int input_c_ = 3;
    int output_size_ = 0;
    
    // Logger
    class Logger : public nvinfer1::ILogger {
    public:
        void log(Severity severity, const char* msg) noexcept override;
    };
    Logger logger_;
    
    // 内部方法
    bool allocateBuffers();
    void freeBuffers();
    bool preprocessGPU(const cv::Mat& img);
    std::vector<Detection> postprocess(float* output, int output_size,
                                        float conf_threshold, float nms_threshold);
#endif
    
    // 配置
    std::string engine_path_;
    std::string onnx_path_;
    bool use_fp16_ = true;
    bool use_int8_ = false;
    float conf_threshold_ = 0.25f;
    float nms_threshold_ = 0.45f;
};

/**
 * @brief TensorRT 构建器 - 用于将 ONNX 转换为 TensorRT Engine
 * 
 * 可在 x86 上交叉构建，但生成的 engine 需在目标 GPU 上运行
 */
class TensorRTEngineBuilder {
public:
    struct BuildOptions {
        bool use_fp16 = true;
        bool use_int8 = false;
        int max_batch_size = 1;
        int workspace_size_mb = 1024;
        std::string calibration_data_path;  // INT8 校准数据
    };
    
    static bool buildEngine(const std::string& onnx_path,
                           const std::string& engine_path,
                           const BuildOptions& options = BuildOptions());
    
    static bool isEngineValid(const std::string& engine_path,
                              const std::string& cuda_arch);
};

}  // namespace benchmark
}  // namespace rivision
