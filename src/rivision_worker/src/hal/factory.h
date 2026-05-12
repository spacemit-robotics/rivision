#pragma once

#include "interface/i_demux.h"
#include "interface/i_decoder.h"
#include "interface/i_encoder.h"
#include "interface/i_graphics.h"
#include "interface/i_mux.h"
#include "interface/i_inference.h"

#include <memory>
#include <string>

namespace rivision::hal {

// =============================================================================
// Platform detection (K3-only build)
// =============================================================================

enum class Platform {
    UNKNOWN,
    SPACEMIT_K3     // RISC-V, SpacemiT MPP + NPU
};

Platform detectPlatform();
const char* platformToString(Platform p);

// =============================================================================
// Factory functions - Create platform-specific implementations
// =============================================================================

std::unique_ptr<IDemux> createDemux();
std::unique_ptr<IDecoder> createDecoder();
std::unique_ptr<IEncoder> createEncoder();
std::unique_ptr<IGraphics> createGraphics();
std::unique_ptr<IMux> createMux();
std::unique_ptr<IInference> createInference(const std::string& device);
std::unique_ptr<IEmbedder> createEmbedder(const std::string& device);

// =============================================================================
// ComponentFactory - Factory with configuration
// =============================================================================

class ComponentFactory {
public:
    static ComponentFactory& instance();
    
    // Get detected platform
    Platform platform() const { return platform_; }
    
    // Create components
    std::unique_ptr<IDemux> createDemux();
    std::unique_ptr<IDecoder> createDecoder();
    std::unique_ptr<IEncoder> createEncoder();
    std::unique_ptr<IGraphics> createGraphics();
    std::unique_ptr<IMux> createMux();
    std::unique_ptr<IInference> createInference(const std::string& device);
    std::unique_ptr<IEmbedder> createEmbedder(const std::string& device);
    
private:
    ComponentFactory();
    Platform platform_ = Platform::UNKNOWN;
};

} // namespace rivision::hal
