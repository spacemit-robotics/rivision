/**
 * K3 MPP Hardware Accelerated Preprocessor Implementation
 * 
 * 使用 SpacemiT K3 RISC-V 的 MPP 库:
 * - VDEC (K1_JPU): 硬件 JPEG 解码
 * - G2D (K1_V2D): 硬件 resize + 色彩空间转换
 */

#include "k3_mpp_preprocessor.h"
#include <chrono>
#include <cstring>
#include <cstdio>

// ★★★ OpenCV 必须在 MPP 头文件之前包含 ★★★
// MPP 的 log.h 定义了 error/info 等宏，会与 OpenCV 冲突
#include <opencv2/opencv.hpp>

// 条件编译: 仅在 K3 平台且启用 MPP 时使用硬件
#ifdef USE_K3_MPP

extern "C" {
#include "vdec.h"
#include "g2d.h"
#include "frame.h"
#include "packet.h"
#include "para.h"
}

// ★ 取消 MPP log.h 中定义的冲突宏
#ifdef error
#undef error
#endif
#ifdef info
#undef info
#endif
#ifdef debug
#undef debug
#endif

#endif // USE_K3_MPP

namespace rivision {
namespace benchmark {

using Clock = std::chrono::high_resolution_clock;
using Duration = std::chrono::duration<double, std::milli>;

// ============================================================
// 实现细节
// ============================================================

struct K3MppPreprocessor::Impl {
#ifdef USE_K3_MPP
    // MPP 上下文
    MppVdecCtx* vdec_ctx = nullptr;     // JPEG 解码器
    MppG2dCtx* g2d_ctx = nullptr;       // 2D 图形处理器
    
    // 帧缓冲
    MppPacket* input_packet = nullptr;  // 输入 JPEG 包
    MppFrame* decode_frame = nullptr;   // 解码输出帧 (NV12)
    MppFrame* rgb_frame = nullptr;      // RGB 输出帧
    
    // 解码参数缓存
    int last_decode_width = 0;
    int last_decode_height = 0;
#endif
    
    // 配置
    int target_w = 640;
    int target_h = 640;
    int cpu_threads = 4;
    
    // 状态
    bool hw_decode_ok = false;
    bool hw_resize_ok = false;
    
    // 统计
    int64_t hw_decode_count = 0;
    int64_t sw_decode_count = 0;
    int64_t hw_resize_count = 0;
    int64_t sw_resize_count = 0;
    
    ~Impl() {
        cleanup();
    }
    
    void cleanup() {
#ifdef USE_K3_MPP
        if (input_packet) {
            PACKET_Free(input_packet);
            PACKET_Destory(input_packet);
            input_packet = nullptr;
        }
        if (decode_frame) {
            FRAME_Free(decode_frame);
            FRAME_Destory(decode_frame);
            decode_frame = nullptr;
        }
        if (rgb_frame) {
            FRAME_Free(rgb_frame);
            FRAME_Destory(rgb_frame);
            rgb_frame = nullptr;
        }
        if (vdec_ctx) {
            VDEC_DestoryChannel(vdec_ctx);
            vdec_ctx = nullptr;
        }
        if (g2d_ctx) {
            G2D_DestoryChannel(g2d_ctx);
            g2d_ctx = nullptr;
        }
#endif
    }
    
    // ★ 软件回退: OpenCV 预处理
    MppPreprocessResult opencv_preprocess(const uint8_t* jpeg_data, size_t jpeg_size,
                                          float* output_tensor) {
        MppPreprocessResult result;
        auto start = Clock::now();
        
        // 1. 解码
        auto decode_start = Clock::now();
        std::vector<uint8_t> buf(jpeg_data, jpeg_data + jpeg_size);
        cv::Mat img = cv::imdecode(buf, cv::IMREAD_COLOR);
        if (img.empty()) {
            result.error = "OpenCV: Failed to decode JPEG";
            return result;
        }
        result.orig_width = img.cols;
        result.orig_height = img.rows;
        result.decode_ms = Duration(Clock::now() - decode_start).count();
        sw_decode_count++;
        
        // 2. Resize
        auto resize_start = Clock::now();
        cv::Mat resized;
        if (img.cols == target_w && img.rows == target_h) {
            resized = img;
        } else {
            cv::resize(img, resized, cv::Size(target_w, target_h), 0, 0, cv::INTER_LINEAR);
        }
        result.resize_ms = Duration(Clock::now() - resize_start).count();
        sw_resize_count++;
        
        // 3. 归一化 + BGR→RGB + HWC→CHW
        auto norm_start = Clock::now();
        const int plane_size = target_h * target_w;
        const float scale = 1.0f / 255.0f;
        
        float* ptr_r = output_tensor;
        float* ptr_g = output_tensor + plane_size;
        float* ptr_b = output_tensor + 2 * plane_size;
        
        // 使用 OpenCV parallel_for_ 加速
        cv::parallel_for_(cv::Range(0, target_h), [&](const cv::Range& range) {
            for (int y = range.start; y < range.end; y++) {
                const uint8_t* row = resized.ptr<uint8_t>(y);
                float* r = ptr_r + y * target_w;
                float* g = ptr_g + y * target_w;
                float* b = ptr_b + y * target_w;
                
                for (int x = 0; x < target_w; x++) {
                    r[x] = row[2] * scale;  // R (BGR→RGB)
                    g[x] = row[1] * scale;  // G
                    b[x] = row[0] * scale;  // B
                    row += 3;
                }
            }
        }, cpu_threads);
        
        result.normalize_ms = Duration(Clock::now() - norm_start).count();
        result.total_ms = Duration(Clock::now() - start).count();
        result.success = true;
        
        return result;
    }
    
#ifdef USE_K3_MPP
    // ★ 硬件加速: MPP JPEG 解码
    bool hw_jpeg_decode(const uint8_t* jpeg_data, size_t jpeg_size,
                        MppFrame* out_frame, double& decode_ms) {
        auto start = Clock::now();
        
        // 准备输入包
        if (!input_packet) {
            input_packet = PACKET_Create();
            if (!input_packet) return false;
            // 预分配足够大的缓冲区 (最大 16MB JPEG)
            if (PACKET_Alloc(input_packet, 16 * 1024 * 1024) != MPP_OK) {
                PACKET_Destory(input_packet);
                input_packet = nullptr;
                return false;
            }
        }
        
        // 复制 JPEG 数据
        void* pkt_data = PACKET_GetDataPointer(input_packet);
        memcpy(pkt_data, jpeg_data, jpeg_size);
        PACKET_SetLength(input_packet, static_cast<int>(jpeg_size));
        
        // 同步解码
        S32 ret = VDEC_Process(vdec_ctx, PACKET_GetBaseData(input_packet),
                               FRAME_GetBaseData(out_frame));
        
        decode_ms = Duration(Clock::now() - start).count();
        
        if (ret != MPP_OK) {
            printf("[MPP] VDEC_Process failed: %d\n", ret);
            return false;
        }
        
        hw_decode_count++;
        return true;
    }
    
    // ★ 硬件加速: MPP resize + CSC
    bool hw_resize_csc(MppFrame* in_frame, MppFrame* out_frame,
                       int src_w, int src_h, double& resize_ms) {
        auto start = Clock::now();
        
        // 设置 G2D 参数
        g2d_ctx->stG2dPara.eG2dCmd = MPP_G2D_CMD_SCALE;
        g2d_ctx->stG2dPara.nInputWidth = src_w;
        g2d_ctx->stG2dPara.nInputHeight = src_h;
        g2d_ctx->stG2dPara.nOutputWidth = target_w;
        g2d_ctx->stG2dPara.nOutputHeight = target_h;
        
        // 源区域 (全图)
        g2d_ctx->stG2dPara.sScalePara.sSrcPosition.nXmin = 0;
        g2d_ctx->stG2dPara.sScalePara.sSrcPosition.nXmax = src_w;
        g2d_ctx->stG2dPara.sScalePara.sSrcPosition.nYmin = 0;
        g2d_ctx->stG2dPara.sScalePara.sSrcPosition.nYmax = src_h;
        
        // 目标区域
        g2d_ctx->stG2dPara.sScalePara.sDstPosition.nXmin = 0;
        g2d_ctx->stG2dPara.sScalePara.sDstPosition.nXmax = target_w;
        g2d_ctx->stG2dPara.sScalePara.sDstPosition.nYmin = 0;
        g2d_ctx->stG2dPara.sScalePara.sDstPosition.nYmax = target_h;
        
        // 自定义缩放比例
        g2d_ctx->stG2dPara.sScalePara.eScale = MPP_SCALE_CUSTOM;
        
        // 执行处理
        S32 ret = G2D_Process(g2d_ctx, FRAME_GetBaseData(in_frame),
                              FRAME_GetBaseData(out_frame));
        
        resize_ms = Duration(Clock::now() - start).count();
        
        if (ret != G2D_RESULT_OK) {
            printf("[MPP] G2D_Process failed: %d\n", ret);
            return false;
        }
        
        hw_resize_count++;
        return true;
    }
    
    // ★ 从 MPP 帧归一化到 tensor (CPU)
    void normalize_frame_to_tensor(MppFrame* frame, float* output_tensor, double& normalize_ms) {
        auto start = Clock::now();
        
        const int plane_size = target_h * target_w;
        const float scale = 1.0f / 255.0f;
        
        // 获取 RGB 数据
        uint8_t* rgb_data = static_cast<uint8_t*>(FRAME_GetDataPointer(frame, 0));
        int stride = FRAME_GetLineStride(frame);
        
        float* ptr_r = output_tensor;
        float* ptr_g = output_tensor + plane_size;
        float* ptr_b = output_tensor + 2 * plane_size;
        
        // RGB 格式直接转换 (不需要 BGR→RGB)
        cv::parallel_for_(cv::Range(0, target_h), [&](const cv::Range& range) {
            for (int y = range.start; y < range.end; y++) {
                const uint8_t* row = rgb_data + y * stride;
                float* r = ptr_r + y * target_w;
                float* g = ptr_g + y * target_w;
                float* b = ptr_b + y * target_w;
                
                for (int x = 0; x < target_w; x++) {
                    r[x] = row[0] * scale;  // R
                    g[x] = row[1] * scale;  // G
                    b[x] = row[2] * scale;  // B
                    row += 3;
                }
            }
        }, cpu_threads);
        
        normalize_ms = Duration(Clock::now() - start).count();
    }
#endif // USE_K3_MPP
};

// ============================================================
// K3MppPreprocessor 实现
// ============================================================

K3MppPreprocessor::K3MppPreprocessor()
    : impl_(std::make_unique<Impl>()) {
    printf("[K3MPP] Constructor called\n");
    fflush(stdout);
}

K3MppPreprocessor::~K3MppPreprocessor() {
    shutdown();
}

bool K3MppPreprocessor::detect_hw() {
#ifdef USE_K3_MPP
    // 尝试创建 VDEC 通道检测硬件
    MppVdecCtx* test_ctx = VDEC_CreateChannel();
    if (test_ctx) {
        test_ctx->eCodecType = CODEC_K1_JPU;
        test_ctx->stVdecPara.eCodingType = CODING_JPEG;
        test_ctx->stVdecPara.nWidth = 640;
        test_ctx->stVdecPara.nHeight = 480;
        test_ctx->stVdecPara.eOutputPixelFormat = PIXEL_FORMAT_NV12;
        
        S32 ret = VDEC_Init(test_ctx);
        VDEC_DestoryChannel(test_ctx);
        
        if (ret == MPP_OK) {
            printf("[K3MPP] Hardware detected: K1 JPU available\n");
            return true;
        }
    }
    printf("[K3MPP] Hardware not available\n");
    return false;
#else
    // 未编译 MPP 支持
    printf("[K3MPP] Not compiled with USE_K3_MPP\n");
    return false;
#endif
}

bool K3MppPreprocessor::init(const MppPreprocessorConfig& config) {
    printf("[K3MPP] init() called\n");
    fflush(stdout);
    
    config_ = config;
    impl_->target_w = config.target_width;
    impl_->target_h = config.target_height;
    impl_->cpu_threads = config.cpu_threads;
    
    // 设置 OpenCV 线程数
    cv::setNumThreads(config.cpu_threads);
    printf("[K3MPP] OpenCV threads set to %d\n", config.cpu_threads);
    fflush(stdout);
    
#ifdef USE_K3_MPP
    printf("[K3MPP] USE_K3_MPP defined, initializing hardware...\n");
    fflush(stdout);
    
    // K3 MPP VPU 说明:
    // - CODEC_V4L2_LINLONV5V7 通过 V4L2 设备 (/dev/video*) 访问 VPU
    // - 设计用于视频流编解码 (H.264/H.265/VP8/VP9/JPEG)
    // - 需要 /dev/video* 设备权限 (video 组)
    // - 对于单张 JPEG 图片预处理，使用 OpenCV 更简单可靠
    //
    // 如需启用 MPP VPU JPEG 解码:
    //   1. sudo usermod -aG video $USER
    //   2. 重新登录
    //   3. 设置环境变量 K3_MPP_VPU=1
    
    const char* use_vpu = getenv("K3_MPP_VPU");
    if (config.enable_hw_decode && use_vpu && strcmp(use_vpu, "1") == 0) {
        printf("[K3MPP] K3_MPP_VPU=1, attempting VPU JPEG decode...\n");
        printf("[K3MPP] Note: Requires /dev/video* access (video group)\n");
        fflush(stdout);
        
        impl_->vdec_ctx = VDEC_CreateChannel();
        if (impl_->vdec_ctx) {
            impl_->vdec_ctx->eCodecType = CODEC_V4L2_LINLONV5V7;
            impl_->vdec_ctx->stVdecPara.eCodingType = CODING_JPEG;
            impl_->vdec_ctx->stVdecPara.nWidth = 4096;
            impl_->vdec_ctx->stVdecPara.nHeight = 4096;
            impl_->vdec_ctx->stVdecPara.nScale = 1;
            impl_->vdec_ctx->stVdecPara.eOutputPixelFormat = PIXEL_FORMAT_NV12;
            
            S32 init_ret = VDEC_Init(impl_->vdec_ctx);
            if (init_ret == MPP_OK) {
                impl_->hw_decode_ok = true;
                printf("[K3MPP] VPU JPEG decode initialized\n");
            } else {
                printf("[K3MPP] VPU init failed (ret=%d), using OpenCV\n", init_ret);
                VDEC_DestoryChannel(impl_->vdec_ctx);
                impl_->vdec_ctx = nullptr;
            }
        }
    } else {
        printf("[K3MPP] Using OpenCV for JPEG decode (fast, no device permission needed)\n");
        fflush(stdout);
    }
    
    // K3 没有 G2D/V2D 硬件，使用 OpenCV 做 resize
    printf("[K3MPP] Using OpenCV for resize (K3 has no G2D hardware)\n");
    fflush(stdout);
    
    // 预分配解码帧
    if (impl_->hw_decode_ok) {
        impl_->decode_frame = FRAME_Create();
        if (impl_->decode_frame) {
            FRAME_SetBufferType(impl_->decode_frame, MPP_FRAME_BUFFERTYPE_DMABUF_INTERNAL);
        }
    }
    
    // 预分配 RGB 输出帧
    if (impl_->hw_resize_ok) {
        impl_->rgb_frame = FRAME_Create();
        if (impl_->rgb_frame) {
            FRAME_SetBufferType(impl_->rgb_frame, MPP_FRAME_BUFFERTYPE_DMABUF_INTERNAL);
            FRAME_Alloc(impl_->rgb_frame, PIXEL_FORMAT_RGB_888, 
                        config.target_width, config.target_height);
        }
    }
    
    hw_available_ = impl_->hw_decode_ok || impl_->hw_resize_ok;
    
    printf("[K3MPP] Init complete: HW_Decode=%s, HW_Resize=%s\n",
           impl_->hw_decode_ok ? "ON" : "OFF",
           impl_->hw_resize_ok ? "ON" : "OFF");
    
#else
    // 无 MPP 支持，仅 OpenCV
    hw_available_ = false;
    printf("[K3MPP] Running in software-only mode (OpenCV)\n");
#endif
    
    initialized_ = true;
    return true;
}

MppPreprocessResult K3MppPreprocessor::preprocess(
    const uint8_t* jpeg_data, size_t jpeg_size, float* output_tensor) {
    
    if (!initialized_) {
        MppPreprocessResult result;
        result.error = "Preprocessor not initialized";
        return result;
    }
    
#ifdef USE_K3_MPP
    // 尝试硬件加速路径
    if (impl_->hw_decode_ok && impl_->hw_resize_ok) {
        MppPreprocessResult result;
        auto start = Clock::now();
        
        // 1. 硬件 JPEG 解码
        if (!impl_->hw_jpeg_decode(jpeg_data, jpeg_size, 
                                    impl_->decode_frame, result.decode_ms)) {
            // 回退到 OpenCV
            printf("[K3MPP] HW decode failed, fallback to OpenCV\n");
            return impl_->opencv_preprocess(jpeg_data, jpeg_size, output_tensor);
        }
        
        result.orig_width = FRAME_GetWidth(impl_->decode_frame);
        result.orig_height = FRAME_GetHeight(impl_->decode_frame);
        
        // 2. 硬件 resize + CSC (NV12 → RGB)
        if (!impl_->hw_resize_csc(impl_->decode_frame, impl_->rgb_frame,
                                   result.orig_width, result.orig_height,
                                   result.resize_ms)) {
            // 回退到 OpenCV (从 NV12 转换)
            printf("[K3MPP] HW resize failed, fallback to OpenCV\n");
            return impl_->opencv_preprocess(jpeg_data, jpeg_size, output_tensor);
        }
        
        // 3. CPU 归一化 (HW 处理后只需轻量转换)
        impl_->normalize_frame_to_tensor(impl_->rgb_frame, output_tensor,
                                          result.normalize_ms);
        
        result.total_ms = Duration(Clock::now() - start).count();
        result.success = true;
        
        return result;
    }
#endif
    
    // OpenCV 软件回退
    return impl_->opencv_preprocess(jpeg_data, jpeg_size, output_tensor);
}

void K3MppPreprocessor::shutdown() {
    if (impl_) {
        // 打印统计
        printf("[K3MPP] Statistics: HW_decode=%ld, SW_decode=%ld, HW_resize=%ld, SW_resize=%ld\n",
               impl_->hw_decode_count, impl_->sw_decode_count,
               impl_->hw_resize_count, impl_->sw_resize_count);
        
        impl_->cleanup();
    }
    initialized_ = false;
}

// ============================================================
// 工厂函数
// ============================================================

std::unique_ptr<K3MppPreprocessor> create_mpp_preprocessor(
    const MppPreprocessorConfig& config) {
    
    printf("[K3MPP] create_mpp_preprocessor() called\n");
    fflush(stdout);
    
    printf("[K3MPP] Creating K3MppPreprocessor object...\n");
    fflush(stdout);
    
    auto preproc = std::make_unique<K3MppPreprocessor>();
    
    printf("[K3MPP] K3MppPreprocessor created, calling init()...\n");
    fflush(stdout);
    
    if (preproc->init(config)) {
        printf("[K3MPP] init() succeeded\n");
        fflush(stdout);
        return preproc;
    }
    printf("[K3MPP] init() failed, returning nullptr\n");
    fflush(stdout);
    return nullptr;
}

} // namespace benchmark
} // namespace rivision
