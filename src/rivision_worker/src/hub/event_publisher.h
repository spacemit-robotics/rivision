#pragma once

#include "rivision/types.h"
#include <memory>
#include <queue>
#include <mutex>

namespace rivision::storage {
class EventCache;
}

namespace rivision::hub {

class HubClient;

// =============================================================================
// EventPublisher - Publishes events to Hub
// =============================================================================

class EventPublisher {
public:
    EventPublisher(HubClient* hub_client, storage::EventCache* event_cache);
    ~EventPublisher();
    
    // Publish detection event
    void publishDetection(const StreamId& stream_id,
                         const std::vector<Detection>& detections,
                         const std::vector<Track>& tracks);
    
    // Publish alert event
    void publishAlert(const AlertEvent& alert);
    
    // Publish camera status
    void publishCameraStatus(const CameraId& camera_id, 
                            const std::string& status);
    
    // Flush cached events (after reconnect)
    void flushCachedEvents();
    
private:
    // Serialize to JSON
    std::string serializeDetection(const StreamId& stream_id,
                                   const std::vector<Detection>& detections,
                                   const std::vector<Track>& tracks);
    std::string serializeAlert(const AlertEvent& alert);
    
    // Try to send, cache on failure
    bool trySend(const std::string& message, const std::string& type);
    
    HubClient* hub_client_;
    storage::EventCache* event_cache_;
};

} // namespace rivision::hub
