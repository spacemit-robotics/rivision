#pragma once

#include "rivision/types.h"
#include <vector>
#include <queue>
#include <mutex>
#include <condition_variable>
#include <thread>
#include <atomic>
#include <functional>

namespace rivision::pipeline {
class Embedder;  // Forward declaration for pipeline::Embedder
}

namespace rivision::core {

using Embedder = rivision::pipeline::Embedder;

class VectorizeManager {
public:
    struct VectorizeRequest {
        enum class Type { IMAGE, TEXT, FACE };
        Type type;
        std::string id;
        Frame frame;
        std::string text;
        BBox roi;  // For face/crop embedding
    };
    
    struct VectorizeResult {
        std::string id;
        std::vector<float> embedding;
        bool success = false;
        std::string error;
    };
    
    using ResultCallback = std::function<void(const VectorizeResult&)>;
    
    VectorizeManager(int queue_size = 100);
    ~VectorizeManager();
    
    void setEmbedder(Embedder* embedder) { embedder_ = embedder; }
    void setResultCallback(ResultCallback cb) { result_callback_ = std::move(cb); }
    
    void start();
    void stop();
    
    bool enqueueImage(const std::string& id, const Frame& frame);
    bool enqueueText(const std::string& id, const std::string& text);
    bool enqueueFace(const std::string& id, const Frame& frame, const BBox& face_roi);
    
    size_t queueSize() const;
    bool isRunning() const { return running_; }
    
    VectorizeResult processSync(const VectorizeRequest& request);
    
private:
    void workerLoop();
    VectorizeResult process(const VectorizeRequest& request);
    
    Embedder* embedder_ = nullptr;
    ResultCallback result_callback_;
    
    std::queue<VectorizeRequest> queue_;
    mutable std::mutex queue_mutex_;
    std::condition_variable queue_cv_;
    
    std::thread worker_thread_;
    std::atomic<bool> running_{false};
    int max_queue_size_;
};

} // namespace rivision::core
