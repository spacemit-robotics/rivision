// SpacemiT MPP Muxer - RTSP push / file muxing using native MPP MUX API
#pragma once

#include "hal/interface/i_mux.h"
#include <memory>
#include <string>

namespace rivision::spacemit {

class MppMux : public hal::IMux {
public:
    MppMux();
    ~MppMux() override;
    
    // IMux interface
    bool open(const Config& cfg) override;
    void close() override;
    bool isOpen() const override;
    bool writeVideo(const hal::StreamPacket& packet) override;
    bool writeAudio(const hal::StreamPacket& packet) override;
    void flush() override;
    int64_t bytesWritten() const override;
    std::string getLastError() const override;
    
private:
    class Impl;
    std::unique_ptr<Impl> impl_;
    
    Config config_;
    std::string last_error_;
    bool opened_ = false;
    int64_t bytes_written_ = 0;
};

} // namespace rivision::spacemit
