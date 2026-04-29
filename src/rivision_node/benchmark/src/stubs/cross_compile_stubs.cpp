// 交叉编译时的桩实现
// 这些函数在没有完整依赖时提供空实现

#include "inference_backend.h"

namespace rivision {
namespace benchmark {

#ifdef NO_ONNXRUNTIME

// LocalYOLOBackend stubs (当没有 ONNX Runtime 时)
struct LocalYOLOBackend::Impl {};

LocalYOLOBackend::LocalYOLOBackend() : impl_(std::make_unique<Impl>()) {}
LocalYOLOBackend::~LocalYOLOBackend() = default;

bool LocalYOLOBackend::init(const BackendConfig& config) {
    (void)config;
    return false;
}

InferenceResult LocalYOLOBackend::infer(const Frame& frame) {
    (void)frame;
    InferenceResult result;
    result.success = false;
    result.error = "Local YOLO backend not available (compiled without ONNX Runtime)";
    return result;
}

std::vector<InferenceResult> LocalYOLOBackend::infer_batch(
    const std::vector<Frame>& frames, int concurrency) {
    (void)frames;
    (void)concurrency;
    return {};
}

BackendInfo LocalYOLOBackend::info() const {
    BackendInfo info;
    info.name = "LocalYOLO";
    info.backend_type = "stub";
    return info;
}

double LocalYOLOBackend::get_memory_usage_mb() const {
    return 0;
}

void LocalYOLOBackend::shutdown() {}

#endif  // NO_ONNXRUNTIME

#ifdef NO_HTTP_BACKENDS

// HTTPYOLOBackend stubs
struct HTTPYOLOBackend::Impl {};

HTTPYOLOBackend::HTTPYOLOBackend() : impl_(std::make_unique<Impl>()) {}
HTTPYOLOBackend::~HTTPYOLOBackend() = default;

bool HTTPYOLOBackend::init(const BackendConfig& config) {
    (void)config;
    return false;
}

InferenceResult HTTPYOLOBackend::infer(const Frame& frame) {
    (void)frame;
    InferenceResult result;
    result.success = false;
    result.error = "HTTP YOLO backend not available (compiled without CURL)";
    return result;
}

std::vector<InferenceResult> HTTPYOLOBackend::infer_batch(
    const std::vector<Frame>& frames, int concurrency) {
    (void)frames;
    (void)concurrency;
    return {};
}

BackendInfo HTTPYOLOBackend::info() const {
    BackendInfo info;
    info.name = "HTTPYOLO";
    info.backend_type = "stub";
    return info;
}

void HTTPYOLOBackend::shutdown() {}

// HTTPVLMBackend stubs
struct HTTPVLMBackend::Impl {};

HTTPVLMBackend::HTTPVLMBackend() : impl_(std::make_unique<Impl>()) {}
HTTPVLMBackend::~HTTPVLMBackend() = default;

bool HTTPVLMBackend::init(const BackendConfig& config) {
    (void)config;
    return false;
}

InferenceResult HTTPVLMBackend::infer(const Frame& frame) {
    (void)frame;
    InferenceResult result;
    result.success = false;
    result.error = "HTTP VLM backend not available (compiled without CURL)";
    return result;
}

BackendInfo HTTPVLMBackend::info() const {
    BackendInfo info;
    info.name = "HTTPVLM";
    info.backend_type = "stub";
    return info;
}

void HTTPVLMBackend::shutdown() {}

#endif  // NO_HTTP_BACKENDS

}  // namespace benchmark
}  // namespace rivision
