#pragma once

#include <string>
#include <vector>
#include <atomic>
#include <unordered_map>
#include <mutex>
#include <chrono>

namespace rivision::utils {

class MetricsCollector {
public:
    static MetricsCollector& instance();
    
    void incrementCounter(const std::string& name, int64_t value = 1);
    
    void setGauge(const std::string& name, double value);
    
    void observeHistogram(const std::string& name, double value);
    
    void recordLatency(const std::string& name, int64_t ms);
    
    int64_t getCounter(const std::string& name) const;
    double getGauge(const std::string& name) const;
    
    std::string toPrometheus() const;
    
    void reset();
    
private:
    MetricsCollector() = default;
    
    mutable std::mutex mutex_;
    std::unordered_map<std::string, std::atomic<int64_t>> counters_;
    std::unordered_map<std::string, std::atomic<double>> gauges_;
    
    struct HistogramData {
        std::vector<int64_t> buckets;
        int64_t count = 0;
        double sum = 0.0;
    };
    std::unordered_map<std::string, HistogramData> histograms_;
};

class ScopedTimer {
public:
    ScopedTimer(const std::string& metric_name);
    ~ScopedTimer();
    
private:
    std::string name_;
    std::chrono::steady_clock::time_point start_;
};

#define METRICS MetricsCollector::instance()
#define SCOPED_TIMER(name) ScopedTimer _timer_##__LINE__(name)

} // namespace rivision::utils
