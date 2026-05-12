#include "hub_client.h"
#include "utils/logger.h"
#include "utils/system_monitor.h"

#include <nlohmann/json.hpp>
#include <curl/curl.h>
#include <regex>
#include <chrono>

using json = nlohmann::json;

namespace rivision::hub {

// CURL write callback
static size_t writeCallback(void* contents, size_t size, size_t nmemb, std::string* s) {
    size_t new_len = size * nmemb;
    s->append(static_cast<char*>(contents), new_len);
    return new_len;
}

// Helper to extract host:port from URL
static std::pair<std::string, int> parseUrl(const std::string& url) {
    std::regex re(R"(https?://([^:/]+)(?::(\d+))?)");
    std::smatch match;
    if (std::regex_search(url, match, re)) {
        std::string host = match[1].str();
        int port = match[2].matched ? std::stoi(match[2].str()) : 80;
        return {host, port};
    }
    return {"", 0};
}

HubClient::HubClient(const std::string& url,
                    const std::string& node_id,
                    const std::string& token)
    : url_(url)
    , node_id_(node_id)
    , token_(token)
    , start_time_(std::chrono::duration_cast<std::chrono::seconds>(
          std::chrono::system_clock::now().time_since_epoch()).count()) {
    curl_global_init(CURL_GLOBAL_DEFAULT);
}

HubClient::~HubClient() {
    disconnect();
    curl_global_cleanup();
}

bool HubClient::registerWithHub() {
    // Build registration URL
    std::string reg_url = url_ + "/api/v1/nodes/register";
    
    // Build registration request matching Hub's expected format
    json req;
    req["node_id"] = node_id_;           // Hub expects node_id, not id
    req["name"] = node_id_;              // Node name
    req["type"] = "worker";              // Node type
    req["ip"] = host_;                   // Our IP
    req["host"] = host_;                 // Also set host
    req["port"] = http_port_;            // Agent port for API
    req["agent_port"] = http_port_;      // Worker HTTP port (same as port)
    req["yolo_enabled"] = yolo_enabled_; // YOLO status
    req["vlm_enabled"] = vlm_enabled_;   // VLM status
    req["capabilities"] = capabilities_; // Model capabilities
    req["version"] = version_;           // Worker version
    req["registration_token"] = token_;
    
    std::string body = req.dump();
    LOG_INFO("Registering with Hub: {} body={}", reg_url, body);
    
    // Send HTTP POST
    CURL* curl = curl_easy_init();
    if (!curl) {
        LOG_ERROR("Failed to init CURL");
        return false;
    }
    
    std::string response;
    struct curl_slist* headers = nullptr;
    headers = curl_slist_append(headers, "Content-Type: application/json");
    
    curl_easy_setopt(curl, CURLOPT_URL, reg_url.c_str());
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, body.c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, 5000);
    curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT_MS, 2000);
    
    CURLcode res = curl_easy_perform(curl);
    
    long http_code = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
    
    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);
    
    if (res != CURLE_OK) {
        LOG_WARN("Registration failed: {}", curl_easy_strerror(res));
        return false;
    }
    
    LOG_INFO("Registration response ({}): {}", http_code, response);
    
    // Parse response
    try {
        auto resp = json::parse(response);
        // Check for success field or status="registered"
        if ((resp.contains("success") && resp["success"].get<bool>()) ||
            (resp.contains("status") && resp["status"].get<std::string>() == "registered")) {
            LOG_INFO("Successfully registered with Hub");
            return true;
        } else {
            std::string error = resp.value("error", "unknown error");
            LOG_WARN("Registration rejected: {}", error);
            return false;
        }
    } catch (const std::exception& e) {
        LOG_WARN("Failed to parse registration response: {}", e.what());
        return http_code == 200;
    }
}

bool HubClient::connect() {
    if (url_.empty()) {
        LOG_WARN("Hub URL not configured, running in standalone mode");
        return false;
    }
    
    LOG_INFO("Connecting to Hub: {}", url_);
    
    running_ = true;
    
    // Start connection loop (includes registration)
    loop_thread_ = std::thread(&HubClient::runLoop, this);
    
    // Start heartbeat thread
    heartbeat_thread_ = std::thread(&HubClient::sendHeartbeat, this);
    
    return true;
}

void HubClient::disconnect() {
    if (!running_) {
        return;
    }
    
    LOG_INFO("Disconnecting from Hub");
    
    running_ = false;
    connected_ = false;
    
    if (loop_thread_.joinable()) {
        loop_thread_.join();
    }
    
    if (heartbeat_thread_.joinable()) {
        heartbeat_thread_.join();
    }
    
    LOG_INFO("Disconnected from Hub");
}

bool HubClient::send(const std::string& message) {
    if (!connected_) {
        LOG_DEBUG("Cannot send: not connected to Hub");
        return false;
    }
    
    // TODO: Implement actual WebSocket send
    // For now, just log
    LOG_DEBUG("Would send to Hub: {} bytes", message.size());
    
    return true;
}

std::string HubClient::lastHeartbeat() const {
    if (last_heartbeat_ == 0) {
        return "";
    }
    return timestampToIso8601(last_heartbeat_);
}

void HubClient::setMessageHandler(MessageHandler handler) {
    message_handler_ = std::move(handler);
}

void HubClient::runLoop() {
    LOG_INFO("Hub connection loop started");
    
    while (running_) {
        if (!connected_) {
            // Try to register with Hub via HTTP
            LOG_INFO("Attempting to register with Hub...");
            
            if (registerWithHub()) {
                connected_ = true;
                last_heartbeat_ = nowMs();
                
                if (on_connect_) {
                    on_connect_();
                }
                LOG_INFO("Connected to Hub");
            } else {
                // Wait before retry
                std::this_thread::sleep_for(std::chrono::milliseconds(reconnect_ms_));
            }
        } else {
            // Just wait - heartbeat thread handles periodic updates
            std::this_thread::sleep_for(std::chrono::milliseconds(1000));
        }
    }
    
    LOG_INFO("Hub connection loop ended");
}

void HubClient::sendHeartbeat() {
    LOG_INFO("Heartbeat thread started");
    
    while (running_) {
        std::this_thread::sleep_for(std::chrono::milliseconds(heartbeat_ms_));
        
        if (!running_) break;
        
        if (connected_) {
            if (sendHeartbeatHttp()) {
                last_heartbeat_ = nowMs();
            } else {
                // Heartbeat failed, mark as disconnected
                LOG_WARN("Heartbeat failed, will re-register");
                connected_ = false;
            }
        }
    }
    
    LOG_INFO("Heartbeat thread ended");
}

bool HubClient::sendHeartbeatHttp() {
    // Call pre-heartbeat callback to update dynamic state (e.g., embed model)
    if (on_pre_heartbeat_) {
        on_pre_heartbeat_();
    }
    
    std::string hb_url = url_ + "/api/v1/nodes/" + node_id_ + "/heartbeat";
    
    // Get real-time system metrics
    auto sys = utils::SystemMonitor::getMetrics();
    
    json req;
    req["host"] = host_;
    req["connections"] = 0;
    req["load"] = sys.load_1;
    req["cpu_percent"] = sys.cpu_percent;
    req["mem_percent"] = sys.mem_percent;
    req["disk_percent"] = sys.disk_percent;
    
    // YOLO status
    json yolo_status;
    yolo_status["enabled"] = yolo_enabled_;
    yolo_status["healthy"] = yolo_enabled_;
    yolo_status["model"] = yolo_model_;
    req["yolo_status"] = yolo_status;
    
    // VLM/Llama status
    json llama_status;
    llama_status["enabled"] = vlm_enabled_;
    llama_status["healthy"] = vlm_enabled_;
    llama_status["model"] = vlm_model_;
    req["llama_status"] = llama_status;
    
    // Embed status
    json embed_status;
    embed_status["enabled"] = !embed_model_.empty();
    embed_status["healthy"] = !embed_model_.empty();
    embed_status["model"] = embed_model_;
    req["embed_status"] = embed_status;
    
    // Version
    req["version"] = version_;
    
    // Capabilities (includes all model info)
    req["capabilities"] = capabilities_;
    
    // System stats (Hub expects memory_percent not mem_percent)
    json sys_stats;
    sys_stats["cpu_percent"] = sys.cpu_percent;
    sys_stats["memory_percent"] = sys.mem_percent;
    sys_stats["disk_percent"] = sys.disk_percent;
    sys_stats["load_1"] = sys.load_1;
    req["system_stats"] = sys_stats;
    
    // Uptime in seconds
    auto now_sec = std::chrono::duration_cast<std::chrono::seconds>(
        std::chrono::system_clock::now().time_since_epoch()).count();
    req["uptime_seconds"] = now_sec - start_time_;
    
    std::string body = req.dump();
    
    CURL* curl = curl_easy_init();
    if (!curl) {
        return false;
    }
    
    std::string response;
    struct curl_slist* headers = nullptr;
    headers = curl_slist_append(headers, "Content-Type: application/json");
    
    curl_easy_setopt(curl, CURLOPT_URL, hb_url.c_str());
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, body.c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, 3000);
    curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT_MS, 1000);
    
    CURLcode res = curl_easy_perform(curl);
    
    long http_code = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
    
    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);
    
    if (res != CURLE_OK) {
        LOG_DEBUG("Heartbeat request failed: {}", curl_easy_strerror(res));
        return false;
    }
    
    return http_code == 200;
}

} // namespace rivision::hub
