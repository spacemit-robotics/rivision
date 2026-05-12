#pragma once

#include "rivision/types.h"
#include <string>
#include <functional>
#include <vector>

namespace rivision::api {

class AnalyticsHandler {
public:
    using GetStatsCallback = std::function<std::vector<HourlyStats>(const StreamId&, Timestamp, Timestamp)>;
    using GetHeatmapCallback = std::function<Heatmap(const StreamId&)>;
    
    AnalyticsHandler();
    ~AnalyticsHandler();
    
    void setStatsCallback(GetStatsCallback cb) { stats_callback_ = std::move(cb); }
    void setHeatmapCallback(GetHeatmapCallback cb) { heatmap_callback_ = std::move(cb); }
    
    std::string handleGetStats(const std::string& stream_id, 
                               const std::string& start, 
                               const std::string& end);
    
    std::string handleGetHeatmap(const std::string& stream_id);
    
    std::string handleGetSummary(const std::string& stream_id);
    
private:
    std::string statsToJson(const std::vector<HourlyStats>& stats);
    std::string heatmapToJson(const Heatmap& heatmap);
    
    GetStatsCallback stats_callback_;
    GetHeatmapCallback heatmap_callback_;
};

} // namespace rivision::api
