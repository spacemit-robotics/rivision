// SpacemiT MPP Muxer implementation using native MPP MUX API
#include "mpp_mux.h"
#include "mpp_common.h"
#include "utils/logger.h"

extern "C" {
#include "mux/mux_api.h"
#include "sys/sys_api.h"
}

#include <cstring>
#include <chrono>

namespace rivision::spacemit {

class MppMux::Impl {
public:
    S32 chn_id_ = 0;
    bool initialized_ = false;
    std::string url_;
    std::chrono::steady_clock::time_point start_time_;
    U64 total_bytes_ = 0;
    
    ~Impl() {
        cleanup();
    }
    
    void cleanup() {
        if (initialized_) {
            MUX_StopChn(chn_id_);
            MUX_DestroyChn(chn_id_);
            initialized_ = false;
        }
        total_bytes_ = 0;
    }
};

MppMux::MppMux() : impl_(std::make_unique<Impl>()) {
    // Initialize MUX module once
    static bool mux_initialized = false;
    if (!mux_initialized) {
        if (MUX_Init() == 0) {
            mux_initialized = true;
            LOG_INFO("MPP MUX module initialized");
        }
    }
}

MppMux::~MppMux() = default;

bool MppMux::open(const Config& cfg) {
    config_ = cfg;
    impl_->cleanup();
    impl_->url_ = cfg.url;
    
    MuxChnAttr attr;
    std::memset(&attr, 0, sizeof(attr));
    
    // Determine output type from URL/path
    if (cfg.url.find("rtsp://") == 0) {
        attr.eOutputType = MUX_OUTPUT_RTSP;
    } else {
        attr.eOutputType = MUX_OUTPUT_FILE;
    }
    
    std::strncpy(attr.szUrl, cfg.url.c_str(), sizeof(attr.szUrl) - 1);
    
    // Set codec type
    if (cfg.video_codec == hal::CodecType::H265) {
        attr.stStreamAttr.eCodecType = MUX_CODEC_H265;
    } else {
        attr.stStreamAttr.eCodecType = MUX_CODEC_H264;
    }
    
    attr.stStreamAttr.u32Width = cfg.width;
    attr.stStreamAttr.u32Height = cfg.height;
    attr.stStreamAttr.u32Fps = cfg.fps > 0 ? cfg.fps : 25;
    attr.stStreamAttr.u32BitrateKbps = cfg.bitrate > 0 ? cfg.bitrate / 1000 : 4000;
    attr.bPreferTcp = MPP_TRUE;
    attr.u32MaxDelayMs = 100;
    
    S32 ret = MUX_CreateChn(impl_->chn_id_, &attr);
    if (ret != 0) {
        last_error_ = "MUX_CreateChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        return false;
    }
    
    ret = MUX_StartChn(impl_->chn_id_);
    if (ret != 0) {
        last_error_ = "MUX_StartChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        MUX_DestroyChn(impl_->chn_id_);
        return false;
    }
    
    impl_->initialized_ = true;
    impl_->start_time_ = std::chrono::steady_clock::now();
    opened_ = true;
    bytes_written_ = 0;
    
    LOG_INFO("MPP Muxer opened: {}", cfg.url);
    return true;
}

void MppMux::close() {
    if (!opened_) return;
    
    impl_->cleanup();
    opened_ = false;
    
    LOG_INFO("MPP Muxer closed: {}", impl_->url_);
}

bool MppMux::isOpen() const {
    return opened_;
}

bool MppMux::writeVideo(const hal::StreamPacket& packet) {
    if (!opened_) return false;
    
    MuxPacket pkt;
    std::memset(&pkt, 0, sizeof(pkt));
    
    pkt.pu8Data = const_cast<U8*>(packet.data.data());
    pkt.u32Size = static_cast<U32>(packet.data.size());
    pkt.bKeyFrame = packet.keyframe ? MPP_TRUE : MPP_FALSE;
    pkt.u64PTS = packet.pts;
    
    // Set codec type from config
    if (config_.video_codec == hal::CodecType::H265) {
        pkt.eCodecType = MUX_CODEC_H265;
    } else {
        pkt.eCodecType = MUX_CODEC_H264;
    }
    
    S32 ret = MUX_SendPacket(impl_->chn_id_, &pkt);
    if (ret == 0) {
        bytes_written_ += packet.data.size();
        return true;
    }
    
    return false;
}

bool MppMux::writeAudio(const hal::StreamPacket& packet) {
    // Audio not implemented yet
    (void)packet;
    return true;
}

void MppMux::flush() {
    // MUX module doesn't require explicit flush
}

int64_t MppMux::bytesWritten() const {
    return bytes_written_;
}

std::string MppMux::getLastError() const {
    return last_error_;
}

} // namespace rivision::spacemit
