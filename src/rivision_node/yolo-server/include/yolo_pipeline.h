/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

#pragma once

#include <atomic>
#include <condition_variable>
#include <functional>
#include <future>
#include <mutex>
#include <queue>
#include <thread>

#include "yolo_detector.h"

namespace yolo {

// ============================================================
// 预处理数据缓存
// ============================================================
struct PreprocData {
    std::vector<float> tensor;  // 预处理后的张量
    int orig_width;
    int orig_height;
    bool valid = false;
    std::string error;
    int64_t request_id;
    std::promise<DetectionResult> promise;
};

// ============================================================
// YOLOPipeline: 异步预处理 + NPU推理流水线
// 目标: 保持 NPU 100% 忙碌
// ============================================================
class YOLOPipeline {
   public:
    explicit YOLOPipeline(int preproc_threads = 2, int queue_size = 4);
    ~YOLOPipeline();

    bool load(const std::string& model_path);

    // 异步检测 (返回 future)
    std::future<DetectionResult> detect_async(const std::vector<uint8_t>& jpeg_data);

    // 同步检测 (等待结果)
    DetectionResult detect(const std::vector<uint8_t>& jpeg_data);

    bool is_loaded() const { return loaded_; }

    // 统计
    int get_queue_size() const;
    int get_active_preproc() const { return active_preproc_.load(); }

   private:
    void preproc_worker();
    void inference_worker();

    // 配置
    int num_preproc_threads_;
    int max_queue_size_;
    bool loaded_ = false;

    // 检测器 (单 Session)
    std::unique_ptr<YOLODetector> detector_;

    // 预处理队列
    std::queue<std::pair<std::vector<uint8_t>, std::shared_ptr<PreprocData>>> preproc_queue_;
    std::mutex preproc_mutex_;
    std::condition_variable preproc_cv_;
    std::condition_variable preproc_space_cv_;

    // 推理队列 (预处理完成的数据)
    std::queue<std::shared_ptr<PreprocData>> infer_queue_;
    std::mutex infer_mutex_;
    std::condition_variable infer_cv_;

    // 线程
    std::vector<std::thread> preproc_threads_;
    std::thread infer_thread_;

    // 控制
    std::atomic<bool> running_{false};
    std::atomic<int> active_preproc_{0};
    std::atomic<int64_t> next_request_id_{0};
};

}  // namespace yolo
