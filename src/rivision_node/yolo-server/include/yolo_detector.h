#pragma once

#include <string>
#include <vector>
#include <memory>
#include <cstdint>
#include <mutex>
#include <queue>
#include <condition_variable>
#include <atomic>
#include <thread>
#include <functional>

namespace yolo {

struct Detection {
    float x1, y1, x2, y2;  // 归一化坐标 [0, 1]
    int class_id;
    std::string class_name;
    float confidence;
};

struct DetectionResult {
    bool success;
    std::vector<Detection> detections;
    int inference_ms;
    std::string error;
};

class YOLODetector {
public:
    YOLODetector();
    ~YOLODetector();
    
    bool load(const std::string& model_path);
    DetectionResult detect(const std::vector<uint8_t>& jpeg_data);
    
    // ★ 从预处理后的张量直接推理 (用于流水线模式)
    DetectionResult detect_from_tensor(const std::vector<float>& tensor,
                                       int orig_width, int orig_height);
    
    bool is_loaded() const { return loaded_; }
    
private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    bool loaded_ = false;
    
    std::vector<float> preprocess(const std::vector<uint8_t>& jpeg_data, int& orig_width, int& orig_height);
    std::vector<Detection> postprocess(const std::vector<float>& output, int orig_width, int orig_height);
};

// ★ P7: 多 Session Worker Pool - 真正并发推理
// 每个 Worker 有独立的 YOLODetector (独立 ONNX Session)
// 解决单 Session 串行化问题，充分利用 K3 多核 + NPU
class YOLOWorkerPool {
public:
    YOLOWorkerPool(int num_workers = 2);
    ~YOLOWorkerPool();
    
    bool load(const std::string& model_path);
    DetectionResult detect(const std::vector<uint8_t>& jpeg_data);
    bool is_loaded() const { return loaded_; }
    int get_num_workers() const { return num_workers_; }
    int get_active_workers() const { return active_workers_.load(); }
    
private:
    int num_workers_;
    std::string model_path_;
    bool loaded_ = false;
    
    // Worker 管理
    std::vector<std::unique_ptr<YOLODetector>> detectors_;
    std::vector<std::mutex> detector_mutexes_;  // 每个 detector 一个锁
    std::atomic<int> next_worker_{0};           // Round-robin 选择
    std::atomic<int> active_workers_{0};        // 当前活跃 worker 数
};

// COCO 类别名称
extern const std::vector<std::string> COCO_CLASSES;

}  // namespace yolo
