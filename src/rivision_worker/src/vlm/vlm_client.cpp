#include "vlm_client.h"
#include "utils/logger.h"

#include <nlohmann/json.hpp>
#include <curl/curl.h>
#include <opencv2/opencv.hpp>
#include <sstream>

using json = nlohmann::json;

namespace rivision::core {

// =============================================================================
// CURL helpers
// =============================================================================

static size_t writeCallback(void* contents, size_t size, size_t nmemb, std::string* s) {
    size_t new_len = size * nmemb;
    s->append(static_cast<char*>(contents), new_len);
    return new_len;
}

// =============================================================================
// VlmClient::Impl
// =============================================================================

class VlmClient::Impl {
public:
    Impl() {
        curl_global_init(CURL_GLOBAL_DEFAULT);
    }
    
    ~Impl() {
        curl_global_cleanup();
    }
    
    std::string post(const std::string& url, const std::string& body, int timeout_ms) {
        CURL* curl = curl_easy_init();
        if (!curl) {
            LOG_ERROR("Failed to init CURL");
            return "";
        }
        
        std::string response;
        
        struct curl_slist* headers = nullptr;
        headers = curl_slist_append(headers, "Content-Type: application/json");
        
        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, body.c_str());
        curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);
        curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, timeout_ms);
        curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT_MS, 2000);
        
        CURLcode res = curl_easy_perform(curl);
        
        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);
        
        if (res != CURLE_OK) {
            LOG_WARN("CURL error: {}", curl_easy_strerror(res));
            return "";
        }
        
        return response;
    }
    
    std::string get(const std::string& url, int timeout_ms) {
        CURL* curl = curl_easy_init();
        if (!curl) {
            return "";
        }
        
        std::string response;
        
        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);
        curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, timeout_ms);
        curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT_MS, 2000);
        
        CURLcode res = curl_easy_perform(curl);
        curl_easy_cleanup(curl);
        
        if (res != CURLE_OK) {
            return "";
        }
        
        return response;
    }
};

// =============================================================================
// VlmClient
// =============================================================================

VlmClient::VlmClient(const std::string& endpoint)
    : endpoint_(endpoint)
    , impl_(std::make_unique<Impl>()) {
    
    // Remove trailing slash
    if (!endpoint_.empty() && endpoint_.back() == '/') {
        endpoint_.pop_back();
    }
}

VlmClient::~VlmClient() = default;

bool VlmClient::checkHealth() {
    std::string url = endpoint_ + "/health";
    std::string response = impl_->get(url, 2000);
    
    if (response.empty()) {
        connected_ = false;
        return false;
    }
    
    try {
        json j = json::parse(response);
        connected_ = j.value("status", "") == "ok";
        return connected_;
    } catch (...) {
        connected_ = false;
        return false;
    }
}

VlmClient::VerifyResult VlmClient::verify(const Frame& frame, const std::string& prompt) {
    std::string image_base64 = encodeFrame(frame);
    return verify(image_base64, prompt);
}

VlmClient::VerifyResult VlmClient::verify(const std::string& image_base64, 
                                          const std::string& prompt) {
    VerifyResult result;
    
    json req;
    req["image"] = image_base64;
    req["prompt"] = prompt;
    req["mode"] = "verify";
    
    std::string url = endpoint_ + "/api/v1/verify";
    std::string response = impl_->post(url, req.dump(), timeout_ms_);
    
    if (response.empty()) {
        result.timeout = true;
        LOG_WARN("VLM verify timeout");
        return result;
    }
    
    try {
        json j = json::parse(response);
        result.verified = j.value("verified", false);
        result.confidence = j.value("confidence", 0.0f);
        result.description = j.value("description", "");
    } catch (const std::exception& e) {
        LOG_WARN("VLM verify parse error: {}", e.what());
        result.timeout = true;
    }
    
    return result;
}

VlmClient::AnalyzeResult VlmClient::analyze(const Frame& frame, const std::string& prompt) {
    AnalyzeResult result;
    
    std::string image_base64 = encodeFrame(frame);
    
    json req;
    req["image"] = image_base64;
    req["prompt"] = prompt.empty() ? "Describe this scene in detail." : prompt;
    
    std::string url = endpoint_ + "/api/v1/analyze";
    std::string response = impl_->post(url, req.dump(), timeout_ms_);
    
    if (response.empty()) {
        result.timeout = true;
        return result;
    }
    
    try {
        json j = json::parse(response);
        result.description = j.value("description", "");
        
        if (j.contains("objects")) {
            for (const auto& obj : j["objects"]) {
                result.objects.push_back(obj.get<std::string>());
            }
        }
        
        if (j.contains("scene_type")) {
            result.scene_type = j["scene_type"].get<std::string>();
        }
    } catch (const std::exception& e) {
        LOG_WARN("VLM analyze parse error: {}", e.what());
        result.timeout = true;
    }
    
    return result;
}

VlmClient::EmotionResult VlmClient::analyzeEmotion(const Frame& face_crop) {
    EmotionResult result;
    
    std::string image_base64 = encodeFrame(face_crop);
    
    json req;
    req["image"] = image_base64;
    req["prompt"] = "What emotion is this person expressing? Respond with JSON: {\"emotion\": \"...\", \"valence\": -1 to 1}";
    
    std::string url = endpoint_ + "/api/v1/emotion";
    std::string response = impl_->post(url, req.dump(), timeout_ms_);
    
    if (response.empty()) {
        result.timeout = true;
        return result;
    }
    
    try {
        json j = json::parse(response);
        result.emotion = j.value("emotion", "neutral");
        result.valence = j.value("valence", 0.0f);
        result.confidence = j.value("confidence", 0.5f);
    } catch (const std::exception& e) {
        LOG_WARN("VLM emotion parse error: {}", e.what());
        // Try keyword parsing
        if (response.find("happy") != std::string::npos) {
            result.emotion = "happy";
            result.valence = 0.7f;
        } else if (response.find("sad") != std::string::npos) {
            result.emotion = "sad";
            result.valence = -0.5f;
        } else if (response.find("angry") != std::string::npos) {
            result.emotion = "angry";
            result.valence = -0.7f;
        } else {
            result.emotion = "neutral";
            result.valence = 0.0f;
        }
        result.confidence = 0.3f;
    }
    
    return result;
}

std::string VlmClient::httpPost(const std::string& path, const std::string& body) {
    return impl_->post(endpoint_ + path, body, timeout_ms_);
}

std::string VlmClient::encodeFrame(const Frame& frame) {
    if (frame.isEmpty()) {
        return "";
    }
    
    try {
        // Convert frame to cv::Mat
        cv::Mat img;
        
        if (frame.format == PixelFormat::RGB24) {
            img = cv::Mat(frame.height, frame.width, CV_8UC3,
                         const_cast<uint8_t*>(frame.cpuData()));
            cv::cvtColor(img, img, cv::COLOR_RGB2BGR);
        } else if (frame.format == PixelFormat::BGR24) {
            img = cv::Mat(frame.height, frame.width, CV_8UC3,
                         const_cast<uint8_t*>(frame.cpuData()));
        } else if (frame.format == PixelFormat::NV12) {
            cv::Mat yuv(frame.height * 3 / 2, frame.width, CV_8UC1,
                       const_cast<uint8_t*>(frame.cpuData()));
            cv::cvtColor(yuv, img, cv::COLOR_YUV2BGR_NV12);
        } else {
            LOG_WARN("Unsupported pixel format for VLM encoding");
            return "";
        }
        
        // Encode to JPEG
        std::vector<uchar> jpeg_buf;
        std::vector<int> params = {cv::IMWRITE_JPEG_QUALITY, 85};
        if (!cv::imencode(".jpg", img, jpeg_buf, params)) {
            LOG_WARN("Failed to encode frame to JPEG");
            return "";
        }
        
        // Base64 encode
        static const char* base64_chars =
            "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        
        std::string result;
        result.reserve(((jpeg_buf.size() + 2) / 3) * 4);
        
        size_t i = 0;
        while (i < jpeg_buf.size()) {
            uint32_t a = i < jpeg_buf.size() ? jpeg_buf[i++] : 0;
            uint32_t b = i < jpeg_buf.size() ? jpeg_buf[i++] : 0;
            uint32_t c = i < jpeg_buf.size() ? jpeg_buf[i++] : 0;
            uint32_t triple = (a << 16) + (b << 8) + c;
            
            result.push_back(base64_chars[(triple >> 18) & 0x3F]);
            result.push_back(base64_chars[(triple >> 12) & 0x3F]);
            result.push_back(base64_chars[(triple >> 6) & 0x3F]);
            result.push_back(base64_chars[triple & 0x3F]);
        }
        
        // Padding
        size_t mod = jpeg_buf.size() % 3;
        if (mod == 1) {
            result[result.size() - 1] = '=';
            result[result.size() - 2] = '=';
        } else if (mod == 2) {
            result[result.size() - 1] = '=';
        }
        
        return result;
        
    } catch (const cv::Exception& e) {
        LOG_ERROR("OpenCV error encoding frame: {}", e.what());
        return "";
    }
}

} // namespace rivision::core
