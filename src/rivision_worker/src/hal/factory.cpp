#include "factory.h"
#include "utils/logger.h"

#include <fstream>
#include <cstring>

#include "backend/spacemit/mpp_demux.h"
#include "backend/spacemit/mpp_decoder.h"
#include "backend/spacemit/mpp_encoder.h"
#include "backend/spacemit/mpp_graphics.h"
#include "backend/spacemit/mpp_mux.h"
#include "backend/spacemit/npu_inference.h"

namespace rivision::hal {

Platform detectPlatform() {
    // K3-only build: detect SpacemiT K3 RISC-V chip.
    std::ifstream cpuinfo("/proc/cpuinfo");
    if (cpuinfo.is_open()) {
        std::string line;
        while (std::getline(cpuinfo, line)) {
            if (line.find("spacemit") != std::string::npos ||
                line.find("x60") != std::string::npos) {
                return Platform::SPACEMIT_K3;
            }
        }
    }
    return Platform::UNKNOWN;
}

const char* platformToString(Platform p) {
    switch (p) {
        case Platform::SPACEMIT_K3: return "SpacemiT K3 (RISC-V)";
        default: return "Unknown";
    }
}

// =============================================================================
// Global factory functions
// =============================================================================

std::unique_ptr<IDemux> createDemux() {
    return ComponentFactory::instance().createDemux();
}

std::unique_ptr<IDecoder> createDecoder() {
    return ComponentFactory::instance().createDecoder();
}

std::unique_ptr<IEncoder> createEncoder() {
    return ComponentFactory::instance().createEncoder();
}

std::unique_ptr<IGraphics> createGraphics() {
    return ComponentFactory::instance().createGraphics();
}

std::unique_ptr<IMux> createMux() {
    return ComponentFactory::instance().createMux();
}

std::unique_ptr<IInference> createInference(const std::string& device) {
    return ComponentFactory::instance().createInference(device);
}

std::unique_ptr<IEmbedder> createEmbedder(const std::string& device) {
    return ComponentFactory::instance().createEmbedder(device);
}

// =============================================================================
// ComponentFactory
// =============================================================================

ComponentFactory& ComponentFactory::instance() {
    static ComponentFactory instance;
    return instance;
}

ComponentFactory::ComponentFactory() {
    platform_ = detectPlatform();
    LOG_INFO("Detected platform: {}", platformToString(platform_));
}

std::unique_ptr<IDemux> ComponentFactory::createDemux() {
    if (platform_ == Platform::SPACEMIT_K3) {
        return std::make_unique<spacemit::MppDemux>();
    }
    LOG_ERROR("No demux backend available");
    return nullptr;
}

std::unique_ptr<IDecoder> ComponentFactory::createDecoder() {
    if (platform_ == Platform::SPACEMIT_K3) {
        return std::make_unique<spacemit::MppDecoder>();
    }
    LOG_ERROR("No decoder backend available");
    return nullptr;
}

std::unique_ptr<IEncoder> ComponentFactory::createEncoder() {
    if (platform_ == Platform::SPACEMIT_K3) {
        return std::make_unique<spacemit::MppEncoder>();
    }
    LOG_ERROR("No encoder backend available");
    return nullptr;
}

std::unique_ptr<IGraphics> ComponentFactory::createGraphics() {
    if (platform_ == Platform::SPACEMIT_K3) {
        return std::make_unique<spacemit::MppGraphics>();
    }
    LOG_ERROR("No graphics backend available");
    return nullptr;
}

std::unique_ptr<IMux> ComponentFactory::createMux() {
    if (platform_ == Platform::SPACEMIT_K3) {
        return std::make_unique<spacemit::MppMux>();
    }
    LOG_ERROR("No mux backend available");
    return nullptr;
}

std::unique_ptr<IInference> ComponentFactory::createInference(const std::string& device) {
    if (device == "npu" && platform_ == Platform::SPACEMIT_K3) {
        return std::make_unique<spacemit::NpuInference>();
    }
    LOG_ERROR("No inference backend available for device: {}", device);
    return nullptr;
}

std::unique_ptr<IEmbedder> ComponentFactory::createEmbedder(const std::string& device) {
    // Placeholder - would create CLIP embedder
    LOG_WARN("Embedder not implemented yet");
    return nullptr;
}

} // namespace rivision::hal
