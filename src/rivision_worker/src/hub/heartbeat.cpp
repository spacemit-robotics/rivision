#include "heartbeat.h"
#include "rivision/types.h"
#include "utils/logger.h"

namespace rivision::hub {

HeartbeatManager::HeartbeatManager(int interval_ms)
    : interval_ms_(interval_ms) {}

HeartbeatManager::~HeartbeatManager() {
    stop();
}

void HeartbeatManager::start() {
    if (running_) return;
    
    running_ = true;
    heartbeat_thread_ = std::thread(&HeartbeatManager::heartbeatLoop, this);
    LOG_INFO("HeartbeatManager started with {}ms interval", interval_ms_);
}

void HeartbeatManager::stop() {
    if (!running_) return;
    
    running_ = false;
    if (heartbeat_thread_.joinable()) {
        heartbeat_thread_.join();
    }
    LOG_INFO("HeartbeatManager stopped");
}

void HeartbeatManager::sendNow() {
    if (!data_provider_ || !send_callback_) {
        return;
    }
    
    auto data = data_provider_();
    
    if (send_callback_(data)) {
        last_sent_time_ = nowMs();
        failed_count_ = 0;
    } else {
        failed_count_++;
        LOG_WARN("Heartbeat send failed (count: {})", failed_count_.load());
    }
}

void HeartbeatManager::heartbeatLoop() {
    while (running_) {
        sendNow();
        
        auto wake_time = std::chrono::steady_clock::now() + 
                         std::chrono::milliseconds(interval_ms_);
        
        while (running_ && std::chrono::steady_clock::now() < wake_time) {
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }
    }
}

} // namespace rivision::hub
