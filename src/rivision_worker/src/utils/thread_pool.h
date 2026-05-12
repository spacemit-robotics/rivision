#pragma once

#include <vector>
#include <queue>
#include <thread>
#include <mutex>
#include <condition_variable>
#include <functional>
#include <future>
#include <atomic>
#include <stdexcept>

namespace rivision::utils {

// =============================================================================
// ThreadPool - A simple C++17 thread pool
// =============================================================================

class ThreadPool {
public:
    explicit ThreadPool(size_t num_threads = std::thread::hardware_concurrency())
        : stop_(false) {
        if (num_threads == 0) {
            num_threads = 2;
        }
        
        workers_.reserve(num_threads);
        for (size_t i = 0; i < num_threads; ++i) {
            workers_.emplace_back([this] {
                while (true) {
                    std::function<void()> task;
                    
                    {
                        std::unique_lock<std::mutex> lock(mutex_);
                        cv_.wait(lock, [this] {
                            return stop_ || !tasks_.empty();
                        });
                        
                        if (stop_ && tasks_.empty()) {
                            return;
                        }
                        
                        task = std::move(tasks_.front());
                        tasks_.pop();
                    }
                    
                    task();
                }
            });
        }
    }
    
    ~ThreadPool() {
        shutdown();
    }
    
    // Non-copyable
    ThreadPool(const ThreadPool&) = delete;
    ThreadPool& operator=(const ThreadPool&) = delete;
    
    // Submit a task and get a future for the result
    template<typename F, typename... Args>
    auto submit(F&& f, Args&&... args)
        -> std::future<typename std::invoke_result_t<F, Args...>> {
        
        using ReturnType = typename std::invoke_result_t<F, Args...>;
        
        auto task = std::make_shared<std::packaged_task<ReturnType()>>(
            std::bind(std::forward<F>(f), std::forward<Args>(args)...)
        );
        
        std::future<ReturnType> result = task->get_future();
        
        {
            std::unique_lock<std::mutex> lock(mutex_);
            
            if (stop_) {
                throw std::runtime_error("Cannot submit to stopped ThreadPool");
            }
            
            tasks_.emplace([task]() { (*task)(); });
        }
        
        cv_.notify_one();
        return result;
    }
    
    // Submit a task without waiting for result
    template<typename F, typename... Args>
    void enqueue(F&& f, Args&&... args) {
        {
            std::unique_lock<std::mutex> lock(mutex_);
            
            if (stop_) {
                return;
            }
            
            tasks_.emplace(std::bind(std::forward<F>(f), std::forward<Args>(args)...));
        }
        
        cv_.notify_one();
    }
    
    // Get number of threads
    size_t size() const {
        return workers_.size();
    }
    
    // Get pending task count
    size_t pending() const {
        std::unique_lock<std::mutex> lock(mutex_);
        return tasks_.size();
    }
    
    // Wait for all tasks to complete
    void wait() {
        std::unique_lock<std::mutex> lock(mutex_);
        cv_done_.wait(lock, [this] {
            return tasks_.empty();
        });
    }
    
    // Shutdown the pool
    void shutdown() {
        {
            std::unique_lock<std::mutex> lock(mutex_);
            if (stop_) return;
            stop_ = true;
        }
        
        cv_.notify_all();
        
        for (auto& worker : workers_) {
            if (worker.joinable()) {
                worker.join();
            }
        }
    }
    
private:
    std::vector<std::thread> workers_;
    std::queue<std::function<void()>> tasks_;
    
    mutable std::mutex mutex_;
    std::condition_variable cv_;
    std::condition_variable cv_done_;
    std::atomic<bool> stop_;
};

// =============================================================================
// ScopedTask - RAII wrapper for async task
// =============================================================================

class ScopedTask {
public:
    template<typename F>
    explicit ScopedTask(ThreadPool& pool, F&& f)
        : future_(pool.submit(std::forward<F>(f))) {}
    
    ~ScopedTask() {
        if (future_.valid()) {
            future_.wait();
        }
    }
    
    void wait() {
        if (future_.valid()) {
            future_.wait();
        }
    }
    
    template<typename Rep, typename Period>
    bool waitFor(const std::chrono::duration<Rep, Period>& timeout) {
        if (!future_.valid()) return true;
        return future_.wait_for(timeout) == std::future_status::ready;
    }
    
private:
    std::future<void> future_;
};

} // namespace rivision::utils
