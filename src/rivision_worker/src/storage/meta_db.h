#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <string>
#include <vector>
#include <memory>
#include <optional>

struct sqlite3;

namespace rivision::storage {

// =============================================================================
// MetaDB - SQLite metadata database
// =============================================================================

class MetaDB {
public:
    MetaDB();
    ~MetaDB();
    
    // Open database
    bool open(const std::string& path);
    
    // Close database
    void close();
    
    // Check if open
    bool isOpen() const { return db_ != nullptr; }
    
    // ========== Detections ==========
    
    bool insertDetection(const Detection& det, const StreamId& stream_id,
                        const CameraId& camera_id, const std::string& thumbnail_path);
    
    std::vector<Detection> queryDetections(
        const CameraId& camera_id,
        Timestamp start_ts,
        Timestamp end_ts,
        int limit = 100
    );
    
    // ========== Alerts ==========
    
    bool insertAlert(const AlertEvent& alert);
    
    std::optional<AlertEvent> getAlert(const AlertId& id);
    
    std::vector<AlertEvent> queryAlerts(
        const CameraId& camera_id,
        Timestamp start_ts,
        Timestamp end_ts,
        const std::string& level = "",
        int limit = 100
    );
    
    bool acknowledgeAlert(const AlertId& id, const std::string& acknowledged_by);
    
    // Alert statistics
    struct AlertStats {
        int total = 0;
        int pending = 0;
        int acknowledged = 0;
        int last_24h = 0;
        int last_hour = 0;
        std::map<std::string, int> by_level;
        std::map<std::string, int> by_type;
    };
    AlertStats getAlertStats();
    
    // ========== Analytics ==========
    
    bool upsertHourlyStats(const HourlyStats& stats);
    
    std::optional<HourlyStats> getHourlyStats(const StreamId& stream_id, Timestamp hour_ts);
    
    std::vector<HourlyStats> queryHourlyStats(
        const StreamId& stream_id,
        Timestamp start_ts,
        Timestamp end_ts
    );
    
    // ========== Recordings ==========
    
    bool insertRecording(const std::string& id, const CameraId& camera_id,
                        const AlertId& alert_id, Timestamp start, Timestamp end,
                        const std::string& path, int64_t size);
    
    std::vector<std::string> queryRecordings(
        const CameraId& camera_id,
        Timestamp start_ts,
        Timestamp end_ts
    );
    
    // ========== Rules ==========
    
    bool insertRule(const RuleConfig& rule);
    bool updateRule(const RuleConfig& rule);
    bool deleteRule(const RuleId& id);
    std::vector<RuleConfig> getAllRules();
    
    // ========== Cleanup ==========
    
    void cleanupOldData(int retain_days);
    
    int64_t getDetectionCount();
    int64_t getAlertCount();
    
private:
    bool createTables();
    bool exec(const std::string& sql);
    
    sqlite3* db_ = nullptr;
    std::string path_;
};

} // namespace rivision::storage
