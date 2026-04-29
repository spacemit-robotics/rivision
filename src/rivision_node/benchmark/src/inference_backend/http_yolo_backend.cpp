#include "inference_backend.h"
#include <curl/curl.h>
#include <queue>
#include <thread>
#include <mutex>
#include <condition_variable>
#include <sstream>
#include <cstring>

namespace rivision {
namespace benchmark {

// ============================================================
// CURL 回调
// ============================================================
static size_t write_callback(void* contents, size_t size, size_t nmemb, std::string* s) {
    size_t total_size = size * nmemb;
    s->append(static_cast<char*>(contents), total_size);
    return total_size;
}

// ============================================================
// HTTP 连接池
// ============================================================
class CurlPool {
public:
    CurlPool(int size) : pool_size_(size) {
        for (int i = 0; i < size; i++) {
            CURL* curl = curl_easy_init();
            if (curl) {
                available_.push(curl);
            }
        }
    }
    
    ~CurlPool() {
        while (!available_.empty()) {
            curl_easy_cleanup(available_.front());
            available_.pop();
        }
    }
    
    CURL* acquire() {
        std::unique_lock<std::mutex> lock(mutex_);
        cv_.wait(lock, [this]() { return !available_.empty(); });
        CURL* curl = available_.front();
        available_.pop();
        return curl;
    }
    
    void release(CURL* curl) {
        curl_easy_reset(curl);
        std::lock_guard<std::mutex> lock(mutex_);
        available_.push(curl);
        cv_.notify_one();
    }
    
    int available_count() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return static_cast<int>(available_.size());
    }

private:
    int pool_size_;
    std::queue<CURL*> available_;
    mutable std::mutex mutex_;
    std::condition_variable cv_;
};

// ============================================================
// HTTPYOLOBackend 实现
// ============================================================
struct HTTPYOLOBackend::Impl {
    std::unique_ptr<CurlPool> curl_pool;
    std::string detect_url;
    struct curl_slist* headers = nullptr;
    
    std::atomic<int> active_requests{0};
    std::atomic<int> pending_requests{0};
};

HTTPYOLOBackend::HTTPYOLOBackend() : impl_(std::make_unique<Impl>()) {
    curl_global_init(CURL_GLOBAL_ALL);
}

HTTPYOLOBackend::~HTTPYOLOBackend() {
    shutdown();
    curl_global_cleanup();
}

bool HTTPYOLOBackend::init(const BackendConfig& config) {
    config_ = config;
    
    // 初始化连接池
    impl_->curl_pool = std::make_unique<CurlPool>(config.http_pool_size);
    impl_->detect_url = config.http_url + "/api/detect/binary";
    
    // 设置 headers
    impl_->headers = curl_slist_append(impl_->headers, "Content-Type: application/octet-stream");
    
    // 检查服务健康
    CURL* curl = curl_easy_init();
    if (curl) {
        std::string response;
        std::string health_url = config.http_url + "/health";
        
        curl_easy_setopt(curl, CURLOPT_URL, health_url.c_str());
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_callback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);
        curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, 5000L);
        
        CURLcode res = curl_easy_perform(curl);
        curl_easy_cleanup(curl);
        
        if (res == CURLE_OK && response.find("\"status\":\"ok\"") != std::string::npos) {
            is_connected_ = true;
            printf("[HTTPYOLOBackend] Connected to %s (pool_size=%d)\n",
                   config.http_url.c_str(), config.http_pool_size);
            return true;
        }
    }
    
    fprintf(stderr, "[HTTPYOLOBackend] Failed to connect to %s\n", config.http_url.c_str());
    return false;
}

InferenceResult HTTPYOLOBackend::infer(const Frame& frame) {
    InferenceResult result;
    result.frame_id = frame.frame_id;
    result.timestamp_us = frame.timestamp_us;
    
    if (!is_connected_) {
        result.error = "Not connected";
        return result;
    }
    
    auto total_start = Clock::now();
    
    // 获取 curl handle
    impl_->pending_requests++;
    CURL* curl = impl_->curl_pool->acquire();
    impl_->pending_requests--;
    impl_->active_requests++;
    
    std::string response;
    
    curl_easy_setopt(curl, CURLOPT_URL, impl_->detect_url.c_str());
    curl_easy_setopt(curl, CURLOPT_POST, 1L);
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, impl_->headers);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, frame.data.data());
    curl_easy_setopt(curl, CURLOPT_POSTFIELDSIZE, static_cast<long>(frame.data.size()));
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_callback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, static_cast<long>(config_.http_timeout_ms));
    
    auto network_start = Clock::now();
    CURLcode res = curl_easy_perform(curl);
    auto network_end = Clock::now();
    
    result.network_ms = Duration(network_end - network_start).count();
    
    long http_code = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
    result.http_status = static_cast<int>(http_code);
    
    impl_->curl_pool->release(curl);
    impl_->active_requests--;
    
    if (res != CURLE_OK) {
        result.error = std::string("CURL error: ") + curl_easy_strerror(res);
        return result;
    }
    
    if (http_code != 200) {
        result.error = "HTTP error: " + std::to_string(http_code);
        return result;
    }
    
    // 解析响应 JSON
    // {"success":true,"detections":[...],"inference_ms":123}
    
    // 简单解析 inference_ms
    size_t pos = response.find("\"inference_ms\":");
    if (pos != std::string::npos) {
        result.inference_ms = std::stod(response.substr(pos + 15));
    }
    
    // 解析 detections
    pos = response.find("\"detections\":[");
    if (pos != std::string::npos) {
        size_t start = pos + 14;
        size_t end = response.find("]", start);
        std::string detections_str = response.substr(start, end - start);
        
        // 计算检测数 (简单计数 "class_id")
        size_t search_pos = 0;
        while ((search_pos = detections_str.find("\"class_id\":", search_pos)) != std::string::npos) {
            Detection det;
            
            // 解析 bbox
            size_t bbox_pos = detections_str.rfind("\"bbox\":[", search_pos);
            if (bbox_pos != std::string::npos) {
                sscanf(detections_str.c_str() + bbox_pos + 8, "%f,%f,%f,%f",
                       &det.x1, &det.y1, &det.x2, &det.y2);
            }
            
            // 解析 class_id
            det.class_id = std::stoi(detections_str.substr(search_pos + 11));
            
            // 解析 confidence
            size_t conf_pos = detections_str.find("\"confidence\":", search_pos);
            if (conf_pos != std::string::npos) {
                det.confidence = std::stof(detections_str.substr(conf_pos + 13));
            }
            
            // 解析 class_name
            size_t name_pos = detections_str.find("\"class_name\":\"", search_pos);
            if (name_pos != std::string::npos) {
                size_t name_start = name_pos + 14;
                size_t name_end = detections_str.find("\"", name_start);
                det.class_name = detections_str.substr(name_start, name_end - name_start);
            }
            
            result.detections.push_back(det);
            search_pos++;
        }
    }
    
    result.success = response.find("\"success\":true") != std::string::npos;
    
    auto total_end = Clock::now();
    result.total_ms = Duration(total_end - total_start).count();
    
    return result;
}

std::vector<InferenceResult> HTTPYOLOBackend::infer_batch(
    const std::vector<Frame>& frames, int concurrency) {
    
    std::vector<InferenceResult> results(frames.size());
    std::vector<std::future<void>> futures;
    
    concurrency = std::min(concurrency, static_cast<int>(frames.size()));
    concurrency = std::min(concurrency, config_.http_pool_size);
    
    std::atomic<size_t> next_idx{0};
    
    for (int t = 0; t < concurrency; t++) {
        futures.push_back(std::async(std::launch::async, [&]() {
            while (true) {
                size_t idx = next_idx.fetch_add(1);
                if (idx >= frames.size()) break;
                results[idx] = infer(frames[idx]);
            }
        }));
    }
    
    for (auto& f : futures) {
        f.get();
    }
    
    return results;
}

BackendInfo HTTPYOLOBackend::info() const {
    BackendInfo info;
    info.name = "HTTP YOLO";
    info.version = "1.0.0";
    info.backend_type = "http";
    info.model_name = config_.http_url;
    info.workers = config_.http_pool_size;
    info.is_loaded = is_connected_;
    return info;
}

int HTTPYOLOBackend::active_connections() const {
    return impl_->active_requests;
}

int HTTPYOLOBackend::pending_requests() const {
    return impl_->pending_requests;
}

void HTTPYOLOBackend::shutdown() {
    if (impl_->headers) {
        curl_slist_free_all(impl_->headers);
        impl_->headers = nullptr;
    }
    impl_->curl_pool.reset();
    is_connected_ = false;
}

// ============================================================
// ConcurrentInferenceWrapper 实现
// ============================================================
ConcurrentInferenceWrapper::ConcurrentInferenceWrapper(InferenceBackend* backend, int concurrency)
    : backend_(backend), concurrency_(concurrency) {
    
    running_ = true;
    
    for (int i = 0; i < concurrency; i++) {
        workers_.emplace_back(&ConcurrentInferenceWrapper::worker_thread, this);
    }
}

ConcurrentInferenceWrapper::~ConcurrentInferenceWrapper() {
    wait_all();
    
    running_ = false;
    cv_.notify_all();
    
    for (auto& w : workers_) {
        if (w.joinable()) {
            w.join();
        }
    }
}

void ConcurrentInferenceWrapper::submit(const Frame& frame, InferenceCallback callback) {
    std::lock_guard<std::mutex> lock(mutex_);
    task_queue_.push({frame, callback});
    cv_.notify_one();
}

void ConcurrentInferenceWrapper::wait_all() {
    std::unique_lock<std::mutex> lock(mutex_);
    cv_.wait(lock, [this]() { return task_queue_.empty(); });
}

int ConcurrentInferenceWrapper::queue_size() const {
    std::lock_guard<std::mutex> lock(const_cast<std::mutex&>(mutex_));
    return static_cast<int>(task_queue_.size());
}

void ConcurrentInferenceWrapper::worker_thread() {
    while (running_) {
        Task task;
        
        {
            std::unique_lock<std::mutex> lock(mutex_);
            cv_.wait(lock, [this]() { return !task_queue_.empty() || !running_; });
            
            if (!running_ && task_queue_.empty()) break;
            
            task = std::move(task_queue_.front());
            task_queue_.pop();
        }
        
        auto result = backend_->infer(task.frame);
        
        if (result.success) {
            completed_count_++;
        } else {
            error_count_++;
        }
        
        if (task.callback) {
            task.callback(result);
        }
    }
}

}  // namespace benchmark
}  // namespace rivision
