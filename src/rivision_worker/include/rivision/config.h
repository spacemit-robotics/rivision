#pragma once

#include <string>
#include <vector>
#include <map>
#include <optional>

namespace rivision {

// =============================================================================
// WorkerConfig - Main configuration
// =============================================================================

struct WorkerConfig {
    std::string node_id = "worker-001";
    std::string version = "1.0.0";  // Worker version
    
    // Model names (auto-populated from config files)
    struct Models {
        std::string yolo;   // e.g., "yolo11n-int8"
        std::string vlm;    // e.g., "fastvlm-0.5b"
        std::string embed;  // e.g., "chinese-clip-vit-b-16"
    } models;
    
    struct Server {
        std::string host = "0.0.0.0";
        int http_port = 8080;
        std::string advertise_host;  // IP to advertise to Hub (auto-detect if empty)
        int max_connections = 100;
        int request_timeout_ms = 30000;
    } server;
    
    struct Hub {
        std::string url;  // ws://hub:8080/ws/worker
        std::string token;
        int reconnect_ms = 3000;
        int heartbeat_ms = 10000;
        int max_reconnect_attempts = 0;  // 0 = infinite
    } hub;
    
    struct Inference {
        std::string yolo_config = "config/yolo.yaml";
        std::string vlm_config = "config/vlm.yaml";
        std::string embed_config = "config/embed.yaml";
        std::string device = "npu";  // npu | cpu
    } inference;
    
    struct Vlm {
        bool enabled = false;
        std::string endpoint = "http://127.0.0.1:8090";
        int timeout_ms = 5000;
    } vlm;
    
    struct Embed {
        std::string url = "http://localhost:18081";  // rivision_embed service
    } embed;
    
    struct Search {
        int max_vectors = 100000;
        int embedding_dim = 512;
        std::string persist_path;
    } search;
    
    struct Storage {
        std::string data_dir = "/opt/rivision/rivision_worker/data";
        std::string meta_db;
        std::string event_cache_db;
        std::string recordings_dir;
        std::string thumbnails_dir;
        std::string vectors_dir;
    } storage;
    
    struct Cleanup {
        int detection_retain_days = 7;
        int recording_retain_days = 30;
        int thumbnail_retain_days = 7;
        int disk_high_watermark_pct = 85;
        int disk_critical_watermark_pct = 95;
        int cleanup_interval_sec = 3600;
    } cleanup;
    
    struct Log {
        std::string level = "info";
        std::string file;
        int max_size_mb = 100;
        int max_files = 5;
        bool console = true;
    } log;
    
    // Load from YAML file
    static WorkerConfig load(const std::string& path);
    
    // Initialize storage paths based on data_dir
    void initPaths() {
        if (storage.meta_db.empty())
            storage.meta_db = storage.data_dir + "/meta.db";
        if (storage.event_cache_db.empty())
            storage.event_cache_db = storage.data_dir + "/events.db";
        if (storage.recordings_dir.empty())
            storage.recordings_dir = storage.data_dir + "/recordings";
        if (storage.thumbnails_dir.empty())
            storage.thumbnails_dir = storage.data_dir + "/thumbnails";
        if (storage.vectors_dir.empty())
            storage.vectors_dir = storage.data_dir + "/vectors";
        if (search.persist_path.empty())
            search.persist_path = storage.vectors_dir + "/vectors.bin";
    }
};

// =============================================================================
// YoloConfig - YOLO inference configuration
// =============================================================================

struct YoloConfig {
    struct Model {
        std::string path = "models/yolo/yolo11n.q.onnx";
        int input_width = 640;
        int input_height = 640;
    } model;
    
    struct Detection {
        float confidence_threshold = 0.5f;
        float nms_threshold = 0.45f;
        std::vector<int> classes;  // Empty = all classes
    } detection;
    
    struct Tracker {
        bool enabled = true;
        std::string type = "bytetrack";
        int max_age = 30;
        int min_hits = 3;
        float iou_threshold = 0.3f;
    } tracker;
    
    struct Emotion {
        bool enabled = false;
        int sample_interval_sec = 5;
        int min_face_size = 64;
    } emotion;
    
    std::vector<std::string> class_names;  // COCO 80 classes
    
    static YoloConfig load(const std::string& path);
    
    // Initialize default COCO classes
    void initDefaultClasses() {
        if (class_names.empty()) {
            class_names = {
                "person", "bicycle", "car", "motorcycle", "airplane", "bus", "train",
                "truck", "boat", "traffic light", "fire hydrant", "stop sign",
                "parking meter", "bench", "bird", "cat", "dog", "horse", "sheep",
                "cow", "elephant", "bear", "zebra", "giraffe", "backpack", "umbrella",
                "handbag", "tie", "suitcase", "frisbee", "skis", "snowboard",
                "sports ball", "kite", "baseball bat", "baseball glove", "skateboard",
                "surfboard", "tennis racket", "bottle", "wine glass", "cup", "fork",
                "knife", "spoon", "bowl", "banana", "apple", "sandwich", "orange",
                "broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair",
                "couch", "potted plant", "bed", "dining table", "toilet", "tv",
                "laptop", "mouse", "remote", "keyboard", "cell phone", "microwave",
                "oven", "toaster", "sink", "refrigerator", "book", "clock", "vase",
                "scissors", "teddy bear", "hair drier", "toothbrush"
            };
        }
    }
};

// =============================================================================
// StreamConfig - Stream configuration
// =============================================================================

enum class StreamMode {
    DIRECT,
    HUB_SIP_REMOTE,
    WORKER_OWL
};

inline const char* streamModeToString(StreamMode mode) {
    switch (mode) {
        case StreamMode::DIRECT: return "direct";
        case StreamMode::HUB_SIP_REMOTE: return "hub_sip_remote";
        case StreamMode::WORKER_OWL: return "worker_owl";
        default: return "direct";
    }
}

inline StreamMode streamModeFromString(const std::string& s) {
    if (s == "hub_sip_remote") return StreamMode::HUB_SIP_REMOTE;
    if (s == "worker_owl") return StreamMode::WORKER_OWL;
    return StreamMode::DIRECT;
}

struct StreamConfig {
    std::string id;
    std::string name;
    std::string rtsp_url;
    std::string camera_id;
    StreamMode stream_mode = StreamMode::DIRECT;
    std::string owl_channel_id;  // for WORKER_OWL mode
    
    struct Detection {
        bool enabled = true;
        float confidence = 0.5f;
        int inference_fps = 5;
        std::vector<int> classes;  // Empty = all
    } detection;
    
    struct RtspOutput {
        int fps = 15;              // Output frame rate (default 15fps)
        int bitrate = 4000000;     // Output bitrate (default 4Mbps)
    } rtsp_output;
    
    struct Tracking {
        bool enabled = true;
        float iou_threshold = 0.3f;
        float confirm_threshold = 0.5f;
        int max_lost_frames = 30;
        int min_hits = 3;
    } tracking;
    
    struct Recording {
        bool enabled = false;
        std::string output_path;
        int bitrate = 2000000;
    } recording;
    
    struct Embedding {
        bool enabled = false;
        int sample_interval_sec = 1;
    } embedding;
    
    struct Demux {
        bool prefer_tcp = true;
        bool low_latency = true;
        int connect_timeout_ms = 5000;
        int reconnect_ms = 3000;
        int max_reconnect = 0;  // 0 = infinite
    } demux;
    
    // RTSP output for annotated stream
    bool rtsp_output_enabled = false;
};

// =============================================================================
// RuleConfig - Rule configuration
// =============================================================================

struct ZoneConfig {
    std::string name;
    std::vector<std::pair<float, float>> polygon;  // Normalized coordinates
    
    bool contains(float x, float y) const {
        // Ray casting algorithm
        int n = polygon.size();
        if (n < 3) return false;
        
        bool inside = false;
        for (int i = 0, j = n - 1; i < n; j = i++) {
            if (((polygon[i].second > y) != (polygon[j].second > y)) &&
                (x < (polygon[j].first - polygon[i].first) * (y - polygon[i].second) / 
                     (polygon[j].second - polygon[i].second) + polygon[i].first)) {
                inside = !inside;
            }
        }
        return inside;
    }
};

struct LineConfig {
    std::pair<float, float> start;
    std::pair<float, float> end;
    std::string direction = "both";  // "in" | "out" | "both"
};

struct RuleConfig {
    std::string id;
    std::string name;
    bool enabled = true;
    std::vector<std::string> streams;  // "*" = all streams
    
    struct Trigger {
        std::string type;  // zone | line_cross | dwell_time | class_count | class_presence | emotion_negative | known_target_matched
        
        // Zone
        std::optional<ZoneConfig> zone;
        
        // Line cross
        std::optional<LineConfig> line;
        
        // Dwell time (seconds)
        int dwell_sec = 0;
        
        // Class count: operator + value
        std::string count_operator;  // ">" | ">=" | "<" | "<=" | "=="
        int count_value = 0;
        
        // Emotion
        float valence_threshold = -0.3f;
        int emotion_duration_sec = 5;
        
        // Known target
        std::string library;
        float min_similarity = 0.85f;
        
        // Common
        std::vector<int> classes;
        float min_confidence = 0.5f;
    } trigger;
    
    struct Action {
        std::string type;  // vlm_verify | alert | notify | record
        
        // VLM verify
        std::string prompt;
        int vlm_timeout_ms = 3000;
        float default_confidence = 0.8f;  // Default confidence when VLM doesn't provide one
        
        // Alert
        std::string level = "warning";  // info | warning | critical
        
        // Notify
        std::vector<std::string> channels;
        std::string content;
        
        // Record
        int pre_sec = 5;
        int post_sec = 10;
    };
    std::vector<Action> actions;
    
    static std::vector<RuleConfig> loadAll(const std::string& path);
};

// =============================================================================
// NotifyConfig - Notification configuration
// =============================================================================

struct NotifyConfig {
    bool enabled = true;
    
    struct WeChat {
        bool enabled = false;
        std::string webhook_url;
        std::vector<std::string> levels;
    } wechat;
    
    struct DingTalk {
        bool enabled = false;
        std::string webhook_url;
        std::string secret;
        std::vector<std::string> levels;
    } dingtalk;
    
    struct Email {
        bool enabled = false;
        std::string smtp_host;
        int smtp_port = 465;
        std::string username;
        std::string password;
        std::string from_addr;
        std::vector<std::string> to_addrs;
        std::vector<std::string> levels;
    } email;
    
    struct Webhook {
        bool enabled = false;
        std::string url;
        std::string method = "POST";
        std::map<std::string, std::string> headers;
        std::vector<std::string> levels;
    } webhook;
    
    std::string message_template;
    
    static NotifyConfig load(const std::string& path);
};

// =============================================================================
// VlmConfig - VLM service configuration
// =============================================================================

struct VlmConfig {
    struct Server {
        std::string host = "0.0.0.0";
        int port = 8090;
    } server;
    
    struct Model {
        std::string path = "models/vlm/fastvlm-0.5b";
        std::string type = "fastvlm";  // fastvlm | qwen3vl
        int context_length = 2048;
        int n_gpu_layers = -1;  // -1 = all
    } model;
    
    struct Inference {
        int max_tokens = 256;
        float temperature = 0.7f;
        float top_p = 0.9f;
        int threads = 4;
    } inference;
    
    static VlmConfig load(const std::string& path);
};

} // namespace rivision
