#pragma once

#include <cstdint>
#include <string>

namespace rivision::utils {

// =============================================================================
// SystemMonitor - System resource monitoring
// =============================================================================

class SystemMonitor {
public:
    struct Metrics {
        float cpu_percent = 0.0f;
        float mem_percent = 0.0f;
        float disk_percent = 0.0f;
        float load_1 = 0.0f;
        float load_5 = 0.0f;
        float load_15 = 0.0f;
        
        int64_t mem_total_kb = 0;
        int64_t mem_available_kb = 0;
        int64_t disk_total_kb = 0;
        int64_t disk_available_kb = 0;
    };
    
    // Get current system metrics
    static Metrics getMetrics(const std::string& disk_path = "/");
    
    // Get individual metrics
    static float getCpuPercent();
    static float getMemPercent();
    static float getDiskPercent(const std::string& path = "/");
    static float getLoad1();
    
private:
    // CPU calculation state
    static int64_t prev_idle_;
    static int64_t prev_total_;
};

} // namespace rivision::utils
