#pragma once

#include "rivision/config.h"
#include <string>
#include <functional>

namespace rivision::api {

class ConfigHandler {
public:
    using GetConfigCallback = std::function<WorkerConfig()>;
    using ReloadCallback = std::function<bool()>;
    
    ConfigHandler();
    ~ConfigHandler();
    
    void setGetConfigCallback(GetConfigCallback cb) { get_callback_ = std::move(cb); }
    void setReloadCallback(ReloadCallback cb) { reload_callback_ = std::move(cb); }
    
    std::string handleGetConfig();
    
    std::string handleReload();
    
private:
    GetConfigCallback get_callback_;
    ReloadCallback reload_callback_;
};

} // namespace rivision::api
