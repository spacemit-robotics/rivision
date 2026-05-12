#include "byte_track.h"
#include <algorithm>
#include <cmath>
#include <limits>

namespace rivision::core {

// =============================================================================
// KalmanFilter
// =============================================================================

KalmanFilter::KalmanFilter() {
    mean.resize(8, 0.0f);
    covariance.resize(64, 0.0f);
}

void KalmanFilter::init(const BBox& bbox) {
    float cx = bbox.centerX();
    float cy = bbox.centerY();
    float a = bbox.width() / std::max(bbox.height(), 0.001f);
    float h = bbox.height();
    
    mean = {cx, cy, a, h, 0, 0, 0, 0};
    
    // Initialize covariance
    std::fill(covariance.begin(), covariance.end(), 0.0f);
    
    float std_weight_position = 1.0f / 20.0f;
    float std_weight_velocity = 1.0f / 160.0f;
    
    float std[] = {
        2 * std_weight_position * h,
        2 * std_weight_position * h,
        1e-2f,
        2 * std_weight_position * h,
        10 * std_weight_velocity * h,
        10 * std_weight_velocity * h,
        1e-5f,
        10 * std_weight_velocity * h
    };
    
    for (int i = 0; i < 8; ++i) {
        covariance[i * 8 + i] = std[i] * std[i];
    }
}

void KalmanFilter::predict() {
    float std_weight_position = 1.0f / 20.0f;
    float std_weight_velocity = 1.0f / 160.0f;
    
    float std[] = {
        std_weight_position * mean[3],
        std_weight_position * mean[3],
        1e-2f,
        std_weight_position * mean[3],
        std_weight_velocity * mean[3],
        std_weight_velocity * mean[3],
        1e-5f,
        std_weight_velocity * mean[3]
    };
    
    // Update mean: x = Fx
    mean[0] += mean[4];
    mean[1] += mean[5];
    mean[2] += mean[6];
    mean[3] += mean[7];
    
    // Update covariance: P = FPF' + Q
    // Simplified: just add process noise
    for (int i = 0; i < 8; ++i) {
        covariance[i * 8 + i] += std[i] * std[i];
    }
}

void KalmanFilter::update(const BBox& bbox) {
    float cx = bbox.centerX();
    float cy = bbox.centerY();
    float a = bbox.width() / std::max(bbox.height(), 0.001f);
    float h = bbox.height();
    
    float std_weight = 1.0f / 20.0f;
    
    float std[] = {
        std_weight * mean[3],
        std_weight * mean[3],
        1e-1f,
        std_weight * mean[3]
    };
    
    // Kalman gain (simplified)
    float measurement[] = {cx, cy, a, h};
    
    for (int i = 0; i < 4; ++i) {
        float innov_cov = covariance[i * 8 + i] + std[i] * std[i];
        float kalman_gain = covariance[i * 8 + i] / innov_cov;
        
        float innovation = measurement[i] - mean[i];
        mean[i] += kalman_gain * innovation;
        
        covariance[i * 8 + i] *= (1 - kalman_gain);
    }
}

BBox KalmanFilter::getBBox() const {
    float cx = mean[0];
    float cy = mean[1];
    float a = mean[2];
    float h = mean[3];
    float w = a * h;
    
    return {cx - w / 2, cy - h / 2, cx + w / 2, cy + h / 2};
}

BBox KalmanFilter::getPredictedBBox() const {
    return getBBox();
}

// =============================================================================
// STrack
// =============================================================================

STrack::STrack(const Detection& det, TrackId id)
    : track_id(id)
    , detection(det)
    , score(det.confidence) {
    
    kalman.init(det.bbox);
    first_position = {det.bbox.centerX(), det.bbox.centerY()};
    last_position = first_position;
}

void STrack::predict() {
    kalman.predict();
}

void STrack::update(const Detection& det, int frame_id_) {
    this->frame_id = frame_id_;
    tracklet_len++;
    time_since_update = 0;
    
    detection = det;
    score = det.confidence;
    
    // Update Kalman
    kalman.update(det.bbox);
    
    // Update position history
    last_position = {det.bbox.centerX(), det.bbox.centerY()};
    
    if (!is_activated) {
        is_activated = true;
        state = Track::State::TRACKED;
    }
}

void STrack::markLost() {
    state = Track::State::LOST;
}

void STrack::markRemoved() {
    state = Track::State::LOST;  // No REMOVED in our enum
}

BBox STrack::getBBox() const {
    return kalman.getBBox();
}

Track STrack::toTrack() const {
    Track t;
    t.id = track_id;
    t.detection = detection;
    t.detection.bbox = getBBox();
    t.age = tracklet_len;
    t.time_since_update = time_since_update;
    t.hits = tracklet_len;
    t.state = state;
    t.cross_direction = cross_direction;
    t.mean = kalman.mean;
    t.covariance = kalman.covariance;
    return t;
}

// =============================================================================
// ByteTrack
// =============================================================================

ByteTrack::ByteTrack() = default;

ByteTrack::ByteTrack(const Config& config)
    : config_(config) {}

std::vector<Track> ByteTrack::update(const std::vector<Detection>& detections, int frame_id) {
    frame_id_ = frame_id;
    
    // Step 1: Predict all tracked tracks
    for (auto& strack : tracked_stracks_) {
        strack->predict();
    }
    for (auto& strack : lost_stracks_) {
        strack->predict();
    }
    
    // Step 2: Separate high and low score detections
    std::vector<Detection> high_dets, low_dets;
    for (const auto& det : detections) {
        if (det.confidence >= config_.track_thresh) {
            high_dets.push_back(det);
        } else if (det.confidence >= config_.det_thresh) {
            low_dets.push_back(det);
        }
    }
    
    // Step 3: First association with high score detections
    std::vector<STrack*> strack_pool;
    for (auto& st : tracked_stracks_) {
        strack_pool.push_back(st.get());
    }
    
    std::vector<std::pair<int, int>> matches;
    std::vector<int> unmatched_tracks, unmatched_dets;
    
    associateDetections(strack_pool, high_dets, matches, 
                       unmatched_tracks, unmatched_dets, config_.match_thresh);
    
    // Update matched tracks
    for (const auto& [track_idx, det_idx] : matches) {
        strack_pool[track_idx]->update(high_dets[det_idx], frame_id);
    }
    
    // Step 4: Second association with low score detections (for remaining tracks)
    std::vector<STrack*> remaining_tracks;
    for (int idx : unmatched_tracks) {
        remaining_tracks.push_back(strack_pool[idx]);
    }
    
    std::vector<std::pair<int, int>> matches2;
    std::vector<int> unmatched_tracks2, unmatched_dets2;
    
    associateDetections(remaining_tracks, low_dets, matches2,
                       unmatched_tracks2, unmatched_dets2, 0.5f);
    
    for (const auto& [track_idx, det_idx] : matches2) {
        remaining_tracks[track_idx]->update(low_dets[det_idx], frame_id);
    }
    
    // Step 5: Handle unmatched tracks - try to match with lost tracks
    std::vector<Detection> remaining_dets;
    for (int idx : unmatched_dets) {
        remaining_dets.push_back(high_dets[idx]);
    }
    
    std::vector<STrack*> lost_pool;
    for (auto& st : lost_stracks_) {
        lost_pool.push_back(st.get());
    }
    
    std::vector<std::pair<int, int>> matches3;
    std::vector<int> unmatched_lost, unmatched_new_dets;
    
    associateDetections(lost_pool, remaining_dets, matches3,
                       unmatched_lost, unmatched_new_dets, config_.match_thresh);
    
    for (const auto& [track_idx, det_idx] : matches3) {
        auto& lost_track = lost_stracks_[track_idx];
        lost_track->update(remaining_dets[det_idx], frame_id);
        lost_track->state = Track::State::TRACKED;
        
        // Move back to tracked
        tracked_stracks_.push_back(std::move(lost_track));
    }
    
    // Remove matched lost tracks
    std::vector<std::unique_ptr<STrack>> new_lost;
    for (size_t i = 0; i < lost_stracks_.size(); ++i) {
        bool matched = false;
        for (const auto& [track_idx, det_idx] : matches3) {
            if (static_cast<size_t>(track_idx) == i) {
                matched = true;
                break;
            }
        }
        if (!matched && lost_stracks_[i]) {
            new_lost.push_back(std::move(lost_stracks_[i]));
        }
    }
    lost_stracks_ = std::move(new_lost);
    
    // Step 6: Mark still-unmatched tracks as lost
    for (int idx : unmatched_tracks2) {
        auto& track = remaining_tracks[idx];
        track->time_since_update++;
        
        if (track->time_since_update > static_cast<int>(config_.track_buffer)) {
            track->markRemoved();
            removed_stracks_.push_back(std::make_unique<STrack>(*track));
        } else {
            track->markLost();
            // Move to lost pool (handled below)
        }
    }
    
    // Step 7: Create new tracks from remaining detections
    for (int idx : unmatched_new_dets) {
        if (remaining_dets[idx].confidence >= config_.track_thresh) {
            auto new_track = std::make_unique<STrack>(remaining_dets[idx], next_track_id_++);
            new_track->start_frame = frame_id;
            tracked_stracks_.push_back(std::move(new_track));
        }
    }
    
    // Step 8: Move lost tracks
    std::vector<std::unique_ptr<STrack>> still_tracked;
    for (auto& st : tracked_stracks_) {
        if (st->state == Track::State::LOST) {
            if (st->time_since_update <= static_cast<int>(config_.track_buffer)) {
                lost_stracks_.push_back(std::move(st));
            }
        } else if (st) {
            still_tracked.push_back(std::move(st));
        }
    }
    tracked_stracks_ = std::move(still_tracked);
    
    // Cleanup old lost tracks
    std::vector<std::unique_ptr<STrack>> still_lost;
    for (auto& st : lost_stracks_) {
        if (st && st->time_since_update <= static_cast<int>(config_.track_buffer)) {
            still_lost.push_back(std::move(st));
        }
    }
    lost_stracks_ = std::move(still_lost);
    
    // Return active tracks
    return getActiveTracks();
}

std::vector<Track> ByteTrack::getActiveTracks() const {
    std::vector<Track> result;
    
    for (const auto& st : tracked_stracks_) {
        if (st->is_activated && st->tracklet_len >= config_.min_hits) {
            result.push_back(st->toTrack());
        }
    }
    
    return result;
}

std::optional<Track> ByteTrack::getTrack(TrackId id) const {
    for (const auto& st : tracked_stracks_) {
        if (st->track_id == id) {
            return st->toTrack();
        }
    }
    return std::nullopt;
}

void ByteTrack::reset() {
    tracked_stracks_.clear();
    lost_stracks_.clear();
    removed_stracks_.clear();
    frame_id_ = 0;
    next_track_id_ = 1;
}

void ByteTrack::associateDetections(
    const std::vector<STrack*>& tracks,
    const std::vector<Detection>& detections,
    std::vector<std::pair<int, int>>& matches,
    std::vector<int>& unmatched_tracks,
    std::vector<int>& unmatched_detections,
    float thresh) {
    
    matches.clear();
    unmatched_tracks.clear();
    unmatched_detections.clear();
    
    if (tracks.empty() || detections.empty()) {
        for (size_t i = 0; i < tracks.size(); ++i) {
            unmatched_tracks.push_back(i);
        }
        for (size_t i = 0; i < detections.size(); ++i) {
            unmatched_detections.push_back(i);
        }
        return;
    }
    
    // Compute cost matrix (1 - IOU)
    auto iou_matrix = computeIouMatrix(tracks, detections);
    
    std::vector<std::vector<float>> cost_matrix(tracks.size());
    for (size_t i = 0; i < tracks.size(); ++i) {
        cost_matrix[i].resize(detections.size());
        for (size_t j = 0; j < detections.size(); ++j) {
            cost_matrix[i][j] = 1.0f - iou_matrix[i][j];
        }
    }
    
    linearAssignment(cost_matrix, 1.0f - thresh, matches, 
                    unmatched_tracks, unmatched_detections);
}

std::vector<std::vector<float>> ByteTrack::computeIouMatrix(
    const std::vector<STrack*>& tracks,
    const std::vector<Detection>& detections) {
    
    std::vector<std::vector<float>> iou_matrix(tracks.size());
    
    for (size_t i = 0; i < tracks.size(); ++i) {
        iou_matrix[i].resize(detections.size());
        BBox track_bbox = tracks[i]->getBBox();
        
        for (size_t j = 0; j < detections.size(); ++j) {
            iou_matrix[i][j] = track_bbox.iou(detections[j].bbox);
        }
    }
    
    return iou_matrix;
}

void ByteTrack::linearAssignment(
    const std::vector<std::vector<float>>& cost_matrix,
    float thresh,
    std::vector<std::pair<int, int>>& matches,
    std::vector<int>& unmatched_a,
    std::vector<int>& unmatched_b) {
    
    // Simplified greedy assignment
    int rows = cost_matrix.size();
    int cols = cost_matrix[0].size();
    
    std::vector<bool> row_matched(rows, false);
    std::vector<bool> col_matched(cols, false);
    
    // Find best matches greedily
    for (int iter = 0; iter < std::min(rows, cols); ++iter) {
        float best_cost = std::numeric_limits<float>::max();
        int best_row = -1, best_col = -1;
        
        for (int i = 0; i < rows; ++i) {
            if (row_matched[i]) continue;
            
            for (int j = 0; j < cols; ++j) {
                if (col_matched[j]) continue;
                
                if (cost_matrix[i][j] < best_cost && cost_matrix[i][j] < thresh) {
                    best_cost = cost_matrix[i][j];
                    best_row = i;
                    best_col = j;
                }
            }
        }
        
        if (best_row >= 0 && best_col >= 0) {
            matches.push_back({best_row, best_col});
            row_matched[best_row] = true;
            col_matched[best_col] = true;
        } else {
            break;
        }
    }
    
    // Collect unmatched
    for (int i = 0; i < rows; ++i) {
        if (!row_matched[i]) {
            unmatched_a.push_back(i);
        }
    }
    
    for (int j = 0; j < cols; ++j) {
        if (!col_matched[j]) {
            unmatched_b.push_back(j);
        }
    }
}

} // namespace rivision::core
