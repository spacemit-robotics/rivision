#include "recording_store.h"
#include "utils/logger.h"
#include <filesystem>

namespace rivision::storage {

namespace fs = std::filesystem;

RecordingStore::RecordingStore() = default;

RecordingStore::~RecordingStore() {
    close();
}

bool RecordingStore::open(const std::string& db_path) {
    fs::create_directories(fs::path(db_path).parent_path());
    
    int rc = sqlite3_open(db_path.c_str(), &db_);
    if (rc != SQLITE_OK) {
        last_error_ = sqlite3_errmsg(db_);
        LOG_ERROR("Failed to open recording store: {}", last_error_);
        return false;
    }
    
    if (!createTables()) {
        return false;
    }
    
    LOG_INFO("Recording store opened: {}", db_path);
    return true;
}

void RecordingStore::close() {
    if (db_) {
        sqlite3_close(db_);
        db_ = nullptr;
    }
}

bool RecordingStore::createTables() {
    const char* sql = R"(
        CREATE TABLE IF NOT EXISTS recordings (
            id TEXT PRIMARY KEY,
            stream_id TEXT NOT NULL,
            camera_id TEXT,
            start_ts INTEGER NOT NULL,
            end_ts INTEGER,
            file_size INTEGER DEFAULT 0,
            file_path TEXT,
            trigger_type TEXT,
            rule_id TEXT,
            alert_id TEXT,
            created_at INTEGER DEFAULT (strftime('%s', 'now') * 1000)
        );
        CREATE INDEX IF NOT EXISTS idx_recordings_stream ON recordings(stream_id);
        CREATE INDEX IF NOT EXISTS idx_recordings_start ON recordings(start_ts);
    )";
    
    char* err_msg = nullptr;
    int rc = sqlite3_exec(db_, sql, nullptr, nullptr, &err_msg);
    
    if (rc != SQLITE_OK) {
        last_error_ = err_msg;
        sqlite3_free(err_msg);
        LOG_ERROR("Failed to create tables: {}", last_error_);
        return false;
    }
    
    return true;
}

bool RecordingStore::insert(const RecordingMeta& recording) {
    const char* sql = R"(
        INSERT INTO recordings (id, stream_id, camera_id, start_ts, end_ts, 
                                file_size, file_path, trigger_type, rule_id, alert_id)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    )";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) {
        last_error_ = sqlite3_errmsg(db_);
        return false;
    }
    
    sqlite3_bind_text(stmt, 1, recording.id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, recording.stream_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, recording.camera_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 4, recording.start_ts);
    sqlite3_bind_int64(stmt, 5, recording.end_ts);
    sqlite3_bind_int64(stmt, 6, recording.file_size);
    sqlite3_bind_text(stmt, 7, recording.file_path.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 8, recording.trigger_type.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 9, recording.rule_id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 10, recording.alert_id.c_str(), -1, SQLITE_TRANSIENT);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    if (rc != SQLITE_DONE) {
        last_error_ = sqlite3_errmsg(db_);
        return false;
    }
    
    return true;
}

std::vector<RecordingMeta> RecordingStore::query(
    const StreamId& stream_id,
    Timestamp start_ts,
    Timestamp end_ts,
    int limit,
    int offset
) {
    std::vector<RecordingMeta> results;
    
    std::string sql = "SELECT * FROM recordings WHERE 1=1";
    if (!stream_id.empty()) sql += " AND stream_id = ?";
    if (start_ts > 0) sql += " AND start_ts >= ?";
    if (end_ts > 0) sql += " AND start_ts <= ?";
    sql += " ORDER BY start_ts DESC LIMIT ? OFFSET ?";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql.c_str(), -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return results;
    
    int idx = 1;
    if (!stream_id.empty()) sqlite3_bind_text(stmt, idx++, stream_id.c_str(), -1, SQLITE_TRANSIENT);
    if (start_ts > 0) sqlite3_bind_int64(stmt, idx++, start_ts);
    if (end_ts > 0) sqlite3_bind_int64(stmt, idx++, end_ts);
    sqlite3_bind_int(stmt, idx++, limit);
    sqlite3_bind_int(stmt, idx++, offset);
    
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        results.push_back(rowToMeta(stmt));
    }
    
    sqlite3_finalize(stmt);
    return results;
}

RecordingMeta RecordingStore::get(const std::string& id) {
    RecordingMeta meta;
    
    const char* sql = "SELECT * FROM recordings WHERE id = ?";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return meta;
    
    sqlite3_bind_text(stmt, 1, id.c_str(), -1, SQLITE_TRANSIENT);
    
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        meta = rowToMeta(stmt);
    }
    
    sqlite3_finalize(stmt);
    return meta;
}

bool RecordingStore::remove(const std::string& id) {
    const char* sql = "DELETE FROM recordings WHERE id = ?";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return false;
    
    sqlite3_bind_text(stmt, 1, id.c_str(), -1, SQLITE_TRANSIENT);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    return rc == SQLITE_DONE;
}

int64_t RecordingStore::getTotalSize() {
    const char* sql = "SELECT COALESCE(SUM(file_size), 0) FROM recordings";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return 0;
    
    int64_t total = 0;
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        total = sqlite3_column_int64(stmt, 0);
    }
    
    sqlite3_finalize(stmt);
    return total;
}

int64_t RecordingStore::getCount() {
    const char* sql = "SELECT COUNT(*) FROM recordings";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return 0;
    
    int64_t count = 0;
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        count = sqlite3_column_int64(stmt, 0);
    }
    
    sqlite3_finalize(stmt);
    return count;
}

std::vector<RecordingMeta> RecordingStore::getOldestRecordings(int limit) {
    std::vector<RecordingMeta> results;
    
    const char* sql = "SELECT * FROM recordings ORDER BY start_ts ASC LIMIT ?";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return results;
    
    sqlite3_bind_int(stmt, 1, limit);
    
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        results.push_back(rowToMeta(stmt));
    }
    
    sqlite3_finalize(stmt);
    return results;
}

RecordingMeta RecordingStore::rowToMeta(sqlite3_stmt* stmt) {
    RecordingMeta meta;
    meta.id = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 0));
    meta.stream_id = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 1));
    
    auto col2 = sqlite3_column_text(stmt, 2);
    if (col2) meta.camera_id = reinterpret_cast<const char*>(col2);
    
    meta.start_ts = sqlite3_column_int64(stmt, 3);
    meta.end_ts = sqlite3_column_int64(stmt, 4);
    meta.file_size = sqlite3_column_int64(stmt, 5);
    
    auto col6 = sqlite3_column_text(stmt, 6);
    if (col6) meta.file_path = reinterpret_cast<const char*>(col6);
    
    auto col7 = sqlite3_column_text(stmt, 7);
    if (col7) meta.trigger_type = reinterpret_cast<const char*>(col7);
    
    auto col8 = sqlite3_column_text(stmt, 8);
    if (col8) meta.rule_id = reinterpret_cast<const char*>(col8);
    
    auto col9 = sqlite3_column_text(stmt, 9);
    if (col9) meta.alert_id = reinterpret_cast<const char*>(col9);
    
    return meta;
}

} // namespace rivision::storage
