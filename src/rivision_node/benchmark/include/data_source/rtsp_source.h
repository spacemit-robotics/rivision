#pragma once

#include "data_source.h"
#include <opencv2/opencv.hpp>
#include <string>
#include <thread>
#include <mutex>
#include <atomic>
#include <queue>
#include <condition_variable>

namespace rivision {
namespace benchmark {

/**
 * @brief RTSP 流数据源
 * 
 * 支持两种模式:
 * 1. 连接外部 RTSP 服务器 (如 go2rtc, rtsp_server)
 * 2. 启动内置 RTSP 服务器将 MP4 转为 RTSP 流
 * 
 * 参考 rivision-cli 中的 rtsp_server 实现
 */
class RTSPSource : public DataSource {
public:
    struct Config {
        std::string url;                    // RTSP URL (rtsp://host:port/stream)
        std::string mp4_path;               // MP4 文件路径 (用于启动内置服务器)
        int rtsp_port = 8554;               // RTSP 服务端口
        std::string rtsp_server_bin;        // rtsp_server 二进制路径
        bool auto_start_server = false;     // 是否自动启动 RTSP 服务器
        bool loop = true;                   // 是否循环播放
        int reconnect_attempts = 3;         // 重连次数
        int reconnect_delay_ms = 1000;      // 重连延迟
        int buffer_size = 10;               // 帧缓冲区大小
        int read_timeout_ms = 5000;         // 读取超时
    };
    
    RTSPSource();
    ~RTSPSource() override;
    
    // DataSource 接口
    bool open(const std::string& source) override;
    bool read(Frame& frame) override;
    void close() override;
    bool is_open() const override;
    
    int width() const override { return width_; }
    int height() const override { return height_; }
    int fps() const override { return static_cast<int>(fps_); }
    int64_t total_frames() const override { return -1; }  // 流无限
    int64_t getCurrentFrame() const { return frame_count_; }
    
    std::string getType() const { return "rtsp"; }
    
    // 高级配置
    bool openWithConfig(const Config& config);
    void setConfig(const Config& config) { config_ = config; }
    
    // RTSP 服务器管理
    bool startRTSPServer(const std::string& mp4_path, int port = 8554);
    void stopRTSPServer();
    bool isServerRunning() const { return server_running_; }
    
    // 状态
    int getDroppedFrames() const { return dropped_frames_; }
    int getReconnectCount() const { return reconnect_count_; }
    double getLatency() const;  // 估算延迟 (ms)

private:
    Config config_;
    cv::VideoCapture cap_;
    
    int width_ = 0;
    int height_ = 0;
    double fps_ = 0;
    int64_t frame_count_ = 0;
    
    // 帧缓冲 (异步读取)
    std::queue<Frame> frame_buffer_;
    std::mutex buffer_mutex_;
    std::condition_variable buffer_cv_;
    std::thread read_thread_;
    std::atomic<bool> running_{false};
    
    // 统计
    int dropped_frames_ = 0;
    int reconnect_count_ = 0;
    
    // RTSP 服务器进程
    std::atomic<bool> server_running_{false};
    pid_t server_pid_ = -1;
    
    // 内部方法
    void readLoop();
    bool reconnect();
    std::string buildRTSPUrl(const std::string& mp4_path, int port);
};

/**
 * @brief RTSP 服务器封装
 * 
 * 封装 rtsp_server 或 go2rtc 进程管理
 */
class RTSPServerManager {
public:
    struct ServerInfo {
        std::string binary_path;
        std::string config_path;
        int port;
        pid_t pid;
        bool running;
    };
    
    static RTSPServerManager& instance();
    
    // 启动 RTSP 服务器
    bool startServer(const std::string& mp4_dir, int port = 8554);
    bool startWithConfig(const std::string& config_path);
    
    // 停止服务器
    void stopServer();
    void stopAll();
    
    // 状态
    bool isRunning() const;
    ServerInfo getServerInfo() const;
    
    // 查找 RTSP 服务器二进制
    static std::string findRTSPServerBinary();
    static std::string findGo2rtcBinary();

private:
    RTSPServerManager() = default;
    ~RTSPServerManager();
    
    ServerInfo server_info_;
    std::mutex mutex_;
    
    bool launchProcess(const std::string& binary, 
                       const std::vector<std::string>& args);
    void killProcess(pid_t pid);
};

}  // namespace benchmark
}  // namespace rivision
