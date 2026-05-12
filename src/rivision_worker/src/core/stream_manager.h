#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <memory>
#include <unordered_map>
#include <shared_mutex>
#include <functional>

namespace rivision::storage {
class StreamStore;
}

namespace rivision::pipeline {
class PipelineManager;
class StreamContext;
}

namespace rivision::core {

// =============================================================================
// StreamManager - Manages multiple video streams
// =============================================================================

class StreamManager {
public:
    explicit StreamManager(pipeline::PipelineManager* pipeline_mgr);
    ~StreamManager();
    
    // Add a new stream
    Result<StreamId> addStream(const StreamConfig& config);
    
    // Remove a stream
    bool removeStream(const StreamId& id);
    
    // Get stream status
    StreamStatus getStatus(const StreamId& id) const;
    
    // Get all streams
    std::vector<StreamConfig> listStreams() const;
    
    // Get stream config
    std::optional<StreamConfig> getConfig(const StreamId& id) const;
    
    // Get stream info with status
    struct StreamInfo {
        StreamConfig config;
        StreamStatus status;
    };
    std::optional<StreamInfo> getInfo(const StreamId& id) const;
    
    // Pause/resume stream
    bool pauseStream(const StreamId& id);
    bool resumeStream(const StreamId& id);
    
    // Stop all streams
    void stopAll();
    
    // Get current frame (for snapshot)
    bool getFrame(const StreamId& id, Frame& frame);
    
    // Get active stream count
    int activeCount() const;
    
    // Check if stream exists
    bool exists(const StreamId& id) const;
    
    // Stream persistence
    void setStore(storage::StreamStore* store);
    
    // Restore streams from persistent store (call after setStore)
    int restoreStreams();
    
private:
    struct StreamEntry {
        StreamConfig config;
        std::shared_ptr<pipeline::StreamContext> context;
        StreamState state = StreamState::CREATED;
        std::string error_msg;
    };
    
    pipeline::PipelineManager* pipeline_mgr_;
    storage::StreamStore* store_ = nullptr;  // Optional persistence
    
    std::unordered_map<StreamId, StreamEntry> streams_;
    mutable std::shared_mutex mutex_;
    
    // Generate unique stream ID if not provided
    std::string generateId() const;
    
    // Internal add without saving to store (for restore)
    Result<StreamId> addStreamInternal(const StreamConfig& config, bool persist);
};

} // namespace rivision::core
