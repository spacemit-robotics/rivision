#include "collector.h"
#include "storage/meta_db.h"
#include "utils/logger.h"

#include <algorithm>

namespace rivision::core {

AnalyticsCollector::AnalyticsCollector(storage::MetaDB* meta_db)
    : meta_db_(meta_db) {
}

AnalyticsCollector::~AnalyticsCollector() {
    flushToDB();
}

void AnalyticsCollector::recordDetection(const StreamId& stream_id, const Detection& det) {
    auto& stats = getStats(stream_id);
    std::lock_guard<std::mutex> lock(stats.mutex);
    
    stats.total_count++;
    
    // Update class distribution
    if (!det.class_name.empty()) {
        stats.class_dist[det.class_name]++;
    }
    
    // Update heatmap
    float cx = det.bbox.centerX();
    float cy = det.bbox.centerY();
    stats.heatmap.recordPosition(cx, cy);
}

void AnalyticsCollector::recordPresence(const StreamId& stream_id, TrackId track_id) {
    auto& stats = getStats(stream_id);
    std::lock_guard<std::mutex> lock(stats.mutex);
    
    stats.current_count++;
    
    // Update peak
    int current = stats.current_count.load();
    int peak = stats.peak_count.load();
    while (current > peak && !stats.peak_count.compare_exchange_weak(peak, current)) {
        // Retry
    }
}

void AnalyticsCollector::recordDwell(const StreamId& stream_id, TrackId track_id, int dwell_sec) {
    auto& stats = getStats(stream_id);
    
    int64_t current_max = stats.max_dwell_sec.load();
    while (dwell_sec > current_max && 
           !stats.max_dwell_sec.compare_exchange_weak(current_max, dwell_sec)) {
        // Retry
    }
}

void AnalyticsCollector::recordEmotion(const StreamId& stream_id, 
                                       const std::string& emotion, 
                                       float valence) {
    auto& stats = getStats(stream_id);
    std::lock_guard<std::mutex> lock(stats.mutex);
    
    stats.emotion_dist[emotion]++;
    stats.valence_sum += valence;
    stats.emotion_count++;
}

HourlyStats AnalyticsCollector::getHourlyStats(const StreamId& stream_id, Timestamp hour_ts) {
    auto& stats = getStats(stream_id);
    std::lock_guard<std::mutex> lock(stats.mutex);
    
    HourlyStats result;
    result.stream_id = stream_id;
    result.hour_ts = hour_ts;
    result.total_count = stats.total_count.load();
    result.peak_count = stats.peak_count.load();
    result.max_dwell_sec = stats.max_dwell_sec.load();
    result.class_dist = stats.class_dist;
    result.emotion_dist = stats.emotion_dist;
    
    if (stats.emotion_count > 0) {
        result.avg_valence = stats.valence_sum / stats.emotion_count;
    }
    
    return result;
}

Heatmap AnalyticsCollector::getHeatmap(const StreamId& stream_id, 
                                       Timestamp start_ts, 
                                       Timestamp end_ts) {
    auto& stats = getStats(stream_id);
    std::lock_guard<std::mutex> lock(stats.mutex);
    
    Heatmap result = stats.heatmap;
    result.start_ts = start_ts;
    result.end_ts = end_ts;
    
    return result;
}

void AnalyticsCollector::flushToDB() {
    if (!meta_db_) return;
    
    std::shared_lock lock(stats_mutex_);
    
    Timestamp hour_ts = getCurrentHour();
    
    for (auto& [stream_id, stats_ptr] : stats_) {
        std::lock_guard<std::mutex> stats_lock(stats_ptr->mutex);
        
        HourlyStats stats;
        stats.stream_id = stream_id;
        stats.hour_ts = hour_ts;
        stats.total_count = stats_ptr->total_count.load();
        stats.peak_count = stats_ptr->peak_count.load();
        stats.max_dwell_sec = stats_ptr->max_dwell_sec.load();
        
        if (stats_ptr->emotion_count > 0) {
            stats.avg_valence = stats_ptr->valence_sum / stats_ptr->emotion_count;
        }
        
        meta_db_->upsertHourlyStats(stats);
    }
    
    LOG_DEBUG("Flushed analytics to database");
}

void AnalyticsCollector::reset(const StreamId& stream_id) {
    std::unique_lock lock(stats_mutex_);
    
    auto it = stats_.find(stream_id);
    if (it != stats_.end()) {
        std::lock_guard<std::mutex> stats_lock(it->second->mutex);
        
        it->second->total_count = 0;
        it->second->current_count = 0;
        it->second->peak_count = 0;
        it->second->max_dwell_sec = 0;
        it->second->class_dist.clear();
        it->second->emotion_dist.clear();
        it->second->valence_sum = 0.0f;
        it->second->emotion_count = 0;
        it->second->heatmap.reset();
    }
}

AnalyticsCollector::StreamStats& AnalyticsCollector::getStats(const StreamId& stream_id) {
    {
        std::shared_lock lock(stats_mutex_);
        auto it = stats_.find(stream_id);
        if (it != stats_.end()) {
            return *it->second;
        }
    }
    
    // Need to create
    std::unique_lock lock(stats_mutex_);
    
    // Double-check
    auto it = stats_.find(stream_id);
    if (it != stats_.end()) {
        return *it->second;
    }
    
    auto stats = std::make_unique<StreamStats>();
    auto& ref = *stats;
    stats_[stream_id] = std::move(stats);
    
    return ref;
}

Timestamp AnalyticsCollector::getCurrentHour() const {
    Timestamp now = nowMs();
    return (now / 3600000) * 3600000;  // Round down to hour
}

} // namespace rivision::core
