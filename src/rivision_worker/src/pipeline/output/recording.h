#pragma once

#include "rivision/types.h"
#include "ring_buffer.h"
#include <memory>
#include <string>
#include <thread>
#include <atomic>
#include <queue>
#include <mutex>
#include <condition_variable>

// Include the actual SnapshotManager from snapshot.h
#include "snapshot.h"

namespace rivision::hal {
class IMux;
class IEncoder;
}

namespace rivision::pipeline {

// =============================================================================
// RecordingManager - Manages video recordings
// =============================================================================

class RecordingManager {
public:
    struct Config {
        std::string output_dir = "/opt/rivision/rivision_worker/data/recordings";
        int pre_record_sec = 5;     // Pre-event buffer
        int post_record_sec = 10;   // Post-event recording
        int max_file_size_mb = 100;
        int max_duration_sec = 300; // 5 minutes max
        int fps = 30;
    };
    
    explicit RecordingManager(const Config& config);
    ~RecordingManager();
    
    // Start recording manager
    bool start();
    
    // Stop recording manager
    void stop();
    
    // Push frame to ring buffer
    void pushFrame(const StreamId& stream_id, const Frame& frame);
    
    // Trigger recording (e.g., on alert)
    struct RecordingRequest {
        StreamId stream_id;
        AlertId alert_id;
        int pre_sec;
        int post_sec;
    };
    std::string triggerRecording(const RecordingRequest& req);
    
    // Get recording path by ID
    std::string getRecordingPath(const std::string& recording_id) const;
    
    // Get active recording count
    int activeCount() const;
    
private:
    void recordLoop();
    
    // Generate recording file path
    std::string generatePath(const StreamId& stream_id, const AlertId& alert_id);
    
    Config config_;
    
    // Per-stream ring buffers
    std::unordered_map<StreamId, std::unique_ptr<FrameRingBuffer>> buffers_;
    mutable std::mutex buffers_mutex_;
    
    // Recording queue
    struct RecordingJob {
        std::string id;
        std::string path;
        StreamId stream_id;
        AlertId alert_id;
        std::vector<Frame> pre_frames;
        int post_sec;
        Timestamp start_ts;
    };
    std::queue<RecordingJob> jobs_;
    std::mutex jobs_mutex_;
    std::condition_variable jobs_cv_;
    
    // Active recordings
    std::unordered_map<std::string, std::string> active_recordings_;  // id -> path
    mutable std::mutex active_mutex_;
    
    std::thread worker_thread_;
    std::atomic<bool> running_{false};
};

} // namespace rivision::pipeline


