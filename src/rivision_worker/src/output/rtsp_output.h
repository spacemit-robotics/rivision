#pragma once

#include "rivision/types.h"
#include <string>
#include <memory>
#include <thread>
#include <mutex>
#include <atomic>
#include <queue>
#include <condition_variable>

namespace rivision::output {

/**
 * RTSP Output Service
 * Encodes frames to H.264 and pushes to RTSP server using FFmpeg
 */
class RtspOutput {
public:
    struct Config {
        std::string stream_id;
        std::string rtsp_url;      // e.g., rtsp://localhost:8554/stream_name
        int width = 1920;
        int height = 1080;
        int fps = 10;
        int bitrate = 2000000;     // 2 Mbps
        std::string preset = "ultrafast";
    };

    explicit RtspOutput(const Config& config);
    ~RtspOutput();

    bool start();
    void stop();
    
    // Push a frame to be encoded and streamed
    void pushFrame(const Frame& frame);
    
    bool isRunning() const { return running_; }
    std::string getLastError() const { return last_error_; }

private:
    void encoderThread();
    bool startFFmpeg();
    void stopFFmpeg();

    Config config_;
    std::atomic<bool> running_{false};
    std::string last_error_;
    
    // Frame queue
    std::queue<Frame> frame_queue_;
    std::mutex queue_mutex_;
    std::condition_variable queue_cv_;
    
    // FFmpeg process
    FILE* ffmpeg_pipe_{nullptr};
    std::thread encoder_thread_;
};

} // namespace rivision::output
