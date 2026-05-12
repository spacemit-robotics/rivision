// SpacemiT MPP Decoder implementation using native VDEC API
#include "mpp_decoder.h"
#include "mpp_common.h"
#include "mpp_channel_manager.h"
#include "utils/logger.h"

#ifdef USE_SPACEMIT_MPP
extern "C" {
#include "vdec/vdec_api.h"
#include "sys/sys_api.h"
#include "sys/vb_api.h"
}
#endif

#include <cstring>

namespace rivision::spacemit {

class MppDecoder::Impl {
public:
#ifdef USE_SPACEMIT_MPP
    ChannelHolder chn_holder_{ChannelManager::ChannelType::DECODER};
    S32 chn_id() const { return chn_holder_.id(); }
#endif
    bool flush_sent_ = false;
    
    ~Impl() {
        cleanup();
    }
    
    void cleanup() {
#ifdef USE_SPACEMIT_MPP
        if (chn_holder_.valid()) {
            VDEC_DisableChn(chn_id());
            VDEC_DestroyChn(chn_id());
        }
#endif
        flush_sent_ = false;
    }
};

MppDecoder::MppDecoder() : impl_(std::make_unique<Impl>()) {
#ifdef USE_SPACEMIT_MPP
    static bool vdec_initialized = false;
    if (!vdec_initialized) {
        if (VDEC_Init() == 0) {
            vdec_initialized = true;
            LOG_INFO("MPP VDEC module initialized");
        }
    }
#endif
}

MppDecoder::~MppDecoder() = default;

bool MppDecoder::open(const Config& cfg) {
    config_ = cfg;
    
#ifdef USE_SPACEMIT_MPP
    // Check if channel was allocated successfully
    if (!impl_->chn_holder_.valid()) {
        last_error_ = "No available decoder channel (max " + 
                      std::to_string(ChannelManager::MAX_DECODER_CHANNELS) + " reached)";
        LOG_ERROR("{}", last_error_);
        return false;
    }
    
    impl_->cleanup();
    
    VdecChnAttr attr;
    std::memset(&attr, 0, sizeof(attr));
    
    // Set codec type
    switch (cfg.codec) {
        case hal::CodecType::H264:
            attr.eCodecType = MPP_STREAM_CODEC_H264;
            break;
        case hal::CodecType::H265:
            attr.eCodecType = MPP_STREAM_CODEC_H265;
            break;
        case hal::CodecType::MJPEG:
            attr.eCodecType = MPP_STREAM_CODEC_MJPEG;
            break;
        default:
            last_error_ = "Unsupported codec";
            return false;
    }
    
    attr.eOutputPixelFormat = MPP_PIXEL_FORMAT_NV12;
    attr.u32Width = cfg.width;
    attr.u32Height = cfg.height;
    attr.u32Align = 16;
    
    S32 ret = VDEC_CreateChn(impl_->chn_id(), &attr);
    if (ret != 0) {
        last_error_ = "VDEC_CreateChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        return false;
    }
    
    ret = VDEC_EnableChn(impl_->chn_id());
    if (ret != 0) {
        last_error_ = "VDEC_EnableChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        VDEC_DestroyChn(impl_->chn_id());
        return false;
    }
    
    initialized_ = true;
    LOG_INFO("MPP Decoder opened: {}x{} on channel {}", cfg.width, cfg.height, impl_->chn_id());
    return true;
#else
    last_error_ = "SpacemiT MPP not available";
    return false;
#endif
}

void MppDecoder::close() {
    impl_->cleanup();
    initialized_ = false;
}

bool MppDecoder::isOpen() const {
    return initialized_;
}

bool MppDecoder::decode(const hal::StreamPacket& packet, Frame& out_frame) {
#ifdef USE_SPACEMIT_MPP
    if (!initialized_) {
        last_error_ = "Decoder not initialized";
        return false;
    }
    
    // Send stream to decoder
    StreamBufferInfo stream;
    std::memset(&stream, 0, sizeof(stream));
    stream.pu8Addr = const_cast<U8*>(packet.data.data());
    stream.u32Size = static_cast<U32>(packet.data.size());
    stream.u64PTS = packet.pts;
    stream.bKeyFrame = packet.keyframe ? MPP_TRUE : MPP_FALSE;
    
    S32 ret = VDEC_SendStream(impl_->chn_id(), &stream, 100);
    if (ret != 0 && ret != ERR_VDEC_TIMEOUT) {
        last_error_ = "VDEC_SendStream failed: " + std::to_string(ret);
        return false;
    }
    
    // Get decoded frame
    VideoFrameInfo frame_info;
    std::memset(&frame_info, 0, sizeof(frame_info));
    
    ret = VDEC_GetFrame(impl_->chn_id(), &frame_info, 100);
    if (ret == ERR_VDEC_NO_FRAME || ret == ERR_VDEC_TIMEOUT) {
        return true;  // No frame yet, not an error
    }
    if (ret != ERR_VDEC_OK) {
        last_error_ = "VDEC_GetFrame failed: " + std::to_string(ret);
        return false;
    }
    
    // Get virtual address from VB buffer
    void* virt_addr = nullptr;
    VB_GetVirAddr(frame_info.ulBufferId, &virt_addr);
    
    // Copy frame data - use CommonFrameInfo for dimensions
    out_frame.width = frame_info.stCommFrameInfo.u32Width;
    out_frame.height = frame_info.stCommFrameInfo.u32Height;
    out_frame.format = PixelFormat::NV12;
    out_frame.timestamp = frame_info.stVFrame.u64PTS;
    
    size_t frame_size = out_frame.width * out_frame.height * 3 / 2;
    std::vector<uint8_t> frame_data(frame_size);
    if (virt_addr) {
        std::memcpy(frame_data.data(), virt_addr, frame_size);
    }
    out_frame.data = std::move(frame_data);
    
    // Release frame back to decoder
    VDEC_ReleaseFrame(impl_->chn_id(), frame_info.ulBufferId);
    
    return true;
#else
    last_error_ = "SpacemiT MPP not available";
    return false;
#endif
}

bool MppDecoder::flush(Frame& out_frame) {
#ifdef USE_SPACEMIT_MPP
    if (!initialized_) return false;
    
    // Send flush signal once
    if (!impl_->flush_sent_) {
        VDEC_Flush(impl_->chn_id());
        impl_->flush_sent_ = true;
    }
    
    // Get remaining frame
    VideoFrameInfo frame_info;
    std::memset(&frame_info, 0, sizeof(frame_info));
    
    S32 ret = VDEC_GetFrame(impl_->chn_id(), &frame_info, 100);
    if (ret != ERR_VDEC_OK) {
        impl_->flush_sent_ = false;
        return false;
    }
    
    void* virt_addr = nullptr;
    VB_GetVirAddr(frame_info.ulBufferId, &virt_addr);
    
    out_frame.width = frame_info.stCommFrameInfo.u32Width;
    out_frame.height = frame_info.stCommFrameInfo.u32Height;
    out_frame.format = PixelFormat::NV12;
    out_frame.timestamp = frame_info.stVFrame.u64PTS;
    
    size_t frame_size = out_frame.width * out_frame.height * 3 / 2;
    std::vector<uint8_t> frame_data(frame_size);
    if (virt_addr) {
        std::memcpy(frame_data.data(), virt_addr, frame_size);
    }
    out_frame.data = std::move(frame_data);
    
    VDEC_ReleaseFrame(impl_->chn_id(), frame_info.ulBufferId);
    return true;
#else
    last_error_ = "SpacemiT MPP not available";
    return false;
#endif
}

std::string MppDecoder::getLastError() const {
    return last_error_;
}

} // namespace rivision::spacemit
