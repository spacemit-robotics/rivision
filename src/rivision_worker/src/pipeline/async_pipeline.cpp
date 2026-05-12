/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *------------------------------------------------------------------------------
 */

#include "async_pipeline.h"
#include <chrono>

namespace rivision::pipeline {

AsyncPipeline::AsyncPipeline(
    std::shared_ptr<hal::IDemux> demux,
    std::shared_ptr<hal::IDecoder> decoder,
    std::shared_ptr<hal::IInference> inference,
    Config config)
    : demux_(std::move(demux))
    , decoder_(std::move(decoder))
    , inference_(std::move(inference))
    , config_(config)
    , packet_queue_(config.decode_queue_size)
    , frame_queue_(config.infer_queue_size)
    , result_queue_(config.result_queue_size)
{
}

AsyncPipeline::~AsyncPipeline() {
    stop();
}

void AsyncPipeline::start() {
    if (running_.exchange(true)) return;
    
    packet_queue_.start();
    frame_queue_.start();
    result_queue_.start();
    
    first_frame_done_ = false;
    first_frame_start_ = std::chrono::steady_clock::now();
    
    demux_thread_ = std::thread(&AsyncPipeline::demuxLoop, this);
    decode_thread_ = std::thread(&AsyncPipeline::decodeLoop, this);
    infer_thread_ = std::thread(&AsyncPipeline::inferLoop, this);
}

void AsyncPipeline::stop() {
    if (!running_.exchange(false)) return;
    
    packet_queue_.stop();
    frame_queue_.stop();
    result_queue_.stop();
    
    if (demux_thread_.joinable()) demux_thread_.join();
    if (decode_thread_.joinable()) decode_thread_.join();
    if (infer_thread_.joinable()) infer_thread_.join();
}

void AsyncPipeline::pause() {
    paused_ = true;
}

void AsyncPipeline::resume() {
    paused_ = false;
}

void AsyncPipeline::demuxLoop() {
    while (running_) {
        if (paused_) {
            std::this_thread::sleep_for(std::chrono::milliseconds(10));
            continue;
        }
        
        auto start = std::chrono::steady_clock::now();
        
        hal::StreamPacket packet;
        if (!demux_->readPacket(packet)) {
            if (demux_->isOpen()) {
                std::this_thread::sleep_for(std::chrono::milliseconds(1));
                continue;
            }
            break;
        }
        
        auto end = std::chrono::steady_clock::now();
        auto us = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
        total_demux_us_ += us;
        
        // Skip frames if queue is backing up
        if (config_.enable_skip_frames && 
            packet_queue_.size() >= config_.decode_queue_size - 1 &&
            !packet.keyframe) {
            continue;
        }
        
        packet_queue_.push(std::move(packet));
    }
}

void AsyncPipeline::decodeLoop() {
    while (running_) {
        auto packet_opt = packet_queue_.pop(true);
        if (!packet_opt) continue;
        
        if (paused_) {
            std::this_thread::sleep_for(std::chrono::milliseconds(10));
            continue;
        }
        
        auto start = std::chrono::steady_clock::now();
        
        hal::Frame frame;
        if (!decoder_->decode(*packet_opt, frame)) {
            continue;
        }
        
        if (frame.data.empty()) continue;
        
        auto end = std::chrono::steady_clock::now();
        auto us = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
        total_decode_us_ += us;
        
        // Skip frames if inference queue is backing up
        if (config_.enable_skip_frames && 
            frame_queue_.size() >= config_.infer_queue_size - 1) {
            continue;
        }
        
        frame_queue_.push(std::move(frame));
    }
}

void AsyncPipeline::inferLoop() {
    while (running_) {
        auto frame_opt = frame_queue_.pop(true);
        if (!frame_opt) continue;
        
        if (paused_) {
            std::this_thread::sleep_for(std::chrono::milliseconds(10));
            continue;
        }
        
        auto start = std::chrono::steady_clock::now();
        
        std::vector<hal::Tensor> outputs;
        if (!inference_->infer(*frame_opt, outputs)) {
            reportError("Inference failed");
            continue;
        }
        
        auto end = std::chrono::steady_clock::now();
        auto us = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
        total_infer_us_ += us;
        frame_count_++;
        
        if (!first_frame_done_.exchange(true)) {
            first_frame_end_ = end;
        }
        
        InferenceResult result;
        result.frame = std::move(*frame_opt);
        result.tensors = std::move(outputs);
        result.timestamp = result.frame.timestamp;
        result.demux_ms = static_cast<float>(total_demux_us_) / frame_count_ / 1000.0f;
        result.decode_ms = static_cast<float>(total_decode_us_) / frame_count_ / 1000.0f;
        result.infer_ms = static_cast<float>(us) / 1000.0f;
        
        if (result_callback_) {
            result_callback_(result);
        }
        
        result_queue_.push(std::move(result), false);
    }
}

bool AsyncPipeline::tryPopResult(InferenceResult& result) {
    return result_queue_.tryPop(result);
}

InferenceResult AsyncPipeline::waitResult() {
    auto result_opt = result_queue_.pop(true);
    if (result_opt) {
        return std::move(*result_opt);
    }
    return {};
}

LatencyStats AsyncPipeline::getLatencyStats() const {
    LatencyStats stats;
    uint64_t count = frame_count_.load();
    
    if (count > 0) {
        stats.demux_ms = static_cast<float>(total_demux_us_) / count / 1000.0f;
        stats.decode_ms = static_cast<float>(total_decode_us_) / count / 1000.0f;
        stats.infer_ms = static_cast<float>(total_infer_us_) / count / 1000.0f;
        stats.frames_processed = count;
        
        // Steady-state: max of individual stage latencies (pipeline)
        stats.steady_ms = std::max({stats.demux_ms, stats.decode_ms, stats.infer_ms});
        
        // E2E for first frame
        if (first_frame_done_) {
            auto e2e_us = std::chrono::duration_cast<std::chrono::microseconds>(
                first_frame_end_ - first_frame_start_).count();
            stats.e2e_ms = static_cast<float>(e2e_us) / 1000.0f;
        }
    }
    
    return stats;
}

void AsyncPipeline::reportError(const std::string& msg) {
    if (error_callback_) {
        error_callback_(msg);
    }
}

} // namespace rivision::pipeline
