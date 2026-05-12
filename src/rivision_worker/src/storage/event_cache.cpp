#include "event_cache.h"
#include "utils/logger.h"

#include <sqlite3.h>

namespace rivision::storage {

EventCache::EventCache() = default;

EventCache::~EventCache() {
    close();
}

bool EventCache::open(const std::string& path) {
    path_ = path;
    
    int rc = sqlite3_open(path.c_str(), &db_);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to open event cache: {}", sqlite3_errmsg(db_));
        sqlite3_close(db_);
        db_ = nullptr;
        return false;
    }
    
    // Enable WAL mode
    sqlite3_exec(db_, "PRAGMA journal_mode=WAL", nullptr, nullptr, nullptr);
    sqlite3_exec(db_, "PRAGMA synchronous=NORMAL", nullptr, nullptr, nullptr);
    
    if (!createTable()) {
        LOG_ERROR("Failed to create event cache table");
        close();
        return false;
    }
    
    LOG_INFO("Opened event cache: {}", path);
    return true;
}

void EventCache::close() {
    if (db_) {
        sqlite3_close(db_);
        db_ = nullptr;
    }
}

bool EventCache::createTable() {
    const char* sql = R"(
        CREATE TABLE IF NOT EXISTS events (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            type TEXT NOT NULL,
            payload TEXT NOT NULL,
            created_at INTEGER NOT NULL
        );
        CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
    )";
    
    char* err_msg = nullptr;
    int rc = sqlite3_exec(db_, sql, nullptr, nullptr, &err_msg);
    
    if (rc != SQLITE_OK) {
        LOG_ERROR("SQL error: {}", err_msg ? err_msg : "unknown");
        sqlite3_free(err_msg);
        return false;
    }
    
    return true;
}

bool EventCache::cacheEvent(const std::string& type, const std::string& payload) {
    if (!db_) return false;
    
    const char* sql = "INSERT INTO events (type, payload, created_at) VALUES (?, ?, ?)";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return false;
    
    sqlite3_bind_text(stmt, 1, type.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, payload.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 3, nowMs());
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    return rc == SQLITE_DONE;
}

std::vector<EventCache::CachedEvent> EventCache::getCachedEvents(int limit) {
    std::vector<CachedEvent> events;
    
    if (!db_) return events;
    
    const char* sql = "SELECT id, type, payload, created_at FROM events ORDER BY created_at ASC LIMIT ?";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return events;
    
    sqlite3_bind_int(stmt, 1, limit);
    
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        CachedEvent event;
        event.id = sqlite3_column_int64(stmt, 0);
        event.type = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 1));
        event.payload = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 2));
        event.created_at = sqlite3_column_int64(stmt, 3);
        events.push_back(std::move(event));
    }
    
    sqlite3_finalize(stmt);
    return events;
}

bool EventCache::removeCachedEvents(const std::vector<int64_t>& ids) {
    if (!db_ || ids.empty()) return false;
    
    // Build SQL with placeholders
    std::string sql = "DELETE FROM events WHERE id IN (";
    for (size_t i = 0; i < ids.size(); ++i) {
        if (i > 0) sql += ",";
        sql += "?";
    }
    sql += ")";
    
    sqlite3_stmt* stmt;
    int rc = sqlite3_prepare_v2(db_, sql.c_str(), -1, &stmt, nullptr);
    if (rc != SQLITE_OK) return false;
    
    for (size_t i = 0; i < ids.size(); ++i) {
        sqlite3_bind_int64(stmt, i + 1, ids[i]);
    }
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    return rc == SQLITE_DONE;
}

int64_t EventCache::count() {
    if (!db_) return 0;
    
    const char* sql = "SELECT COUNT(*) FROM events";
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

void EventCache::clear() {
    if (!db_) return;
    
    sqlite3_exec(db_, "DELETE FROM events", nullptr, nullptr, nullptr);
    sqlite3_exec(db_, "VACUUM", nullptr, nullptr, nullptr);
    
    LOG_DEBUG("Cleared event cache");
}

} // namespace rivision::storage
