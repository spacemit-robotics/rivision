#pragma once

#include "rivision/types.h"
#include "i_demux.h"
#include <memory>
#include <functional>
#include <vector>

namespace rivision::hal {

// =============================================================================
// IEncoder - Video encoder interface
// =============================================================================

class IEncoder {
public:
    virtual ~IEncoder() = default;
    
    struct Config {
        CodecType codec = CodecType::H264;
        int width = 0;
        int height = 0;
        int fps = 30;
        int bitrate = 2000000;  // 2 Mbps
        int gop_size = 30;      // Keyframe interval
        
        PixelFormat input_format = PixelFormat::NV12;
        
        // Quality preset
        std::string preset = "medium";  // ultrafast, fast, medium, slow
        
        // Hardware options
        bool zero_copy = true;
    };
    
    // Open encoder
    virtual bool open(const Config& cfg) = 0;
    
    // Close encoder
    virtual void close() = 0;
    
    // Check if open
    virtual bool isOpen() const = 0;
    
    // Encode frame
    virtual bool encode(const Frame& frame, StreamPacket& out_packet) = 0;
    
    // Flush remaining packets
    virtual bool flush(StreamPacket& out_packet) = 0;
    
    // Force keyframe on next encode
    virtual void forceKeyframe() { force_keyframe_ = true; }
    
    // Get last error
    virtual std::string getLastError() const = 0;
    
    // Get extradata (SPS/PPS for H.264)
    virtual const std::vector<uint8_t>& getExtraData() const { return extradata_; }
    
    // SpacemiT MPP binding mode
    virtual bool supportsBind() const { return false; }
    virtual void* getSinkNode() { return nullptr; }
    virtual void* getSrcNode() { return nullptr; }
    
protected:
    Config config_;
    bool force_keyframe_ = false;
    std::vector<uint8_t> extradata_;
};

// Factory function
std::unique_ptr<IEncoder> createEncoder();

} // namespace rivision::hal
