#include "data_source/rtsp_source.h"
#include <iostream>
#include <fstream>
#include <sstream>
#include <chrono>
#include <cstdlib>
#include <signal.h>
#include <sys/wait.h>
#include <unistd.h>

namespace rivision {
namespace benchmark {

// ============================================================
// RTSPSource 实现
// ============================================================

RTSPSource::RTSPSource() = default;

RTSPSource::~RTSPSource() {
    close();
    stopRTSPServer();
}

bool RTSPSource::open(const std::string& source) {
    Config config;
    
    // 判断是 URL 还是文件路径
    if (source.find("rtsp://") == 0) {
        config.url = source;
        config.auto_start_server = false;
    } else if (source.find(".mp4") != std::string::npos || 
               source.find(".MP4") != std::string::npos) {
        config.mp4_path = source;
        config.auto_start_server = true;
    } else {
        std::cerr << "[RTSP] Unknown source format: " << source << std::endl;
        return false;
    }
    
    return openWithConfig(config);
}

bool RTSPSource::openWithConfig(const Config& config) {
    config_ = config;
    
    std::string rtsp_url = config_.url;
    
    // 如果需要自动启动 RTSP 服务器
    if (config_.auto_start_server && !config_.mp4_path.empty()) {
        if (!startRTSPServer(config_.mp4_path, config_.rtsp_port)) {
            std::cerr << "[RTSP] Failed to start RTSP server" << std::endl;
            return false;
        }
        rtsp_url = buildRTSPUrl(config_.mp4_path, config_.rtsp_port);
    }
    
    if (rtsp_url.empty()) {
        std::cerr << "[RTSP] No RTSP URL specified" << std::endl;
        return false;
    }
    
    // 使用 OpenCV 连接 RTSP
    std::cout << "[RTSP] Connecting to: " << rtsp_url << std::endl;
    
    // 设置 RTSP 参数
    cap_.set(cv::CAP_PROP_BUFFERSIZE, 1);  // 最小缓冲
    
    // 尝试连接
    for (int attempt = 0; attempt <= config_.reconnect_attempts; attempt++) {
        if (cap_.open(rtsp_url, cv::CAP_FFMPEG)) {
            break;
        }
        if (attempt < config_.reconnect_attempts) {
            std::cout << "[RTSP] Connection failed, retrying... (" 
                      << (attempt + 1) << "/" << config_.reconnect_attempts << ")" << std::endl;
            std::this_thread::sleep_for(
                std::chrono::milliseconds(config_.reconnect_delay_ms));
        }
    }
    
    if (!cap_.isOpened()) {
        std::cerr << "[RTSP] Failed to connect after " 
                  << config_.reconnect_attempts << " attempts" << std::endl;
        return false;
    }
    
    // 获取流信息
    width_ = static_cast<int>(cap_.get(cv::CAP_PROP_FRAME_WIDTH));
    height_ = static_cast<int>(cap_.get(cv::CAP_PROP_FRAME_HEIGHT));
    fps_ = cap_.get(cv::CAP_PROP_FPS);
    if (fps_ <= 0) fps_ = 25.0;  // 默认帧率
    
    std::cout << "[RTSP] Connected: " << width_ << "x" << height_ 
              << " @ " << fps_ << " fps" << std::endl;
    
    // 启动异步读取线程
    running_ = true;
    read_thread_ = std::thread(&RTSPSource::readLoop, this);
    
    return true;
}

bool RTSPSource::read(Frame& frame) {
    if (!running_) return false;
    
    std::unique_lock<std::mutex> lock(buffer_mutex_);
    
    // 等待帧
    if (frame_buffer_.empty()) {
        auto timeout = std::chrono::milliseconds(config_.read_timeout_ms);
        if (!buffer_cv_.wait_for(lock, timeout, [this] { 
            return !frame_buffer_.empty() || !running_; 
        })) {
            return false;  // 超时
        }
    }
    
    if (frame_buffer_.empty() || !running_) {
        return false;
    }
    
    frame = std::move(frame_buffer_.front());
    frame_buffer_.pop();
    frame_count_++;
    
    return true;
}

void RTSPSource::close() {
    running_ = false;
    buffer_cv_.notify_all();
    
    if (read_thread_.joinable()) {
        read_thread_.join();
    }
    
    if (cap_.isOpened()) {
        cap_.release();
    }
    
    // 清空缓冲区
    std::lock_guard<std::mutex> lock(buffer_mutex_);
    while (!frame_buffer_.empty()) {
        frame_buffer_.pop();
    }
}

bool RTSPSource::is_open() const {
    return cap_.isOpened() && running_;
}

void RTSPSource::readLoop() {
    while (running_) {
        cv::Mat mat;
        if (!cap_.read(mat)) {
            if (!running_) break;
            
            // 尝试重连
            if (!reconnect()) {
                std::cerr << "[RTSP] Reconnection failed, stopping" << std::endl;
                running_ = false;
                break;
            }
            continue;
        }
        
        // 编码为 JPEG
        std::vector<uchar> jpeg_buffer;
        cv::imencode(".jpg", mat, jpeg_buffer);
        
        Frame frame;
        frame.data = std::vector<uint8_t>(jpeg_buffer.begin(), jpeg_buffer.end());
        frame.width = mat.cols;
        frame.height = mat.rows;
        frame.channels = mat.channels();
        frame.is_jpeg = true;
        frame.timestamp_us = std::chrono::duration_cast<std::chrono::microseconds>(
            std::chrono::steady_clock::now().time_since_epoch()).count();
        frame.frame_id = frame_count_++;
        
        // 添加到缓冲区
        {
            std::lock_guard<std::mutex> lock(buffer_mutex_);
            
            // 如果缓冲区满，丢弃旧帧
            while (frame_buffer_.size() >= static_cast<size_t>(config_.buffer_size)) {
                frame_buffer_.pop();
                dropped_frames_++;
            }
            
            frame_buffer_.push(std::move(frame));
        }
        buffer_cv_.notify_one();
    }
}

bool RTSPSource::reconnect() {
    reconnect_count_++;
    std::cout << "[RTSP] Attempting reconnect #" << reconnect_count_ << std::endl;
    
    cap_.release();
    std::this_thread::sleep_for(std::chrono::milliseconds(config_.reconnect_delay_ms));
    
    std::string url = config_.url.empty() ? 
        buildRTSPUrl(config_.mp4_path, config_.rtsp_port) : config_.url;
    
    return cap_.open(url, cv::CAP_FFMPEG);
}

double RTSPSource::getLatency() const {
    // 估算延迟: 缓冲帧数 / FPS
    std::lock_guard<std::mutex> lock(const_cast<std::mutex&>(buffer_mutex_));
    return (frame_buffer_.size() / fps_) * 1000.0;
}

std::string RTSPSource::buildRTSPUrl(const std::string& mp4_path, int port) {
    // 从文件路径提取流名称
    std::string filename = mp4_path;
    size_t pos = filename.find_last_of("/\\");
    if (pos != std::string::npos) {
        filename = filename.substr(pos + 1);
    }
    
    return "rtsp://localhost:" + std::to_string(port) + "/" + filename;
}

bool RTSPSource::startRTSPServer(const std::string& mp4_path, int port) {
    if (server_running_) {
        return true;
    }
    
    // 查找 rtsp_server 二进制
    std::string rtsp_bin = config_.rtsp_server_bin;
    if (rtsp_bin.empty()) {
        rtsp_bin = RTSPServerManager::findRTSPServerBinary();
    }
    
    if (rtsp_bin.empty() || access(rtsp_bin.c_str(), X_OK) != 0) {
        std::cerr << "[RTSP] rtsp_server binary not found" << std::endl;
        return false;
    }
    
    // Fork 启动服务器进程
    server_pid_ = fork();
    if (server_pid_ < 0) {
        std::cerr << "[RTSP] Failed to fork" << std::endl;
        return false;
    }
    
    if (server_pid_ == 0) {
        // 子进程
        std::string port_str = std::to_string(port);
        execl(rtsp_bin.c_str(), rtsp_bin.c_str(),
              "-p", port_str.c_str(),
              "-f", mp4_path.c_str(),
              config_.loop ? "-l" : nullptr,
              nullptr);
        exit(1);
    }
    
    // 等待服务器启动
    std::this_thread::sleep_for(std::chrono::seconds(1));
    
    // 检查进程是否存活
    int status;
    pid_t result = waitpid(server_pid_, &status, WNOHANG);
    if (result == server_pid_) {
        std::cerr << "[RTSP] Server process exited immediately" << std::endl;
        return false;
    }
    
    server_running_ = true;
    std::cout << "[RTSP] Server started on port " << port << std::endl;
    
    return true;
}

void RTSPSource::stopRTSPServer() {
    if (server_pid_ > 0 && server_running_) {
        kill(server_pid_, SIGTERM);
        waitpid(server_pid_, nullptr, 0);
        server_pid_ = -1;
        server_running_ = false;
        std::cout << "[RTSP] Server stopped" << std::endl;
    }
}

// ============================================================
// RTSPServerManager 实现
// ============================================================

RTSPServerManager& RTSPServerManager::instance() {
    static RTSPServerManager instance;
    return instance;
}

RTSPServerManager::~RTSPServerManager() {
    stopAll();
}

bool RTSPServerManager::startServer(const std::string& mp4_dir, int port) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    if (server_info_.running) {
        return true;
    }
    
    std::string binary = findRTSPServerBinary();
    if (binary.empty()) {
        binary = findGo2rtcBinary();
    }
    
    if (binary.empty()) {
        std::cerr << "[RTSPManager] No RTSP server binary found" << std::endl;
        return false;
    }
    
    std::vector<std::string> args = {"-p", std::to_string(port), "-d", mp4_dir};
    
    if (!launchProcess(binary, args)) {
        return false;
    }
    
    server_info_.binary_path = binary;
    server_info_.port = port;
    server_info_.running = true;
    
    return true;
}

void RTSPServerManager::stopServer() {
    std::lock_guard<std::mutex> lock(mutex_);
    
    if (server_info_.running && server_info_.pid > 0) {
        killProcess(server_info_.pid);
        server_info_.running = false;
        server_info_.pid = -1;
    }
}

void RTSPServerManager::stopAll() {
    stopServer();
}

bool RTSPServerManager::isRunning() const {
    return server_info_.running;
}

RTSPServerManager::ServerInfo RTSPServerManager::getServerInfo() const {
    return server_info_;
}

std::string RTSPServerManager::findRTSPServerBinary() {
    // 搜索路径
    std::vector<std::string> paths = {
        "./rtsp_server",
        "/usr/local/bin/rtsp_server",
        "/usr/bin/rtsp_server",
        "../bin/rtsp_server",
    };
    
    // 添加平台特定路径
#ifdef __x86_64__
    paths.push_back("./rtsp_server-linux-amd64");
#elif defined(__riscv)
    paths.push_back("./rtsp_server-linux-riscv64");
#elif defined(__aarch64__)
    paths.push_back("./rtsp_server-linux-arm64");
#endif
    
    for (const auto& path : paths) {
        if (access(path.c_str(), X_OK) == 0) {
            return path;
        }
    }
    
    return "";
}

std::string RTSPServerManager::findGo2rtcBinary() {
    std::vector<std::string> paths = {
        "./go2rtc",
        "/usr/local/bin/go2rtc",
        "/usr/bin/go2rtc",
    };
    
#ifdef __x86_64__
    paths.push_back("./go2rtc-linux-amd64");
#elif defined(__riscv)
    paths.push_back("./go2rtc-linux-riscv64");
#elif defined(__aarch64__)
    paths.push_back("./go2rtc-linux-arm64");
#endif
    
    for (const auto& path : paths) {
        if (access(path.c_str(), X_OK) == 0) {
            return path;
        }
    }
    
    return "";
}

bool RTSPServerManager::launchProcess(const std::string& binary,
                                       const std::vector<std::string>& args) {
    pid_t pid = fork();
    if (pid < 0) {
        return false;
    }
    
    if (pid == 0) {
        // 子进程
        std::vector<char*> argv;
        argv.push_back(const_cast<char*>(binary.c_str()));
        for (const auto& arg : args) {
            argv.push_back(const_cast<char*>(arg.c_str()));
        }
        argv.push_back(nullptr);
        
        execv(binary.c_str(), argv.data());
        exit(1);
    }
    
    server_info_.pid = pid;
    
    // 等待启动
    std::this_thread::sleep_for(std::chrono::milliseconds(500));
    
    // 检查进程
    int status;
    pid_t result = waitpid(pid, &status, WNOHANG);
    if (result == pid) {
        return false;
    }
    
    return true;
}

void RTSPServerManager::killProcess(pid_t pid) {
    if (pid > 0) {
        kill(pid, SIGTERM);
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
        
        int status;
        if (waitpid(pid, &status, WNOHANG) == 0) {
            kill(pid, SIGKILL);
            waitpid(pid, &status, 0);
        }
    }
}

}  // namespace benchmark
}  // namespace rivision
