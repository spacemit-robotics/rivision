#include "stream_store.h"
#include "utils/logger.h"
#include <sqlite3.h>
#include <nlohmann/json.hpp>

using json = nlohmann::json;

namespace rivision::storage {

StreamStore::StreamStore() = default;

StreamStore::~StreamStore() {
    close();
}

bool StreamStore::open(const std::string& path) {
    if (db_) {
        close();
    }
    
    path_ = path;
    
    int rc = sqlite3_open(path.c_str(), &db_);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to open stream store: {}", sqlite3_errmsg(db_));
        sqlite3_close(db_);
        db_ = nullptr;
        return false;
    }
    
    // Enable WAL mode for better concurrency
    sqlite3_exec(db_, "PRAGMA journal_mode=WAL", nullptr, nullptr, nullptr);
    sqlite3_exec(db_, "PRAGMA synchronous=NORMAL", nullptr, nullptr, nullptr);
    
    if (!createTable()) {
        close();
        return false;
    }
    
    LOG_INFO("Stream store opened: {}", path);
    return true;
}

void StreamStore::close() {
    if (db_) {
        sqlite3_close(db_);
        db_ = nullptr;
        LOG_DEBUG("Stream store closed");
    }
}

bool StreamStore::createTable() {
    const char* sql = R"(
        CREATE TABLE IF NOT EXISTS streams (
            id TEXT PRIMARY KEY,
            config_json TEXT NOT NULL,
            created_at INTEGER DEFAULT (strftime('%s', 'now')),
            updated_at INTEGER DEFAULT (strftime('%s', 'now'))
        )
    )";
    
    char* err_msg = nullptr;
    int rc = sqlite3_exec(db_, sql, nullptr, nullptr, &err_msg);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to create streams table: {}", err_msg);
        sqlite3_free(err_msg);
        return false;
    }
    
    return true;
}

std::string StreamStore::serializeConfig(const StreamConfig& config) {
    json j;
    
    j["id"] = config.id;
    j["name"] = config.name;
    j["rtsp_url"] = config.rtsp_url;
    j["camera_id"] = config.camera_id;
    j["stream_mode"] = streamModeToString(config.stream_mode);
    j["owl_channel_id"] = config.owl_channel_id;
    j["rtsp_output_enabled"] = config.rtsp_output_enabled;
    
    // Detection
    j["detection"]["enabled"] = config.detection.enabled;
    j["detection"]["confidence"] = config.detection.confidence;
    j["detection"]["inference_fps"] = config.detection.inference_fps;
    j["detection"]["classes"] = config.detection.classes;
    
    // RTSP Output
    j["rtsp_output"]["fps"] = config.rtsp_output.fps;
    j["rtsp_output"]["bitrate"] = config.rtsp_output.bitrate;
    
    // Tracking
    j["tracking"]["enabled"] = config.tracking.enabled;
    j["tracking"]["iou_threshold"] = config.tracking.iou_threshold;
    j["tracking"]["confirm_threshold"] = config.tracking.confirm_threshold;
    j["tracking"]["max_lost_frames"] = config.tracking.max_lost_frames;
    j["tracking"]["min_hits"] = config.tracking.min_hits;
    
    // Recording
    j["recording"]["enabled"] = config.recording.enabled;
    j["recording"]["output_path"] = config.recording.output_path;
    j["recording"]["bitrate"] = config.recording.bitrate;
    
    // Embedding
    j["embedding"]["enabled"] = config.embedding.enabled;
    j["embedding"]["sample_interval_sec"] = config.embedding.sample_interval_sec;
    
    // Demux
    j["demux"]["prefer_tcp"] = config.demux.prefer_tcp;
    j["demux"]["low_latency"] = config.demux.low_latency;
    j["demux"]["connect_timeout_ms"] = config.demux.connect_timeout_ms;
    j["demux"]["reconnect_ms"] = config.demux.reconnect_ms;
    j["demux"]["max_reconnect"] = config.demux.max_reconnect;
    
    return j.dump();
}

bool StreamStore::deserializeConfig(const std::string& json_str, StreamConfig& config) {
    try {
        json j = json::parse(json_str);
        
        config.id = j.value("id", "");
        config.name = j.value("name", "");
        config.rtsp_url = j.value("rtsp_url", "");
        config.camera_id = j.value("camera_id", "");
        config.stream_mode = streamModeFromString(j.value("stream_mode", "direct"));
        config.owl_channel_id = j.value("owl_channel_id", "");
        config.rtsp_output_enabled = j.value("rtsp_output_enabled", false);
        
        // Detection
        if (j.contains("detection")) {
            auto& det = j["detection"];
            config.detection.enabled = det.value("enabled", true);
            config.detection.confidence = det.value("confidence", 0.5f);
            config.detection.inference_fps = det.value("inference_fps", 5);
            if (det.contains("classes")) {
                config.detection.classes = det["classes"].get<std::vector<int>>();
            }
        }
        
        // RTSP Output
        if (j.contains("rtsp_output")) {
            auto& out = j["rtsp_output"];
            config.rtsp_output.fps = out.value("fps", 15);
            config.rtsp_output.bitrate = out.value("bitrate", 4000000);
        }
        
        // Tracking
        if (j.contains("tracking")) {
            auto& trk = j["tracking"];
            config.tracking.enabled = trk.value("enabled", true);
            config.tracking.iou_threshold = trk.value("iou_threshold", 0.3f);
            config.tracking.confirm_threshold = trk.value("confirm_threshold", 0.5f);
            config.tracking.max_lost_frames = trk.value("max_lost_frames", 30);
            config.tracking.min_hits = trk.value("min_hits", 3);
        }
        
        // Recording
        if (j.contains("recording")) {
            auto& rec = j["recording"];
            config.recording.enabled = rec.value("enabled", false);
            config.recording.output_path = rec.value("output_path", "");
            config.recording.bitrate = rec.value("bitrate", 2000000);
        }
        
        // Embedding
        if (j.contains("embedding")) {
            auto& emb = j["embedding"];
            config.embedding.enabled = emb.value("enabled", false);
            config.embedding.sample_interval_sec = emb.value("sample_interval_sec", 1);
        }
        
        // Demux
        if (j.contains("demux")) {
            auto& dmx = j["demux"];
            config.demux.prefer_tcp = dmx.value("prefer_tcp", true);
            config.demux.low_latency = dmx.value("low_latency", true);
            config.demux.connect_timeout_ms = dmx.value("connect_timeout_ms", 5000);
            config.demux.reconnect_ms = dmx.value("reconnect_ms", 3000);
            config.demux.max_reconnect = dmx.value("max_reconnect", 0);
        }
        
        return true;
    } catch (const std::exception& e) {
        LOG_ERROR("Failed to deserialize stream config: {}", e.what());
        return false;
    }
}

bool StreamStore::save(const StreamConfig& config) {
    if (!db_) return false;
    
    const char* sql = R"(
        INSERT INTO streams (id, config_json, updated_at)
        VALUES (?, ?, strftime('%s', 'now'))
        ON CONFLICT(id) DO UPDATE SET
            config_json = excluded.config_json,
            updated_at = strftime('%s', 'now')
    )";
    
    sqlite3_stmt* stmt = nullptr;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to prepare save statement: {}", sqlite3_errmsg(db_));
        return false;
    }
    
    std::string json_str = serializeConfig(config);
    
    sqlite3_bind_text(stmt, 1, config.id.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, json_str.c_str(), -1, SQLITE_TRANSIENT);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    if (rc != SQLITE_DONE) {
        LOG_ERROR("Failed to save stream config: {}", sqlite3_errmsg(db_));
        return false;
    }
    
    LOG_DEBUG("Stream config saved: {}", config.id);
    return true;
}

bool StreamStore::remove(const std::string& stream_id) {
    if (!db_) return false;
    
    const char* sql = "DELETE FROM streams WHERE id = ?";
    
    sqlite3_stmt* stmt = nullptr;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to prepare remove statement: {}", sqlite3_errmsg(db_));
        return false;
    }
    
    sqlite3_bind_text(stmt, 1, stream_id.c_str(), -1, SQLITE_TRANSIENT);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    if (rc != SQLITE_DONE) {
        LOG_ERROR("Failed to remove stream config: {}", sqlite3_errmsg(db_));
        return false;
    }
    
    LOG_DEBUG("Stream config removed: {}", stream_id);
    return true;
}

std::vector<StreamConfig> StreamStore::loadAll() {
    std::vector<StreamConfig> configs;
    
    if (!db_) return configs;
    
    const char* sql = "SELECT config_json FROM streams ORDER BY created_at";
    
    sqlite3_stmt* stmt = nullptr;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to prepare loadAll statement: {}", sqlite3_errmsg(db_));
        return configs;
    }
    
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        const char* json_str = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 0));
        if (json_str) {
            StreamConfig config;
            if (deserializeConfig(json_str, config)) {
                configs.push_back(std::move(config));
            }
        }
    }
    
    sqlite3_finalize(stmt);
    
    LOG_INFO("Loaded {} stream configs from store", configs.size());
    return configs;
}

std::optional<StreamConfig> StreamStore::load(const std::string& stream_id) {
    if (!db_) return std::nullopt;
    
    const char* sql = "SELECT config_json FROM streams WHERE id = ?";
    
    sqlite3_stmt* stmt = nullptr;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to prepare load statement: {}", sqlite3_errmsg(db_));
        return std::nullopt;
    }
    
    sqlite3_bind_text(stmt, 1, stream_id.c_str(), -1, SQLITE_TRANSIENT);
    
    std::optional<StreamConfig> result;
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        const char* json_str = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 0));
        if (json_str) {
            StreamConfig config;
            if (deserializeConfig(json_str, config)) {
                result = std::move(config);
            }
        }
    }
    
    sqlite3_finalize(stmt);
    return result;
}

bool StreamStore::exists(const std::string& stream_id) {
    if (!db_) return false;
    
    const char* sql = "SELECT 1 FROM streams WHERE id = ?";
    
    sqlite3_stmt* stmt = nullptr;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) {
        return false;
    }
    
    sqlite3_bind_text(stmt, 1, stream_id.c_str(), -1, SQLITE_TRANSIENT);
    
    bool found = (sqlite3_step(stmt) == SQLITE_ROW);
    sqlite3_finalize(stmt);
    
    return found;
}

void StreamStore::clear() {
    if (!db_) return;
    
    char* err_msg = nullptr;
    int rc = sqlite3_exec(db_, "DELETE FROM streams", nullptr, nullptr, &err_msg);
    if (rc != SQLITE_OK) {
        LOG_ERROR("Failed to clear streams: {}", err_msg);
        sqlite3_free(err_msg);
    } else {
        LOG_INFO("Stream store cleared");
    }
}

int64_t StreamStore::count() {
    if (!db_) return 0;
    
    const char* sql = "SELECT COUNT(*) FROM streams";
    
    sqlite3_stmt* stmt = nullptr;
    int rc = sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr);
    if (rc != SQLITE_OK) {
        return 0;
    }
    
    int64_t result = 0;
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        result = sqlite3_column_int64(stmt, 0);
    }
    
    sqlite3_finalize(stmt);
    return result;
}

} // namespace rivision::storage
