#pragma once

#include "benchmark_common.h"
#include <memory>
#include <vector>
#include <future>
#include <queue>
#include <thread>
#include <mutex>
#include <condition_variable>

namespace rivision {
namespace benchmark {

// ============================================================
// 推理后端基类
// ============================================================
class InferenceBackend {
public:
    virtual ~InferenceBackend() = default;
    
    // 初始化
    virtual bool init(const BackendConfig& config) = 0;
    
    // 单帧推理 (同步)
    virtual InferenceResult infer(const Frame& frame) = 0;
    
    // 批量推理 (可并发)
    virtual std::vector<InferenceResult> infer_batch(
        const std::vector<Frame>& frames, int concurrency = 1);
    
    // 异步推理 (非阻塞)
    virtual std::future<InferenceResult> infer_async(const Frame& frame);
    
    // 后端信息
    virtual BackendInfo info() const = 0;
    virtual int max_concurrency() const { return 1; }
    virtual bool is_loaded() const = 0;
    
    // 资源信息
    virtual double get_memory_usage_mb() const { return 0; }
    
    // 关闭
    virtual void shutdown() = 0;
    
    // 工厂方法
    static std::unique_ptr<InferenceBackend> create(const std::string& type);
};

// ============================================================
// 本地 YOLO 推理后端
// ============================================================
class LocalYOLOBackend : public InferenceBackend {
public:
    LocalYOLOBackend();
    ~LocalYOLOBackend() override;
    
    bool init(const BackendConfig& config) override;
    InferenceResult infer(const Frame& frame) override;
    std::vector<InferenceResult> infer_batch(
        const std::vector<Frame>& frames, int concurrency = 1) override;
    
    BackendInfo info() const override;
    int max_concurrency() const override { return num_workers_; }
    bool is_loaded() const override { return is_loaded_; }
    double get_memory_usage_mb() const override;
    
    void shutdown() override;

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    BackendConfig config_;
    int num_workers_ = 1;
    bool is_loaded_ = false;
};

// ============================================================
// 本地 VLM 推理后端 (llama.cpp)
// ============================================================
class LocalVLMBackend : public InferenceBackend {
public:
    LocalVLMBackend();
    ~LocalVLMBackend() override;
    
    bool init(const BackendConfig& config) override;
    InferenceResult infer(const Frame& frame) override;
    
    BackendInfo info() const override;
    int max_concurrency() const override { return 1; }  // VLM 通常单并发
    bool is_loaded() const override { return is_loaded_; }
    double get_memory_usage_mb() const override;
    
    void shutdown() override;

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    BackendConfig config_;
    bool is_loaded_ = false;
};

// ============================================================
// HTTP YOLO 推理后端 (yolo-server)
// ============================================================
class HTTPYOLOBackend : public InferenceBackend {
public:
    HTTPYOLOBackend();
    ~HTTPYOLOBackend() override;
    
    bool init(const BackendConfig& config) override;
    InferenceResult infer(const Frame& frame) override;
    std::vector<InferenceResult> infer_batch(
        const std::vector<Frame>& frames, int concurrency = 1) override;
    
    BackendInfo info() const override;
    int max_concurrency() const override { return config_.http_pool_size; }
    bool is_loaded() const override { return is_connected_; }
    
    void shutdown() override;
    
    // 连接池统计
    int active_connections() const;
    int pending_requests() const;

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    BackendConfig config_;
    bool is_connected_ = false;
};

// ============================================================
// HTTP VLM 推理后端 (llama-server / OpenAI compatible)
// ============================================================
class HTTPVLMBackend : public InferenceBackend {
public:
    HTTPVLMBackend();
    ~HTTPVLMBackend() override;
    
    bool init(const BackendConfig& config) override;
    InferenceResult infer(const Frame& frame) override;
    
    BackendInfo info() const override;
    int max_concurrency() const override { return config_.http_pool_size; }
    bool is_loaded() const override { return is_connected_; }
    
    void shutdown() override;

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    BackendConfig config_;
    bool is_connected_ = false;
};

// ============================================================
// 并发推理包装器
// ============================================================
class ConcurrentInferenceWrapper {
public:
    ConcurrentInferenceWrapper(InferenceBackend* backend, int concurrency);
    ~ConcurrentInferenceWrapper();
    
    // 提交任务
    void submit(const Frame& frame, InferenceCallback callback);
    
    // 等待所有任务完成
    void wait_all();
    
    // 统计
    int64_t completed_count() const { return completed_count_; }
    int64_t error_count() const { return error_count_; }
    int queue_size() const;

private:
    void worker_thread();
    
    InferenceBackend* backend_;
    int concurrency_;
    
    struct Task {
        Frame frame;
        InferenceCallback callback;
    };
    
    std::queue<Task> task_queue_;
    std::mutex mutex_;
    std::condition_variable cv_;
    std::vector<std::thread> workers_;
    
    std::atomic<bool> running_{false};
    std::atomic<int64_t> completed_count_{0};
    std::atomic<int64_t> error_count_{0};
};

}  // namespace benchmark
}  // namespace rivision
