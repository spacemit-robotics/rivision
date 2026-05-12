#include "analytics_handler.h"
#include <nlohmann/json.hpp>
#include <sstream>

namespace rivision::api {

using json = nlohmann::json;

AnalyticsHandler::AnalyticsHandler() = default;
AnalyticsHandler::~AnalyticsHandler() = default;

std::string AnalyticsHandler::handleGetStats(
    const std::string& stream_id,
    const std::string& start,
    const std::string& end
) {
    if (!stats_callback_) {
        return R"({"error":"stats handler not configured"})";
    }
    
    Timestamp start_ts = start.empty() ? 0 : std::stoll(start);
    Timestamp end_ts = end.empty() ? 0 : std::stoll(end);
    
    auto stats = stats_callback_(stream_id, start_ts, end_ts);
    return statsToJson(stats);
}

std::string AnalyticsHandler::handleGetHeatmap(const std::string& stream_id) {
    if (!heatmap_callback_) {
        return R"({"error":"heatmap handler not configured"})";
    }
    
    auto heatmap = heatmap_callback_(stream_id);
    return heatmapToJson(heatmap);
}

std::string AnalyticsHandler::handleGetSummary(const std::string& stream_id) {
    return R"({"error":"summary not implemented"})";
}

std::string AnalyticsHandler::statsToJson(const std::vector<HourlyStats>& stats) {
    json arr = json::array();
    
    for (const auto& s : stats) {
        json item;
        item["hour_ts"] = s.hour_ts;
        item["stream_id"] = s.stream_id;
        item["total_count"] = s.total_count;
        item["peak_count"] = s.peak_count;
        item["max_dwell_sec"] = s.max_dwell_sec;
        item["avg_valence"] = s.avg_valence;
        item["emotion_count"] = s.emotion_count;
        
        json class_dist;
        for (const auto& [k, v] : s.class_dist) {
            class_dist[k] = v;
        }
        item["class_dist"] = class_dist;
        
        json emotion_dist;
        for (const auto& [k, v] : s.emotion_dist) {
            emotion_dist[k] = v;
        }
        item["emotion_dist"] = emotion_dist;
        
        arr.push_back(item);
    }
    
    json result;
    result["stats"] = arr;
    result["count"] = stats.size();
    
    return result.dump();
}

std::string AnalyticsHandler::heatmapToJson(const Heatmap& heatmap) {
    json result;
    result["start_ts"] = heatmap.start_ts;
    result["end_ts"] = heatmap.end_ts;
    result["rows"] = Heatmap::ROWS;
    result["cols"] = Heatmap::COLS;
    
    json grid = json::array();
    for (const auto& row : heatmap.grid) {
        grid.push_back(row);
    }
    result["grid"] = grid;
    
    return result.dump();
}

} // namespace rivision::api
