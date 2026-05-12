#include "ws_server.h"
#include "utils/logger.h"
#include <nlohmann/json.hpp>
#include <sstream>

namespace rivision::api {

using json = nlohmann::json;

class WebSocketServer::Impl {
public:
    // Placeholder for actual WebSocket implementation (oatpp-websocket, etc.)
};

WebSocketServer::WebSocketServer() : impl_(std::make_unique<Impl>()) {}
WebSocketServer::~WebSocketServer() { stop(); }

bool WebSocketServer::start(int port) {
    LOG_INFO("WebSocket server starting on port {}", port);
    running_ = true;
    return true;
}

void WebSocketServer::stop() {
    if (!running_) return;
    running_ = false;
    LOG_INFO("WebSocket server stopped");
}

void WebSocketServer::broadcast(const std::string& message) {
    std::lock_guard lock(clients_mutex_);
    for (const auto& client : clients_) {
        sendTo(client, message);
    }
}

void WebSocketServer::sendTo(const std::string& client_id, const std::string& message) {
    // Placeholder - actual implementation depends on WebSocket library
    LOG_DEBUG("WS send to {}: {} bytes", client_id, message.size());
}

void WebSocketServer::broadcastDetection(
    const StreamId& stream_id,
    const std::vector<Detection>& detections,
    const std::vector<Track>& tracks
) {
    json msg;
    msg["type"] = "detection";
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
            {"class_id", t.detection.class_id},
            {"class_name", t.detection.class_name},
            {"state", t.state == Track::State::TRACKED ? "tracked" : 
                      t.state == Track::State::NEW ? "new" : "lost"}
        });
    }
    msg["tracks"] = trks;
    
    broadcast(msg.dump());
}

void WebSocketServer::broadcastAlert(const AlertEvent& alert) {
    json msg;
    msg["type"] = "alert";
    msg["id"] = alert.id;
    msg["rule_id"] = alert.rule_id;
    msg["rule_name"] = alert.rule_name;
    msg["stream_id"] = alert.stream_id;
    msg["camera_id"] = alert.camera_id;
    msg["level"] = alertLevelToString(alert.level);
    msg["trigger_type"] = alert.trigger_type;
    msg["zone_name"] = alert.zone_name;
    msg["direction"] = alert.direction;
    msg["timestamp"] = alert.timestamp;
    
    if (!alert.thumbnail_base64.empty()) {
        msg["thumbnail"] = alert.thumbnail_base64;
    }
    
    if (alert.vlm_result) {
        msg["vlm_result"] = {
            {"verified", alert.vlm_result->verified},
            {"confidence", alert.vlm_result->confidence},
            {"description", alert.vlm_result->description},
            {"timeout", alert.vlm_result->timeout}
        };
    }
    
    broadcast(msg.dump());
}

int WebSocketServer::getClientCount() const {
    std::lock_guard lock(clients_mutex_);
    return static_cast<int>(clients_.size());
}

} // namespace rivision::api
