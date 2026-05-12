/*
 *------------------------------------------------------------------------------
 * Copyright 2025-2026 SPACEMIT. All rights reserved.
 * BSD-style license.
 *------------------------------------------------------------------------------
 */

#include "mpp_encoder_lowlatency.h"
#include "mpp_common.h"
#include <chrono>
#include <cstring>

#ifdef USE_SPACEMIT_MPP
#include "venc_api.h"
#endif

namespace rivision::spacemit {

class MppEncoderLowLatency::Impl {
public:
#ifdef USE_SPACEMIT_MPP
    VENC_HANDLE venc_handle_ = nullptr;
    VENC_CHN venc_chn_ = -1;
#endif
    EncodeStats stats_;
    bool is_open_ = false;
    bool force_keyframe_ = false;
    
    std::chrono::steady_clock::time_point last_encode_start_;
    uint64_t total_encode_us_ = 0;
    uint64_t total_bytes_ = 0;
};

MppEncoderLowLatency::MppEncoderLowLatency()
    : impl_(std::make_unique<Impl>())
{
}

MppEncoderLowLatency::~MppEncoderLowLatency() {
    close();
}

bool MppEncoderLowLatency::open(const Config& cfg) {
    LowLatencyConfig ll_cfg;
    ll_cfg.codec = cfg.codec;
    ll_cfg.width = cfg.width;
    ll_cfg.height = cfg.height;
    ll_cfg.fps = cfg.fps;
    ll_cfg.bitrate = cfg.bitrate;
    ll_cfg.rc_mode = cfg.rc_mode;
    return openLowLatency(ll_cfg);
}

bool MppEncoderLowLatency::openLowLatency(const LowLatencyConfig& cfg) {
    if (impl_->is_open_) {
        close();
    }
    
    ll_config_ = cfg;
    
#ifdef USE_SPACEMIT_MPP
    // Allocate encoder channel
    impl_->venc_chn_ = ChannelManager::instance().allocate(ChannelType::ENCODER);
    if (impl_->venc_chn_ < 0) {
        last_error_ = "No encoder channel available";
        return false;
    }
    
    VENC_CONFIG venc_cfg = {};
    venc_cfg.codec = (cfg.codec == hal::CodecType::H264) ? 
                      VENC_CODEC_H264 : VENC_CODEC_H265;
    venc_cfg.width = cfg.width;
    venc_cfg.height = cfg.height;
    venc_cfg.fps = cfg.fps;
    venc_cfg.bitrate = cfg.bitrate;
    
    // Low-latency settings
    venc_cfg.profile = VENC_PROFILE_BASELINE;  // No B-frames
    venc_cfg.gop_size = cfg.gdr_refresh_period;
    venc_cfg.max_qp = cfg.max_qp;
    venc_cfg.min_qp = cfg.min_qp;
    
    // Enable GDR
    if (cfg.enable_gradual_refresh) {
        venc_cfg.refresh_mode = VENC_REFRESH_GDR;
        venc_cfg.gdr_period = cfg.gdr_refresh_period;
    } else {
        venc_cfg.refresh_mode = VENC_REFRESH_IDR;
    }
    
    // Slices for parallelism
    venc_cfg.slice_mode = VENC_SLICE_FIXED_COUNT;
    venc_cfg.slice_count = cfg.slice_count;
    
    // Zero-latency
    if (cfg.zero_latency_mode) {
        venc_cfg.rc_mode = VENC_RC_CBR;
        venc_cfg.vbv_buffer_size = cfg.vbv_buffer_size ? 
                                   cfg.vbv_buffer_size : (cfg.bitrate / cfg.fps);
    }
    
    int ret = VENC_Create(impl_->venc_chn_, &venc_cfg, &impl_->venc_handle_);
    if (ret != 0) {
        ChannelManager::instance().release(ChannelType::ENCODER, impl_->venc_chn_);
        last_error_ = "VENC_Create failed: " + std::to_string(ret);
        return false;
    }
    
    ret = VENC_Start(impl_->venc_handle_);
    if (ret != 0) {
        VENC_Destroy(impl_->venc_handle_);
        ChannelManager::instance().release(ChannelType::ENCODER, impl_->venc_chn_);
        last_error_ = "VENC_Start failed: " + std::to_string(ret);
        return false;
    }
    
    impl_->is_open_ = true;
    return true;
#else
    impl_->is_open_ = true;
    return true;
#endif
}

void MppEncoderLowLatency::close() {
    if (!impl_->is_open_) return;
    
#ifdef USE_SPACEMIT_MPP
    if (impl_->venc_handle_) {
        VENC_Stop(impl_->venc_handle_);
        VENC_Destroy(impl_->venc_handle_);
        impl_->venc_handle_ = nullptr;
    }
    if (impl_->venc_chn_ >= 0) {
        ChannelManager::instance().release(ChannelType::ENCODER, impl_->venc_chn_);
        impl_->venc_chn_ = -1;
    }
#endif
    
    impl_->is_open_ = false;
}

bool MppEncoderLowLatency::encode(const hal::Frame& frame, hal::StreamPacket& out_packet) {
    if (!impl_->is_open_) {
        last_error_ = "Encoder not open";
        return false;
    }
    
    impl_->last_encode_start_ = std::chrono::steady_clock::now();
    
#ifdef USE_SPACEMIT_MPP
    // Apply ROI if configured
    if (ll_config_.enable_roi && !roi_regions_.empty()) {
        applyRoiMap();
    }
    
    VENC_FRAME input = {};
    input.width = frame.width;
    input.height = frame.height;
    input.format = VENC_FORMAT_NV12;
    input.pts = frame.timestamp;
    input.data = const_cast<uint8_t*>(frame.data.data());
    input.size = frame.data.size();
    
    if (impl_->force_keyframe_) {
        input.force_idr = 1;
        impl_->force_keyframe_ = false;
    }
    
    VENC_STREAM output = {};
    int ret = VENC_SendFrame(impl_->venc_handle_, &input, 1000);
    if (ret != 0) {
        last_error_ = "VENC_SendFrame failed: " + std::to_string(ret);
        return false;
    }
    
    ret = VENC_GetStream(impl_->venc_handle_, &output, 1000);
    if (ret != 0) {
        last_error_ = "VENC_GetStream failed: " + std::to_string(ret);
        return false;
    }
    
    out_packet.data.resize(output.size);
    std::memcpy(out_packet.data.data(), output.data, output.size);
    out_packet.pts = output.pts;
    out_packet.dts = output.dts;
    out_packet.keyframe = output.is_keyframe;
    out_packet.type = hal::StreamPacket::Type::VIDEO;
    
    VENC_ReleaseStream(impl_->venc_handle_, &output);
    
#else
    // Stub for testing without MPP
    out_packet.data = frame.data;
    out_packet.pts = frame.timestamp;
    out_packet.dts = frame.timestamp;
    out_packet.keyframe = (impl_->stats_.total_frames % ll_config_.gdr_refresh_period == 0);
    out_packet.type = hal::StreamPacket::Type::VIDEO;
#endif
    
    auto end = std::chrono::steady_clock::now();
    auto us = std::chrono::duration_cast<std::chrono::microseconds>(
        end - impl_->last_encode_start_).count();
    impl_->total_encode_us_ += us;
    impl_->total_bytes_ += out_packet.data.size();
    impl_->stats_.total_frames++;
    
    if (out_packet.keyframe) {
        impl_->stats_.keyframes++;
    }
    
    return true;
}

bool MppEncoderLowLatency::flush(hal::StreamPacket& out_packet) {
#ifdef USE_SPACEMIT_MPP
    if (!impl_->venc_handle_) return false;
    
    VENC_STREAM output = {};
    int ret = VENC_GetStream(impl_->venc_handle_, &output, 100);
    if (ret != 0) return false;
    
    out_packet.data.resize(output.size);
    std::memcpy(out_packet.data.data(), output.data, output.size);
    out_packet.pts = output.pts;
    out_packet.dts = output.dts;
    out_packet.keyframe = output.is_keyframe;
    
    VENC_ReleaseStream(impl_->venc_handle_, &output);
    return true;
#else
    (void)out_packet;
    return false;
#endif
}

void MppEncoderLowLatency::setRoiRegions(const std::vector<RoiRegion>& regions) {
    roi_regions_ = regions;
}

void MppEncoderLowLatency::clearRoiRegions() {
    roi_regions_.clear();
}

void MppEncoderLowLatency::setRoiFromDetections(const std::vector<hal::Tensor>& detections,
                                                 int frame_width, int frame_height) {
    roi_regions_.clear();
    
    // Parse YOLO output format [x1, y1, x2, y2, conf, class]
    // Simplified: assume first tensor is detections
    if (detections.empty() || detections[0].data.empty()) return;
    
    const auto& det = detections[0];
    size_t num_dets = det.data.size() / 6;
    
    for (size_t i = 0; i < num_dets && i < 10; i++) {  // Max 10 ROIs
        size_t offset = i * 6;
        float x1 = det.data[offset] * frame_width;
        float y1 = det.data[offset + 1] * frame_height;
        float x2 = det.data[offset + 2] * frame_width;
        float y2 = det.data[offset + 3] * frame_height;
        float conf = det.data[offset + 4];
        
        if (conf < 0.5f) continue;
        
        RoiRegion roi;
        roi.x = static_cast<int>(x1);
        roi.y = static_cast<int>(y1);
        roi.width = static_cast<int>(x2 - x1);
        roi.height = static_cast<int>(y2 - y1);
        roi.qp_delta = -5.0f;  // Higher quality for detections
        roi.priority = static_cast<int>(conf * 100);
        
        roi_regions_.push_back(roi);
    }
}

void MppEncoderLowLatency::setBitrate(int bitrate) {
    ll_config_.bitrate = bitrate;
    
#ifdef USE_SPACEMIT_MPP
    if (impl_->venc_handle_) {
        VENC_SetBitrate(impl_->venc_handle_, bitrate);
    }
#endif
}

void MppEncoderLowLatency::setQpRange(int min_qp, int max_qp) {
    ll_config_.min_qp = min_qp;
    ll_config_.max_qp = max_qp;
    
#ifdef USE_SPACEMIT_MPP
    if (impl_->venc_handle_) {
        VENC_SetQpRange(impl_->venc_handle_, min_qp, max_qp);
    }
#endif
}

void MppEncoderLowLatency::forceKeyframe() {
    impl_->force_keyframe_ = true;
}

MppEncoderLowLatency::EncodeStats MppEncoderLowLatency::getStats() const {
    EncodeStats stats = impl_->stats_;
    
    if (stats.total_frames > 0) {
        stats.avg_encode_ms = static_cast<float>(impl_->total_encode_us_) / 
                              stats.total_frames / 1000.0f;
        // Approximate bitrate from total bytes
        stats.avg_bitrate_kbps = static_cast<float>(impl_->total_bytes_ * 8) /
                                  stats.total_frames * ll_config_.fps / 1000.0f;
    }
    
    return stats;
}

void MppEncoderLowLatency::applyRoiMap() {
#ifdef USE_SPACEMIT_MPP
    if (!impl_->venc_handle_ || roi_regions_.empty()) return;
    
    VENC_ROI_CONFIG roi_cfg = {};
    roi_cfg.roi_count = std::min(roi_regions_.size(), (size_t)8);
    
    for (size_t i = 0; i < roi_cfg.roi_count; i++) {
        roi_cfg.rois[i].x = roi_regions_[i].x;
        roi_cfg.rois[i].y = roi_regions_[i].y;
        roi_cfg.rois[i].width = roi_regions_[i].width;
        roi_cfg.rois[i].height = roi_regions_[i].height;
        roi_cfg.rois[i].qp_delta = static_cast<int>(roi_regions_[i].qp_delta);
    }
    
    VENC_SetRoi(impl_->venc_handle_, &roi_cfg);
#endif
}

} // namespace rivision::spacemit
