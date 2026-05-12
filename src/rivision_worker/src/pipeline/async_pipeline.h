/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *
 * @File      :    async_pipeline.h
 * @Brief     :    Asynchronous video analysis pipeline with pipelining.
 *
 * Optimization: Pipeline parallelism to reduce end-to-end latency.
 * Before: Demux→Decode→Infer→Post (serial, ~40ms per frame)
 * After:  Overlapped stages, steady-state latency ~15ms
 *------------------------------------------------------------------------------
 */

#pragma once

#include "stage.h"
#include "frame_queue.h"
#include "hal/interface/i_demux.h"
#include "hal/interface/i_decoder.h"
#include "hal/interface/i_inference.h"

#include <atomic>
#include <functional>
#include <memory>
#include <thread>
#include <vector>

namespace rivision::pipeline {

struct InferenceResult {
    hal::Frame frame;
    std::vector<hal::Tensor> tensors;
    int64_t timestamp;
    float demux_ms;
    float decode_ms;
    float infer_ms;
};

struct LatencyStats {
    float demux_ms = 0;
    float decode_ms = 0;
    float infer_ms = 0;
    float e2e_ms = 0;       // End-to-end (first frame)
    float steady_ms = 0;    // Steady-state frame interval
    uint64_t frames_processed = 0;
};

class AsyncPipeline {
public:
    using ResultCallback = std::function<void(InferenceResult)>;
    using ErrorCallback = std::function<void(const std::string&)>;
    
    struct Config {
        size_t decode_queue_size = 4;
        size_t infer_queue_size = 2;
        size_t result_queue_size = 4;
        bool enable_skip_frames = true;
        int max_skip_frames = 2;
    };
    
    AsyncPipeline(
        std::shared_ptr<hal::IDemux> demux,
        std::shared_ptr<hal::IDecoder> decoder,
        std::shared_ptr<hal::IInference> inference,
        Config config = {});
    
    ~AsyncPipeline();
    
    void start();
    void stop();
    bool isRunning() const { return running_; }
    
    void setResultCallback(ResultCallback cb) { result_callback_ = std::move(cb); }
    void setErrorCallback(ErrorCallback cb) { error_callback_ = std::move(cb); }
    
    bool tryPopResult(InferenceResult& result);
    InferenceResult waitResult();
    
    LatencyStats getLatencyStats() const;
    
    void pause();
    void resume();
    bool isPaused() const { return paused_; }

private:
    void demuxLoop();
    void decodeLoop();
    void inferLoop();
    
    void reportError(const std::string& msg);
    
    std::shared_ptr<hal::IDemux> demux_;
    std::shared_ptr<hal::IDecoder> decoder_;
    std::shared_ptr<hal::IInference> inference_;
    
    Config config_;
    
    PacketQueue packet_queue_;
    DecodedFrameQueue frame_queue_;
    FrameQueue<InferenceResult> result_queue_;
    
    std::thread demux_thread_;
    std::thread decode_thread_;
    std::thread infer_thread_;
    
    std::atomic<bool> running_{false};
    std::atomic<bool> paused_{false};
    
    ResultCallback result_callback_;
    ErrorCallback error_callback_;
    
    // Latency tracking
    std::atomic<uint64_t> total_demux_us_{0};
    std::atomic<uint64_t> total_decode_us_{0};
    std::atomic<uint64_t> total_infer_us_{0};
    std::atomic<uint64_t> frame_count_{0};
    
    std::chrono::steady_clock::time_point first_frame_start_;
    std::chrono::steady_clock::time_point first_frame_end_;
    std::atomic<bool> first_frame_done_{false};
};

} // namespace rivision::pipeline
