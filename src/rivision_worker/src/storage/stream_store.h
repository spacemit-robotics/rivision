#pragma once

#include "rivision/config.h"
#include <string>
#include <vector>
#include <memory>

struct sqlite3;

namespace rivision::storage {

// =============================================================================
// StreamStore - SQLite-backed stream configuration persistence
// =============================================================================
// Persists stream configurations so they survive Worker restarts.
// On startup, StreamManager loads saved configs and auto-restarts streams.

class StreamStore {
public:
    StreamStore();
    ~StreamStore();
    
    // Open database (creates if not exists)
    bool open(const std::string& path);
    
    // Close database
    void close();
    
    // Check if open
    bool isOpen() const { return db_ != nullptr; }
    
    // Save stream config (insert or update)
    bool save(const StreamConfig& config);
    
    // Remove stream config
    bool remove(const std::string& stream_id);
    
    // Load all saved stream configs
    std::vector<StreamConfig> loadAll();
    
    // Load single stream config
    std::optional<StreamConfig> load(const std::string& stream_id);
    
    // Check if stream exists
    bool exists(const std::string& stream_id);
    
    // Clear all saved configs
    void clear();
    
    // Get count of saved streams
    int64_t count();
    
private:
    bool createTable();
    
    // Serialize/deserialize StreamConfig to/from JSON
    std::string serializeConfig(const StreamConfig& config);
    bool deserializeConfig(const std::string& json, StreamConfig& config);
    
    sqlite3* db_ = nullptr;
    std::string path_;
};

} // namespace rivision::storage
