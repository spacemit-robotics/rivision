#include "ws_hub_client.h"
#include "utils/logger.h"
#include <nlohmann/json.hpp>
#include <chrono>

namespace rivision::api {

using json = nlohmann::json;

class WsHubClient::Impl {
public:
    // Placeholder for actual WebSocket client implementation
};

WsHubClient::WsHubClient() : impl_(std::make_unique<Impl>()) {}

WsHubClient::~WsHubClient() {
    disconnect();
}

bool WsHubClient::connect() {
    if (url_.empty()) {
        LOG_ERROR("Hub URL not set");
        return false;
    }
    
    should_reconnect_ = true;
    
    if (doConnect()) {
        return true;
    }
    
    reconnect_thread_ = std::thread(&WsHubClient::reconnectLoop, this);
    return false;
}

void WsHubClient::disconnect() {
    should_reconnect_ = false;
    connected_ = false;
    
    if (reconnect_thread_.joinable()) {
        reconnect_thread_.join();
    }
    
    if (disconnect_handler_) {
        disconnect_handler_("manual disconnect");
    }
    
    LOG_INFO("Disconnected from Hub");
}

bool WsHubClient::doConnect() {
    LOG_INFO("Connecting to Hub: {}", url_);
    
    // Placeholder - actual WebSocket connection
    connected_ = true;
    
    if (connect_handler_) {
        connect_handler_();
    }
    
    return connected_;
}

void WsHubClient::reconnectLoop() {
    while (should_reconnect_ && !connected_) {
        std::this_thread::sleep_for(std::chrono::milliseconds(reconnect_ms_));
        
        if (!should_reconnect_) break;
        
        LOG_INFO("Attempting to reconnect to Hub...");
        if (doConnect()) {
            break;
        }
    }
}

bool WsHubClient::send(const std::string& message) {
    if (!connected_) {
        return false;
    }
    
    LOG_DEBUG("Sending to Hub: {} bytes", message.size());
    return true;
}

bool WsHubClient::sendDetection(
    const StreamId& stream_id,
    const std::vector<Detection>& detections,
    const std::vector<Track>& tracks
) {
    json msg;
    msg["type"] = "detection";
    msg["node_id"] = node_id_;
    msg["stream_id"] = stream_id;
    msg["timestamp"] = nowMs();
    
    json dets = json::array();
    for (const auto& d : detections) {
        dets.push_back({
            {"bbox", {d.bbox.x1, d.bbox.y1, d.bbox.x2, d.bbox.y2}},
            {"class_id", d.class_id},
            {"class_name", d.class_name},
            {"confidence", d.confidence}
        });
    }
    msg["detections"] = dets;
    
    json trks = json::array();
    for (const auto& t : tracks) {
        trks.push_back({
            {"track_id", t.id},
            {"bbox", {t.detection.bbox.x1, t.detection.bbox.y1,
                      t.detection.bbox.x2, t.detection.bbox.y2}},
            {"class_id", t.detection.class_id}
        });
    }
    msg["tracks"] = trks;
    
    return send(msg.dump());
}

bool WsHubClient::sendAlert(const AlertEvent& alert) {
    json msg;
    msg["type"] = "alert";
    msg["node_id"] = node_id_;
    msg["id"] = alert.id;
    msg["rule_id"] = alert.rule_id;
    msg["rule_name"] = alert.rule_name;
    msg["stream_id"] = alert.stream_id;
    msg["camera_id"] = alert.camera_id;
    msg["level"] = alertLevelToString(alert.level);
    msg["trigger_type"] = alert.trigger_type;
    msg["zone_name"] = alert.zone_name;
    msg["timestamp"] = alert.timestamp;
    
    if (alert.vlm_result) {
        msg["vlm_result"] = {
            {"verified", alert.vlm_result->verified},
            {"description", alert.vlm_result->description}
        };
    }
    
    return send(msg.dump());
}

bool WsHubClient::sendHeartbeat(const std::string& heartbeat_json) {
    json msg;
    msg["type"] = "heartbeat";
    msg["node_id"] = node_id_;
    msg["data"] = json::parse(heartbeat_json);
    
    return send(msg.dump());
}

} // namespace rivision::api
