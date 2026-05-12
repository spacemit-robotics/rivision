#pragma once

#include "rivision/types.h"
#include <string>
#include <functional>
#include <thread>
#include <atomic>
#include <chrono>

namespace rivision::hub {

struct HeartbeatData {
    std::string node_id;
    int64_t uptime_s = 0;
    int active_streams = 0;
    int cached_events = 0;
    float cpu_percent = 0.0f;
    float mem_percent = 0.0f;
    float disk_percent = 0.0f;
    float load_1 = 0.0f;
    std::vector<std::string> models;
};

class HeartbeatManager {
public:
    using DataProvider = std::function<HeartbeatData()>;
    using SendCallback = std::function<bool(const HeartbeatData&)>;
    
    HeartbeatManager(int interval_ms = 10000);
    ~HeartbeatManager();
    
    void setDataProvider(DataProvider provider) { data_provider_ = std::move(provider); }
    void setSendCallback(SendCallback callback) { send_callback_ = std::move(callback); }
    
    void start();
    void stop();
    
    void sendNow();
    
    bool isRunning() const { return running_; }
    int64_t getLastSentTime() const { return last_sent_time_; }
    int getFailedCount() const { return failed_count_; }
    
private:
    void heartbeatLoop();
    
    DataProvider data_provider_;
    SendCallback send_callback_;
    
    std::thread heartbeat_thread_;
    std::atomic<bool> running_{false};
    
    int interval_ms_;
    std::atomic<int64_t> last_sent_time_{0};
    std::atomic<int> failed_count_{0};
};

} // namespace rivision::hub
