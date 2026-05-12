#include "stream_handler.h"
#include <nlohmann/json.hpp>
#include <sstream>

namespace rivision::api {

using json = nlohmann::json;

StreamHandler::StreamHandler() = default;
StreamHandler::~StreamHandler() = default;

std::string StreamHandler::handleListStreams() {
    if (!get_all_callback_) {
        return R"({"error":"handler not configured"})";
    }
    
    auto streams = get_all_callback_();
    
    json result = json::array();
    for (const auto& s : streams) {
        result.push_back(json::parse(streamStatusToJson(s)));
    }
    
    return result.dump();
}

std::string StreamHandler::handleGetStream(const std::string& id) {
    if (!get_callback_) {
        return R"({"error":"handler not configured"})";
    }
    
    auto status = get_callback_(id);
    return streamStatusToJson(status);
}

std::string StreamHandler::handleAddStream(const std::string& body) {
    if (!add_callback_) {
        return R"({"error":"handler not configured"})";
    }
    
    try {
        auto config = parseStreamConfig(body);
        
        if (add_callback_(config)) {
            return R"({"success":true,"stream_id":")" + config.id + "\"}";
        } else {
            return R"({"error":"failed to add stream"})";
        }
        
    } catch (const std::exception& e) {
        return R"({"error":")" + std::string(e.what()) + "\"}";
    }
}

std::string StreamHandler::handleRemoveStream(const std::string& id) {
    if (!remove_callback_) {
        return R"({"error":"handler not configured"})";
    }
    
    if (remove_callback_(id)) {
        return R"({"success":true})";
    } else {
        return R"({"error":"failed to remove stream"})";
    }
}

std::string StreamHandler::handleUpdateStream(const std::string& id, const std::string& body) {
    return R"({"error":"not implemented"})";
}

StreamConfig StreamHandler::parseStreamConfig(const std::string& jsonStr) {
    json j = json::parse(jsonStr);
    StreamConfig config;
    
    if (j.contains("id")) config.id = j["id"].get<std::string>();
    if (j.contains("name")) config.name = j["name"].get<std::string>();
    if (j.contains("rtsp_url")) config.rtsp_url = j["rtsp_url"].get<std::string>();
    if (j.contains("camera_id")) config.camera_id = j["camera_id"].get<std::string>();
    
    if (j.contains("stream_mode")) {
        config.stream_mode = streamModeFromString(j["stream_mode"].get<std::string>());
    }
    
    if (j.contains("detection")) {
        auto& det = j["detection"];
        if (det.contains("enabled")) config.detection.enabled = det["enabled"].get<bool>();
        if (det.contains("confidence")) config.detection.confidence = det["confidence"].get<float>();
        if (det.contains("inference_fps")) config.detection.inference_fps = det["inference_fps"].get<int>();
    }
    
    return config;
}

std::string StreamHandler::streamStatusToJson(const StreamStatus& status) {
    std::ostringstream oss;
    oss << "{";
    oss << "\"state\":\"" << streamStateToString(status.state) << "\",";
    if (!status.error_msg.empty()) {
        oss << "\"error_msg\":\"" << status.error_msg << "\",";
    }
    oss << "\"started_at\":" << status.started_at << ",";
    oss << "\"frames_processed\":" << status.frames_processed << ",";
    oss << "\"detections_count\":" << status.detections_count << ",";
    oss << "\"fps\":" << status.fps << ",";
    oss << "\"reconnect_count\":" << status.reconnect_count;
    oss << "}";
    return oss.str();
}

} // namespace rivision::api
