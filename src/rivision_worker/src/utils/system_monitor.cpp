#include "system_monitor.h"

#include <fstream>
#include <sstream>
#include <vector>
#include <sys/statvfs.h>

namespace rivision::utils {

int64_t SystemMonitor::prev_idle_ = 0;
int64_t SystemMonitor::prev_total_ = 0;

SystemMonitor::Metrics SystemMonitor::getMetrics(const std::string& disk_path) {
    Metrics m;
    
    m.cpu_percent = getCpuPercent();
    m.mem_percent = getMemPercent();
    m.disk_percent = getDiskPercent(disk_path);
    m.load_1 = getLoad1();
    
    // Get detailed memory info
    std::ifstream meminfo("/proc/meminfo");
    if (meminfo.is_open()) {
        std::string line;
        while (std::getline(meminfo, line)) {
            std::istringstream iss(line);
            std::string key;
            int64_t value;
            iss >> key >> value;
            
            if (key == "MemTotal:") {
                m.mem_total_kb = value;
            } else if (key == "MemAvailable:") {
                m.mem_available_kb = value;
            }
        }
    }
    
    // Get disk info
    struct statvfs stat;
    if (statvfs(disk_path.c_str(), &stat) == 0) {
        m.disk_total_kb = (stat.f_blocks * stat.f_frsize) / 1024;
        m.disk_available_kb = (stat.f_bavail * stat.f_frsize) / 1024;
    }
    
    // Get load averages
    std::ifstream loadavg("/proc/loadavg");
    if (loadavg.is_open()) {
        loadavg >> m.load_1 >> m.load_5 >> m.load_15;
    }
    
    return m;
}

float SystemMonitor::getCpuPercent() {
    std::ifstream stat("/proc/stat");
    if (!stat.is_open()) {
        return 0.0f;
    }
    
    std::string cpu;
    int64_t user, nice, system, idle, iowait, irq, softirq, steal;
    
    stat >> cpu >> user >> nice >> system >> idle >> iowait >> irq >> softirq >> steal;
    
    int64_t idle_time = idle + iowait;
    int64_t total_time = user + nice + system + idle + iowait + irq + softirq + steal;
    
    int64_t delta_idle = idle_time - prev_idle_;
    int64_t delta_total = total_time - prev_total_;
    
    prev_idle_ = idle_time;
    prev_total_ = total_time;
    
    if (delta_total == 0) {
        return 0.0f;
    }
    
    float cpu_percent = 100.0f * (1.0f - static_cast<float>(delta_idle) / delta_total);
    return std::max(0.0f, std::min(100.0f, cpu_percent));
}

float SystemMonitor::getMemPercent() {
    std::ifstream meminfo("/proc/meminfo");
    if (!meminfo.is_open()) {
        return 0.0f;
    }
    
    int64_t mem_total = 0;
    int64_t mem_available = 0;
    
    std::string line;
    while (std::getline(meminfo, line)) {
        std::istringstream iss(line);
        std::string key;
        int64_t value;
        iss >> key >> value;
        
        if (key == "MemTotal:") {
            mem_total = value;
        } else if (key == "MemAvailable:") {
            mem_available = value;
        }
    }
    
    if (mem_total == 0) {
        return 0.0f;
    }
    
    return 100.0f * (1.0f - static_cast<float>(mem_available) / mem_total);
}

float SystemMonitor::getDiskPercent(const std::string& path) {
    struct statvfs stat;
    
    if (statvfs(path.c_str(), &stat) != 0) {
        return 0.0f;
    }
    
    uint64_t total = stat.f_blocks * stat.f_frsize;
    uint64_t available = stat.f_bavail * stat.f_frsize;
    
    if (total == 0) {
        return 0.0f;
    }
    
    return 100.0f * (1.0f - static_cast<float>(available) / total);
}

float SystemMonitor::getLoad1() {
    std::ifstream loadavg("/proc/loadavg");
    if (!loadavg.is_open()) {
        return 0.0f;
    }
    
    float load1;
    loadavg >> load1;
    
    return load1;
}

} // namespace rivision::utils
