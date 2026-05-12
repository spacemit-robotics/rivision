#include "router.h"
#include "core/worker.h"
#include "core/stream_manager.h"
#include "core/rules/engine.h"
#include "core/analytics/collector.h"
#include "core/search/engine.h"
#include "storage/meta_db.h"
#include "storage/vector_store.h"
#include "utils/logger.h"
#include "utils/system_monitor.h"

#include <oatpp/parser/json/mapping/ObjectMapper.hpp>
#include <nlohmann/json.hpp>
#include <opencv2/opencv.hpp>
#include <sstream>

using json = nlohmann::json;

namespace rivision::api {

std::shared_ptr<ApiRouter> ApiRouter::createShared(core::Worker* worker) {
    auto objectMapper = oatpp::parser::json::mapping::ObjectMapper::createShared();
    auto router = std::make_shared<ApiRouter>(objectMapper);
    router->worker_ = worker;
    return router;
}

// =============================================================================
// Health
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleHealth() {
    auto health = worker_->getHealth();
    
    json j;
    j["status"] = healthLevelToString(health.status);
    j["uptime_s"] = health.uptime_s;
    
    j["streams"]["active"] = health.streams.active;
    j["streams"]["max"] = health.streams.max;
    
    j["inference"]["connected"] = health.inference.connected;
    j["inference"]["latency_ms"] = health.inference.latency_ms;
    
    j["embed"]["connected"] = health.embed.connected;
    j["embed"]["latency_ms"] = health.embed.latency_ms;
    
    j["milvus"]["vectors"] = health.vector_store.vectors;
    j["milvus"]["max"] = health.vector_store.max;
    
    j["hub"]["connected"] = health.hub.connected;
    j["hub"]["last_heartbeat"] = health.hub.last_heartbeat;
    
    j["system"]["cpu_pct"] = health.system.cpu_pct;
    j["system"]["mem_pct"] = health.system.mem_pct;
    j["system"]["disk_pct"] = health.system.disk_pct;
    
    if (health.degradation_level > 0) {
        j["degradation"]["level"] = health.degradation_level;
        j["degradation"]["reason"] = health.degradation_reason;
    }
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

// =============================================================================
// Streams
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleListStreams() {
    auto streams = worker_->streamManager()->listStreams();
    
    json j = json::array();
    for (const auto& s : streams) {
        auto status = worker_->streamManager()->getStatus(s.id);
        
        json stream;
        stream["id"] = s.id;
        stream["name"] = s.name;
        stream["rtsp_url"] = s.rtsp_url;
        stream["camera_id"] = s.camera_id;
        stream["state"] = streamStateToString(status.state);
        stream["fps"] = status.fps;
        stream["frames_processed"] = status.frames_processed;
        stream["detections_count"] = status.detections_count;
        
        j.push_back(stream);
    }
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleAddStream(const oatpp::String& body) {
    try {
        json req = json::parse(body->c_str());
        
        StreamConfig config;
        config.id = req.value("id", "");
        config.name = req.value("name", "");
        config.rtsp_url = req.value("rtsp_url", "");
        config.camera_id = req.value("camera_id", "");
        
        if (req.contains("detection")) {
            config.detection.enabled = req["detection"].value("enabled", true);
            config.detection.confidence = req["detection"].value("confidence", 0.5f);
            config.detection.inference_fps = req["detection"].value("inference_fps", 5);
        }
        
        if (req.contains("tracking")) {
            config.tracking.enabled = req["tracking"].value("enabled", true);
        }
        
        // RTSP output for annotated stream
        config.rtsp_output_enabled = req.value("rtsp_output", false);
        
        // RTSP output settings (fps, bitrate)
        if (req.contains("rtsp_output_settings")) {
            config.rtsp_output.fps = req["rtsp_output_settings"].value("fps", 15);
            config.rtsp_output.bitrate = req["rtsp_output_settings"].value("bitrate", 4000000);
        }
        
        if (config.rtsp_url.empty()) {
            json err;
            err["error"] = "rtsp_url is required";
            auto response = createResponse(Status::CODE_400, err.dump().c_str());
            response->putHeader("Content-Type", "application/json");
            return response;
        }
        
        auto result = worker_->streamManager()->addStream(config);
        
        if (result.ok()) {
            json j;
            j["id"] = result.value();
            j["message"] = "Stream added";
            
            auto response = createResponse(Status::CODE_201, j.dump().c_str());
            response->putHeader("Content-Type", "application/json");
            return response;
        } else {
            json err;
            err["error"] = result.error();
            auto response = createResponse(Status::CODE_400, err.dump().c_str());
            response->putHeader("Content-Type", "application/json");
            return response;
        }
        
    } catch (const std::exception& e) {
        json err;
        err["error"] = e.what();
        auto response = createResponse(Status::CODE_400, err.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    }
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleGetStream(const oatpp::String& id) {
    auto info = worker_->streamManager()->getInfo(id->c_str());
    
    if (!info) {
        json err;
        err["error"] = "Stream not found";
        auto response = createResponse(Status::CODE_404, err.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    }
    
    json j;
    j["id"] = info->config.id;
    j["name"] = info->config.name;
    j["rtsp_url"] = info->config.rtsp_url;
    j["camera_id"] = info->config.camera_id;
    j["state"] = streamStateToString(info->status.state);
    j["fps"] = info->status.fps;
    j["frames_processed"] = info->status.frames_processed;
    j["detections_count"] = info->status.detections_count;
    j["reconnect_count"] = info->status.reconnect_count;
    
    if (!info->status.error_msg.empty()) {
        j["error"] = info->status.error_msg;
    }
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleRemoveStream(const oatpp::String& id) {
    bool removed = worker_->streamManager()->removeStream(id->c_str());
    
    if (removed) {
        json j;
        j["message"] = "Stream removed";
        auto response = createResponse(Status::CODE_200, j.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    } else {
        json err;
        err["error"] = "Stream not found";
        auto response = createResponse(Status::CODE_404, err.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    }
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleSnapshot(const oatpp::String& id) {
    Frame frame;
    if (!worker_->streamManager()->getFrame(id->c_str(), frame)) {
        json err;
        err["error"] = "Stream not found or no frame available";
        auto response = createResponse(Status::CODE_404, err.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    }
    
    // Encode frame to JPEG using OpenCV
    try {
        uint8_t* data = frame.cpuData();
        if (!data || frame.width == 0 || frame.height == 0) {
            json err;
            err["error"] = "Invalid frame data";
            auto response = createResponse(Status::CODE_500, err.dump().c_str());
            response->putHeader("Content-Type", "application/json");
            return response;
        }
        
        cv::Mat img(frame.height, frame.width, CV_8UC3, data);
        
        // Convert RGB to BGR for OpenCV
        cv::Mat bgr;
        cv::cvtColor(img, bgr, cv::COLOR_RGB2BGR);
        
        // Encode to JPEG
        std::vector<uchar> jpeg_buffer;
        std::vector<int> params = {cv::IMWRITE_JPEG_QUALITY, 85};
        cv::imencode(".jpg", bgr, jpeg_buffer, params);
        
        // Create response with JPEG data
        auto body = oatpp::String(reinterpret_cast<const char*>(jpeg_buffer.data()), jpeg_buffer.size());
        auto response = createResponse(Status::CODE_200, body);
        response->putHeader("Content-Type", "image/jpeg");
        return response;
        
    } catch (const cv::Exception& e) {
        json err;
        err["error"] = std::string("Image encoding failed: ") + e.what();
        auto response = createResponse(Status::CODE_500, err.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    }
}

// =============================================================================
// Search
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleSearch(const oatpp::String& body) {
    return handleSearchText(body);  // Default to text search
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleSearchText(const oatpp::String& body) {
    try {
        json req = json::parse(body->c_str());
        
        std::string query = req.value("query", "");
        int top_k = req.value("top_k", 10);
        
        // TODO: Implement actual search
        json j;
        j["results"] = json::array();
        j["total"] = 0;
        j["query"] = query;
        
        auto response = createResponse(Status::CODE_200, j.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
        
    } catch (const std::exception& e) {
        json err;
        err["error"] = e.what();
        auto response = createResponse(Status::CODE_400, err.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    }
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleSearchImage(const oatpp::String& body) {
    // Similar to text search
    json j;
    j["results"] = json::array();
    j["total"] = 0;
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleSearchHybrid(const oatpp::String& body) {
    json j;
    j["results"] = json::array();
    j["total"] = 0;
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleSearchFaces(const oatpp::String& body) {
    json j;
    j["results"] = json::array();
    j["total"] = 0;
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

// =============================================================================
// Analytics
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleStats() {
    json j;
    j["streams"] = worker_->streamManager()->activeCount();
    j["detection_count"] = worker_->metaDB() ? worker_->metaDB()->getDetectionCount() : 0;
    j["alert_count"] = worker_->metaDB() ? worker_->metaDB()->getAlertCount() : 0;
    j["vector_count"] = worker_->vectorStore() ? worker_->vectorStore()->size() : 0;
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleHeatmap() {
    json j;
    j["grid"] = json::array();
    j["rows"] = Heatmap::ROWS;
    j["cols"] = Heatmap::COLS;
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

// =============================================================================
// Alerts
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleListAlerts() {
    json j = json::array();
    // TODO: Query alerts from MetaDB
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleAlertStats() {
    if (!worker_->metaDB()) {
        json err;
        err["error"] = "Database not available";
        auto response = createResponse(Status::CODE_500, err.dump().c_str());
        response->putHeader("Content-Type", "application/json");
        return response;
    }
    
    auto stats = worker_->metaDB()->getAlertStats();
    
    json j;
    j["total"] = stats.total;
    j["pending"] = stats.pending;
    j["acknowledged"] = stats.acknowledged;
    j["last_24h"] = stats.last_24h;
    j["last_hour"] = stats.last_hour;
    j["by_level"] = stats.by_level;
    j["by_type"] = stats.by_type;
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleAckAlert(const oatpp::String& id) {
    if (worker_->metaDB()) {
        worker_->metaDB()->acknowledgeAlert(id->c_str(), "api");
    }
    
    json j;
    j["message"] = "Alert acknowledged";
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

// =============================================================================
// Rules
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleListRules() {
    auto rules = worker_->rulesEngine()->getRules();
    
    json j = json::array();
    for (const auto& r : rules) {
        json rule;
        rule["id"] = r.id;
        rule["name"] = r.name;
        rule["enabled"] = r.enabled;
        rule["trigger_type"] = r.trigger.type;
        j.push_back(rule);
    }
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleAddRule(const oatpp::String& body) {
    // TODO: Parse and add rule
    json j;
    j["message"] = "Rule add not implemented";
    
    auto response = createResponse(Status::CODE_501, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleUpdateRule(
    const oatpp::String& id, const oatpp::String& body) {
    json j;
    j["message"] = "Rule update not implemented";
    
    auto response = createResponse(Status::CODE_501, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleDeleteRule(const oatpp::String& id) {
    worker_->rulesEngine()->removeRule(id->c_str());
    
    json j;
    j["message"] = "Rule deleted";
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

// =============================================================================
// Config
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleGetConfig() {
    const auto& cfg = worker_->config();
    
    json j;
    j["node_id"] = cfg.node_id;
    j["server"]["host"] = cfg.server.host;
    j["server"]["http_port"] = cfg.server.http_port;
    j["hub"]["url"] = cfg.hub.url;
    j["inference"]["device"] = cfg.inference.device;
    j["vlm"]["enabled"] = cfg.vlm.enabled;
    j["vlm"]["endpoint"] = cfg.vlm.endpoint;
    
    auto response = createResponse(Status::CODE_200, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleUpdateConfig(const oatpp::String& body) {
    json j;
    j["message"] = "Config update not implemented";
    
    auto response = createResponse(Status::CODE_501, j.dump().c_str());
    response->putHeader("Content-Type", "application/json");
    return response;
}

// =============================================================================
// Metrics
// =============================================================================

std::shared_ptr<ApiRouter::OutgoingResponse> ApiRouter::handleMetrics() {
    auto health = worker_->getHealth();
    auto sys = utils::SystemMonitor::getMetrics();
    
    std::ostringstream oss;
    
    // Prometheus format
    oss << "# HELP rivision_up Worker is up\n";
    oss << "# TYPE rivision_up gauge\n";
    oss << "rivision_up 1\n";
    
    oss << "# HELP rivision_uptime_seconds Worker uptime\n";
    oss << "# TYPE rivision_uptime_seconds counter\n";
    oss << "rivision_uptime_seconds " << health.uptime_s << "\n";
    
    oss << "# HELP rivision_streams_active Active stream count\n";
    oss << "# TYPE rivision_streams_active gauge\n";
    oss << "rivision_streams_active " << health.streams.active << "\n";
    
    oss << "# HELP rivision_inference_latency_ms YOLO inference latency\n";
    oss << "# TYPE rivision_inference_latency_ms gauge\n";
    oss << "rivision_inference_latency_ms " << health.inference.latency_ms << "\n";
    
    oss << "# HELP rivision_vectors_total Total vectors in store\n";
    oss << "# TYPE rivision_vectors_total gauge\n";
    oss << "rivision_vectors_total " << health.vector_store.vectors << "\n";
    
    oss << "# HELP node_cpu_percent CPU usage percent\n";
    oss << "# TYPE node_cpu_percent gauge\n";
    oss << "node_cpu_percent " << sys.cpu_percent << "\n";
    
    oss << "# HELP node_memory_percent Memory usage percent\n";
    oss << "# TYPE node_memory_percent gauge\n";
    oss << "node_memory_percent " << sys.mem_percent << "\n";
    
    oss << "# HELP node_disk_percent Disk usage percent\n";
    oss << "# TYPE node_disk_percent gauge\n";
    oss << "node_disk_percent " << sys.disk_percent << "\n";
    
    oss << "# HELP node_load1 System load average (1m)\n";
    oss << "# TYPE node_load1 gauge\n";
    oss << "node_load1 " << sys.load_1 << "\n";
    
    auto response = createResponse(Status::CODE_200, oss.str().c_str());
    response->putHeader("Content-Type", "text/plain; version=0.0.4");
    return response;
}

} // namespace rivision::api
