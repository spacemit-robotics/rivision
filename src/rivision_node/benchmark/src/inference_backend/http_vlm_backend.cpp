#include "inference_backend.h"
#include <curl/curl.h>
#include <sstream>
#include <cstring>
#include <regex>

namespace rivision {
namespace benchmark {

// ============================================================
// CURL 回调
// ============================================================
static size_t vlm_write_callback(void* contents, size_t size, size_t nmemb, std::string* s) {
    size_t total_size = size * nmemb;
    s->append(static_cast<char*>(contents), total_size);
    return total_size;
}

// Base64 编码
static const char base64_chars[] = 
    "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    "abcdefghijklmnopqrstuvwxyz"
    "0123456789+/";

static std::string base64_encode(const uint8_t* data, size_t len) {
    std::string result;
    result.reserve((len + 2) / 3 * 4);
    
    for (size_t i = 0; i < len; i += 3) {
        uint32_t n = static_cast<uint32_t>(data[i]) << 16;
        if (i + 1 < len) n |= static_cast<uint32_t>(data[i + 1]) << 8;
        if (i + 2 < len) n |= static_cast<uint32_t>(data[i + 2]);
        
        result += base64_chars[(n >> 18) & 0x3f];
        result += base64_chars[(n >> 12) & 0x3f];
        result += (i + 1 < len) ? base64_chars[(n >> 6) & 0x3f] : '=';
        result += (i + 2 < len) ? base64_chars[n & 0x3f] : '=';
    }
    
    return result;
}

// ============================================================
// HTTPVLMBackend 实现
// ============================================================
struct HTTPVLMBackend::Impl {
    CURL* curl = nullptr;
    std::string api_url;
    struct curl_slist* headers = nullptr;
    std::string prompt;
    int max_tokens = 512;
    float temperature = 0.3f;
};

HTTPVLMBackend::HTTPVLMBackend() : impl_(std::make_unique<Impl>()) {
    curl_global_init(CURL_GLOBAL_ALL);
}

HTTPVLMBackend::~HTTPVLMBackend() {
    shutdown();
    curl_global_cleanup();
}

bool HTTPVLMBackend::init(const BackendConfig& config) {
    config_ = config;
    
    impl_->curl = curl_easy_init();
    if (!impl_->curl) {
        fprintf(stderr, "[HTTPVLMBackend] Failed to init CURL\n");
        return false;
    }
    
    // 支持 OpenAI 兼容 API 或 llama-server
    impl_->api_url = config.http_url + "/v1/chat/completions";
    impl_->prompt = config.prompt;
    impl_->max_tokens = config.max_tokens;
    impl_->temperature = config.temperature;
    
    impl_->headers = curl_slist_append(impl_->headers, "Content-Type: application/json");
    
    // 检查服务
    std::string health_url = config.http_url + "/health";
    std::string response;
    
    curl_easy_setopt(impl_->curl, CURLOPT_URL, health_url.c_str());
    curl_easy_setopt(impl_->curl, CURLOPT_WRITEFUNCTION, vlm_write_callback);
    curl_easy_setopt(impl_->curl, CURLOPT_WRITEDATA, &response);
    curl_easy_setopt(impl_->curl, CURLOPT_TIMEOUT_MS, 5000L);
    
    CURLcode res = curl_easy_perform(impl_->curl);
    curl_easy_reset(impl_->curl);
    
    if (res == CURLE_OK) {
        is_connected_ = true;
        printf("[HTTPVLMBackend] Connected to %s\n", config.http_url.c_str());
        return true;
    }
    
    fprintf(stderr, "[HTTPVLMBackend] Failed to connect to %s\n", config.http_url.c_str());
    return false;
}

InferenceResult HTTPVLMBackend::infer(const Frame& frame) {
    InferenceResult result;
    result.frame_id = frame.frame_id;
    result.timestamp_us = frame.timestamp_us;
    
    if (!is_connected_) {
        result.error = "Not connected";
        return result;
    }
    
    auto total_start = Clock::now();
    
    // Base64 编码图片
    std::string image_base64 = base64_encode(frame.data.data(), frame.data.size());
    
    // 构建 OpenAI 兼容请求
    // {
    //   "model": "vision",
    //   "messages": [{
    //     "role": "user",
    //     "content": [
    //       {"type": "text", "text": "..."},
    //       {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,..."}}
    //     ]
    //   }],
    //   "max_tokens": 512
    // }
    
    std::ostringstream json;
    json << R"({"model":"vision","messages":[{"role":"user","content":[)";
    json << R"({"type":"text","text":")" << impl_->prompt << R"("},)";
    json << R"({"type":"image_url","image_url":{"url":"data:image/jpeg;base64,)" << image_base64 << R"("}}]}],)";
    json << R"("max_tokens":)" << impl_->max_tokens;
    json << R"(,"temperature":)" << impl_->temperature;
    json << R"(,"stream":false})";
    
    std::string request_body = json.str();
    std::string response;
    
    curl_easy_setopt(impl_->curl, CURLOPT_URL, impl_->api_url.c_str());
    curl_easy_setopt(impl_->curl, CURLOPT_POST, 1L);
    curl_easy_setopt(impl_->curl, CURLOPT_HTTPHEADER, impl_->headers);
    curl_easy_setopt(impl_->curl, CURLOPT_POSTFIELDS, request_body.c_str());
    curl_easy_setopt(impl_->curl, CURLOPT_POSTFIELDSIZE, static_cast<long>(request_body.size()));
    curl_easy_setopt(impl_->curl, CURLOPT_WRITEFUNCTION, vlm_write_callback);
    curl_easy_setopt(impl_->curl, CURLOPT_WRITEDATA, &response);
    curl_easy_setopt(impl_->curl, CURLOPT_TIMEOUT_MS, static_cast<long>(config_.http_timeout_ms));
    
    auto network_start = Clock::now();
    CURLcode res = curl_easy_perform(impl_->curl);
    auto network_end = Clock::now();
    
    result.network_ms = Duration(network_end - network_start).count();
    
    long http_code = 0;
    curl_easy_getinfo(impl_->curl, CURLINFO_RESPONSE_CODE, &http_code);
    result.http_status = static_cast<int>(http_code);
    
    curl_easy_reset(impl_->curl);
    
    if (res != CURLE_OK) {
        result.error = std::string("CURL error: ") + curl_easy_strerror(res);
        return result;
    }
    
    if (http_code != 200) {
        result.error = "HTTP error: " + std::to_string(http_code);
        return result;
    }
    
    // 解析响应
    // {"choices":[{"message":{"content":"..."}}],"usage":{"completion_tokens":123}}
    
    // 提取 content
    std::regex content_regex("\"content\"\\s*:\\s*\"([^\"]*)\"");
    std::smatch match;
    if (std::regex_search(response, match, content_regex)) {
        result.text_output = match[1].str();
    }
    
    // 提取 completion_tokens
    std::regex tokens_regex("\"completion_tokens\"\\s*:\\s*(\\d+)");
    if (std::regex_search(response, match, tokens_regex)) {
        result.tokens_generated = std::stoi(match[1].str());
    }
    
    // 计算 tokens/s
    auto total_end = Clock::now();
    result.total_ms = Duration(total_end - total_start).count();
    result.inference_ms = result.network_ms;
    
    if (result.total_ms > 0 && result.tokens_generated > 0) {
        result.tokens_per_sec = result.tokens_generated * 1000.0 / result.total_ms;
    }
    
    result.ttft_ms = result.network_ms;  // 简化：首 token 延迟 ≈ 网络延迟
    
    result.success = !result.text_output.empty();
    return result;
}

BackendInfo HTTPVLMBackend::info() const {
    BackendInfo info;
    info.name = "HTTP VLM";
    info.version = "1.0.0";
    info.backend_type = "http";
    info.model_name = config_.http_url;
    info.workers = config_.http_pool_size;
    info.is_loaded = is_connected_;
    return info;
}

void HTTPVLMBackend::shutdown() {
    if (impl_->curl) {
        curl_easy_cleanup(impl_->curl);
        impl_->curl = nullptr;
    }
    if (impl_->headers) {
        curl_slist_free_all(impl_->headers);
        impl_->headers = nullptr;
    }
    is_connected_ = false;
}

}  // namespace benchmark
}  // namespace rivision
