#pragma once

#include "rivision/types.h"
#include <string>
#include <functional>
#include <memory>

namespace rivision::hal {

// =============================================================================
// Stream Packet - Container for demuxed packets
// =============================================================================

struct StreamPacket {
    enum class Type { VIDEO, AUDIO, SUBTITLE };
    Type type = Type::VIDEO;
    
    std::vector<uint8_t> data;
    int64_t pts = 0;      // Presentation timestamp
    int64_t dts = 0;      // Decode timestamp
    bool keyframe = false;
    
    int stream_index = 0;
};

// =============================================================================
// Codec information
// =============================================================================

enum class CodecType {
    UNKNOWN,
    H264,
    H265,
    VP8,
    VP9,
    AV1,
    MJPEG
};

inline const char* codecTypeToString(CodecType codec) {
    switch (codec) {
        case CodecType::H264: return "h264";
        case CodecType::H265: return "h265";
        case CodecType::VP8: return "vp8";
        case CodecType::VP9: return "vp9";
        case CodecType::AV1: return "av1";
        case CodecType::MJPEG: return "mjpeg";
        default: return "unknown";
    }
}

// =============================================================================
// IDemux - Demuxer interface
// =============================================================================

class IDemux {
public:
    virtual ~IDemux() = default;
    
    struct Config {
        std::string url;
        bool prefer_tcp = true;
        bool low_latency = true;
        int connect_timeout_ms = 5000;
        int reconnect_ms = 3000;
        std::map<std::string, std::string> options;
    };
    
    struct StreamInfo {
        CodecType codec = CodecType::UNKNOWN;
        int width = 0;
        int height = 0;
        float fps = 0.0f;
        int bitrate = 0;
        
        // Codec-specific data (SPS/PPS for H264)
        std::vector<uint8_t> extradata;
    };
    
    // Open stream
    virtual bool open(const Config& cfg) = 0;
    
    // Close stream
    virtual void close() = 0;
    
    // Check if open
    virtual bool isOpen() const = 0;
    
    // Get stream info
    virtual StreamInfo getStreamInfo() const = 0;
    
    // Read next packet (blocking)
    virtual bool readPacket(StreamPacket& packet) = 0;
    
    // Callback mode (non-blocking)
    using PacketCallback = std::function<void(const StreamPacket&)>;
    virtual void setCallback(PacketCallback cb) { packet_callback_ = std::move(cb); }
    
    // Start/stop callback thread
    virtual bool startCallback() { return false; }
    virtual void stopCallback() {}
    
    // SpacemiT MPP binding mode
    virtual bool supportsBind() const { return false; }
    virtual void* getSrcNode() { return nullptr; }
    
protected:
    PacketCallback packet_callback_;
};

// Factory function
std::unique_ptr<IDemux> createDemux();

} // namespace rivision::hal
