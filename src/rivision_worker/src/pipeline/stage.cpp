/**
 * @file stage.cpp
 * @brief Pipeline stage implementation
 */

#include "stage.h"
#include <chrono>
#include <climits>
#include <cstdint>

namespace rivision {
namespace pipeline {

// Stage base class implementation
// Note: Most functionality is in the header as templates
// This file provides any non-template implementations

void StageStats::reset() {
    frames_processed = 0;
    frames_dropped = 0;
    total_latency_us = 0;
    max_latency_us = 0;
    min_latency_us = UINT64_MAX;
    errors = 0;
}

double StageStats::avgLatencyMs() const {
    if (frames_processed == 0) return 0.0;
    return static_cast<double>(total_latency_us) / frames_processed / 1000.0;
}

double StageStats::maxLatencyMs() const {
    return static_cast<double>(max_latency_us) / 1000.0;
}

double StageStats::minLatencyMs() const {
    if (min_latency_us == UINT64_MAX) return 0.0;
    return static_cast<double>(min_latency_us) / 1000.0;
}

void StageStats::recordLatency(uint64_t latency_us) {
    total_latency_us += latency_us;
    if (latency_us > max_latency_us) max_latency_us = latency_us;
    if (latency_us < min_latency_us) min_latency_us = latency_us;
    frames_processed++;
}

} // namespace pipeline
} // namespace rivision
