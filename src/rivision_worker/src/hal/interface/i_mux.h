#pragma once

#include "rivision/types.h"
#include "i_demux.h"
#include <memory>
#include <string>
#include <vector>

namespace rivision::hal {

// =============================================================================
// IMux - Muxer interface (RTSP push / file output)
// =============================================================================

class IMux {
public:
    virtual ~IMux() = default;
    
    struct Config {
        std::string url;          // rtsp://... or file path
        CodecType video_codec = CodecType::H264;
        int width = 0;
        int height = 0;
        int fps = 30;
        int bitrate = 2000000;
        
        // Video extradata (SPS/PPS for H.264)
        std::vector<uint8_t> extradata;
        
        // Audio (optional)
        bool has_audio = false;
        int audio_sample_rate = 44100;
        int audio_channels = 2;
    };
    
    // Open muxer
    virtual bool open(const Config& cfg) = 0;
    
    // Close muxer
    virtual void close() = 0;
    
    // Check if open
    virtual bool isOpen() const = 0;
    
    // Write video packet
    virtual bool writeVideo(const StreamPacket& packet) = 0;
    
    // Write audio packet
    virtual bool writeAudio(const StreamPacket& packet) = 0;
    
    // Flush buffers
    virtual void flush() = 0;
    
    // Get bytes written
    virtual int64_t bytesWritten() const = 0;
    
    // Get last error
    virtual std::string getLastError() const = 0;
};

// Factory function
std::unique_ptr<IMux> createMux();

} // namespace rivision::hal
