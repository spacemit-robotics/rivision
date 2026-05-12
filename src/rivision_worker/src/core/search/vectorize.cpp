#include "vectorize.h"
#include "pipeline/inference/embedder.h"
#include "utils/logger.h"

namespace rivision::core {

VectorizeManager::VectorizeManager(int queue_size)
    : max_queue_size_(queue_size) {}

VectorizeManager::~VectorizeManager() {
    stop();
}

void VectorizeManager::start() {
    if (running_) return;
    
    running_ = true;
    worker_thread_ = std::thread(&VectorizeManager::workerLoop, this);
    LOG_INFO("VectorizeManager started");
}

void VectorizeManager::stop() {
    if (!running_) return;
    
    running_ = false;
    queue_cv_.notify_all();
    
    if (worker_thread_.joinable()) {
        worker_thread_.join();
    }
    LOG_INFO("VectorizeManager stopped");
}

bool VectorizeManager::enqueueImage(const std::string& id, const Frame& frame) {
    std::lock_guard lock(queue_mutex_);
    
    if (queue_.size() >= static_cast<size_t>(max_queue_size_)) {
        LOG_WARN("Vectorize queue full, dropping request {}", id);
        return false;
    }
    
    VectorizeRequest req;
    req.type = VectorizeRequest::Type::IMAGE;
    req.id = id;
    req.frame = frame;
    
    queue_.push(std::move(req));
    queue_cv_.notify_one();
    
    return true;
}

bool VectorizeManager::enqueueText(const std::string& id, const std::string& text) {
    std::lock_guard lock(queue_mutex_);
    
    if (queue_.size() >= static_cast<size_t>(max_queue_size_)) {
        LOG_WARN("Vectorize queue full, dropping request {}", id);
        return false;
    }
    
    VectorizeRequest req;
    req.type = VectorizeRequest::Type::TEXT;
    req.id = id;
    req.text = text;
    
    queue_.push(std::move(req));
    queue_cv_.notify_one();
    
    return true;
}

bool VectorizeManager::enqueueFace(const std::string& id, const Frame& frame, const BBox& face_roi) {
    std::lock_guard lock(queue_mutex_);
    
    if (queue_.size() >= static_cast<size_t>(max_queue_size_)) {
        LOG_WARN("Vectorize queue full, dropping request {}", id);
        return false;
    }
    
    VectorizeRequest req;
    req.type = VectorizeRequest::Type::FACE;
    req.id = id;
    req.frame = frame;
    req.roi = face_roi;
    
    queue_.push(std::move(req));
    queue_cv_.notify_one();
    
    return true;
}

size_t VectorizeManager::queueSize() const {
    std::lock_guard lock(queue_mutex_);
    return queue_.size();
}

void VectorizeManager::workerLoop() {
    while (running_) {
        VectorizeRequest request;
        
        {
            std::unique_lock lock(queue_mutex_);
            queue_cv_.wait(lock, [this] {
                return !running_ || !queue_.empty();
            });
            
            if (!running_ && queue_.empty()) {
                break;
            }
            
            if (queue_.empty()) {
                continue;
            }
            
            request = std::move(queue_.front());
            queue_.pop();
        }
        
        auto result = process(request);
        
        if (result_callback_) {
            result_callback_(result);
        }
    }
}

VectorizeManager::VectorizeResult VectorizeManager::process(const VectorizeRequest& request) {
    VectorizeResult result;
    result.id = request.id;
    
    if (!embedder_) {
        result.error = "Embedder not set";
        return result;
    }
    
    try {
        bool success = false;
        switch (request.type) {
            case VectorizeRequest::Type::IMAGE:
                success = embedder_->embed(request.frame, result.embedding);
                break;
                
            case VectorizeRequest::Type::TEXT:
                success = embedder_->embedText(request.text, result.embedding);
                break;
                
            case VectorizeRequest::Type::FACE:
                // For face, we embed the full frame (crop should be done before)
                success = embedder_->embed(request.frame, result.embedding);
                break;
        }
        
        result.success = success && !result.embedding.empty();
        if (!result.success) {
            result.error = embedder_->getLastError();
            if (result.error.empty()) {
                result.error = "Empty embedding returned";
            }
        }
        
    } catch (const std::exception& e) {
        result.error = e.what();
        LOG_ERROR("Vectorize failed for {}: {}", request.id, e.what());
    }
    
    return result;
}

VectorizeManager::VectorizeResult VectorizeManager::processSync(const VectorizeRequest& request) {
    return process(request);
}

} // namespace rivision::core
