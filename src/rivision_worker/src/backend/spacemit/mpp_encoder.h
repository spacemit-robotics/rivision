// SpacemiT MPP Encoder - VPU hardware encoding using native MPP VENC API
#pragma once

#include "hal/interface/i_encoder.h"
#include <memory>
#include <string>

namespace rivision::spacemit {

class MppEncoder : public hal::IEncoder {
public:
    MppEncoder();
    ~MppEncoder() override;
    
    // IEncoder interface - exact match
    bool open(const Config& cfg) override;
    void close() override;
    bool isOpen() const override;
    bool encode(const Frame& frame, hal::StreamPacket& out_packet) override;
    bool flush(hal::StreamPacket& out_packet) override;
    std::string getLastError() const override;
    
private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    Config config_;
    std::string last_error_;
    bool initialized_ = false;
};

} // namespace rivision::spacemit
