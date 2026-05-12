#pragma once

#include "rivision/types.h"
#include <memory>
#include <unordered_map>
#include <shared_mutex>
#include <atomic>

namespace rivision::storage {
class MetaDB;
}

namespace rivision::core {

// =============================================================================
// AnalyticsCollector - Collects and aggregates analytics data
// =============================================================================

class AnalyticsCollector {
public:
    explicit AnalyticsCollector(storage::MetaDB* meta_db);
    ~AnalyticsCollector();
    
    // Record detection
    void recordDetection(const StreamId& stream_id, const Detection& det);
    
    // Record presence (for peak count)
    void recordPresence(const StreamId& stream_id, TrackId track_id);
    
    // Record dwell time
    void recordDwell(const StreamId& stream_id, TrackId track_id, int dwell_sec);
    
    // Record emotion (G1 feature)
    void recordEmotion(const StreamId& stream_id, const std::string& emotion, float valence);
    
    // Get hourly stats
    HourlyStats getHourlyStats(const StreamId& stream_id, Timestamp hour_ts);
    
    // Get heatmap
    Heatmap getHeatmap(const StreamId& stream_id, Timestamp start_ts, Timestamp end_ts);
    
    // Flush to database
    void flushToDB();
    
    // Reset stats for stream
    void reset(const StreamId& stream_id);
    
private:
    storage::MetaDB* meta_db_;
    
    struct StreamStats {
        std::atomic<int64_t> total_count{0};
        std::atomic<int> current_count{0};
        std::atomic<int> peak_count{0};
        std::atomic<int64_t> max_dwell_sec{0};
        
        Heatmap heatmap;
        
        std::map<std::string, int> class_dist;
        std::map<std::string, int> emotion_dist;
        float valence_sum = 0.0f;
        int emotion_count = 0;
        
        std::mutex mutex;
    };
    
    std::unordered_map<StreamId, std::unique_ptr<StreamStats>> stats_;
    mutable std::shared_mutex stats_mutex_;
    
    // Get or create stats for stream
    StreamStats& getStats(const StreamId& stream_id);
    
    // Get current hour timestamp
    Timestamp getCurrentHour() const;
};

} // namespace rivision::core
