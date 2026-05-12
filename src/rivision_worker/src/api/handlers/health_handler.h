#pragma once

#include "rivision/types.h"
#include <functional>
#include <string>

namespace rivision::api {

class HealthHandler {
public:
    using HealthProvider = std::function<HealthStatus()>;
    
    HealthHandler();
    ~HealthHandler();
    
    void setHealthProvider(HealthProvider provider) { health_provider_ = std::move(provider); }
    
    std::string handleHealth();
    
    std::string handleReady();
    
    std::string handleLive();
    
private:
    HealthProvider health_provider_;
};

} // namespace rivision::api
