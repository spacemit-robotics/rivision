// SpacemiT MPP Encoder implementation using native MPP VENC API
#include "mpp_encoder.h"
#include "mpp_common.h"
#include "mpp_channel_manager.h"
#include "utils/logger.h"

#ifdef USE_SPACEMIT_MPP
extern "C" {
#include "venc/venc_api.h"
#include "sys/sys_api.h"
}
#endif

#include <cstring>

namespace rivision::spacemit {

class MppEncoder::Impl {
public:
#ifdef USE_SPACEMIT_MPP
    ChannelHolder chn_holder_{ChannelManager::ChannelType::ENCODER};
    S32 chn_id() const { return chn_holder_.id(); }
#endif
    VbBuffer input_buffer_;
    int64_t pts_ = 0;
    bool flush_sent_ = false;
    
    ~Impl() {
        cleanup();
    }
    
    void cleanup() {
#ifdef USE_SPACEMIT_MPP
        if (chn_holder_.valid()) {
            VENC_DisableChn(chn_id());
            VENC_DestroyChn(chn_id());
        }
#endif
        freeVbBuffer(input_buffer_);
        flush_sent_ = false;
        pts_ = 0;
    }
};

MppEncoder::MppEncoder() : impl_(std::make_unique<Impl>()) {
#ifdef USE_SPACEMIT_MPP
    static bool venc_initialized = false;
    if (!venc_initialized) {
        if (VENC_Init() == 0) {
            venc_initialized = true;
            LOG_INFO("MPP VENC module initialized");
        }
    }
#endif
}

MppEncoder::~MppEncoder() = default;

bool MppEncoder::open(const Config& cfg) {
    config_ = cfg;
    
#ifdef USE_SPACEMIT_MPP
    // Check if channel was allocated successfully
    if (!impl_->chn_holder_.valid()) {
        last_error_ = "No available encoder channel (max " + 
                      std::to_string(ChannelManager::MAX_ENCODER_CHANNELS) + " reached)";
        LOG_ERROR("{}", last_error_);
        return false;
    }
    
    impl_->cleanup();
    
    VencChnAttr attr;
    std::memset(&attr, 0, sizeof(attr));
    
    // Set codec type
    if (cfg.codec == hal::CodecType::H265) {
        attr.eCodecType = MPP_STREAM_CODEC_H265;
    } else {
        attr.eCodecType = MPP_STREAM_CODEC_H264;
    }
    
    attr.eInputPixelFormat = MPP_PIXEL_FORMAT_NV12;
    attr.u32Width = cfg.width;
    attr.u32Height = cfg.height;
    attr.u32Bitrate = cfg.bitrate > 0 ? cfg.bitrate : 2000000;
    attr.u32FrameRate = cfg.fps > 0 ? cfg.fps : 25;
    attr.u32Gop = cfg.gop_size > 0 ? cfg.gop_size : 30;
    attr.eRcMode = VENC_RC_MODE_CBR;
    
    S32 ret = VENC_CreateChn(impl_->chn_id(), &attr);
    if (ret != 0) {
        last_error_ = "VENC_CreateChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        return false;
    }
    
    ret = VENC_EnableChn(impl_->chn_id());
    if (ret != 0) {
        last_error_ = "VENC_EnableChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        VENC_DestroyChn(impl_->chn_id());
        return false;
    }
    
    // Allocate input buffer
    size_t frame_size = cfg.width * cfg.height * 3 / 2;
    impl_->input_buffer_ = allocVbBuffer(frame_size);
    if (impl_->input_buffer_.size == 0) {
        last_error_ = "Failed to allocate input buffer";
        impl_->cleanup();
        return false;
    }
    
    initialized_ = true;
    LOG_INFO("MPP Encoder opened: {}x{} {}kbps",
             cfg.width, cfg.height, cfg.bitrate / 1000);
    return true;
#else
    last_error_ = "SpacemiT MPP not available";
    return false;
#endif
}

void MppEncoder::close() {
    impl_->cleanup();
    initialized_ = false;
}

bool MppEncoder::isOpen() const {
    return initialized_;
}

bool MppEncoder::encode(const Frame& frame, hal::StreamPacket& out_packet) {
#ifdef USE_SPACEMIT_MPP
    if (!initialized_) {
        last_error_ = "Encoder not initialized";
        return false;
    }
    
    // Copy frame data to VB buffer
    size_t frame_size = config_.width * config_.height * 3 / 2;
    const uint8_t* frame_ptr = frame.cpuData();
    if (frame_ptr) {
        std::memcpy(impl_->input_buffer_.virt_addr, frame_ptr, frame_size);
    }
    
    // Prepare input frame
    VideoFrameInfo vframe;
    std::memset(&vframe, 0, sizeof(vframe));
    vframe.stVFrame.u64PlanePhyAddr[0] = impl_->input_buffer_.phys_addr;
    vframe.stVFrame.ulPlaneVirAddr[0] = (unsigned long)impl_->input_buffer_.virt_addr;
    vframe.stVFrame.u64PTS = impl_->pts_++;
    vframe.stCommFrameInfo.u32Width = config_.width;
    vframe.stCommFrameInfo.u32Height = config_.height;
    vframe.stCommFrameInfo.ePixelFormat = MPP_PIXEL_FORMAT_NV12;
    vframe.eFrameType = FRAME_TYPE_VENC;
    
    S32 ret = VENC_SendFrame(impl_->chn_id(), &vframe, 0);
    if (ret != 0) {
        last_error_ = "VENC_SendFrame failed: " + std::to_string(ret);
        return false;
    }
    
    // Get encoded stream
    StreamBufferInfo stream;
    std::memset(&stream, 0, sizeof(stream));
    
    ret = VENC_GetStream(impl_->chn_id(), &stream, 100);
    if (ret == ERR_VENC_NO_STREAM || ret == ERR_VENC_TIMEOUT) {
        return true;  // No output yet, not an error
    }
    if (ret != ERR_VENC_OK) {
        last_error_ = "VENC_GetStream failed: " + std::to_string(ret);
        return false;
    }
    
    // Copy to output packet
    out_packet.data.assign(stream.pu8Addr, stream.pu8Addr + stream.u32Size);
    out_packet.pts = stream.u64PTS;
    out_packet.dts = stream.u64PTS;
    out_packet.keyframe = stream.bKeyFrame;
    out_packet.type = hal::StreamPacket::Type::VIDEO;
    
    VENC_ReleaseStream(impl_->chn_id(), &stream);
    return true;
#else
    last_error_ = "SpacemiT MPP not available";
    return false;
#endif
}

bool MppEncoder::flush(hal::StreamPacket& out_packet) {
#ifdef USE_SPACEMIT_MPP
    if (!initialized_) return false;
    
    // Send flush signal once
    if (!impl_->flush_sent_) {
        VENC_SendFrame(impl_->chn_id(), nullptr, 0);
        impl_->flush_sent_ = true;
    }
    
    // Get remaining stream
    StreamBufferInfo stream;
    std::memset(&stream, 0, sizeof(stream));
    
    S32 ret = VENC_GetStream(impl_->chn_id(), &stream, 100);
    if (ret != ERR_VENC_OK) {
        impl_->flush_sent_ = false;
        return false;
    }
    
    out_packet.data.assign(stream.pu8Addr, stream.pu8Addr + stream.u32Size);
    out_packet.pts = stream.u64PTS;
    out_packet.dts = stream.u64PTS;
    out_packet.keyframe = stream.bKeyFrame;
    out_packet.type = hal::StreamPacket::Type::VIDEO;
    
    VENC_ReleaseStream(impl_->chn_id(), &stream);
    return true;
#else
    last_error_ = "SpacemiT MPP not available";
    return false;
#endif
}

std::string MppEncoder::getLastError() const {
    return last_error_;
}

} // namespace rivision::spacemit
