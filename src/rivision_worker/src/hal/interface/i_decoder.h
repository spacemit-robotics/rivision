#pragma once

#include "rivision/types.h"
#include "i_demux.h"
#include <memory>
#include <functional>

namespace rivision::hal {

// =============================================================================
// IDecoder - Video decoder interface
// =============================================================================

class IDecoder {
public:
    virtual ~IDecoder() = default;
    
    struct Config {
        CodecType codec = CodecType::H264;
        int width = 0;
        int height = 0;
        PixelFormat output_format = PixelFormat::NV12;
        
        // Codec-specific data
        std::vector<uint8_t> extradata;
        
        // Hardware options
        bool zero_copy = true;  // Use DMA buffers if available
        int buffer_count = 4;   // Number of output buffers
    };
    
    // Open decoder
    virtual bool open(const Config& cfg) = 0;
    
    // Close decoder
    virtual void close() = 0;
    
    // Check if open
    virtual bool isOpen() const = 0;
    
    // Decode packet, get frame
    virtual bool decode(const StreamPacket& packet, Frame& out_frame) = 0;
    
    // Flush remaining frames
    virtual bool flush(Frame& out_frame) = 0;
    
    // Get last error
    virtual std::string getLastError() const = 0;
    
    // SpacemiT MPP binding mode
    virtual bool supportsBind() const { return false; }
    virtual void* getSinkNode() { return nullptr; }
    virtual void* getSrcNode() { return nullptr; }
    
protected:
    Config config_;
};

// Factory function
std::unique_ptr<IDecoder> createDecoder();

} // namespace rivision::hal
