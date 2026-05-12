#pragma once

#include <algorithm>
#include <string>
#include <vector>
#include <map>
#include <cstdint>
#include <optional>
#include <variant>
#include <memory>
#include <functional>
#include <chrono>

namespace rivision {

// =============================================================================
// Type Aliases
// =============================================================================

using StreamId = std::string;
using CameraId = std::string;
using TrackId = uint64_t;
using DetectionId = std::string;
using RuleId = std::string;
using AlertId = std::string;
using FrameId = uint64_t;
using Timestamp = int64_t;  // milliseconds since epoch

// =============================================================================
// BoundingBox
// =============================================================================

struct BBox {
    float x1 = 0.0f;
    float y1 = 0.0f;
    float x2 = 0.0f;
    float y2 = 0.0f;
    
    float width() const { return x2 - x1; }
    float height() const { return y2 - y1; }
    float area() const { return width() * height(); }
    float centerX() const { return (x1 + x2) / 2.0f; }
    float centerY() const { return (y1 + y2) / 2.0f; }
    
    float iou(const BBox& other) const {
        float inter_x1 = std::max(x1, other.x1);
        float inter_y1 = std::max(y1, other.y1);
        float inter_x2 = std::min(x2, other.x2);
        float inter_y2 = std::min(y2, other.y2);
        
        if (inter_x1 >= inter_x2 || inter_y1 >= inter_y2) {
            return 0.0f;
        }
        
        float inter_area = (inter_x2 - inter_x1) * (inter_y2 - inter_y1);
        float union_area = area() + other.area() - inter_area;
        
        return union_area > 0.0f ? inter_area / union_area : 0.0f;
    }
    
    bool contains(float px, float py) const {
        return px >= x1 && px <= x2 && py >= y1 && py <= y2;
    }
    
    BBox scale(float sx, float sy) const {
        return {x1 * sx, y1 * sy, x2 * sx, y2 * sy};
    }
};

// =============================================================================
// Detection
// =============================================================================

struct Detection {
    BBox bbox;
    int class_id = -1;
    std::string class_name;
    float confidence = 0.0f;
    std::optional<std::vector<float>> embedding;
    
    struct Attributes {
        std::optional<std::string> color;
        std::optional<std::string> gender;
        std::optional<int> age_estimate;
    } attrs;
    
    // Helper methods
    bool isValid() const { return class_id >= 0 && confidence > 0.0f; }
};

// =============================================================================
// Track
// =============================================================================

struct Track {
    TrackId id = 0;
    Detection detection;
    int age = 0;
    int time_since_update = 0;
    int hits = 0;
    std::vector<BBox> history;
    
    enum class State { NEW, TRACKED, LOST };
    State state = State::NEW;
    
    enum class CrossDirection { NONE, IN, OUT };
    CrossDirection cross_direction = CrossDirection::NONE;
    
    // Kalman state (for ByteTrack)
    std::vector<float> mean;   // [x, y, a, h, vx, vy, va, vh]
    std::vector<float> covariance;
    
    bool isConfirmed() const { return state == State::TRACKED && hits >= 3; }
    bool isLost() const { return state == State::LOST; }
};

// =============================================================================
// Frame
// =============================================================================

enum class PixelFormat {
    NV12,
    YUV420P,
    RGB24,
    BGR24,
    RGBA,
    BGRA,
    GRAY
};

struct Frame {
    FrameId id = 0;
    Timestamp timestamp = 0;
    int width = 0;
    int height = 0;
    PixelFormat format = PixelFormat::NV12;
    
    // Data storage: CPU memory or DMA handle
    std::variant<
        std::vector<uint8_t>,    // CPU memory
        void*                     // DMA handle (SpacemiT VB)
    > data;
    
    // Stride for each plane
    std::vector<int> strides;
    
    bool isDma() const { return std::holds_alternative<void*>(data); }
    bool isEmpty() const { 
        if (isDma()) return std::get<void*>(data) == nullptr;
        return std::get<std::vector<uint8_t>>(data).empty();
    }
    
    uint8_t* cpuData() {
        if (!isDma()) return std::get<std::vector<uint8_t>>(data).data();
        return nullptr;
    }
    
    const uint8_t* cpuData() const {
        if (!isDma()) return std::get<std::vector<uint8_t>>(data).data();
        return nullptr;
    }
    
    void* dmaHandle() {
        if (isDma()) return std::get<void*>(data);
        return nullptr;
    }
};

// =============================================================================
// StreamState
// =============================================================================

enum class StreamState {
    CREATED,
    CONNECTING,
    RUNNING,
    PAUSED,
    RECONNECTING,
    ERROR,
    STOPPED
};

inline const char* streamStateToString(StreamState state) {
    switch (state) {
        case StreamState::CREATED: return "created";
        case StreamState::CONNECTING: return "connecting";
        case StreamState::RUNNING: return "running";
        case StreamState::PAUSED: return "paused";
        case StreamState::RECONNECTING: return "reconnecting";
        case StreamState::ERROR: return "error";
        case StreamState::STOPPED: return "stopped";
        default: return "unknown";
    }
}

struct StreamStatus {
    StreamState state = StreamState::CREATED;
    std::string error_msg;
    Timestamp started_at = 0;
    int64_t frames_processed = 0;
    int64_t detections_count = 0;
    float fps = 0.0f;
    int reconnect_count = 0;
};

// =============================================================================
// AlertEvent
// =============================================================================

enum class AlertLevel {
    INFO,
    WARNING,
    CRITICAL
};

inline const char* alertLevelToString(AlertLevel level) {
    switch (level) {
        case AlertLevel::INFO: return "info";
        case AlertLevel::WARNING: return "warning";
        case AlertLevel::CRITICAL: return "critical";
        default: return "unknown";
    }
}

inline AlertLevel alertLevelFromString(const std::string& s) {
    if (s == "critical") return AlertLevel::CRITICAL;
    if (s == "warning") return AlertLevel::WARNING;
    return AlertLevel::INFO;
}

struct AlertEvent {
    AlertId id;
    RuleId rule_id;
    std::string rule_name;
    StreamId stream_id;
    CameraId camera_id;
    AlertLevel level = AlertLevel::INFO;
    std::string trigger_type;
    std::string zone_name;
    std::string direction;
    Timestamp timestamp = 0;
    std::string thumbnail_base64;
    
    struct Record {
        bool enabled = false;
        std::string path;
        int pre_sec = 5;
        int post_sec = 10;
    } record;
    
    struct VlmResult {
        bool verified = false;
        float confidence = 0.0f;
        std::string description;
        bool timeout = false;
    };
    std::optional<VlmResult> vlm_result;
};

// =============================================================================
// SearchResult
// =============================================================================

struct SearchResult {
    DetectionId id;
    float score = 0.0f;
    Timestamp timestamp = 0;
    CameraId camera_id;
    std::string thumbnail_path;
    BBox bbox;
    std::string class_name;
    
    struct Metadata {
        std::optional<std::string> scene_description;
        std::optional<std::string> matched_query;
    } meta;
};

// =============================================================================
// Statistics
// =============================================================================

struct HourlyStats {
    Timestamp hour_ts = 0;
    StreamId stream_id;
    int total_count = 0;
    int peak_count = 0;
    int64_t max_dwell_sec = 0;
    std::map<std::string, int> class_dist;
    std::map<std::string, int> emotion_dist;
    float avg_valence = 0.0f;
    int emotion_count = 0;
};

struct Heatmap {
    static constexpr int ROWS = 6;
    static constexpr int COLS = 8;
    
    std::vector<std::vector<int>> grid;
    Timestamp start_ts = 0;
    Timestamp end_ts = 0;
    
    Heatmap() : grid(ROWS, std::vector<int>(COLS, 0)) {}
    
    void recordPosition(float x, float y) {
        int col = static_cast<int>(x * COLS);
        int row = static_cast<int>(y * ROWS);
        col = std::clamp(col, 0, COLS - 1);
        row = std::clamp(row, 0, ROWS - 1);
        grid[row][col]++;
    }
    
    void reset() {
        for (auto& row : grid) {
            std::fill(row.begin(), row.end(), 0);
        }
    }
};

// =============================================================================
// HealthStatus
// =============================================================================

enum class HealthLevel {
    HEALTHY,
    DEGRADED,
    UNHEALTHY
};

inline const char* healthLevelToString(HealthLevel level) {
    switch (level) {
        case HealthLevel::HEALTHY: return "healthy";
        case HealthLevel::DEGRADED: return "degraded";
        case HealthLevel::UNHEALTHY: return "unhealthy";
        default: return "unknown";
    }
}

struct HealthStatus {
    HealthLevel status = HealthLevel::HEALTHY;
    int64_t uptime_s = 0;
    
    struct {
        int active = 0;
        int max = 9;
    } streams;
    
    struct {
        bool connected = false;
        int latency_ms = 0;
    } inference;
    
    struct {
        bool connected = false;
        int latency_ms = 0;
    } embed;
    
    struct {
        int64_t vectors = 0;
        int64_t max = 100000;
    } vector_store;
    
    struct {
        bool connected = false;
        std::string last_heartbeat;
    } hub;
    
    struct {
        float cpu_pct = 0.0f;
        float mem_pct = 0.0f;
        float disk_pct = 0.0f;
        float load_1 = 0.0f;
    } system;
    
    int degradation_level = 0;
    std::string degradation_reason;
};

// =============================================================================
// Result type
// =============================================================================

// Error tag for Result construction
struct ErrorTag {};
inline constexpr ErrorTag make_error{};

template<typename T>
class Result {
public:
    Result(T value) : value_(std::move(value)), has_value_(true) {}
    Result(ErrorTag, std::string error) : error_(std::move(error)), has_value_(false) {}
    
    // Factory methods
    static Result<T> success(T value) { return Result(std::move(value)); }
    static Result<T> failure(std::string error) { return Result(make_error, std::move(error)); }
    
    bool ok() const { return has_value_; }
    bool hasError() const { return !has_value_; }
    
    T& value() { return value_; }
    const T& value() const { return value_; }
    const std::string& error() const { return error_; }
    
    T valueOr(T default_value) const {
        return has_value_ ? value_ : default_value;
    }
    
private:
    T value_{};
    std::string error_;
    bool has_value_;
};

// =============================================================================
// Callbacks
// =============================================================================

using DetectionCallback = std::function<void(
    const StreamId&, 
    const std::vector<Detection>&, 
    const std::vector<Track>&
)>;

using AlertCallback = std::function<void(const AlertEvent&)>;

using FrameCallback = std::function<void(const StreamId&, const Frame&)>;

// =============================================================================
// Time utilities
// =============================================================================

inline Timestamp nowMs() {
    return std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::system_clock::now().time_since_epoch()
    ).count();
}

inline std::string timestampToIso8601(Timestamp ts) {
    auto tp = std::chrono::system_clock::time_point(
        std::chrono::milliseconds(ts)
    );
    auto time_t = std::chrono::system_clock::to_time_t(tp);
    char buf[32];
    std::strftime(buf, sizeof(buf), "%Y-%m-%dT%H:%M:%SZ", std::gmtime(&time_t));
    return buf;
}

} // namespace rivision
