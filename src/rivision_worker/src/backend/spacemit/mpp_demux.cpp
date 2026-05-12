// SpacemiT MPP Demuxer implementation using native DEMUX API
#include "mpp_demux.h"
#include "mpp_common.h"
#include "utils/logger.h"

extern "C" {
#include "demux/demux_api.h"
#include "sys/sys_api.h"
}

#include <cstring>
#include <mutex>
#include <condition_variable>
#include <queue>

namespace rivision::spacemit {

// Forward declaration of callback data structure
struct DemuxCallbackData {
    std::queue<hal::StreamPacket> packet_queue;
    std::mutex queue_mutex;
    std::condition_variable queue_cv;
    static constexpr size_t MAX_QUEUE_SIZE = 30;
    
    void enqueuePacket(const DemuxPacket* pkt) {
        hal::StreamPacket packet;
        packet.data.assign(pkt->pu8Data, pkt->pu8Data + pkt->u32Size);
        packet.pts = pkt->u64PTS;
        packet.dts = pkt->u64PTS;
        packet.keyframe = pkt->bKeyFrame;
        packet.type = hal::StreamPacket::Type::VIDEO;
        
        std::lock_guard<std::mutex> lock(queue_mutex);
        if (packet_queue.size() < MAX_QUEUE_SIZE) {
            packet_queue.push(std::move(packet));
            queue_cv.notify_one();
        }
    }
};

// C-style callback for MPP DEMUX
static S32 demux_packet_callback(S32 s32ChnId, const DemuxPacket* pstPkt, VOID* pPriv) {
    (void)s32ChnId;
    auto* cb_data = static_cast<DemuxCallbackData*>(pPriv);
    if (cb_data && pstPkt) {
        cb_data->enqueuePacket(pstPkt);
    }
    return 0;
}

class MppDemux::Impl {
public:
    S32 chn_id_ = 0;
    bool initialized_ = false;
    DemuxStreamInfo stream_info_;
    DemuxCallbackData cb_data_;
    
    ~Impl() {
        cleanup();
    }
    
    void cleanup() {
        if (initialized_) {
            DEMUX_StopChn(chn_id_);
            DEMUX_DestroyChn(chn_id_);
            initialized_ = false;
        }
        
        std::lock_guard<std::mutex> lock(cb_data_.queue_mutex);
        while (!cb_data_.packet_queue.empty()) {
            cb_data_.packet_queue.pop();
        }
    }
};

MppDemux::MppDemux() : impl_(std::make_unique<Impl>()) {
    // Initialize DEMUX module once
    static bool demux_initialized = false;
    if (!demux_initialized) {
        if (DEMUX_Init() == 0) {
            demux_initialized = true;
            LOG_INFO("MPP DEMUX module initialized");
        }
    }
}

MppDemux::~MppDemux() = default;

bool MppDemux::open(const Config& cfg) {
    config_ = cfg;
    impl_->cleanup();
    
    DemuxChnAttr attr;
    std::memset(&attr, 0, sizeof(attr));
    
    attr.eInputType = DEMUX_INPUT_RTSP;
    attr.bPreferTcp = cfg.prefer_tcp ? MPP_TRUE : MPP_FALSE;
    attr.bLowLatency = cfg.low_latency ? MPP_TRUE : MPP_FALSE;
    attr.u32OpenTimeoutMs = cfg.connect_timeout_ms;
    attr.u32RwTimeoutMs = cfg.connect_timeout_ms;
    attr.u32ReconnectMs = 2000;
    
    std::strncpy(attr.szUrl, cfg.url.c_str(), sizeof(attr.szUrl) - 1);
    
    S32 ret = DEMUX_CreateChn(impl_->chn_id_, &attr);
    if (ret != 0) {
        last_error_ = "DEMUX_CreateChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        return false;
    }
    
    // Set packet callback
    ret = DEMUX_SetPacketCallback(impl_->chn_id_, demux_packet_callback, &impl_->cb_data_);
    if (ret != 0) {
        last_error_ = "DEMUX_SetPacketCallback failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        DEMUX_DestroyChn(impl_->chn_id_);
        return false;
    }
    
    // Start channel
    ret = DEMUX_StartChn(impl_->chn_id_);
    if (ret != 0) {
        last_error_ = "DEMUX_StartChn failed: " + std::to_string(ret);
        LOG_ERROR("{}", last_error_);
        DEMUX_DestroyChn(impl_->chn_id_);
        return false;
    }
    
    // Get stream info
    ret = DEMUX_GetStreamInfo(impl_->chn_id_, &impl_->stream_info_);
    if (ret != 0) {
        LOG_WARN("DEMUX_GetStreamInfo failed, will get info from first packet");
    }
    
    impl_->initialized_ = true;
    opened_ = true;
    LOG_INFO("MPP Demuxer opened: {}", cfg.url);
    return true;
}

void MppDemux::close() {
    impl_->cleanup();
    opened_ = false;
}

bool MppDemux::isOpen() const {
    return opened_;
}

hal::IDemux::StreamInfo MppDemux::getStreamInfo() const {
    StreamInfo info;
    
    if (!opened_) return info;
    
    info.width = impl_->stream_info_.u32Width;
    info.height = impl_->stream_info_.u32Height;
    info.fps = impl_->stream_info_.u32Fps;
    
    switch (impl_->stream_info_.eCodecType) {
        case DEMUX_CODEC_H264:
            info.codec = hal::CodecType::H264;
            break;
        case DEMUX_CODEC_H265:
            info.codec = hal::CodecType::H265;
            break;
        default:
            info.codec = hal::CodecType::UNKNOWN;
            break;
    }
    
    return info;
}

bool MppDemux::readPacket(hal::StreamPacket& packet) {
    if (!opened_) return false;
    
    auto& cb = impl_->cb_data_;
    std::unique_lock<std::mutex> lock(cb.queue_mutex);
    
    // Wait for packet with timeout
    if (cb.queue_cv.wait_for(lock, std::chrono::milliseconds(1000), 
            [&cb] { return !cb.packet_queue.empty(); })) {
        packet = std::move(cb.packet_queue.front());
        cb.packet_queue.pop();
        return true;
    }
    
    return false;
}

} // namespace rivision::spacemit
