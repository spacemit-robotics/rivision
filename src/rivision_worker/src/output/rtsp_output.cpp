#include "rtsp_output.h"
#include "utils/logger.h"
#include <cstdio>
#include <sstream>
#include <cstdlib>

namespace rivision::output {

// Create stream in go2rtc via API (required before RTSP PUBLISH)
static bool createGo2rtcStream(const std::string& stream_id) {
    // go2rtc requires stream to be pre-defined before accepting RTSP PUBLISH
    // PUT http://127.0.0.1:1984/api/streams?name=<stream_id>
    std::ostringstream cmd;
    cmd << "curl -s -X PUT 'http://127.0.0.1:1984/api/streams?name=" << stream_id << "' >/dev/null 2>&1";
    int ret = system(cmd.str().c_str());
    if (ret == 0) {
        LOG_INFO("[RtspOutput] Created go2rtc stream: {}", stream_id);
        return true;
    }
    LOG_WARN("[RtspOutput] Failed to create go2rtc stream: {} (ret={})", stream_id, ret);
    return false;  // Non-fatal, stream might already exist
}

RtspOutput::RtspOutput(const Config& config) : config_(config) {}

RtspOutput::~RtspOutput() {
    stop();
}

bool RtspOutput::start() {
    if (running_) {
        return true;
    }
    
    // Create stream in go2rtc first (required for RTSP PUBLISH)
    createGo2rtcStream(config_.stream_id);
    
    if (!startFFmpeg()) {
        return false;
    }
    
    running_ = true;
    LOG_INFO("Starting encoder thread for {}", config_.stream_id);
    utils::Logger::flush();
    encoder_thread_ = std::thread(&RtspOutput::encoderThread, this);
    LOG_INFO("Encoder thread created for {}", config_.stream_id);
    utils::Logger::flush();
    
    return true;
}

void RtspOutput::stop() {
    if (!running_) {
        return;
    }
    
    running_ = false;
    queue_cv_.notify_all();
    
    if (encoder_thread_.joinable()) {
        encoder_thread_.join();
    }
    
    stopFFmpeg();
    LOG_INFO("RTSP output stopped: {}", config_.stream_id);
}

void RtspOutput::pushFrame(const Frame& frame) {
    static int push_count = 0;
    push_count++;
    if (push_count % 50 == 1) {
        LOG_INFO("pushFrame called: count={}, running={}, frame.empty={}", 
                 push_count, running_, frame.isEmpty());
        utils::Logger::flush();
    }
    
    if (!running_) {
        return;
    }
    
    std::lock_guard<std::mutex> lock(queue_mutex_);
    
    // Keep only a few frames in queue to avoid lag
    while (frame_queue_.size() > 3) {
        frame_queue_.pop();
    }
    
    frame_queue_.push(frame);
    queue_cv_.notify_one();
}

bool RtspOutput::startFFmpeg() {
    // Build FFmpeg command
    // Input: raw RGB24 frames via pipe
    // Output: H.264 encoded RTSP stream
    std::ostringstream cmd;
    cmd << "ffmpeg -y "
        << "-f rawvideo "
        << "-pix_fmt rgb24 "
        << "-s " << config_.width << "x" << config_.height << " "
        << "-r " << config_.fps << " "
        << "-i - "
        << "-c:v libx264 "
        << "-preset " << config_.preset << " "
        << "-tune zerolatency "
        << "-b:v " << config_.bitrate << " "
        << "-maxrate " << config_.bitrate << " "
        << "-bufsize " << (config_.bitrate / 2) << " "
        << "-pix_fmt yuv420p "
        << "-g " << (config_.fps * 2) << " "  // GOP size = 2 seconds
        << "-f rtsp "
        << "-rtsp_transport tcp "
        << config_.rtsp_url
        << " 2>/dev/null";
    
    LOG_INFO("Starting FFmpeg: {}", cmd.str());
    
    ffmpeg_pipe_ = popen(cmd.str().c_str(), "w");
    if (!ffmpeg_pipe_) {
        last_error_ = "Failed to start FFmpeg process";
        LOG_ERROR("{}", last_error_);
        return false;
    }
    
    return true;
}

void RtspOutput::stopFFmpeg() {
    if (ffmpeg_pipe_) {
        pclose(ffmpeg_pipe_);
        ffmpeg_pipe_ = nullptr;
    }
}

void RtspOutput::encoderThread() {
    LOG_INFO("RTSP encoder thread started: {} ({}x{}, expect {} bytes)", 
             config_.stream_id, config_.width, config_.height,
             config_.width * config_.height * 3);
    utils::Logger::flush();
    
    size_t frame_size = config_.width * config_.height * 3;  // RGB24
    int frame_count = 0;
    
    while (running_) {
        Frame frame;
        
        {
            std::unique_lock<std::mutex> lock(queue_mutex_);
            queue_cv_.wait_for(lock, std::chrono::milliseconds(1000), [this] { 
                return !frame_queue_.empty() || !running_; 
            });
            
            if (!running_ && frame_queue_.empty()) {
                break;
            }
            
            if (!frame_queue_.empty()) {
                frame = std::move(frame_queue_.front());
                frame_queue_.pop();
            } else {
                continue;  // Timeout, no frame
            }
        }
        
        if (frame.isEmpty()) {
            LOG_WARN("RTSP encoder: got empty frame");
            continue;
        }
        
        // Write frame to FFmpeg
        uint8_t* data = frame.cpuData();
        if (data && ffmpeg_pipe_) {
            size_t actual_size = frame.width * frame.height * 3;
            if (actual_size == frame_size) {
                size_t written = fwrite(data, 1, frame_size, ffmpeg_pipe_);
                fflush(ffmpeg_pipe_);
                frame_count++;
                if (frame_count % 50 == 1) {
                    LOG_INFO("RTSP encoder: wrote frame {} ({} bytes)", frame_count, written);
                }
            } else {
                LOG_WARN("RTSP encoder: frame size mismatch: {} vs {}", actual_size, frame_size);
            }
        } else {
            LOG_WARN("RTSP encoder: no data or pipe closed (data={}, pipe={})", 
                     data ? "valid" : "null", ffmpeg_pipe_ ? "valid" : "null");
        }
    }
    
    LOG_DEBUG("RTSP encoder thread stopped: {}", config_.stream_id);
}

} // namespace rivision::output
