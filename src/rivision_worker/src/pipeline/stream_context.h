#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <memory>
#include <thread>
#include <atomic>
#include <mutex>

namespace rivision::hal {
class IDemux;
class IDecoder;
class IEncoder;
class IGraphics;
class IMux;
}

namespace rivision::core {
class ByteTrack;
}

namespace rivision::output {
class RtspOutput;
}

namespace rivision::pipeline {

using Frame = rivision::Frame;

class YoloService;
class Embedder;

// =============================================================================
// StreamContext - Single stream processing context
// =============================================================================

class StreamContext {
public:
    StreamContext(const StreamConfig& config, 
                 YoloService* yolo_service,
                 DetectionCallback detection_cb);
    ~StreamContext();
    
    // Start processing
    bool start();
    
    // Stop processing
    void stop();
    
    // Pause/resume
    void pause();
    void resume();
    
    // Get status
    StreamStatus getStatus() const;
    
    // Get last error
    std::string getLastError() const;
    
    // Get latest frame (for snapshot)
    bool getLatestFrame(Frame& frame);
    
private:
    // Main processing loop
    void run();
    
    // Process single frame (with inference)
    bool processFrame(const Frame& frame);
    
    // Draw cached detections on frame
    void drawDetections(Frame& frame, const std::vector<Detection>& detections);
    
    StreamConfig config_;
    YoloService* yolo_service_;
    DetectionCallback detection_callback_;
    
    std::unique_ptr<hal::IDemux> demux_;
    std::unique_ptr<hal::IDecoder> decoder_;
    std::unique_ptr<hal::IEncoder> encoder_;
    std::unique_ptr<hal::IGraphics> graphics_;
    std::unique_ptr<hal::IMux> mux_;
    std::unique_ptr<core::ByteTrack> tracker_;
    std::unique_ptr<output::RtspOutput> rtsp_output_;
    
    std::thread worker_thread_;
    std::atomic<bool> running_{false};
    std::atomic<bool> paused_{false};
    
    // Status
    StreamState state_ = StreamState::CREATED;
    std::string last_error_;
    Timestamp started_at_ = 0;
    std::atomic<int64_t> frames_processed_{0};
    std::atomic<int64_t> frames_output_{0};  // Encoded frames sent to RTSP
    std::atomic<int64_t> detections_count_{0};
    std::atomic<float> fps_{0.0f};           // Output FPS (not decode FPS)
    int reconnect_count_ = 0;
    
    // Latest frame for snapshot
    Frame latest_frame_;
    mutable std::mutex frame_mutex_;
    
    // Cached detections for drawing on non-inference frames
    std::vector<Detection> cached_detections_;
};

} // namespace rivision::pipeline
