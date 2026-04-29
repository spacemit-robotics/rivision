/**
 * K3 MPP Hardware Accelerated Preprocessor
 * 
 * 使用 SpacemiT K3 RISC-V 芯片的 MPP 库进行硬件加速预处理:
 * - K1 JPU: JPEG 硬件解码
 * - K1 V2D: 硬件 resize + 色彩空间转换
 * 
 * 可将 YOLO 预处理时间从 30-50ms 降低到 8-12ms
 */

#ifndef K3_MPP_PREPROCESSOR_H
#define K3_MPP_PREPROCESSOR_H

#include <cstdint>
#include <cstddef>
#include <memory>
#include <string>
#include <vector>
#include <functional>

namespace rivision {
namespace benchmark {

// 预处理结果
struct MppPreprocessResult {
    bool success = false;
    std::string error;
    
    int orig_width = 0;
    int orig_height = 0;
    
    double decode_ms = 0;      // JPEG 解码时间
    double resize_ms = 0;      // resize + CSC 时间
    double normalize_ms = 0;   // CPU 归一化时间
    double total_ms = 0;       // 总预处理时间
};

// K3 MPP 预处理器配置
struct MppPreprocessorConfig {
    int target_width = 640;    // 目标宽度
    int target_height = 640;   // 目标高度
    bool enable_hw_decode = true;   // 启用硬件 JPEG 解码
    bool enable_hw_resize = true;   // 启用硬件 resize
    int cpu_threads = 4;       // CPU 线程数 (用于归一化)
};

/**
 * K3 MPP 硬件加速预处理器
 * 
 * 使用方式:
 *   K3MppPreprocessor preproc;
 *   if (preproc.init(config)) {
 *       float tensor[3*640*640];
 *       auto result = preproc.preprocess(jpeg_data, jpeg_size, tensor);
 *   }
 */
class K3MppPreprocessor {
public:
    K3MppPreprocessor();
    ~K3MppPreprocessor();
    
    // 禁止拷贝
    K3MppPreprocessor(const K3MppPreprocessor&) = delete;
    K3MppPreprocessor& operator=(const K3MppPreprocessor&) = delete;
    
    /**
     * 初始化 MPP 硬件
     * @param config 预处理器配置
     * @return 是否成功
     */
    bool init(const MppPreprocessorConfig& config);
    
    /**
     * 预处理 JPEG 图像
     * 
     * 流程: JPEG → JPU解码(NV12) → V2D(resize+CSC) → CPU归一化 → CHW tensor
     * 
     * @param jpeg_data JPEG 数据指针
     * @param jpeg_size JPEG 数据大小
     * @param output_tensor 输出 tensor (需预分配 3*H*W floats)
     * @return 预处理结果
     */
    MppPreprocessResult preprocess(const uint8_t* jpeg_data, size_t jpeg_size,
                                   float* output_tensor);
    
    /**
     * 预处理 (从 vector)
     */
    MppPreprocessResult preprocess(const std::vector<uint8_t>& jpeg_data,
                                   float* output_tensor) {
        return preprocess(jpeg_data.data(), jpeg_data.size(), output_tensor);
    }
    
    /**
     * 检查硬件是否可用
     */
    bool is_hw_available() const { return hw_available_; }
    
    /**
     * 获取配置
     */
    const MppPreprocessorConfig& config() const { return config_; }
    
    /**
     * 关闭并释放资源
     */
    void shutdown();
    
    /**
     * 静态方法: 检测 K3 MPP 硬件是否可用
     */
    static bool detect_hw();

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    MppPreprocessorConfig config_;
    bool initialized_ = false;
    bool hw_available_ = false;
};

/**
 * 创建预处理器的工厂函数
 * 自动检测硬件可用性
 */
std::unique_ptr<K3MppPreprocessor> create_mpp_preprocessor(
    const MppPreprocessorConfig& config);

} // namespace benchmark
} // namespace rivision

#endif // K3_MPP_PREPROCESSOR_H
