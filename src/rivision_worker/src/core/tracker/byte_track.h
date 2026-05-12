#pragma once

#include "rivision/types.h"
#include <vector>
#include <unordered_map>

namespace rivision::core {

// =============================================================================
// KalmanFilter - 8-state Kalman filter for tracking
// =============================================================================

class KalmanFilter {
public:
    KalmanFilter();
    
    // Initialize with detection bbox [x, y, a, h] where a = aspect ratio
    void init(const BBox& bbox);
    
    // Predict next state
    void predict();
    
    // Update with detection
    void update(const BBox& bbox);
    
    // Get current state as bbox
    BBox getBBox() const;
    
    // Get predicted bbox (after predict, before update)
    BBox getPredictedBBox() const;
    
    // State: [x, y, a, h, vx, vy, va, vh]
    std::vector<float> mean;
    std::vector<float> covariance;  // 8x8 flattened
    
private:
    static constexpr float chi2inv95[9] = {
        0, 3.841459, 5.991465, 7.814728, 9.487729,
        11.070498, 12.591587, 14.067140, 15.507313
    };
};

// =============================================================================
// STrack - Single track
// =============================================================================

class STrack {
public:
    STrack(const Detection& det, TrackId id);
    
    // Predict next position
    void predict();
    
    // Update with matched detection
    void update(const Detection& det, int frame_id);
    
    // Mark as lost
    void markLost();
    
    // Mark as removed
    void markRemoved();
    
    // Get current bbox
    BBox getBBox() const;
    
    // Convert to Track
    Track toTrack() const;
    
    // State
    TrackId track_id;
    Detection detection;
    KalmanFilter kalman;
    
    Track::State state = Track::State::NEW;
    int start_frame = 0;
    int frame_id = 0;
    int tracklet_len = 0;
    int time_since_update = 0;
    
    float score = 0.0f;
    bool is_activated = false;
    
    // Line crossing
    Track::CrossDirection cross_direction = Track::CrossDirection::NONE;
    std::pair<float, float> first_position;
    std::pair<float, float> last_position;
};

// =============================================================================
// ByteTrack - Multi-object tracker
// =============================================================================

class ByteTrack {
public:
    struct Config {
        float track_thresh = 0.5f;      // Detection threshold for tracking
        float track_buffer = 30;         // Frames to keep lost tracks
        float match_thresh = 0.8f;       // IOU threshold for matching
        int min_hits = 3;                // Minimum hits to confirm track
        float det_thresh = 0.3f;         // Low detection threshold
    };
    
    ByteTrack();
    explicit ByteTrack(const Config& config);
    
    // Update tracker with new detections
    std::vector<Track> update(const std::vector<Detection>& detections, int frame_id);
    
    // Get active tracks
    std::vector<Track> getActiveTracks() const;
    
    // Get track by ID
    std::optional<Track> getTrack(TrackId id) const;
    
    // Reset tracker
    void reset();
    
private:
    // Association using IOU
    void associateDetections(
        const std::vector<STrack*>& tracks,
        const std::vector<Detection>& detections,
        std::vector<std::pair<int, int>>& matches,
        std::vector<int>& unmatched_tracks,
        std::vector<int>& unmatched_detections,
        float thresh);
    
    // Compute IOU matrix
    std::vector<std::vector<float>> computeIouMatrix(
        const std::vector<STrack*>& tracks,
        const std::vector<Detection>& detections);
    
    // Hungarian algorithm (simplified)
    void linearAssignment(
        const std::vector<std::vector<float>>& cost_matrix,
        float thresh,
        std::vector<std::pair<int, int>>& matches,
        std::vector<int>& unmatched_a,
        std::vector<int>& unmatched_b);
    
    Config config_;
    
    // Track pools
    std::vector<std::unique_ptr<STrack>> tracked_stracks_;
    std::vector<std::unique_ptr<STrack>> lost_stracks_;
    std::vector<std::unique_ptr<STrack>> removed_stracks_;
    
    int frame_id_ = 0;
    TrackId next_track_id_ = 1;
};

} // namespace rivision::core
