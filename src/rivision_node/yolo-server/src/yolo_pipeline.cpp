/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "yolo_pipeline.h"

#include <chrono>
#include <cstdio>
#include <memory>
#include <string>
#include <utility>
#include <vector>

#include "image_utils.h"

namespace yolo {

YOLOPipeline::YOLOPipeline(int preproc_threads, int queue_size)
    : num_preproc_threads_(preproc_threads > 0 ? preproc_threads : 2),
      max_queue_size_(queue_size > 0 ? queue_size : 4) {
    printf("[YOLOPipeline] 初始化: %d 预处理线程, 队列=%d\n", num_preproc_threads_, max_queue_size_);
}

YOLOPipeline::~YOLOPipeline() {
    running_ = false;

    // 唤醒所有等待的线程
    preproc_cv_.notify_all();
    preproc_space_cv_.notify_all();
    infer_cv_.notify_all();

    // 等待线程结束
    for (auto& t : preproc_threads_) {
        if (t.joinable()) t.join();
    }
    if (infer_thread_.joinable()) {
        infer_thread_.join();
    }

    printf("[YOLOPipeline] 销毁\n");
}

bool YOLOPipeline::load(const std::string& model_path) {
    detector_ = std::make_unique<YOLODetector>();
    if (!detector_->load(model_path)) {
        return false;
    }

    running_ = true;

    // 启动预处理线程
    for (int i = 0; i < num_preproc_threads_; i++) {
        preproc_threads_.emplace_back(&YOLOPipeline::preproc_worker, this);
    }

    // 启动推理线程
    infer_thread_ = std::thread(&YOLOPipeline::inference_worker, this);

    loaded_ = true;
    printf("[YOLOPipeline] ✓ 流水线就绪\n");
    return true;
}

void YOLOPipeline::preproc_worker() {
    while (running_) {
        std::pair<std::vector<uint8_t>, std::shared_ptr<PreprocData>> item;

        {
            std::unique_lock<std::mutex> lock(preproc_mutex_);
            preproc_cv_.wait(lock, [this]() { return !preproc_queue_.empty() || !running_; });

            if (!running_ && preproc_queue_.empty()) break;
            if (preproc_queue_.empty()) continue;

            item = std::move(preproc_queue_.front());
            preproc_queue_.pop();
        }
        preproc_space_cv_.notify_one();

        active_preproc_.fetch_add(1);

        // 执行预处理
        auto& jpeg_data = item.first;
        auto& data = item.second;

        image::ImageData img;
        if (image::decode_jpeg(jpeg_data, img)) {
            data->orig_width = img.width;
            data->orig_height = img.height;
            data->tensor = image::resize_and_normalize(img, 640, 640);
            data->valid = !data->tensor.empty();
            if (!data->valid) {
                data->error = "Failed to preprocess";
            }
        } else {
            data->error = "Failed to decode JPEG";
        }

        active_preproc_.fetch_sub(1);

        // 放入推理队列
        {
            std::lock_guard<std::mutex> lock(infer_mutex_);
            infer_queue_.push(data);
        }
        infer_cv_.notify_one();
    }
}

void YOLOPipeline::inference_worker() {
    while (running_) {
        std::shared_ptr<PreprocData> data;

        {
            std::unique_lock<std::mutex> lock(infer_mutex_);
            infer_cv_.wait(lock, [this]() { return !infer_queue_.empty() || !running_; });

            if (!running_ && infer_queue_.empty()) break;
            if (infer_queue_.empty()) continue;

            data = std::move(infer_queue_.front());
            infer_queue_.pop();
        }

        DetectionResult result;

        if (!data->valid) {
            result.success = false;
            result.error = data->error;
        } else {
            // ★ NPU 推理 (这是唯一使用 NPU 的地方)
            auto start = std::chrono::high_resolution_clock::now();

            // 直接调用底层推理 (跳过预处理)
            result = detector_->detect_from_tensor(data->tensor, data->orig_width, data->orig_height);

            auto end = std::chrono::high_resolution_clock::now();
            result.inference_ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();
        }

        // 设置结果
        data->promise.set_value(std::move(result));
    }
}

std::future<DetectionResult> YOLOPipeline::detect_async(const std::vector<uint8_t>& jpeg_data) {
    auto data = std::make_shared<PreprocData>();
    data->request_id = next_request_id_.fetch_add(1);

    auto future = data->promise.get_future();

    {
        std::unique_lock<std::mutex> lock(preproc_mutex_);
        // 等待队列有空间
        preproc_space_cv_.wait(
            lock, [this]() { return preproc_queue_.size() < static_cast<size_t>(max_queue_size_) || !running_; });

        if (!running_) {
            DetectionResult result;
            result.success = false;
            result.error = "Pipeline stopped";
            data->promise.set_value(result);
            return future;
        }

        preproc_queue_.push({jpeg_data, data});
    }
    preproc_cv_.notify_one();

    return future;
}

DetectionResult YOLOPipeline::detect(const std::vector<uint8_t>& jpeg_data) { return detect_async(jpeg_data).get(); }

int YOLOPipeline::get_queue_size() const {
    std::lock_guard<std::mutex> lock1(const_cast<std::mutex&>(preproc_mutex_));
    std::lock_guard<std::mutex> lock2(const_cast<std::mutex&>(infer_mutex_));
    return static_cast<int>(preproc_queue_.size() + infer_queue_.size());
}

}  // namespace yolo
