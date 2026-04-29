#include "ffmpeg_preprocessor.h"
#include <chrono>
#include <cstdlib>
#include <cstring>
#include <unistd.h>
#include <vector>

#ifdef USE_LIBYUV
#include <libyuv.h>
#endif

extern "C" {
#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libswscale/swscale.h>
#include <libavutil/imgutils.h>
#include <libavutil/opt.h>
}

namespace rivision {
namespace benchmark {

struct FFmpegPreprocessor::Impl {
    Config config;
    
    // Decoder context
    const AVCodec* decoder = nullptr;
    AVCodecContext* dec_ctx = nullptr;
    AVCodecParserContext* parser = nullptr;
    
    // Scaling context
    SwsContext* sws_ctx = nullptr;
    
    // Frames
    AVFrame* decoded_frame = nullptr;
    AVFrame* scaled_frame = nullptr;
    
    // Packet for decoding
    AVPacket* packet = nullptr;
    
    // Intermediate buffer for RGB output
    std::vector<uint8_t> rgb_buffer;
    
    // Decoder name
    std::string decoder_name;
    
    ~Impl() {
        cleanup();
    }
    
    void cleanup() {
        if (sws_ctx) {
            sws_freeContext(sws_ctx);
            sws_ctx = nullptr;
        }
        if (decoded_frame) {
            av_frame_free(&decoded_frame);
        }
        if (scaled_frame) {
            av_frame_free(&scaled_frame);
        }
        if (packet) {
            av_packet_free(&packet);
        }
        if (parser) {
            av_parser_close(parser);
            parser = nullptr;
        }
        if (dec_ctx) {
            avcodec_free_context(&dec_ctx);
        }
    }
};

FFmpegPreprocessor::FFmpegPreprocessor() : impl_(std::make_unique<Impl>()) {
}

FFmpegPreprocessor::~FFmpegPreprocessor() = default;

bool FFmpegPreprocessor::init(const Config& config) {
    impl_->config = config;
    
    // ★ 抑制 MPP-DEBUG/ERROR 日志
    setenv("MPP_LOG_LEVEL", "error", 0);  // 仅显示严重错误
    av_log_set_level(AV_LOG_FATAL);       // 仅显示致命错误
    
    // Check environment variable for decoder selection
    const char* force_sw = std::getenv("FFMPEG_FORCE_SW");
    bool use_software_only = (force_sw && std::atoi(force_sw) != 0);
    
    // Try to find hardware MJPEG decoder first (K3: mjpeg_stcodec)
    if (config.prefer_hw_decoder && !use_software_only) {
        impl_->decoder = avcodec_find_decoder_by_name("mjpeg_stcodec");
        if (impl_->decoder) {
            hw_decode_enabled_ = true;
            impl_->decoder_name = "mjpeg_stcodec";
        }
    }
    
    // Fallback to software decoder
    if (!impl_->decoder) {
        impl_->decoder = avcodec_find_decoder(AV_CODEC_ID_MJPEG);
        if (!impl_->decoder) {
            printf("[FFmpeg] ERROR: No MJPEG decoder found!\n");
            return false;
        }
        impl_->decoder_name = impl_->decoder->name;
        hw_decode_enabled_ = false;
    }
    
    // Allocate codec context
    impl_->dec_ctx = avcodec_alloc_context3(impl_->decoder);
    if (!impl_->dec_ctx) {
        printf("[FFmpeg] ERROR: Failed to allocate decoder context\n");
        return false;
    }
    
    // Set threading options for software decoder
    if (!hw_decode_enabled_) {
        impl_->dec_ctx->thread_count = config.cpu_threads;
        impl_->dec_ctx->thread_type = FF_THREAD_FRAME | FF_THREAD_SLICE;
    }
    
    // Open codec
    int ret = avcodec_open2(impl_->dec_ctx, impl_->decoder, nullptr);
    bool need_fallback = (ret < 0);
    
    if (ret < 0) {
        need_fallback = true;
    }
    
    // Try fallback to software if hardware failed
    if (need_fallback && hw_decode_enabled_) {
        if (impl_->dec_ctx) {
            avcodec_free_context(&impl_->dec_ctx);
        }
        
        impl_->decoder = avcodec_find_decoder(AV_CODEC_ID_MJPEG);
        if (impl_->decoder) {
            impl_->dec_ctx = avcodec_alloc_context3(impl_->decoder);
            impl_->dec_ctx->thread_count = config.cpu_threads;
            impl_->dec_ctx->thread_type = FF_THREAD_FRAME | FF_THREAD_SLICE;
            ret = avcodec_open2(impl_->dec_ctx, impl_->decoder, nullptr);
            if (ret >= 0) {
                impl_->decoder_name = impl_->decoder->name;
                hw_decode_enabled_ = false;
                need_fallback = false;
            }
        }
    }
    
    if (need_fallback && ret < 0) {
        printf("[FFmpeg] ERROR: All decoders failed\n");
        return false;
    }
    
    // Allocate frames
    impl_->decoded_frame = av_frame_alloc();
    impl_->scaled_frame = av_frame_alloc();
    impl_->packet = av_packet_alloc();
    
    if (!impl_->decoded_frame || !impl_->scaled_frame || !impl_->packet) {
        printf("[FFmpeg] ERROR: Failed to allocate frames/packet\n");
        return false;
    }
    
    // Pre-allocate scaled frame buffer (RGB24)
    impl_->scaled_frame->format = AV_PIX_FMT_RGB24;
    impl_->scaled_frame->width = config.target_width;
    impl_->scaled_frame->height = config.target_height;
    ret = av_frame_get_buffer(impl_->scaled_frame, 32);
    if (ret < 0) {
        printf("[FFmpeg] ERROR: Failed to allocate scaled frame buffer\n");
        return false;
    }
    
    // Pre-allocate RGB buffer
    impl_->rgb_buffer.resize(config.target_width * config.target_height * 3);
    
    // FFmpeg 初始化完成
    return true;
}

bool FFmpegPreprocessor::preprocess(const uint8_t* jpeg_data, size_t jpeg_size,
                                     float* output_buffer,
                                     int target_width, int target_height) {
    if (!impl_->dec_ctx || !jpeg_data || !output_buffer) {
        return false;
    }
    
    // Setup packet with JPEG data
    impl_->packet->data = const_cast<uint8_t*>(jpeg_data);
    impl_->packet->size = static_cast<int>(jpeg_size);
    
    // ★ 开始计时：只测量 send_packet + receive_frame
    auto t0 = std::chrono::high_resolution_clock::now();
    
    // Send packet to decoder
    int ret = avcodec_send_packet(impl_->dec_ctx, impl_->packet);
    if (ret == AVERROR(EAGAIN)) {
        // 解码器缓冲区满，drain 输出后重试
        while (avcodec_receive_frame(impl_->dec_ctx, impl_->decoded_frame) == 0) {
            av_frame_unref(impl_->decoded_frame);
        }
        ret = avcodec_send_packet(impl_->dec_ctx, impl_->packet);
    }
    if (ret < 0 && ret != AVERROR(EAGAIN)) {
        char errbuf[256];
        av_strerror(ret, errbuf, sizeof(errbuf));
        printf("[FFmpeg] ERROR: send_packet failed: %s\n", errbuf);
        return false;
    }
    
    // Receive decoded frame - V4L2 M2M 需要等待硬件完成
    int retry_count = 0;
    const int max_retries = 100;  // ★ 足够的重试次数
    
    while (retry_count < max_retries) {
        ret = avcodec_receive_frame(impl_->dec_ctx, impl_->decoded_frame);
        
        if (ret == 0) {
            break;  // Success
        } else if (ret == AVERROR(EAGAIN)) {
            retry_count++;
            // ★ 不使用 usleep，直接重试（V4L2 内部会阻塞等待）
            continue;
        } else {
            char errbuf[256];
            av_strerror(ret, errbuf, sizeof(errbuf));
            printf("[FFmpeg] ERROR: receive_frame failed: %s\n", errbuf);
            return false;
        }
    }
    
    if (ret != 0) {
        printf("[FFmpeg] ERROR: receive_frame timeout after %d retries\n", max_retries);
        return false;
    }
    
    auto t1 = std::chrono::high_resolution_clock::now();
    last_decode_time_ms_ = std::chrono::duration<double, std::milli>(t1 - t0).count();
    
    // Save original dimensions
    last_orig_width_ = impl_->decoded_frame->width;
    last_orig_height_ = impl_->decoded_frame->height;
    
    const int src_width = impl_->decoded_frame->width;
    const int src_height = impl_->decoded_frame->height;
    const AVPixelFormat src_format = static_cast<AVPixelFormat>(impl_->decoded_frame->format);
    
    auto t2 = std::chrono::high_resolution_clock::now();
    
#ifdef USE_LIBYUV
    // ★★★ libyuv 方案: RVV 优化的 YUV 缩放和颜色转换 ★★★
    // 支持的格式:
    //   - YUV420P, YUVJ420P (4:2:0)
    //   - YUV422P, YUVJ422P (4:2:2) - JPEG 常见格式
    //   - YUV444P, YUVJ444P (4:4:4)
    //   - NV12, NV21
    bool is_420 = (src_format == AV_PIX_FMT_YUV420P || src_format == AV_PIX_FMT_YUVJ420P);
    bool is_422 = (src_format == AV_PIX_FMT_YUV422P || src_format == AV_PIX_FMT_YUVJ422P);
    bool is_444 = (src_format == AV_PIX_FMT_YUV444P || src_format == AV_PIX_FMT_YUVJ444P);
    bool is_nv = (src_format == AV_PIX_FMT_NV12 || src_format == AV_PIX_FMT_NV21);
    bool use_libyuv = is_420 || is_422 || is_444 || is_nv;
    
    if (use_libyuv) {
        last_scaler_name_ = "libyuv";
        
        // 分配缩放后的 YUV 缓冲区 (I420 格式)
        const int dst_uv_w = target_width / 2;
        const int dst_uv_h = target_height / 2;
        std::vector<uint8_t> scaled_y(target_width * target_height);
        std::vector<uint8_t> scaled_u(dst_uv_w * dst_uv_h);
        std::vector<uint8_t> scaled_v(dst_uv_w * dst_uv_h);
        
        // 源 YUV 数据指针 (将转换为 I420)
        const uint8_t* src_y = impl_->decoded_frame->data[0];
        const uint8_t* src_u = impl_->decoded_frame->data[1];
        const uint8_t* src_v = impl_->decoded_frame->data[2];
        int src_stride_y = impl_->decoded_frame->linesize[0];
        int src_stride_u = impl_->decoded_frame->linesize[1];
        int src_stride_v = impl_->decoded_frame->linesize[2];
        
        // 临时 I420 缓冲区 (用于 422/444/NV12 转换)
        std::vector<uint8_t> tmp_y, tmp_u, tmp_v;
        const int src_uv_w_420 = src_width / 2;
        const int src_uv_h_420 = src_height / 2;
        
        if (is_422) {
            // YUV422P -> I420: 需要垂直下采样 U/V
            tmp_u.resize(src_uv_w_420 * src_uv_h_420);
            tmp_v.resize(src_uv_w_420 * src_uv_h_420);
            
            libyuv::I422ToI420(
                src_y, src_stride_y,
                src_u, src_stride_u,
                src_v, src_stride_v,
                const_cast<uint8_t*>(src_y), src_stride_y,  // Y 不变
                tmp_u.data(), src_uv_w_420,
                tmp_v.data(), src_uv_w_420,
                src_width, src_height);
            
            src_u = tmp_u.data();
            src_v = tmp_v.data();
            src_stride_u = src_uv_w_420;
            src_stride_v = src_uv_w_420;
        } else if (is_444) {
            // YUV444P -> I420: 需要水平和垂直下采样 U/V
            tmp_u.resize(src_uv_w_420 * src_uv_h_420);
            tmp_v.resize(src_uv_w_420 * src_uv_h_420);
            
            libyuv::I444ToI420(
                src_y, src_stride_y,
                src_u, src_stride_u,
                src_v, src_stride_v,
                const_cast<uint8_t*>(src_y), src_stride_y,  // Y 不变
                tmp_u.data(), src_uv_w_420,
                tmp_v.data(), src_uv_w_420,
                src_width, src_height);
            
            src_u = tmp_u.data();
            src_v = tmp_v.data();
            src_stride_u = src_uv_w_420;
            src_stride_v = src_uv_w_420;
        } else if (is_nv) {
            // NV12/NV21 -> I420: 分离 UV 平面
            tmp_u.resize(src_uv_w_420 * src_uv_h_420);
            tmp_v.resize(src_uv_w_420 * src_uv_h_420);
            
            const uint8_t* uv_plane = impl_->decoded_frame->data[1];
            int uv_stride = impl_->decoded_frame->linesize[1];
            
            if (src_format == AV_PIX_FMT_NV12) {
                libyuv::SplitUVPlane(uv_plane, uv_stride,
                                     tmp_u.data(), src_uv_w_420,
                                     tmp_v.data(), src_uv_w_420,
                                     src_uv_w_420, src_uv_h_420);
            } else {  // NV21
                libyuv::SplitUVPlane(uv_plane, uv_stride,
                                     tmp_v.data(), src_uv_w_420,
                                     tmp_u.data(), src_uv_w_420,
                                     src_uv_w_420, src_uv_h_420);
            }
            src_u = tmp_u.data();
            src_v = tmp_v.data();
            src_stride_u = src_uv_w_420;
            src_stride_v = src_uv_w_420;
        }
        // else: is_420, 直接使用原始数据
        
        // 1. I420Scale: 缩放 YUV420P (RVV 优化)
        libyuv::I420Scale(
            src_y, src_stride_y,
            src_u, src_stride_u,
            src_v, src_stride_v,
            src_width, src_height,
            scaled_y.data(), target_width,
            scaled_u.data(), dst_uv_w,
            scaled_v.data(), dst_uv_w,
            target_width, target_height,
            libyuv::kFilterNone);  // 最快的最近邻
        
        // 2. I420ToRGB24: 颜色转换 (RVV 优化)
        std::vector<uint8_t> rgb_buf(target_width * target_height * 3);
        libyuv::I420ToRGB24(
            scaled_y.data(), target_width,
            scaled_u.data(), dst_uv_w,
            scaled_v.data(), dst_uv_w,
            rgb_buf.data(), target_width * 3,
            target_width, target_height);
        
        auto t3 = std::chrono::high_resolution_clock::now();
        last_scale_time_ms_ = std::chrono::duration<double, std::milli>(t3 - t2).count();
        
        // 3. RGB uint8 → float32 NCHW
        const int total_pixels = target_width * target_height;
        float* r_plane = output_buffer;
        float* g_plane = output_buffer + total_pixels;
        float* b_plane = output_buffer + total_pixels * 2;
        constexpr float scale_factor = 1.0f / 255.0f;
        
        const uint8_t* src = rgb_buf.data();
        for (int i = 0; i < total_pixels; ++i) {
            r_plane[i] = src[0] * scale_factor;
            g_plane[i] = src[1] * scale_factor;
            b_plane[i] = src[2] * scale_factor;
            src += 3;
        }
        
        auto t4 = std::chrono::high_resolution_clock::now();
        last_convert_time_ms_ = std::chrono::duration<double, std::milli>(t4 - t3).count();
    } else
#endif
    {
        // ★ swscale 回退方案 ★
        last_scaler_name_ = "swscale";
        if (!impl_->sws_ctx || 
            impl_->decoded_frame->width != impl_->dec_ctx->width ||
            impl_->decoded_frame->height != impl_->dec_ctx->height) {
            
            if (impl_->sws_ctx) {
                sws_freeContext(impl_->sws_ctx);
            }
            
            impl_->sws_ctx = sws_getContext(
                src_width, src_height, src_format,
                target_width, target_height,
                AV_PIX_FMT_RGB24,
                SWS_POINT, nullptr, nullptr, nullptr);
            
            if (!impl_->sws_ctx) {
                printf("[FFmpeg] ERROR: Failed to create scaling context\n");
                return false;
            }
        }
        
        if (impl_->scaled_frame->width != target_width || 
            impl_->scaled_frame->height != target_height) {
            av_frame_unref(impl_->scaled_frame);
            impl_->scaled_frame->format = AV_PIX_FMT_RGB24;
            impl_->scaled_frame->width = target_width;
            impl_->scaled_frame->height = target_height;
            av_frame_get_buffer(impl_->scaled_frame, 32);
        }
        
        ret = sws_scale(impl_->sws_ctx,
                        impl_->decoded_frame->data, impl_->decoded_frame->linesize,
                        0, src_height,
                        impl_->scaled_frame->data, impl_->scaled_frame->linesize);
        
        if (ret <= 0) {
            printf("[FFmpeg] ERROR: sws_scale failed\n");
            return false;
        }
        
        auto t3 = std::chrono::high_resolution_clock::now();
        last_scale_time_ms_ = std::chrono::duration<double, std::milli>(t3 - t2).count();
        
        // RGB uint8 → float32 NCHW
        const int total_pixels = target_width * target_height;
        const uint8_t* rgb_ptr = impl_->scaled_frame->data[0];
        const int linesize = impl_->scaled_frame->linesize[0];
        
        float* r_plane = output_buffer;
        float* g_plane = output_buffer + total_pixels;
        float* b_plane = output_buffer + total_pixels * 2;
        constexpr float scale_factor = 1.0f / 255.0f;
        
        if (linesize == target_width * 3) {
            const uint8_t* src = rgb_ptr;
            for (int i = 0; i < total_pixels; ++i) {
                r_plane[i] = src[0] * scale_factor;
                g_plane[i] = src[1] * scale_factor;
                b_plane[i] = src[2] * scale_factor;
                src += 3;
            }
        } else {
            for (int y = 0; y < target_height; ++y) {
                const uint8_t* row = rgb_ptr + y * linesize;
                const int row_offset = y * target_width;
                for (int x = 0; x < target_width; ++x) {
                    const int idx = row_offset + x;
                    const uint8_t* pixel = row + x * 3;
                    r_plane[idx] = pixel[0] * scale_factor;
                    g_plane[idx] = pixel[1] * scale_factor;
                    b_plane[idx] = pixel[2] * scale_factor;
                }
            }
        }
        
        auto t4 = std::chrono::high_resolution_clock::now();
        last_convert_time_ms_ = std::chrono::duration<double, std::milli>(t4 - t3).count();
    }
    
    // Unref decoded frame for next use
    av_frame_unref(impl_->decoded_frame);
    
    return true;
}

// Factory function
std::unique_ptr<IPreprocessor> create_ffmpeg_preprocessor(
    int target_width, int target_height, int cpu_threads, bool prefer_hw) {
    
    auto preprocessor = std::make_unique<FFmpegPreprocessor>();
    FFmpegPreprocessor::Config config;
    config.target_width = target_width;
    config.target_height = target_height;
    config.cpu_threads = cpu_threads;
    config.prefer_hw_decoder = prefer_hw;
    
    if (!preprocessor->init(config)) {
        printf("[FFmpeg] Failed to initialize FFmpeg preprocessor\n");
        return nullptr;
    }
    
    return preprocessor;
}

void FFmpegPreprocessor::flush() {
    if (impl_ && impl_->dec_ctx) {
        // ★ 刷新解码器，丢弃所有待处理帧
        avcodec_flush_buffers(impl_->dec_ctx);
        printf("[FFmpeg] Decoder flushed (ready for batch mode)\n");
        fflush(stdout);
    }
}

// ============================================================
// ★★★ 方案 1: 分离 decode 和 scale/convert ★★★
// ============================================================

bool FFmpegPreprocessor::decodeOnly(const uint8_t* jpeg_data, size_t jpeg_size,
                                     DecodeResult& result) {
    if (!impl_->dec_ctx || !jpeg_data) {
        return false;
    }
    
    result.success = false;
    
    // Setup packet with JPEG data
    impl_->packet->data = const_cast<uint8_t*>(jpeg_data);
    impl_->packet->size = static_cast<int>(jpeg_size);
    
    auto t0 = std::chrono::high_resolution_clock::now();
    
    // Send packet to decoder
    int ret = avcodec_send_packet(impl_->dec_ctx, impl_->packet);
    if (ret == AVERROR(EAGAIN)) {
        while (avcodec_receive_frame(impl_->dec_ctx, impl_->decoded_frame) == 0) {
            av_frame_unref(impl_->decoded_frame);
        }
        ret = avcodec_send_packet(impl_->dec_ctx, impl_->packet);
    }
    if (ret < 0 && ret != AVERROR(EAGAIN)) {
        return false;
    }
    
    // Receive decoded frame
    int retry_count = 0;
    const int max_retries = 100;
    
    while (retry_count < max_retries) {
        ret = avcodec_receive_frame(impl_->dec_ctx, impl_->decoded_frame);
        if (ret == 0) break;
        if (ret == AVERROR(EAGAIN)) {
            retry_count++;
            continue;
        }
        return false;
    }
    
    if (ret != 0) return false;
    
    auto t1 = std::chrono::high_resolution_clock::now();
    last_decode_time_ms_ = std::chrono::duration<double, std::milli>(t1 - t0).count();
    
    // 拷贝解码结果到 DecodeResult (释放 mutex 后仍可使用)
    result.width = impl_->decoded_frame->width;
    result.height = impl_->decoded_frame->height;
    result.format = impl_->decoded_frame->format;
    
    last_orig_width_ = result.width;
    last_orig_height_ = result.height;
    
    // 拷贝 Y 平面
    result.y_stride = impl_->decoded_frame->linesize[0];
    result.y_plane.resize(result.y_stride * result.height);
    memcpy(result.y_plane.data(), impl_->decoded_frame->data[0], 
           result.y_stride * result.height);
    
    // 拷贝 U/V 平面 (对于 YUV420, 高度是一半)
    AVPixelFormat fmt = static_cast<AVPixelFormat>(result.format);
    int uv_height = result.height;
    if (fmt == AV_PIX_FMT_YUV420P || fmt == AV_PIX_FMT_YUVJ420P ||
        fmt == AV_PIX_FMT_NV12 || fmt == AV_PIX_FMT_NV21) {
        uv_height = result.height / 2;
    }
    
    if (impl_->decoded_frame->data[1]) {
        result.u_stride = impl_->decoded_frame->linesize[1];
        result.u_plane.resize(result.u_stride * uv_height);
        memcpy(result.u_plane.data(), impl_->decoded_frame->data[1],
               result.u_stride * uv_height);
    }
    
    if (impl_->decoded_frame->data[2]) {
        result.v_stride = impl_->decoded_frame->linesize[2];
        result.v_plane.resize(result.v_stride * uv_height);
        memcpy(result.v_plane.data(), impl_->decoded_frame->data[2],
               result.v_stride * uv_height);
    }
    
    // Unref decoded frame
    av_frame_unref(impl_->decoded_frame);
    
    result.success = true;
    return true;
}

bool FFmpegPreprocessor::scaleAndConvert(const DecodeResult& decoded,
                                          float* output_buffer,
                                          int target_width, int target_height) {
    if (!decoded.success || !output_buffer) {
        return false;
    }
    
    auto t2 = std::chrono::high_resolution_clock::now();
    
    const int src_width = decoded.width;
    const int src_height = decoded.height;
    const AVPixelFormat src_format = static_cast<AVPixelFormat>(decoded.format);
    
    // ★ swscale 路径 (使用拷贝的 YUV 数据)
    last_scaler_name_ = "swscale";
    
    // 创建临时 sws context
    SwsContext* sws_ctx = sws_getContext(
        src_width, src_height, src_format,
        target_width, target_height, AV_PIX_FMT_RGB24,
        SWS_POINT, nullptr, nullptr, nullptr);
    
    if (!sws_ctx) {
        return false;
    }
    
    // 准备输入数据
    const uint8_t* src_data[4] = {
        decoded.y_plane.data(),
        decoded.u_plane.data(),
        decoded.v_plane.data(),
        nullptr
    };
    int src_linesize[4] = {
        decoded.y_stride,
        decoded.u_stride,
        decoded.v_stride,
        0
    };
    
    // 分配输出缓冲区
    std::vector<uint8_t> rgb_buf(target_width * target_height * 3);
    uint8_t* dst_data[4] = { rgb_buf.data(), nullptr, nullptr, nullptr };
    int dst_linesize[4] = { target_width * 3, 0, 0, 0 };
    
    sws_scale(sws_ctx, src_data, src_linesize, 0, src_height,
              dst_data, dst_linesize);
    
    sws_freeContext(sws_ctx);
    
    auto t3 = std::chrono::high_resolution_clock::now();
    last_scale_time_ms_ = std::chrono::duration<double, std::milli>(t3 - t2).count();
    
    // RGB → float32 NCHW
    const int total_pixels = target_width * target_height;
    float* r_plane = output_buffer;
    float* g_plane = output_buffer + total_pixels;
    float* b_plane = output_buffer + total_pixels * 2;
    constexpr float scale_factor = 1.0f / 255.0f;
    
    const uint8_t* src = rgb_buf.data();
    for (int i = 0; i < total_pixels; ++i) {
        r_plane[i] = src[0] * scale_factor;
        g_plane[i] = src[1] * scale_factor;
        b_plane[i] = src[2] * scale_factor;
        src += 3;
    }
    
    auto t4 = std::chrono::high_resolution_clock::now();
    last_convert_time_ms_ = std::chrono::duration<double, std::milli>(t4 - t3).count();
    
    return true;
}

}  // namespace benchmark
}  // namespace rivision
