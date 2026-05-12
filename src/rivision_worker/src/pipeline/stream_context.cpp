#include "stream_context.h"
#include "inference/yolo_service.h"
#include "hal/interface/i_demux.h"
#include "hal/interface/i_decoder.h"
#include "hal/interface/i_encoder.h"
#include "hal/interface/i_graphics.h"
#include "hal/interface/i_mux.h"
#include "hal/factory.h"
#include "core/tracker/byte_track.h"
#include "output/rtsp_output.h"
#include "utils/logger.h"

#include <chrono>

namespace rivision::pipeline {

StreamContext::StreamContext(const StreamConfig& config,
                            YoloService* yolo_service,
                            DetectionCallback detection_cb)
    : config_(config)
    , yolo_service_(yolo_service)
    , detection_callback_(std::move(detection_cb)) {
}

StreamContext::~StreamContext() {
    stop();
}

bool StreamContext::start() {
    if (running_) {
        LOG_WARN("StreamContext {} already running", config_.id);
        return true;
    }
    
    LOG_INFO("Starting stream context: id={}, url={}", config_.id, config_.rtsp_url);
    
    // Create demuxer
    demux_ = hal::createDemux();
    if (!demux_) {
        last_error_ = "Failed to create demuxer";
        state_ = StreamState::ERROR;
        return false;
    }
    
    // Configure demuxer
    hal::IDemux::Config demux_cfg;
    demux_cfg.url = config_.rtsp_url;
    demux_cfg.prefer_tcp = config_.demux.prefer_tcp;
    demux_cfg.low_latency = config_.demux.low_latency;
    demux_cfg.connect_timeout_ms = config_.demux.connect_timeout_ms;
    
    state_ = StreamState::CONNECTING;
    
    if (!demux_->open(demux_cfg)) {
        last_error_ = "Failed to open stream: " + config_.rtsp_url;
        state_ = StreamState::ERROR;
        return false;
    }
    
    // Get stream info
    auto stream_info = demux_->getStreamInfo();
    LOG_INFO("Stream info: {}x{} @ {:.1f}fps, codec={}",
             stream_info.width, stream_info.height, stream_info.fps,
             hal::codecTypeToString(stream_info.codec));
    
    // Create decoder
    decoder_ = hal::createDecoder();
    if (!decoder_) {
        last_error_ = "Failed to create decoder";
        state_ = StreamState::ERROR;
        demux_->close();
        return false;
    }
    
    hal::IDecoder::Config dec_cfg;
    dec_cfg.codec = stream_info.codec;
    dec_cfg.width = stream_info.width;
    dec_cfg.height = stream_info.height;
    dec_cfg.output_format = PixelFormat::RGB24;  // For YOLO
    dec_cfg.extradata = stream_info.extradata;
    
    if (!decoder_->open(dec_cfg)) {
        last_error_ = "Failed to open decoder";
        state_ = StreamState::ERROR;
        demux_->close();
        return false;
    }
    
    // Create tracker if enabled
    if (config_.tracking.enabled) {
        core::ByteTrack::Config track_cfg;
        track_cfg.track_thresh = config_.tracking.confirm_threshold;
        track_cfg.match_thresh = config_.tracking.iou_threshold;
        track_cfg.track_buffer = config_.tracking.max_lost_frames;
        track_cfg.min_hits = config_.tracking.min_hits;
        tracker_ = std::make_unique<core::ByteTrack>(track_cfg);
        LOG_INFO("ByteTrack tracker initialized");
    }
    
    // Create graphics for drawing detections
    LOG_INFO("Creating graphics for stream: {}", config_.id);
    utils::Logger::flush();
    graphics_ = hal::createGraphics();
    if (graphics_) {
        hal::IGraphics::Config gfx_cfg;
        gfx_cfg.width = stream_info.width;
        gfx_cfg.height = stream_info.height;
        gfx_cfg.font_size = 16;
        if (graphics_->init(gfx_cfg)) {
            LOG_INFO("Graphics initialized: {}x{} for stream {}", stream_info.width, stream_info.height, config_.id);
            utils::Logger::flush();
        } else {
            LOG_ERROR("Graphics init failed for stream {}", config_.id);
            utils::Logger::flush();
            graphics_.reset();
        }
    } else {
        LOG_ERROR("createGraphics() returned nullptr for stream {}", config_.id);
        utils::Logger::flush();
    }
    
    // Initialize RTSP output using HAL encoder + mux
    LOG_INFO("RTSP output check: enabled={} for stream {}", config_.rtsp_output_enabled, config_.id);
    utils::Logger::flush();
    if (config_.rtsp_output_enabled) {
        LOG_INFO("Creating HAL encoder + mux for stream {}", config_.id);
        utils::Logger::flush();
        
        // Create encoder with configurable output FPS
        LOG_INFO("Config rtsp_output: fps={}, bitrate={}", config_.rtsp_output.fps, config_.rtsp_output.bitrate);
        int output_fps = config_.rtsp_output.fps > 0 ? config_.rtsp_output.fps : 15;
        int output_bitrate = config_.rtsp_output.bitrate > 0 ? config_.rtsp_output.bitrate : 4000000;
        
        encoder_ = hal::createEncoder();
        if (encoder_) {
            hal::IEncoder::Config enc_cfg;
            enc_cfg.codec = hal::CodecType::H264;
            enc_cfg.width = stream_info.width;
            enc_cfg.height = stream_info.height;
            enc_cfg.fps = output_fps;
            enc_cfg.bitrate = output_bitrate;
            enc_cfg.gop_size = output_fps;  // 1 second GOP for faster seek/recovery
            enc_cfg.preset = "ultrafast";
            
            if (encoder_->open(enc_cfg)) {
                LOG_INFO("HAL encoder opened: {}x{} {}fps", enc_cfg.width, enc_cfg.height, enc_cfg.fps);
            } else {
                LOG_WARN("Failed to open HAL encoder: {}", encoder_->getLastError());
                encoder_.reset();
            }
        }
        
        // Create mux (RTSP output)
        if (encoder_) {
            // Pre-create stream in go2rtc (required for RTSP PUBLISH)
            std::string go2rtc_cmd = "curl -s -X PUT 'http://127.0.0.1:1984/api/streams?name=" 
                                   + config_.id + "' >/dev/null 2>&1";
            int ret = system(go2rtc_cmd.c_str());
            if (ret == 0) {
                LOG_INFO("Created go2rtc stream: {}", config_.id);
            }
            
            mux_ = hal::createMux();
            if (mux_) {
                hal::IMux::Config mux_cfg;
                mux_cfg.url = "rtsp://127.0.0.1:8554/" + config_.id;
                mux_cfg.video_codec = hal::CodecType::H264;
                mux_cfg.width = stream_info.width;
                mux_cfg.height = stream_info.height;
                mux_cfg.fps = output_fps;
                mux_cfg.bitrate = output_bitrate;
                mux_cfg.extradata = encoder_->getExtraData();  // Pass SPS/PPS
                
                if (mux_->open(mux_cfg)) {
                    LOG_INFO("HAL mux opened: {}", mux_cfg.url);
                } else {
                    LOG_WARN("Failed to open HAL mux: {}", mux_->getLastError());
                    mux_.reset();
                    encoder_.reset();  // No point keeping encoder without mux
                }
            }
        }
        utils::Logger::flush();
    }
    LOG_INFO("RTSP output init done for stream {}", config_.id);
    utils::Logger::flush();
    
    // Start processing thread
    started_at_ = nowMs();
    running_ = true;
    paused_ = false;
    state_ = StreamState::RUNNING;
    
    worker_thread_ = std::thread(&StreamContext::run, this);
    
    LOG_INFO("Stream context started: {}", config_.id);
    return true;
}

void StreamContext::stop() {
    if (!running_) {
        return;
    }
    
    LOG_INFO("Stopping stream context: {}", config_.id);
    
    running_ = false;
    paused_ = false;
    
    if (worker_thread_.joinable()) {
        worker_thread_.join();
    }
    
    if (decoder_) {
        decoder_->close();
        decoder_.reset();
    }
    
    if (demux_) {
        demux_->close();
        demux_.reset();
    }
    
    if (tracker_) {
        tracker_->reset();
        tracker_.reset();
    }
    
    // Clean up HAL encoder + mux
    if (mux_) {
        mux_->close();
        mux_.reset();
    }
    
    if (encoder_) {
        encoder_->close();
        encoder_.reset();
    }
    
    if (rtsp_output_) {
        rtsp_output_->stop();
        rtsp_output_.reset();
    }
    
    state_ = StreamState::STOPPED;
    LOG_INFO("Stream context stopped: {}", config_.id);
}

void StreamContext::pause() {
    paused_ = true;
    state_ = StreamState::PAUSED;
    LOG_DEBUG("Stream paused: {}", config_.id);
}

void StreamContext::resume() {
    paused_ = false;
    state_ = StreamState::RUNNING;
    LOG_DEBUG("Stream resumed: {}", config_.id);
}

StreamStatus StreamContext::getStatus() const {
    StreamStatus status;
    status.state = state_;
    status.error_msg = last_error_;
    status.started_at = started_at_;
    status.frames_processed = frames_processed_.load();
    status.detections_count = detections_count_.load();
    status.fps = fps_.load();
    status.reconnect_count = reconnect_count_;
    return status;
}

std::string StreamContext::getLastError() const {
    return last_error_;
}

bool StreamContext::getLatestFrame(Frame& frame) {
    std::lock_guard<std::mutex> lock(frame_mutex_);
    
    if (latest_frame_.isEmpty()) {
        return false;
    }
    
    frame = latest_frame_;
    return true;
}

void StreamContext::run() {
    LOG_DEBUG("Stream processing thread started: {}", config_.id);
    
    hal::StreamPacket packet;
    Frame frame;
    
    auto last_fps_update = std::chrono::steady_clock::now();
    int64_t last_output_count = 0;        // For output FPS calculation
    
    int inference_interval = 30 / config_.detection.inference_fps;  // Assume 30fps input
    if (inference_interval < 1) inference_interval = 1;
    int frame_counter = 0;
    
    // Output encoding interval: calculate from desired output_fps
    // output_interval = input_fps / output_fps = 30 / output_fps
    int output_fps = config_.rtsp_output.fps > 0 ? config_.rtsp_output.fps : 15;
    int output_interval = 30 / output_fps;
    if (output_interval < 1) output_interval = 1;
    int output_counter = 0;
    
    LOG_INFO("Frame rate control: input=30fps, inference_interval={}, output_interval={} ({}fps)", 
             inference_interval, output_interval, output_fps);
    
    while (running_) {
        // Check pause
        if (paused_) {
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
            continue;
        }
        
        // Read packet
        if (!demux_->readPacket(packet)) {
            // Handle reconnect
            LOG_WARN("Read packet failed, attempting reconnect...");
            state_ = StreamState::RECONNECTING;
            reconnect_count_++;
            
            std::this_thread::sleep_for(
                std::chrono::milliseconds(config_.demux.reconnect_ms));
            
            // Try to reopen
            demux_->close();
            
            hal::IDemux::Config demux_cfg;
            demux_cfg.url = config_.rtsp_url;
            demux_cfg.prefer_tcp = config_.demux.prefer_tcp;
            
            if (demux_->open(demux_cfg)) {
                state_ = StreamState::RUNNING;
                LOG_INFO("Reconnected to stream: {}", config_.id);
            } else {
                if (config_.demux.max_reconnect > 0 && 
                    reconnect_count_ >= config_.demux.max_reconnect) {
                    last_error_ = "Max reconnect attempts reached";
                    state_ = StreamState::ERROR;
                    break;
                }
            }
            continue;
        }
        
        // Skip non-video packets
        if (packet.type != hal::StreamPacket::Type::VIDEO) {
            continue;
        }
        
        // Decode
        if (!decoder_->decode(packet, frame)) {
            continue;
        }
        
        frames_processed_++;
        frame_counter++;
        
        // Run inference at specified FPS
        bool did_inference = false;
        if (config_.detection.enabled && frame_counter >= inference_interval) {
            frame_counter = 0;
            did_inference = processFrame(frame);  // processFrame runs inference and draws
        }
        
        // Handle frame output (inference frames always output, others at output_interval)
        output_counter++;
        bool should_output = did_inference || (output_counter >= output_interval);
        
        if (should_output && !did_inference) {
            output_counter = 0;
            Frame output_frame = frame;
            
            // Draw cached detections
            if (config_.detection.enabled && graphics_ && !cached_detections_.empty()) {
                drawDetections(output_frame, cached_detections_);
            }
            
            // Store and encode
            {
                std::lock_guard<std::mutex> lock(frame_mutex_);
                latest_frame_ = output_frame;
            }
            
            if (encoder_ && mux_) {
                hal::StreamPacket pkt;
                if (encoder_->encode(output_frame, pkt)) {
                    mux_->writeVideo(pkt);
                    frames_output_++;
                }
            }
        } else if (!should_output) {
            // Just store raw frame for snapshot
            std::lock_guard<std::mutex> lock(frame_mutex_);
            latest_frame_ = frame;
        }
        
        // Update FPS (output FPS, not decode FPS)
        auto now = std::chrono::steady_clock::now();
        auto elapsed = std::chrono::duration<float>(now - last_fps_update).count();
        
        if (elapsed >= 1.0f) {
            int64_t current_output = frames_output_.load();
            fps_ = (current_output - last_output_count) / elapsed;  // Output FPS
            last_output_count = current_output;
            last_fps_update = now;
        }
    }
    
    LOG_DEBUG("Stream processing thread exited: {}", config_.id);
}

bool StreamContext::processFrame(const Frame& frame) {
    if (!yolo_service_) {
        return false;
    }
    
    // Run YOLO detection
    auto detections = yolo_service_->detect(frame);
    
    detections_count_ += detections.size();
    
    // Filter by classes if specified
    if (!config_.detection.classes.empty()) {
        detections.erase(
            std::remove_if(detections.begin(), detections.end(),
                [this](const Detection& d) {
                    return std::find(config_.detection.classes.begin(),
                                    config_.detection.classes.end(),
                                    d.class_id) == config_.detection.classes.end();
                }),
            detections.end());
    }
    
    // Filter by confidence
    detections.erase(
        std::remove_if(detections.begin(), detections.end(),
            [this](const Detection& d) {
                return d.confidence < config_.detection.confidence;
            }),
        detections.end());
    
    // Cache detections for reuse on non-inference frames
    cached_detections_ = detections;
    
    // Run tracker
    std::vector<Track> tracks;
    if (tracker_ && config_.tracking.enabled) {
        tracks = tracker_->update(detections, static_cast<int>(frames_processed_.load()));
    }
    
    // Clone frame for drawing
    Frame draw_frame = frame;
    
    // Draw detections on frame if graphics available
    // Limit to top N detections to avoid cluttering the image
    const size_t MAX_DRAW_DETECTIONS = 50;
    if (graphics_ && !detections.empty()) {
        size_t num_to_draw = std::min(detections.size(), MAX_DRAW_DETECTIONS);
        
        // Color palette for different classes
        static const hal::Color colors[] = {
            {0, 255, 0},    // Green - person
            {255, 0, 0},    // Blue - vehicle
            {0, 0, 255},    // Red - other
            {255, 255, 0},  // Cyan
            {255, 0, 255},  // Magenta
            {0, 255, 255},  // Yellow
        };
        
        for (size_t i = 0; i < num_to_draw; ++i) {
            const auto& det = detections[i];
            // Scale normalized [0,1] coordinates to pixel coordinates
            BBox bbox{
                det.bbox.x1 * draw_frame.width,
                det.bbox.y1 * draw_frame.height,
                det.bbox.x2 * draw_frame.width,
                det.bbox.y2 * draw_frame.height
            };
            hal::Color color = colors[det.class_id % 6];
            
            // Draw bounding box
            graphics_->drawRect(draw_frame, bbox, color, 2);
            
            // Draw label with confidence
            char label[64];
            snprintf(label, sizeof(label), "%s %.0f%%", 
                     det.class_name.c_str(), det.confidence * 100);
            int text_y = static_cast<int>(bbox.y1) - 5;
            if (text_y < 15) text_y = static_cast<int>(bbox.y1) + 15;
            graphics_->drawText(draw_frame, label, 
                               static_cast<int>(bbox.x1), text_y, color);
        }
    }
    
    // Always store the frame (with or without annotations)
    {
        std::lock_guard<std::mutex> lock(frame_mutex_);
        latest_frame_ = draw_frame;
    }
    
    // Push to RTSP output using HAL encoder + mux
    if (encoder_ && mux_) {
        static int enc_frame_count = 0;
        enc_frame_count++;
        
        hal::StreamPacket pkt;
        if (encoder_->encode(draw_frame, pkt)) {
            frames_output_++;  // Count output frames
            if (enc_frame_count % 50 == 1) {
                LOG_INFO("Encoder produced packet: {} bytes, keyframe={}", pkt.data.size(), pkt.keyframe);
            }
            if (!mux_->writeVideo(pkt)) {
                LOG_WARN("Mux writeVideo failed: {}", mux_->getLastError());
            }
        } else {
            if (enc_frame_count % 50 == 1) {
                LOG_DEBUG("Encoder no output yet (frame {})", enc_frame_count);
            }
        }
    }
    
    // Legacy: Push to RtspOutput if enabled (fallback)
    if (rtsp_output_) {
        rtsp_output_->pushFrame(draw_frame);
    }
    
    // Invoke callback
    if (detection_callback_ && (!detections.empty() || !tracks.empty())) {
        detection_callback_(config_.id, detections, tracks);
    }
    
    return true;
}

void StreamContext::drawDetections(Frame& frame, const std::vector<Detection>& detections) {
    if (!graphics_ || detections.empty()) {
        return;
    }
    
    static const hal::Color colors[] = {
        {0, 255, 0},    // Green - person
        {255, 0, 0},    // Blue - vehicle
        {0, 0, 255},    // Red - other
        {255, 255, 0},  // Cyan
        {255, 0, 255},  // Magenta
        {0, 255, 255},  // Yellow
    };
    
    const size_t MAX_DRAW = 50;
    size_t num_to_draw = std::min(detections.size(), MAX_DRAW);
    
    for (size_t i = 0; i < num_to_draw; ++i) {
        const auto& det = detections[i];
        BBox bbox{
            det.bbox.x1 * frame.width,
            det.bbox.y1 * frame.height,
            det.bbox.x2 * frame.width,
            det.bbox.y2 * frame.height
        };
        hal::Color color = colors[det.class_id % 6];
        
        graphics_->drawRect(frame, bbox, color, 2);
        
        char label[64];
        snprintf(label, sizeof(label), "%s %.0f%%", 
                 det.class_name.c_str(), det.confidence * 100);
        int text_y = static_cast<int>(bbox.y1) - 5;
        if (text_y < 15) text_y = static_cast<int>(bbox.y1) + 15;
        graphics_->drawText(frame, label, static_cast<int>(bbox.x1), text_y, color);
    }
}

} // namespace rivision::pipeline
