#include "event_publisher.h"
#include "hub_client.h"
#include "storage/event_cache.h"
#include "utils/logger.h"

#include <nlohmann/json.hpp>

using json = nlohmann::json;

namespace rivision::hub {

EventPublisher::EventPublisher(HubClient* hub_client, storage::EventCache* event_cache)
    : hub_client_(hub_client)
    , event_cache_(event_cache) {
}

EventPublisher::~EventPublisher() = default;

void EventPublisher::publishDetection(const StreamId& stream_id,
                                     const std::vector<Detection>& detections,
                                     const std::vector<Track>& tracks) {
    std::string message = serializeDetection(stream_id, detections, tracks);
    trySend(message, "detection");
}

void EventPublisher::publishAlert(const AlertEvent& alert) {
    std::string message = serializeAlert(alert);
    trySend(message, "alert");
}

void EventPublisher::publishCameraStatus(const CameraId& camera_id,
                                        const std::string& status) {
    json j;
    j["type"] = "camera_status";
    j["camera_id"] = camera_id;
    j["status"] = status;
    j["timestamp"] = nowMs();
    
    trySend(j.dump(), "camera_status");
}

void EventPublisher::flushCachedEvents() {
    if (!event_cache_ || !hub_client_ || !hub_client_->isConnected()) {
        return;
    }
    
    LOG_INFO("Flushing cached events...");
    
    auto events = event_cache_->getCachedEvents(100);
    std::vector<int64_t> sent_ids;
    
    for (const auto& event : events) {
        if (hub_client_->send(event.payload)) {
            sent_ids.push_back(event.id);
        } else {
            break;  // Stop on first failure
        }
    }
    
    if (!sent_ids.empty()) {
        event_cache_->removeCachedEvents(sent_ids);
        LOG_INFO("Flushed {} cached events", sent_ids.size());
    }
}

std::string EventPublisher::serializeDetection(const StreamId& stream_id,
                                               const std::vector<Detection>& detections,
                                               const std::vector<Track>& tracks) {
    json j;
    j["type"] = "detection";
    j["stream_id"] = stream_id;
    j["timestamp"] = nowMs();
    
    // Detections
    json dets = json::array();
    for (const auto& det : detections) {
        json d;
        d["class_id"] = det.class_id;
        d["class_name"] = det.class_name;
        d["confidence"] = det.confidence;
        d["bbox"]["x1"] = det.bbox.x1;
        d["bbox"]["y1"] = det.bbox.y1;
        d["bbox"]["x2"] = det.bbox.x2;
        d["bbox"]["y2"] = det.bbox.y2;
        dets.push_back(d);
    }
    j["detections"] = dets;
    
    // Tracks
    json trks = json::array();
    for (const auto& track : tracks) {
        json t;
        t["track_id"] = track.id;
        t["class_id"] = track.detection.class_id;
        t["class_name"] = track.detection.class_name;
        t["bbox"]["x1"] = track.detection.bbox.x1;
        t["bbox"]["y1"] = track.detection.bbox.y1;
        t["bbox"]["x2"] = track.detection.bbox.x2;
        t["bbox"]["y2"] = track.detection.bbox.y2;
        t["age"] = track.age;
        trks.push_back(t);
    }
    j["tracks"] = trks;
    
    return j.dump();
}

std::string EventPublisher::serializeAlert(const AlertEvent& alert) {
    json j;
    j["type"] = "alert";
    j["alert_id"] = alert.id;
    j["rule_id"] = alert.rule_id;
    j["rule_name"] = alert.rule_name;
    j["stream_id"] = alert.stream_id;
    j["camera_id"] = alert.camera_id;
    j["level"] = alertLevelToString(alert.level);
    j["trigger_type"] = alert.trigger_type;
    j["zone_name"] = alert.zone_name;
    j["direction"] = alert.direction;
    j["timestamp"] = alert.timestamp;
    
    if (!alert.thumbnail_base64.empty()) {
        j["thumbnail_base64"] = alert.thumbnail_base64;
    }
    
    if (alert.record.enabled) {
        j["record"]["enabled"] = true;
        j["record"]["path"] = alert.record.path;
        j["record"]["pre_sec"] = alert.record.pre_sec;
        j["record"]["post_sec"] = alert.record.post_sec;
    }
    
    if (alert.vlm_result) {
        j["vlm_result"]["verified"] = alert.vlm_result->verified;
        j["vlm_result"]["confidence"] = alert.vlm_result->confidence;
        j["vlm_result"]["description"] = alert.vlm_result->description;
        if (alert.vlm_result->timeout) {
            j["vlm_timeout"] = true;
        }
    }
    
    return j.dump();
}

bool EventPublisher::trySend(const std::string& message, const std::string& type) {
    if (hub_client_ && hub_client_->isConnected()) {
        if (hub_client_->send(message)) {
            return true;
        }
    }
    
    // Cache for later
    if (event_cache_) {
        event_cache_->cacheEvent(type, message);
        LOG_DEBUG("Cached {} event for later delivery", type);
    }
    
    return false;
}

} // namespace rivision::hub
