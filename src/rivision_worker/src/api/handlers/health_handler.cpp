#include "health_handler.h"
#include <sstream>

namespace rivision::api {

HealthHandler::HealthHandler() = default;
HealthHandler::~HealthHandler() = default;

std::string HealthHandler::handleHealth() {
    if (!health_provider_) {
        return R"({"status":"unknown","error":"health provider not set"})";
    }
    
    auto health = health_provider_();
    
    std::ostringstream oss;
    oss << "{";
    oss << "\"status\":\"" << healthLevelToString(health.status) << "\",";
    oss << "\"uptime_s\":" << health.uptime_s << ",";
    oss << "\"streams\":{";
    oss << "\"active\":" << health.streams.active << ",";
    oss << "\"max\":" << health.streams.max << "},";
    oss << "\"inference\":{";
    oss << "\"connected\":" << (health.inference.connected ? "true" : "false") << ",";
    oss << "\"latency_ms\":" << health.inference.latency_ms << "},";
    oss << "\"embed\":{";
    oss << "\"connected\":" << (health.embed.connected ? "true" : "false") << ",";
    oss << "\"latency_ms\":" << health.embed.latency_ms << "},";
    oss << "\"vector_store\":{";
    oss << "\"vectors\":" << health.vector_store.vectors << ",";
    oss << "\"max\":" << health.vector_store.max << "},";
    oss << "\"hub\":{";
    oss << "\"connected\":" << (health.hub.connected ? "true" : "false") << ",";
    oss << "\"last_heartbeat\":\"" << health.hub.last_heartbeat << "\"},";
    oss << "\"system\":{";
    oss << "\"cpu_pct\":" << health.system.cpu_pct << ",";
    oss << "\"mem_pct\":" << health.system.mem_pct << ",";
    oss << "\"disk_pct\":" << health.system.disk_pct << ",";
    oss << "\"load_1\":" << health.system.load_1 << "},";
    oss << "\"degradation_level\":" << health.degradation_level;
    if (!health.degradation_reason.empty()) {
        oss << ",\"degradation_reason\":\"" << health.degradation_reason << "\"";
    }
    oss << "}";
    
    return oss.str();
}

std::string HealthHandler::handleReady() {
    if (!health_provider_) {
        return R"({"ready":false,"reason":"health provider not set"})";
    }
    
    auto health = health_provider_();
    bool ready = (health.status != HealthLevel::UNHEALTHY);
    
    std::ostringstream oss;
    oss << "{\"ready\":" << (ready ? "true" : "false");
    if (!ready) {
        oss << ",\"reason\":\"" << health.degradation_reason << "\"";
    }
    oss << "}";
    
    return oss.str();
}

std::string HealthHandler::handleLive() {
    return R"({"live":true})";
}

} // namespace rivision::api
