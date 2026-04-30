#include "yolo_detector.h"
#include "yolo_pipeline.h"
#include "http_server.h"
#include "image_utils.h"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <signal.h>
#include <sstream>
#include <memory>

// ★ 抽象检测器接口，支持 WorkerPool 和 Pipeline 两种模式
class IDetector {
public:
    virtual ~IDetector() = default;
    virtual bool load(const std::string& model_path) = 0;
    virtual yolo::DetectionResult detect(const std::vector<uint8_t>& jpeg_data) = 0;
    virtual bool is_loaded() const = 0;
    virtual int get_workers() const = 0;
    virtual int get_active() const = 0;
    virtual std::string get_mode() const = 0;
};

// WorkerPool 模式适配器
class WorkerPoolDetector : public IDetector {
public:
    explicit WorkerPoolDetector(int workers) : pool_(workers) {}
    bool load(const std::string& model_path) override { return pool_.load(model_path); }
    yolo::DetectionResult detect(const std::vector<uint8_t>& jpeg_data) override { return pool_.detect(jpeg_data); }
    bool is_loaded() const override { return pool_.is_loaded(); }
    int get_workers() const override { return pool_.get_num_workers(); }
    int get_active() const override { return pool_.get_active_workers(); }
    std::string get_mode() const override { return "worker_pool"; }
private:
    yolo::YOLOWorkerPool pool_;
};

// Pipeline 模式适配器 (★ K3 优化: 预处理流水线保持 NPU 忙碌)
class PipelineDetector : public IDetector {
public:
    explicit PipelineDetector(int preproc_threads, int queue_size) 
        : pipeline_(preproc_threads, queue_size) {}
    bool load(const std::string& model_path) override { return pipeline_.load(model_path); }
    yolo::DetectionResult detect(const std::vector<uint8_t>& jpeg_data) override { return pipeline_.detect(jpeg_data); }
    bool is_loaded() const override { return pipeline_.is_loaded(); }
    int get_workers() const override { return 1; }  // Pipeline 模式单 Session
    int get_active() const override { return pipeline_.get_active_preproc(); }
    std::string get_mode() const override { return "pipeline"; }
private:
    yolo::YOLOPipeline pipeline_;
};

static http::Server* g_server = nullptr;

void signal_handler(int sig) {
    printf("\n[YOLO Server] Shutting down...\n");
    if (g_server) g_server->stop();
}

std::string escape_json(const std::string& s) {
    std::string result;
    for (char c : s) {
        switch (c) {
            case '"': result += "\\\""; break;
            case '\\': result += "\\\\"; break;
            case '\n': result += "\\n"; break;
            case '\r': result += "\\r"; break;
            case '\t': result += "\\t"; break;
            default: result += c;
        }
    }
    return result;
}

std::string parse_json_string(const std::string& json, const std::string& key) {
    std::string search = "\"" + key + "\"";
    size_t pos = json.find(search);
    if (pos == std::string::npos) return "";
    pos = json.find(':', pos);
    if (pos == std::string::npos) return "";
    pos = json.find('"', pos + 1);
    if (pos == std::string::npos) return "";
    size_t start = pos + 1;
    size_t end = start;
    while (end < json.size() && json[end] != '"') {
        if (json[end] == '\\') end++;
        end++;
    }
    return json.substr(start, end - start);
}

int main(int argc, char* argv[]) {
    std::string model_path = "models/yolov8n.onnx";
    const char* host = "0.0.0.0";
    int port = 9081;
    float conf_thresh = 0.25f;
    float nms_thresh = 0.45f;

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--model") == 0 && i + 1 < argc) {
            model_path = argv[++i];
        } else if (strcmp(argv[i], "--host") == 0 && i + 1 < argc) {
            host = argv[++i];
        } else if (strcmp(argv[i], "--port") == 0 && i + 1 < argc) {
            port = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--conf-thresh") == 0 && i + 1 < argc) {
            conf_thresh = atof(argv[++i]);
        } else if (strcmp(argv[i], "--nms-thresh") == 0 && i + 1 < argc) {
            nms_thresh = atof(argv[++i]);
        } else if (strcmp(argv[i], "--help") == 0) {
            printf("Usage: yolo-server [options]\n");
            printf("  --model PATH       Model file path\n");
            printf("  --host HOST        Listen host (default: 0.0.0.0)\n");
            printf("  --port PORT        Listen port (default: 9081)\n");
            printf("  --conf-thresh F    Confidence threshold (default: 0.25)\n");
            printf("  --nms-thresh F     NMS threshold (default: 0.45)\n");
            return 0;
        }
    }

    // ★ 模式选择: worker_pool (默认) 或 pipeline (K3 优化)
    // YOLO_MODE=pipeline 启用预处理流水线模式
    std::string mode = "worker_pool";
    const char* env_mode = std::getenv("YOLO_MODE");
    if (env_mode && std::string(env_mode) == "pipeline") {
        mode = "pipeline";
    }
    
    // ★ P7: 从环境变量读取 worker 数量 (默认 2，K3 建议 2-4)
    int num_workers = 2;
    const char* env_workers = std::getenv("YOLO_WORKERS");
    if (env_workers) {
        num_workers = atoi(env_workers);
        if (num_workers < 1) num_workers = 1;
        if (num_workers > 8) num_workers = 8;
    }

    printf("========================================\n");
    printf("  RiVision YOLO Server v1.2.0\n");
    printf("========================================\n");
    printf("Model: %s\n", model_path.c_str());
    printf("Mode: %s (set YOLO_MODE=pipeline for K3 NPU optimization)\n", mode.c_str());
    if (mode == "worker_pool") {
        printf("Workers: %d (set YOLO_WORKERS=N to change)\n", num_workers);
    } else {
        printf("Preproc Threads: %d, Queue: 4\n", num_workers);
    }
    printf("Listen: %s:%d\n", host, port);
    printf("----------------------------------------\n");

    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);

    // ★ 根据模式创建检测器
    std::unique_ptr<IDetector> detector;
    if (mode == "pipeline") {
        // Pipeline 模式: 单 Session + 预处理流水线 (K3 NPU 优化)
        detector = std::make_unique<PipelineDetector>(num_workers, 4);
    } else {
        // WorkerPool 模式: 多 Session 并发 (默认)
        detector = std::make_unique<WorkerPoolDetector>(num_workers);
    }
    
    if (!detector->load(model_path)) {
        fprintf(stderr, "[ERROR] Failed to load model: %s\n", model_path.c_str());
        return 1;
    }

    http::Server server;
    g_server = &server;

    // 健康检查 (P7: 显示 worker 状态)
    server.get("/health", [&detector](const http::Request&, http::Response& res) {
        std::ostringstream oss;
        oss << "{\"status\":\"ok\"";
        oss << ",\"model_loaded\":" << (detector->is_loaded() ? "true" : "false");
        oss << ",\"mode\":\"" << detector->get_mode() << "\"";
        oss << ",\"workers\":" << detector->get_workers();
        oss << ",\"active\":" << detector->get_active();
        oss << "}";
        res.set_json(oss.str());
    });

    // 状态端点 (P7: 显示并发能力)
    // ★ 修复：添加 healthy 字段，node-agent 依赖此字段判断服务健康状态
    server.get("/status", [&detector](const http::Request&, http::Response& res) {
        bool is_healthy = detector->is_loaded();
        std::ostringstream oss;
        oss << "{\"healthy\":" << (is_healthy ? "true" : "false");
        oss << ",\"loaded\":" << (detector->is_loaded() ? "true" : "false");
        oss << ",\"service\":\"yolo-server\"";
        oss << ",\"version\":\"1.2.0\"";
        oss << ",\"mode\":\"" << detector->get_mode() << "\"";
        oss << ",\"workers\":" << detector->get_workers();
        oss << ",\"active\":" << detector->get_active();
        oss << "}";
        res.set_json(oss.str());
    });

    // ★ P5 优化: 二进制接口 - 直接接收 JPEG，跳过 base64 编解码
    // 省去: Gateway base64_encode (~1-3ms) + yolo-server base64_decode (~1-2ms)
    // 对于 4 摄像头 × 15fps，节省 120-300ms/秒
    server.post("/api/detect/binary", [&detector](const http::Request& req, http::Response& res) {
      try {
        if (req.body.empty()) {
            res.status_code = 400;
            res.set_json("{\"success\":false,\"error\":\"empty body\"}");
            return;
        }

        // 直接使用原始二进制数据
        std::vector<uint8_t> jpeg_data(req.body.begin(), req.body.end());
        auto result = detector->detect(jpeg_data);

        std::ostringstream json;
        json << "{\"success\":" << (result.success ? "true" : "false");
        json << ",\"detections\":[";
        for (size_t i = 0; i < result.detections.size(); i++) {
            const auto& det = result.detections[i];
            if (i > 0) json << ",";
            json << "{\"bbox\":[" << det.x1 << "," << det.y1 << ","
                 << det.x2 << "," << det.y2 << "]";
            json << ",\"class_id\":" << det.class_id;
            json << ",\"class_name\":\"" << det.class_name << "\"";
            json << ",\"confidence\":" << det.confidence << "}";
        }
        json << "],\"inference_ms\":" << result.inference_ms;
        if (!result.error.empty()) {
            json << ",\"error\":\"" << escape_json(result.error) << "\"";
        }
        json << "}";
        res.set_json(json.str());

        printf("[YOLO] binary %zu objects in %dms\n",
               result.detections.size(), result.inference_ms);
      } catch (const std::exception& e) {
        fprintf(stderr, "[ERROR] %s\n", e.what());
        res.status_code = 500;
        res.set_json("{\"success\":false,\"error\":\"" + escape_json(e.what()) + "\"}");
      } catch (...) {
        res.status_code = 500;
        res.set_json("{\"success\":false,\"error\":\"unknown error\"}");
      }
    });

    // 检测端点 (兼容旧接口，使用 base64)
    server.post("/api/detect", [&detector](const http::Request& req, http::Response& res) {
      try {
        std::string image_base64 = parse_json_string(req.body, "image");
        if (image_base64.empty()) {
            res.status_code = 400;
            res.set_json("{\"success\":false,\"error\":\"missing image field\"}");
            return;
        }

        auto decoded = image::base64_decode(image_base64);
        std::vector<uint8_t> jpeg_data(decoded.begin(), decoded.end());
        auto result = detector->detect(jpeg_data);

        std::ostringstream json;
        json << "{\"success\":" << (result.success ? "true" : "false");
        json << ",\"detections\":[";
        for (size_t i = 0; i < result.detections.size(); i++) {
            const auto& det = result.detections[i];
            if (i > 0) json << ",";
            json << "{\"bbox\":[" << det.x1 << "," << det.y1 << ","
                 << det.x2 << "," << det.y2 << "]";
            json << ",\"class_id\":" << det.class_id;
            json << ",\"class_name\":\"" << det.class_name << "\"";
            json << ",\"confidence\":" << det.confidence << "}";
        }
        json << "],\"inference_ms\":" << result.inference_ms;
        if (!result.error.empty()) {
            json << ",\"error\":\"" << escape_json(result.error) << "\"";
        }
        json << "}";
        res.set_json(json.str());

        printf("[YOLO] %zu objects in %dms\n",
               result.detections.size(), result.inference_ms);
      } catch (const std::exception& e) {
        fprintf(stderr, "[ERROR] %s\n", e.what());
        res.status_code = 500;
        res.set_json("{\"success\":false,\"error\":\"" + escape_json(e.what()) + "\"}");
      } catch (...) {
        res.status_code = 500;
        res.set_json("{\"success\":false,\"error\":\"unknown error\"}");
      }
    });

    printf("[YOLO Server] Starting...\n");
    if (!server.listen(host, port)) {
        fprintf(stderr, "[ERROR] Failed to start on %s:%d\n", host, port);
        return 1;
    }
    return 0;
}
