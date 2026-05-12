#pragma once

#include "rivision/types.h"
#include <vector>
#include <shared_mutex>
#include <string>

namespace rivision::core {

class HeatmapManager {
public:
    static constexpr int DEFAULT_ROWS = 6;
    static constexpr int DEFAULT_COLS = 8;
    
    struct HeatmapData {
        StreamId stream_id;
        Timestamp start_ts = 0;
        Timestamp end_ts = 0;
        std::vector<std::vector<int>> grid;
        int total_points = 0;
    };
    
    HeatmapManager(int rows = DEFAULT_ROWS, int cols = DEFAULT_COLS);
    ~HeatmapManager();
    
    void recordPosition(const StreamId& stream_id, float x, float y);
    
    void recordDetection(const StreamId& stream_id, const Detection& det);
    
    HeatmapData getHeatmap(const StreamId& stream_id) const;
    
    std::vector<HeatmapData> getAllHeatmaps() const;
    
    void reset(const StreamId& stream_id);
    
    void resetAll();
    
    std::vector<std::vector<float>> getNormalizedGrid(const StreamId& stream_id) const;
    
    std::string toJson(const StreamId& stream_id) const;
    
private:
    int rows_;
    int cols_;
    std::unordered_map<StreamId, HeatmapData> heatmaps_;
    mutable std::shared_mutex mutex_;
    
    void ensureHeatmap(const StreamId& stream_id);
};

} // namespace rivision::core
