#pragma once

#include "rivision/types.h"
#include <string>
#include <vector>
#include <memory>

struct sqlite3;

namespace rivision::storage {

// =============================================================================
// EventCache - SQLite-backed offline event cache
// =============================================================================

class EventCache {
public:
    EventCache();
    ~EventCache();
    
    // Open cache database
    bool open(const std::string& path);
    
    // Close database
    void close();
    
    // Check if open
    bool isOpen() const { return db_ != nullptr; }
    
    // Cache an event (for offline mode)
    bool cacheEvent(const std::string& type, const std::string& payload);
    
    // Get cached events (oldest first)
    struct CachedEvent {
        int64_t id;
        std::string type;
        std::string payload;
        Timestamp created_at;
    };
    std::vector<CachedEvent> getCachedEvents(int limit = 100);
    
    // Remove cached events after successful send
    bool removeCachedEvents(const std::vector<int64_t>& ids);
    
    // Get cached event count
    int64_t count();
    
    // Clear all cached events
    void clear();
    
private:
    bool createTable();
    
    sqlite3* db_ = nullptr;
    std::string path_;
};

} // namespace rivision::storage
