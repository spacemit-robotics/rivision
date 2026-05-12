#pragma once

#include "rivision/types.h"
#include <string>
#include <vector>
#include <sqlite3.h>

namespace rivision::storage {

struct RecordingMeta {
    std::string id;
    StreamId stream_id;
    CameraId camera_id;
    Timestamp start_ts = 0;
    Timestamp end_ts = 0;
    int64_t file_size = 0;
    std::string file_path;
    std::string trigger_type;
    RuleId rule_id;
    AlertId alert_id;
};

class RecordingStore {
public:
    RecordingStore();
    ~RecordingStore();
    
    bool open(const std::string& db_path);
    void close();
    
    bool insert(const RecordingMeta& recording);
    
    std::vector<RecordingMeta> query(
        const StreamId& stream_id = "",
        Timestamp start_ts = 0,
        Timestamp end_ts = 0,
        int limit = 100,
        int offset = 0
    );
    
    RecordingMeta get(const std::string& id);
    
    bool remove(const std::string& id);
    
    int64_t getTotalSize();
    
    int64_t getCount();
    
    std::vector<RecordingMeta> getOldestRecordings(int limit);
    
    std::string getLastError() const { return last_error_; }
    
private:
    bool createTables();
    RecordingMeta rowToMeta(sqlite3_stmt* stmt);
    
    sqlite3* db_ = nullptr;
    std::string last_error_;
};

} // namespace rivision::storage
