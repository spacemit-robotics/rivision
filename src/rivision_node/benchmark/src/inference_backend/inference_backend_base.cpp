// InferenceBackend 基类默认实现

#include "inference_backend.h"
#include <thread>

namespace rivision {
namespace benchmark {

// 默认批量推理实现 - 串行调用 infer
std::vector<InferenceResult> InferenceBackend::infer_batch(
    const std::vector<Frame>& frames, int concurrency) {
    
    std::vector<InferenceResult> results;
    results.reserve(frames.size());
    
    if (concurrency <= 1) {
        // 串行处理
        for (const auto& frame : frames) {
            results.push_back(infer(frame));
        }
    } else {
        // 并行处理
        std::vector<std::future<InferenceResult>> futures;
        futures.reserve(frames.size());
        
        for (const auto& frame : frames) {
            futures.push_back(std::async(std::launch::async, 
                [this, &frame]() { return infer(frame); }));
        }
        
        for (auto& f : futures) {
            results.push_back(f.get());
        }
    }
    
    return results;
}

// 默认异步推理实现
std::future<InferenceResult> InferenceBackend::infer_async(const Frame& frame) {
    return std::async(std::launch::async, [this, frame]() {
        return infer(frame);
    });
}


}  // namespace benchmark
}  // namespace rivision
