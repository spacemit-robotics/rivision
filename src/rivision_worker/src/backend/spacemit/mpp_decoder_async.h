/**
 * @file mpp_decoder_async.h
 * @brief Asynchronous VPU decoder wrapper for SpacemiT K3
 * 
 * Provides non-blocking decode API with callback-based frame delivery.
 * Uses separate decode thread to overlap decoding with inference.
 */

#ifndef RIVISION_BACKEND_SPACEMIT_MPP_DECODER_ASYNC_H
#define RIVISION_BACKEND_SPACEMIT_MPP_DECODER_ASYNC_H

#include "hal/interface/i_decoder.h"
#include "pipeline/frame_queue.h"
#include <memory>
#include <thread>
#include <atomic>
#include <functional>

namespace rivision {
namespace spacemit {

/**
 * @brief Async decoder wrapping MppDecoder with threaded output
 */
class MppDecoderAsync {
public:
    using FrameCallback = std::function<void(hal::Frame&& frame)>;
    using ErrorCallback = std::function<void(hal::ErrorCode code, const std::string& msg)>;

    struct Config {
        hal::CodecType codec = hal::CodecType::H264;
        int output_queue_size = 8;      // Max frames in output queue
        bool enable_zero_copy = true;   // Use VB direct output
        int timeout_ms = 100;           // Decode timeout per frame
    };

    struct Stats {
        uint64_t frames_decoded = 0;
        uint64_t frames_dropped = 0;
        uint64_t decode_errors = 0;
        double avg_decode_ms = 0.0;
        double max_decode_ms = 0.0;
        int queue_depth = 0;            // Current output queue depth
    };

public:
    MppDecoderAsync();
    ~MppDecoderAsync();

    /**
     * @brief Initialize async decoder
     * @param config Decoder configuration
     * @return ErrorCode::SUCCESS on success
     */
    hal::ErrorCode open(const Config& config);

    /**
     * @brief Shutdown decoder and release resources
     */
    void close();

    /**
     * @brief Submit packet for async decoding (non-blocking)
     * @param packet Compressed video packet
     * @return ErrorCode::SUCCESS if queued, QUEUE_FULL if buffer full
     */
    hal::ErrorCode submitPacket(const hal::StreamPacket& packet);

    /**
     * @brief Set callback for decoded frames
     * @param callback Function called when frame is ready
     */
    void setFrameCallback(FrameCallback callback);

    /**
     * @brief Set callback for decode errors
     * @param callback Function called on decode error
     */
    void setErrorCallback(ErrorCallback callback);

    /**
     * @brief Try to get next decoded frame (non-blocking)
     * @param frame Output frame
     * @return true if frame available, false if queue empty
     */
    bool tryGetFrame(hal::Frame& frame);

    /**
     * @brief Wait for next decoded frame (blocking)
     * @param frame Output frame
     * @param timeout_ms Max wait time
     * @return true if frame received, false on timeout
     */
    bool waitFrame(hal::Frame& frame, int timeout_ms = 100);

    /**
     * @brief Get decoder statistics
     */
    Stats getStats() const;

    /**
     * @brief Check if decoder is running
     */
    bool isRunning() const { return running_.load(); }

    /**
     * @brief Get current output queue depth
     */
    int getQueueDepth() const;

private:
    void decodeThread();
    void updateStats(double decode_ms);

private:
    std::unique_ptr<hal::IDecoder> decoder_;
    Config config_;
    
    // Input queue (packets to decode)
    pipeline::FrameQueue<hal::StreamPacket> input_queue_;
    
    // Output queue (decoded frames)
    pipeline::FrameQueue<hal::Frame> output_queue_;
    
    // Decode thread
    std::thread decode_thread_;
    std::atomic<bool> running_{false};
    
    // Callbacks
    FrameCallback frame_callback_;
    ErrorCallback error_callback_;
    std::mutex callback_mutex_;
    
    // Statistics
    mutable std::mutex stats_mutex_;
    Stats stats_;
    double total_decode_ms_ = 0.0;
};

} // namespace spacemit
} // namespace rivision

#endif // RIVISION_BACKEND_SPACEMIT_MPP_DECODER_ASYNC_H
