#include "metrics.h"
#include <sstream>
#include <iomanip>
#include <cmath>

namespace rivision::utils {

MetricsCollector& MetricsCollector::instance() {
    static MetricsCollector instance;
    return instance;
}

void MetricsCollector::incrementCounter(const std::string& name, int64_t value) {
    std::lock_guard lock(mutex_);
    counters_[name] += value;
}

void MetricsCollector::setGauge(const std::string& name, double value) {
    std::lock_guard lock(mutex_);
    gauges_[name].store(value);
}

void MetricsCollector::observeHistogram(const std::string& name, double value) {
    std::lock_guard lock(mutex_);
    auto& h = histograms_[name];
    h.count++;
    h.sum += value;
    
    static const std::vector<double> bounds = {
        0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0
    };
    
    if (h.buckets.empty()) {
        h.buckets.resize(bounds.size() + 1, 0);
    }
    
    for (size_t i = 0; i < bounds.size(); i++) {
        if (value <= bounds[i]) {
            h.buckets[i]++;
            break;
        }
    }
    h.buckets.back()++;  // +Inf bucket
}

void MetricsCollector::recordLatency(const std::string& name, int64_t ms) {
    observeHistogram(name + "_seconds", static_cast<double>(ms) / 1000.0);
}

int64_t MetricsCollector::getCounter(const std::string& name) const {
    std::lock_guard lock(mutex_);
    auto it = counters_.find(name);
    if (it != counters_.end()) {
        return it->second.load();
    }
    return 0;
}

double MetricsCollector::getGauge(const std::string& name) const {
    std::lock_guard lock(mutex_);
    auto it = gauges_.find(name);
    if (it != gauges_.end()) {
        return it->second.load();
    }
    return 0.0;
}

std::string MetricsCollector::toPrometheus() const {
    std::lock_guard lock(mutex_);
    std::ostringstream oss;
    
    for (const auto& [name, value] : counters_) {
        oss << "# TYPE " << name << " counter\n";
        oss << name << " " << value.load() << "\n";
    }
    
    for (const auto& [name, value] : gauges_) {
        oss << "# TYPE " << name << " gauge\n";
        oss << name << " " << std::fixed << std::setprecision(6) << value.load() << "\n";
    }
    
    static const std::vector<double> bounds = {
        0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0
    };
    
    for (const auto& [name, h] : histograms_) {
        oss << "# TYPE " << name << " histogram\n";
        int64_t cumulative = 0;
        for (size_t i = 0; i < bounds.size() && i < h.buckets.size(); i++) {
            cumulative += h.buckets[i];
            oss << name << "_bucket{le=\"" << bounds[i] << "\"} " << cumulative << "\n";
        }
        oss << name << "_bucket{le=\"+Inf\"} " << h.count << "\n";
        oss << name << "_sum " << h.sum << "\n";
        oss << name << "_count " << h.count << "\n";
    }
    
    return oss.str();
}

void MetricsCollector::reset() {
    std::lock_guard lock(mutex_);
    counters_.clear();
    gauges_.clear();
    histograms_.clear();
}

ScopedTimer::ScopedTimer(const std::string& metric_name)
    : name_(metric_name)
    , start_(std::chrono::steady_clock::now()) {}

ScopedTimer::~ScopedTimer() {
    auto end = std::chrono::steady_clock::now();
    auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start_).count();
    METRICS.recordLatency(name_, ms);
}

} // namespace rivision::utils
