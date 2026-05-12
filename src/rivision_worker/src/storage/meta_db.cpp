#include "meta_db.h"
#include "utils/logger.h"

#include <sqlite3.h>
#include <sstream>
#include <atomic>

namespace rivision::storage {

MetaDB::MetaDB() = default;

MetaDB::~MetaDB() {
    close();
}

bool MetaDB::open(const std::string& path) {
    path_ = path;
    
    int rc = sqlite3_open(path.c_str(), &db_);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to open database {}: {}", path, sqlite3_errmsg(db_));
        sqlite3_close(db_);
        db_ = nullptr;
        return false;
    }
    
    // Enable WAL mode
    exec("PRAGMA journal_mode=WAL");
    exec("PRAGMA synchronous=NORMAL");
    exec("PRAGMA cache_size=10000");
    
    // Create tables
    if (!createTables()) {
        LOG_ERROR("Failed to create tables");
        close();
        return false;
    }
    
    LOG_INFO("Opened database: {}", path);
    return true;
}

void MetaDB::close() {
    if (db_) {
        sqlite3_close(db_);
        db_ = nullptr;
        LOG_DEBUG("Closed database: {}", path_);
    }
}

bool MetaDB::createTables() {
    const char* schema = R"(
        CREATE TABLE IF NOT EXISTS detections (
            id TEXT PRIMARY KEY,
            camera_id TEXT NOT NULL,
            stream_id TEXT NOT NULL,
            timestamp INTEGER NOT NULL,
            bbox_x1 REAL, bbox_y1 REAL, bbox_x2 REAL, bbox_y2 REAL,
            class_id INTEGER,
            class_name TEXT,
            confidence REAL,
            track_id INTEGER,
            thumbnail_path TEXT,
            created_at INTEGER
        );
        CREATE INDEX IF NOT EXISTS idx_det_camera_ts ON detections(camera_id, timestamp);
        CREATE INDEX IF NOT EXISTS idx_det_class ON detections(class_name, timestamp);

        CREATE TABLE IF NOT EXISTS alerts (
            id TEXT PRIMARY KEY,
            rule_id TEXT NOT NULL,
            rule_name TEXT,
            stream_id TEXT NOT NULL,
            camera_id TEXT NOT NULL,
            level TEXT NOT NULL,
            trigger_type TEXT,
            zone_name TEXT,
            direction TEXT,
            timestamp INTEGER NOT NULL,
            thumbnail_path TEXT,
            recording_path TEXT,
            vlm_verified INTEGER,
            vlm_confidence REAL,
            vlm_description TEXT,
            vlm_timeout INTEGER DEFAULT 0,
            acknowledged INTEGER DEFAULT 0,
            acknowledged_at INTEGER,
            acknowledged_by TEXT,
            created_at INTEGER
        );
        CREATE INDEX IF NOT EXISTS idx_alert_camera_ts ON alerts(camera_id, timestamp);
        CREATE INDEX IF NOT EXISTS idx_alert_level ON alerts(level, timestamp);

        CREATE TABLE IF NOT EXISTS analytics_hourly (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            stream_id TEXT NOT NULL,
            hour_ts INTEGER NOT NULL,
            total_count INTEGER DEFAULT 0,
            peak_count INTEGER DEFAULT 0,
            max_dwell_sec INTEGER DEFAULT 0,
            class_dist TEXT,
            emotion_dist TEXT,
            avg_valence REAL,
            UNIQUE(stream_id, hour_ts)
        );

        CREATE TABLE IF NOT EXISTS recordings (
            id TEXT PRIMARY KEY,
            camera_id TEXT NOT NULL,
            alert_id TEXT,
            start_time INTEGER,
            end_time INTEGER,
            file_path TEXT,
            file_size INTEGER,
            created_at INTEGER
        );

        CREATE TABLE IF NOT EXISTS rules (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            enabled INTEGER DEFAULT 1,
            config TEXT NOT NULL,
            created_at INTEGER,
            updated_at INTEGER
        );
    )";
    
    return exec(schema);
}

bool MetaDB::exec(const std::string& sql) {
    char* err_msg = nullptr;
    int rc = sqlite3_exec(db_, sql.c_str(), nullptr, nullptr, &err_msg);
    
    if (rc != SQLITE_OK) {
        LOG_ERROR("SQL error: {}", err_msg ? err_msg : "unknown");
        sqlite3_free(err_msg);
        return false;
    }
    
    return true;
}

bool MetaDB::insertDetection(const Detection& det, const StreamId& stream_id,
                            const CameraId& camera_id, const std::string& thumbnail_path) {
    if (!db_) return false;
    
    static std::atomic<uint64_t> det_counter{0};
    std::string id = "det-" + std::to_string(nowMs()) + "-" + std::to_string(det_counter++);
    
    const char* sql = R"(
        INSERT INTO detections (id, camera_id, stream_id, timestamp, 
            bbox_x1, bbox_y1, bbox_x2, bbox_y2, class_id, class_name, 
            confidence, thumbnail_path, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    )";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return false;
    
    int64_t now = nowMs();
    
    sqlite3_bind_text(stmt, 1, id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, camera_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, stream_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 4, now);
    sqlite3_bind_double(stmt, 5, det.bbox.x1);
    sqlite3_bind_double(stmt, 6, det.bbox.y1);
    sqlite3_bind_double(stmt, 7, det.bbox.x2);
    sqlite3_bind_double(stmt, 8, det.bbox.y2);
    sqlite3_bind_int(stmt, 9, det.class_id);
    sqlite3_bind_text(stmt, 10, det.class_name.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_double(stmt, 11, det.confidence);
    sqlite3_bind_text(stmt, 12, thumbnail_path.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 13, now);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    return rc == SQLITE_DONE;
}

bool MetaDB::insertAlert(const AlertEvent& alert) {
    if (!db_) return false;
    
    const char* sql = R"(
        INSERT INTO alerts (id, rule_id, rule_name, stream_id, camera_id, level,
            trigger_type, zone_name, direction, timestamp, recording_path,
            vlm_verified, vlm_confidence, vlm_description, vlm_timeout, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    )";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return false;
    
    sqlite3_bind_text(stmt, 1, alert.id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, alert.rule_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, alert.rule_name.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 4, alert.stream_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 5, alert.camera_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 6, alertLevelToString(alert.level), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 7, alert.trigger_type.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 8, alert.zone_name.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 9, alert.direction.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 10, alert.timestamp);
    sqlite3_bind_text(stmt, 11, alert.record.path.c_str(), -1, SQLITE_TRANSIENT);
    
    if (alert.vlm_result) {
        sqlite3_bind_int(stmt, 12, alert.vlm_result->verified ? 1 : 0);
        sqlite3_bind_double(stmt, 13, alert.vlm_result->confidence);
        sqlite3_bind_text(stmt, 14, alert.vlm_result->description.c_str(), -1, SQLITE_TRANSIENT);
        sqlite3_bind_int(stmt, 15, alert.vlm_result->timeout ? 1 : 0);
    } else {
        sqlite3_bind_null(stmt, 12);
        sqlite3_bind_null(stmt, 13);
        sqlite3_bind_null(stmt, 14);
        sqlite3_bind_int(stmt, 15, 0);
    }
    
    sqlite3_bind_int64(stmt, 16, nowMs());
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    return rc == SQLITE_DONE;
}

MetaDB::AlertStats MetaDB::getAlertStats() {
    AlertStats stats;
    if (!db_) return stats;
    
    int64_t now = nowMs();
    int64_t hour_ago = now - 3600000;
    int64_t day_ago = now - 86400000;
    
    // Total and acknowledged
    {
        const char* sql = "SELECT COUNT(*), SUM(acknowledged) FROM alerts";
        sqlite3_stmt* stmt;
        if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) == SQLITE_OK) {
            if (sqlite3_step(stmt) == SQLITE_ROW) {
                stats.total = sqlite3_column_int(stmt, 0);
                stats.acknowledged = sqlite3_column_int(stmt, 1);
                stats.pending = stats.total - stats.acknowledged;
            }
            sqlite3_finalize(stmt);
        }
    }
    
    // Last hour
    {
        const char* sql = "SELECT COUNT(*) FROM alerts WHERE timestamp > ?";
        sqlite3_stmt* stmt;
        if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) == SQLITE_OK) {
            sqlite3_bind_int64(stmt, 1, hour_ago);
            if (sqlite3_step(stmt) == SQLITE_ROW) {
                stats.last_hour = sqlite3_column_int(stmt, 0);
            }
            sqlite3_finalize(stmt);
        }
    }
    
    // Last 24h
    {
        const char* sql = "SELECT COUNT(*) FROM alerts WHERE timestamp > ?";
        sqlite3_stmt* stmt;
        if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) == SQLITE_OK) {
            sqlite3_bind_int64(stmt, 1, day_ago);
            if (sqlite3_step(stmt) == SQLITE_ROW) {
                stats.last_24h = sqlite3_column_int(stmt, 0);
            }
            sqlite3_finalize(stmt);
        }
    }
    
    // By level
    {
        const char* sql = "SELECT level, COUNT(*) FROM alerts GROUP BY level";
        sqlite3_stmt* stmt;
        if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) == SQLITE_OK) {
            while (sqlite3_step(stmt) == SQLITE_ROW) {
                std::string level = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 0));
                int count = sqlite3_column_int(stmt, 1);
                stats.by_level[level] = count;
            }
            sqlite3_finalize(stmt);
        }
    }
    
    // By type
    {
        const char* sql = "SELECT trigger_type, COUNT(*) FROM alerts GROUP BY trigger_type";
        sqlite3_stmt* stmt;
        if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) == SQLITE_OK) {
            while (sqlite3_step(stmt) == SQLITE_ROW) {
                const char* type = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 0));
                if (type) {
                    int count = sqlite3_column_int(stmt, 1);
                    stats.by_type[type] = count;
                }
            }
            sqlite3_finalize(stmt);
        }
    }
    
    return stats;
}

bool MetaDB::acknowledgeAlert(const AlertId& id, const std::string& acknowledged_by) {
    if (!db_) return false;
    
    const char* sql = "UPDATE alerts SET acknowledged = 1, acknowledged_at = ?, acknowledged_by = ? WHERE id = ?";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return false;
    
    sqlite3_bind_int64(stmt, 1, nowMs());
    sqlite3_bind_text(stmt, 2, acknowledged_by.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, id.c_str(), -1, SQLITE_TRANSIENT);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    return rc == SQLITE_DONE;
}

bool MetaDB::upsertHourlyStats(const HourlyStats& stats) {
    if (!db_) return false;
    
    const char* sql = R"(
        INSERT INTO analytics_hourly (stream_id, hour_ts, total_count, peak_count, 
            max_dwell_sec, avg_valence)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(stream_id, hour_ts) DO UPDATE SET
            total_count = excluded.total_count,
            peak_count = excluded.peak_count,
            max_dwell_sec = excluded.max_dwell_sec,
            avg_valence = excluded.avg_valence
    )";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return false;
    
    sqlite3_bind_text(stmt, 1, stats.stream_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 2, stats.hour_ts);
    sqlite3_bind_int(stmt, 3, stats.total_count);
    sqlite3_bind_int(stmt, 4, stats.peak_count);
    sqlite3_bind_int64(stmt, 5, stats.max_dwell_sec);
    sqlite3_bind_double(stmt, 6, stats.avg_valence);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    return rc == SQLITE_DONE;
}

void MetaDB::cleanupOldData(int retain_days) {
    if (!db_) return;
    
    int64_t cutoff = nowMs() - (retain_days * 86400000LL);
    
    std::string sql = "DELETE FROM detections WHERE timestamp < " + std::to_string(cutoff);
    exec(sql);
    
    sql = "DELETE FROM alerts WHERE timestamp < " + std::to_string(cutoff);
    exec(sql);
    
    sql = "DELETE FROM analytics_hourly WHERE hour_ts < " + std::to_string(cutoff);
    exec(sql);
    
    // Vacuum to reclaim space
    exec("VACUUM");
    
    LOG_INFO("Cleaned up data older than {} days", retain_days);
}

int64_t MetaDB::getDetectionCount() {
    if (!db_) return 0;
    
    const char* sql = "SELECT COUNT(*) FROM detections";
    sqlite3_stmt* stmt;
    int64_t count = 0;
    
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) == SQLITE_OK) {
        if (sqlite3_step(stmt) == SQLITE_ROW) {
            count = sqlite3_column_int64(stmt, 0);
        }
        sqlite3_finalize(stmt);
    }
    
    return count;
}

int64_t MetaDB::getAlertCount() {
    if (!db_) return 0;
    
    const char* sql = "SELECT COUNT(*) FROM alerts";
    sqlite3_stmt* stmt;
    int64_t count = 0;
    
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) == SQLITE_OK) {
        if (sqlite3_step(stmt) == SQLITE_ROW) {
            count = sqlite3_column_int64(stmt, 0);
        }
        sqlite3_finalize(stmt);
    }
    
    return count;
}

} // namespace rivision::storage
