#pragma once

#include "hal/interface/i_decoder.h"
#include "mpp_common.h"
#include <memory>

namespace rivision::spacemit {

// =============================================================================
// MppDecoder - SpacemiT VPU decoder
// =============================================================================

class MppDecoder : public hal::IDecoder {
public:
    MppDecoder();
    ~MppDecoder() override;
    
    bool open(const Config& cfg) override;
    void close() override;
    bool isOpen() const override;
    bool decode(const hal::StreamPacket& packet, Frame& out_frame) override;
    bool flush(Frame& out_frame) override;
    std::string getLastError() const override;
    
private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    Config config_;
    bool initialized_ = false;
    std::string last_error_;
};

} // namespace rivision::spacemit
