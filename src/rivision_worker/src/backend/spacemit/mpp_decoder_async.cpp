/**
 * @file mpp_decoder_async.cpp
 * @brief Asynchronous VPU decoder implementation
 */

#include "mpp_decoder_async.h"
#include "mpp_decoder.h"
#include <chrono>

namespace rivision {
namespace spacemit {

MppDecoderAsync::MppDecoderAsync()
    : input_queue_(16)
    , output_queue_(8) {
}

MppDecoderAsync::~MppDecoderAsync() {
    close();
}

hal::ErrorCode MppDecoderAsync::open(const Config& config) {
    if (running_.load()) {
        return hal::ErrorCode::ALREADY_INITIALIZED;
    }
    
    config_ = config;
    
    // Create underlying sync decoder
    decoder_ = std::make_unique<MppDecoder>();
    
    hal::DecoderConfig dec_cfg;
    dec_cfg.codec = config.codec;
    dec_cfg.output_format = hal::PixelFormat::NV12;
    dec_cfg.extra_frame_buffers = config.output_queue_size;
    
    auto ret = decoder_->open(dec_cfg);
    if (ret != hal::ErrorCode::SUCCESS) {
        decoder_.reset();
        return ret;
    }
    
    // Initialize queues
    output_queue_ = pipeline::FrameQueue<hal::Frame>(config.output_queue_size);
    input_queue_.start();
    output_queue_.start();
    
    // Start decode thread
    running_.store(true);
    decode_thread_ = std::thread(&MppDecoderAsync::decodeThread, this);
    
    return hal::ErrorCode::SUCCESS;
}

void MppDecoderAsync::close() {
    if (!running_.load()) {
        return;
    }
    
    running_.store(false);
    input_queue_.stop();
    output_queue_.stop();
    
    if (decode_thread_.joinable()) {
        decode_thread_.join();
    }
    
    if (decoder_) {
        decoder_->close();
        decoder_.reset();
    }
}

hal::ErrorCode MppDecoderAsync::submitPacket(const hal::StreamPacket& packet) {
    if (!running_.load()) {
        return hal::ErrorCode::NOT_INITIALIZED;
    }
    
    if (input_queue_.full()) {
        return hal::ErrorCode::BUFFER_FULL;
    }
    
    // Copy packet to queue
    hal::StreamPacket pkt_copy = packet;
    input_queue_.push(std::move(pkt_copy));
    
    return hal::ErrorCode::SUCCESS;
}

void MppDecoderAsync::setFrameCallback(FrameCallback callback) {
    std::lock_guard<std::mutex> lock(callback_mutex_);
    frame_callback_ = std::move(callback);
}

void MppDecoderAsync::setErrorCallback(ErrorCallback callback) {
    std::lock_guard<std::mutex> lock(callback_mutex_);
    error_callback_ = std::move(callback);
}

bool MppDecoderAsync::tryGetFrame(hal::Frame& frame) {
    return output_queue_.tryPop(frame);
}

bool MppDecoderAsync::waitFrame(hal::Frame& frame, int timeout_ms) {
    auto start = std::chrono::steady_clock::now();
    
    while (running_.load()) {
        if (output_queue_.tryPop(frame)) {
            return true;
        }
        
        auto elapsed = std::chrono::steady_clock::now() - start;
        if (std::chrono::duration_cast<std::chrono::milliseconds>(elapsed).count() >= timeout_ms) {
            return false;
        }
        
        std::this_thread::sleep_for(std::chrono::milliseconds(1));
    }
    
    return false;
}

MppDecoderAsync::Stats MppDecoderAsync::getStats() const {
    std::lock_guard<std::mutex> lock(stats_mutex_);
    Stats s = stats_;
    s.queue_depth = output_queue_.size();
    return s;
}

int MppDecoderAsync::getQueueDepth() const {
    return output_queue_.size();
}

void MppDecoderAsync::decodeThread() {
    hal::StreamPacket packet;
    hal::Frame frame;
    
    while (running_.load()) {
        // Get packet from input queue
        if (!input_queue_.tryPop(packet)) {
            std::this_thread::sleep_for(std::chrono::milliseconds(1));
            continue;
        }
        
        auto start = std::chrono::steady_clock::now();
        
        // Decode packet
        auto ret = decoder_->decode(packet, frame);
        
        auto end = std::chrono::steady_clock::now();
        double decode_ms = std::chrono::duration<double, std::milli>(end - start).count();
        
        if (ret == hal::ErrorCode::SUCCESS) {
            // Update statistics
            updateStats(decode_ms);
            
            // Try to push to output queue
            if (output_queue_.full()) {
                // Drop oldest frame if queue full
                hal::Frame dropped;
                output_queue_.tryPop(dropped);
                
                std::lock_guard<std::mutex> lock(stats_mutex_);
                stats_.frames_dropped++;
            }
            
            // Deliver frame via callback or queue
            {
                std::lock_guard<std::mutex> lock(callback_mutex_);
                if (frame_callback_) {
                    hal::Frame frame_copy = frame;
                    frame_callback_(std::move(frame_copy));
                }
            }
            
            output_queue_.push(std::move(frame));
            
        } else if (ret == hal::ErrorCode::NEED_MORE_DATA) {
            // Normal case - need more packets
            continue;
            
        } else {
            // Decode error
            {
                std::lock_guard<std::mutex> lock(stats_mutex_);
                stats_.decode_errors++;
            }
            
            std::lock_guard<std::mutex> lock(callback_mutex_);
            if (error_callback_) {
                error_callback_(ret, "Decode failed");
            }
        }
    }
}

void MppDecoderAsync::updateStats(double decode_ms) {
    std::lock_guard<std::mutex> lock(stats_mutex_);
    
    stats_.frames_decoded++;
    total_decode_ms_ += decode_ms;
    stats_.avg_decode_ms = total_decode_ms_ / stats_.frames_decoded;
    
    if (decode_ms > stats_.max_decode_ms) {
        stats_.max_decode_ms = decode_ms;
    }
}

} // namespace spacemit
} // namespace rivision
