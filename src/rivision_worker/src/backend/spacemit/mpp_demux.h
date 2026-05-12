// SpacemiT MPP Demuxer - RTSP/file demuxing using native MPP DEMUX API
#pragma once

#include "hal/interface/i_demux.h"
#include <memory>
#include <string>

namespace rivision::spacemit {

class MppDemux : public hal::IDemux {
public:
    MppDemux();
    ~MppDemux() override;
    
    // IDemux interface - exact match
    bool open(const Config& cfg) override;
    void close() override;
    bool isOpen() const override;
    StreamInfo getStreamInfo() const override;
    bool readPacket(hal::StreamPacket& packet) override;
    
private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    Config config_;
    std::string last_error_;
    bool opened_ = false;
};

} // namespace rivision::spacemit
