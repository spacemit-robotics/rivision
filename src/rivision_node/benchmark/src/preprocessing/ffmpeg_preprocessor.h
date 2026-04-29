#pragma once

#include "preprocessor_interface.h"
#include <memory>
#include <string>
#include <vector>  // ★ DecodeResult 需要

namespace rivision {
namespace benchmark {

/**
 * FFmpeg-based preprocessor with hardware JPEG decoding support
 * 
 * On K3 platform, uses mjpeg_stcodec for hardware JPEG decoding
 * Falls back to software decoder if hardware not available
 */
class FFmpegPreprocessor : public IPreprocessor {
public:
    struct Config {
        int target_width = 640;
        int target_height = 640;
        int cpu_threads = 4;
        bool prefer_hw_decoder = true;  // Use mjpeg_stcodec on K3
    };
    
    FFmpegPreprocessor();
    ~FFmpegPreprocessor() override;
    
    bool init(const Config& config);
    
    // IPreprocessor interface
    bool preprocess(const uint8_t* jpeg_data, size_t jpeg_size,
                    float* output_buffer,
                    int target_width, int target_height) override;
    
    std::string getName() const override { return "FFmpeg"; }
    bool isHardwareAccelerated() const override { return hw_decode_enabled_; }
    
    // ★ 重置解码器状态 (在 warmup 和 batch 之间调用)
    void flush();
    
    // ★★★ 方案 1: 分离 decode 和 scale/convert (减少 mutex 粒度) ★★★
    // 使用方式:
    //   1. 在 mutex 内调用 decodeOnly() - 只做硬件解码 (~0.8ms)
    //   2. mutex 外调用 scaleAndConvert() - 并行执行 (~5ms)
    struct DecodeResult {
        bool success = false;
        int width = 0;
        int height = 0;
        int format = 0;  // AVPixelFormat
        std::vector<uint8_t> y_plane;
        std::vector<uint8_t> u_plane;
        std::vector<uint8_t> v_plane;
        int y_stride = 0;
        int u_stride = 0;
        int v_stride = 0;
    };
    
    // 只做硬件解码，拷贝 YUV 数据到 result (需要 mutex 保护)
    bool decodeOnly(const uint8_t* jpeg_data, size_t jpeg_size, DecodeResult& result);
    
    // 做 scale 和 convert (可以并行，无需 mutex)
    bool scaleAndConvert(const DecodeResult& decoded, float* output_buffer,
                         int target_width, int target_height);
    
    // Statistics
    double getLastDecodeTimeMs() const { return last_decode_time_ms_; }
    double getLastScaleTimeMs() const { return last_scale_time_ms_; }
    double getLastConvertTimeMs() const { return last_convert_time_ms_; }
    const char* getScalerName() const { return last_scaler_name_; }
    
    // Original image dimensions (after last preprocess)
    int getLastOrigWidth() const { return last_orig_width_; }
    int getLastOrigHeight() const { return last_orig_height_; }
    
private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    bool hw_decode_enabled_ = false;
    double last_decode_time_ms_ = 0;
    double last_scale_time_ms_ = 0;
    double last_convert_time_ms_ = 0;
    const char* last_scaler_name_ = "swscale";
    int last_orig_width_ = 0;
    int last_orig_height_ = 0;
};

// Factory function
std::unique_ptr<IPreprocessor> create_ffmpeg_preprocessor(
    int target_width = 640,
    int target_height = 640,
    int cpu_threads = 4,
    bool prefer_hw = true);

}  // namespace benchmark
}  // namespace rivision
