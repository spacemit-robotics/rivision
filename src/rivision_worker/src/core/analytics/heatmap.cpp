#include "heatmap.h"
#include <algorithm>
#include <sstream>
#include <mutex>

namespace rivision::core {

HeatmapManager::HeatmapManager(int rows, int cols)
    : rows_(rows), cols_(cols) {}

HeatmapManager::~HeatmapManager() = default;

void HeatmapManager::ensureHeatmap(const StreamId& stream_id) {
    auto it = heatmaps_.find(stream_id);
    if (it == heatmaps_.end()) {
        HeatmapData data;
        data.stream_id = stream_id;
        data.start_ts = nowMs();
        data.grid.resize(rows_, std::vector<int>(cols_, 0));
        heatmaps_[stream_id] = std::move(data);
    }
}

void HeatmapManager::recordPosition(const StreamId& stream_id, float x, float y) {
    std::unique_lock lock(mutex_);
    
    ensureHeatmap(stream_id);
    auto& data = heatmaps_[stream_id];
    
    int col = static_cast<int>(x * cols_);
    int row = static_cast<int>(y * rows_);
    col = std::clamp(col, 0, cols_ - 1);
    row = std::clamp(row, 0, rows_ - 1);
    
    data.grid[row][col]++;
    data.total_points++;
    data.end_ts = nowMs();
}

void HeatmapManager::recordDetection(const StreamId& stream_id, const Detection& det) {
    float cx = det.bbox.centerX();
    float cy = det.bbox.centerY();
    recordPosition(stream_id, cx, cy);
}

HeatmapManager::HeatmapData HeatmapManager::getHeatmap(const StreamId& stream_id) const {
    std::shared_lock lock(mutex_);
    
    auto it = heatmaps_.find(stream_id);
    if (it != heatmaps_.end()) {
        return it->second;
    }
    
    HeatmapData empty;
    empty.stream_id = stream_id;
    empty.grid.resize(rows_, std::vector<int>(cols_, 0));
    return empty;
}

std::vector<HeatmapManager::HeatmapData> HeatmapManager::getAllHeatmaps() const {
    std::shared_lock lock(mutex_);
    
    std::vector<HeatmapData> result;
    result.reserve(heatmaps_.size());
    for (const auto& [id, data] : heatmaps_) {
        result.push_back(data);
    }
    return result;
}

void HeatmapManager::reset(const StreamId& stream_id) {
    std::unique_lock lock(mutex_);
    
    auto it = heatmaps_.find(stream_id);
    if (it != heatmaps_.end()) {
        for (auto& row : it->second.grid) {
            std::fill(row.begin(), row.end(), 0);
        }
        it->second.total_points = 0;
        it->second.start_ts = nowMs();
        it->second.end_ts = 0;
    }
}

void HeatmapManager::resetAll() {
    std::unique_lock lock(mutex_);
    
    for (auto& [id, data] : heatmaps_) {
        for (auto& row : data.grid) {
            std::fill(row.begin(), row.end(), 0);
        }
        data.total_points = 0;
        data.start_ts = nowMs();
        data.end_ts = 0;
    }
}

std::vector<std::vector<float>> HeatmapManager::getNormalizedGrid(const StreamId& stream_id) const {
    std::shared_lock lock(mutex_);
    
    std::vector<std::vector<float>> normalized(rows_, std::vector<float>(cols_, 0.0f));
    
    auto it = heatmaps_.find(stream_id);
    if (it == heatmaps_.end()) {
        return normalized;
    }
    
    const auto& data = it->second;
    
    int max_val = 0;
    for (const auto& row : data.grid) {
        for (int val : row) {
            max_val = std::max(max_val, val);
        }
    }
    
    if (max_val > 0) {
        for (int r = 0; r < rows_; r++) {
            for (int c = 0; c < cols_; c++) {
                normalized[r][c] = static_cast<float>(data.grid[r][c]) / static_cast<float>(max_val);
            }
        }
    }
    
    return normalized;
}

std::string HeatmapManager::toJson(const StreamId& stream_id) const {
    auto data = getHeatmap(stream_id);
    
    std::ostringstream oss;
    oss << "{";
    oss << "\"stream_id\":\"" << stream_id << "\",";
    oss << "\"start_ts\":" << data.start_ts << ",";
    oss << "\"end_ts\":" << data.end_ts << ",";
    oss << "\"total_points\":" << data.total_points << ",";
    oss << "\"rows\":" << rows_ << ",";
    oss << "\"cols\":" << cols_ << ",";
    oss << "\"grid\":[";
    
    for (int r = 0; r < rows_; r++) {
        if (r > 0) oss << ",";
        oss << "[";
        for (int c = 0; c < cols_; c++) {
            if (c > 0) oss << ",";
            oss << data.grid[r][c];
        }
        oss << "]";
    }
    oss << "]}";
    
    return oss.str();
}

} // namespace rivision::core
