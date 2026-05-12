#include "vlm_server.h"
#include "llm_engine.h"
#include "vision_encoder.h"
#include "utils/logger.h"
#include <chrono>

namespace rivision::vlm {

class VlmServer::HttpServer {
public:
    // Placeholder for HTTP server implementation
};

VlmServer::VlmServer() = default;
VlmServer::~VlmServer() { stop(); }

bool VlmServer::init(const VlmConfig& config) {
    config_ = config;
    
    // Initialize vision encoder
    vision_encoder_ = std::make_unique<VisionEncoder>();
    if (!vision_encoder_->init(config.model.path + "/vision_encoder.onnx")) {
        LOG_ERROR("Failed to init vision encoder");
        return false;
    }
    
    // Initialize LLM engine
    llm_engine_ = std::make_unique<LlmEngine>();
    LlmEngine::Config llm_cfg;
    llm_cfg.model_path = config.model.path;
    llm_cfg.context_length = config.model.context_length;
    llm_cfg.n_gpu_layers = config.model.n_gpu_layers;
    llm_cfg.threads = config.inference.threads;
    
    if (!llm_engine_->init(llm_cfg)) {
        LOG_ERROR("Failed to init LLM engine");
        return false;
    }
    
    ready_ = true;
    LOG_INFO("VLM Server initialized with model: {}", config.model.path);
    return true;
}

bool VlmServer::start(int port) {
    if (!ready_) {
        LOG_ERROR("VLM Server not initialized");
        return false;
    }
    
    // Start HTTP server
    http_server_ = std::make_unique<HttpServer>();
    running_ = true;
    
    LOG_INFO("VLM Server started on port {}", port);
    return true;
}

void VlmServer::stop() {
    if (!running_) return;
    
    running_ = false;
    http_server_.reset();
    
    LOG_INFO("VLM Server stopped");
}

VlmServer::Response VlmServer::process(const Request& request) {
    Response response;
    
    if (!ready_) {
        response.error = "Server not ready";
        return response;
    }
    
    auto start = std::chrono::steady_clock::now();
    
    try {
        // 1. Encode image to vision features
        std::vector<float> vision_features;
        if (!request.image_data.empty()) {
            vision_features = vision_encoder_->encode(
                request.image_data.data(),
                request.image_width,
                request.image_height
            );
        }
        
        // 2. Generate text with LLM
        LlmEngine::GenerateRequest gen_req;
        gen_req.prompt = request.prompt;
        gen_req.vision_features = vision_features;
        gen_req.max_tokens = request.max_tokens;
        gen_req.temperature = request.temperature;
        
        auto gen_result = llm_engine_->generate(gen_req);
        
        response.success = gen_result.success;
        response.text = gen_result.text;
        response.tokens_generated = gen_result.tokens_generated;
        
        // Simple confidence estimation
        response.confidence = gen_result.success ? 0.85f : 0.0f;
        
    } catch (const std::exception& e) {
        response.error = e.what();
    }
    
    auto end = std::chrono::steady_clock::now();
    response.latency_ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();
    
    return response;
}

bool VlmServer::isReady() const {
    return ready_ && running_;
}

std::string VlmServer::getModelInfo() const {
    return config_.model.type + " @ " + config_.model.path;
}

} // namespace rivision::vlm
