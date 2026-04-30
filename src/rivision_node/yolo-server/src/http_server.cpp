/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "http_server.h"

#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>

#include <atomic>
#include <condition_variable>
#include <cstdio>
#include <cstring>
#include <functional>
#include <map>
#include <memory>
#include <mutex>
#include <queue>
#include <sstream>
#include <string>
#include <thread>
#include <utility>
#include <vector>

// ============================================================
// ThreadPool: 避免每个请求创建新线程导致的内存累积
// ============================================================
class ThreadPool {
   public:
    // ★ 增加默认线程数到 8，更好隐藏网络 I/O 延迟
    explicit ThreadPool(size_t num_threads = 8) : stop_(false) {
        for (size_t i = 0; i < num_threads; ++i) {
            workers_.emplace_back([this] {
                while (true) {
                    std::function<void()> task;
                    {
                        std::unique_lock<std::mutex> lock(queue_mutex_);
                        condition_.wait(lock, [this] { return stop_ || !tasks_.empty(); });
                        if (stop_ && tasks_.empty()) return;
                        task = std::move(tasks_.front());
                        tasks_.pop();
                    }
                    task();
                }
            });
        }
        printf("[ThreadPool] Created with %zu workers\n", num_threads);
    }

    ~ThreadPool() {
        {
            std::unique_lock<std::mutex> lock(queue_mutex_);
            stop_ = true;
        }
        condition_.notify_all();
        for (std::thread& worker : workers_) {
            if (worker.joinable()) worker.join();
        }
        printf("[ThreadPool] Destroyed\n");
    }

    void enqueue(std::function<void()> task) {
        {
            std::unique_lock<std::mutex> lock(queue_mutex_);
            if (stop_) return;
            tasks_.emplace(std::move(task));
        }
        condition_.notify_one();
    }

   private:
    std::vector<std::thread> workers_;
    std::queue<std::function<void()>> tasks_;
    std::mutex queue_mutex_;
    std::condition_variable condition_;
    bool stop_;
};

namespace http {

struct Server::Impl {
    int server_fd = -1;
    std::atomic<bool> running{false};
    std::map<std::string, RequestHandler> get_handlers;
    std::map<std::string, RequestHandler> post_handlers;
    std::unique_ptr<ThreadPool> thread_pool;

    void handle_connection(int client_fd) {
        char buffer[65536];
        ssize_t bytes_read = recv(client_fd, buffer, sizeof(buffer) - 1, 0);

        if (bytes_read <= 0) {
            close(client_fd);
            return;
        }

        buffer[bytes_read] = '\0';

        Request req;
        Response res;

        // 解析请求行
        std::istringstream iss(buffer);
        std::string line;
        std::getline(iss, line);

        std::istringstream request_line(line);
        request_line >> req.method >> req.path;

        // 解析头部
        size_t content_length = 0;
        while (std::getline(iss, line) && line != "\r" && !line.empty()) {
            if (line.back() == '\r') line.pop_back();
            size_t pos = line.find(':');
            if (pos != std::string::npos) {
                std::string key = line.substr(0, pos);
                std::string value = line.substr(pos + 1);
                while (!value.empty() && value[0] == ' ') value.erase(0, 1);
                req.headers[key] = value;

                if (key == "Content-Length") {
                    content_length = std::stoul(value);
                }
            }
        }

        // 解析body（★ 修复：支持二进制数据，不能用 C 字符串操作）
        const char* body_start = strstr(buffer, "\r\n\r\n");
        if (body_start) {
            size_t header_len = (body_start + 4) - buffer;
            size_t body_in_buffer = bytes_read - header_len;
            req.body.assign(body_start + 4, body_in_buffer);

            // 如果body不完整，继续读取
            while (req.body.size() < content_length) {
                bytes_read = recv(client_fd, buffer, sizeof(buffer), 0);
                if (bytes_read <= 0) break;
                // ★ 使用 append(ptr, len) 支持二进制数据
                req.body.append(buffer, bytes_read);
            }
        }

        // 设置CORS头
        res.headers["Access-Control-Allow-Origin"] = "*";
        res.headers["Access-Control-Allow-Methods"] = "GET, POST, OPTIONS";
        res.headers["Access-Control-Allow-Headers"] = "Content-Type";

        // 处理OPTIONS请求
        if (req.method == "OPTIONS") {
            res.status_code = 204;
        } else {
            // 路由
            RequestHandler* handler = nullptr;
            if (req.method == "GET") {
                auto it = get_handlers.find(req.path);
                if (it != get_handlers.end()) handler = &it->second;
            } else if (req.method == "POST") {
                auto it = post_handlers.find(req.path);
                if (it != post_handlers.end()) handler = &it->second;
            }

            if (handler) {
                try {
                    (*handler)(req, res);
                } catch (const std::exception& e) {
                    res.status_code = 500;
                    res.set_json("{\"error\":\"" + std::string(e.what()) + "\"}");
                }
            } else {
                res.status_code = 404;
                res.set_json("{\"error\":\"Not found\"}");
            }
        }

        // 构建响应
        std::ostringstream response;
        response << "HTTP/1.1 " << res.status_code << " OK\r\n";
        for (const auto& [key, value] : res.headers) {
            response << key << ": " << value << "\r\n";
        }
        response << "Content-Length: " << res.body.size() << "\r\n";
        response << "\r\n";
        response << res.body;

        std::string response_str = response.str();
        send(client_fd, response_str.c_str(), response_str.size(), 0);

        close(client_fd);
    }
};

Server::Server() : impl_(std::make_unique<Impl>()) {}

Server::~Server() { stop(); }

void Server::get(const std::string& path, RequestHandler handler) { impl_->get_handlers[path] = handler; }

void Server::post(const std::string& path, RequestHandler handler) { impl_->post_handlers[path] = handler; }

bool Server::listen(const std::string& host, int port) {
    impl_->server_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (impl_->server_fd < 0) {
        return false;
    }

    int opt = 1;
    setsockopt(impl_->server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

    struct sockaddr_in address;
    address.sin_family = AF_INET;
    address.sin_addr.s_addr = INADDR_ANY;
    address.sin_port = htons(port);

    if (bind(impl_->server_fd, (struct sockaddr*)&address, sizeof(address)) < 0) {
        close(impl_->server_fd);
        return false;
    }

    if (::listen(impl_->server_fd, 10) < 0) {
        close(impl_->server_fd);
        return false;
    }

    impl_->running = true;

    // 初始化线程池（4个工作线程，适合大多数场景）
    impl_->thread_pool = std::make_unique<ThreadPool>(4);

    printf("[HTTP] Server listening on %s:%d\n", host.c_str(), port);

    while (impl_->running) {
        struct sockaddr_in client_addr;
        socklen_t client_len = sizeof(client_addr);

        int client_fd = accept(impl_->server_fd, (struct sockaddr*)&client_addr, &client_len);
        if (client_fd < 0) {
            if (impl_->running) continue;
            break;
        }

        // 使用线程池处理连接（避免 detach 导致的内存累积）
        impl_->thread_pool->enqueue([this, client_fd]() { impl_->handle_connection(client_fd); });
    }

    return true;
}

void Server::stop() {
    impl_->running = false;
    if (impl_->server_fd >= 0) {
        close(impl_->server_fd);
        impl_->server_fd = -1;
    }
    // 销毁线程池，等待所有工作线程完成
    impl_->thread_pool.reset();
}

}  // namespace http
