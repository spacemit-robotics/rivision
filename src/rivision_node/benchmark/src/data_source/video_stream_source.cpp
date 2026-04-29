#include "data_source.h"
#include <opencv2/opencv.hpp>
#include <queue>
#include <thread>
#include <mutex>
#include <condition_variable>

namespace rivision {
namespace benchmark {

// ============================================================
// VideoStreamSource 实现
// ============================================================
struct VideoStreamSource::Impl {
    cv::VideoCapture cap;
    cv::Mat frame_mat;
    std::vector<uchar> jpeg_buffer;
    std::vector<int> jpeg_params = {cv::IMWRITE_JPEG_QUALITY, 90};
    
    // 缓冲区
    std::queue<Frame> buffer;
    std::mutex mutex;
    std::condition_variable cv;
    std::thread reader_thread;
    std::atomic<bool> running{false};
    
    int64_t frame_counter = 0;
};

VideoStreamSource::VideoStreamSource() : impl_(std::make_unique<Impl>()) {}
VideoStreamSource::~VideoStreamSource() { close(); }

bool VideoStreamSource::open(const std::string& source) {
    // 设置 FFmpeg 参数用于低延迟
    impl_->cap.set(cv::CAP_PROP_BUFFERSIZE, 1);
    
    // 尝试打开流
    // RTSP: rtsp://...
    // HTTP-FLV: http://...
    // V4L2: /dev/video0
    impl_->cap.open(source);
    
    if (!impl_->cap.isOpened()) {
        fprintf(stderr, "[VideoStreamSource] Failed to open: %s\n", source.c_str());
        return false;
    }
    
    fps_ = static_cast<int>(impl_->cap.get(cv::CAP_PROP_FPS));
    if (fps_ <= 0) fps_ = 30;  // 默认 30fps
    
    width_ = static_cast<int>(impl_->cap.get(cv::CAP_PROP_FRAME_WIDTH));
    height_ = static_cast<int>(impl_->cap.get(cv::CAP_PROP_FRAME_HEIGHT));
    is_open_ = true;
    
    printf("[VideoStreamSource] Opened: %s (%dx%d, %d fps)\n",
           source.c_str(), width_, height_, fps_);
    
    // 启动后台读取线程
    impl_->running = true;
    impl_->reader_thread = std::thread([this]() {
        cv::Mat mat;
        std::vector<uchar> buf;
        std::vector<int> params = {cv::IMWRITE_JPEG_QUALITY, 90};
        
        while (impl_->running) {
            if (!impl_->cap.read(mat)) {
                std::this_thread::sleep_for(std::chrono::milliseconds(10));
                continue;
            }
            
            cv::imencode(".jpg", mat, buf, params);
            
            Frame frame;
            frame.data = buf;
            frame.width = mat.cols;
            frame.height = mat.rows;
            frame.channels = mat.channels();
            frame.timestamp_us = now_us();
            frame.frame_id = impl_->frame_counter++;
            frame.is_jpeg = true;
            
            {
                std::unique_lock<std::mutex> lock(impl_->mutex);
                
                // 丢弃旧帧以保持低延迟
                while (impl_->buffer.size() >= static_cast<size_t>(buffer_size_)) {
                    impl_->buffer.pop();
                }
                
                impl_->buffer.push(std::move(frame));
                impl_->cv.notify_one();
            }
        }
    });
    
    return true;
}

bool VideoStreamSource::read(Frame& frame) {
    if (!is_open_) return false;
    
    std::unique_lock<std::mutex> lock(impl_->mutex);
    
    // 等待帧
    if (!impl_->cv.wait_for(lock, std::chrono::seconds(5), [this]() {
        return !impl_->buffer.empty() || !impl_->running;
    })) {
        return false;  // 超时
    }
    
    if (impl_->buffer.empty()) {
        return false;
    }
    
    frame = std::move(impl_->buffer.front());
    impl_->buffer.pop();
    
    return true;
}

void VideoStreamSource::close() {
    impl_->running = false;
    impl_->cv.notify_all();
    
    if (impl_->reader_thread.joinable()) {
        impl_->reader_thread.join();
    }
    
    if (impl_->cap.isOpened()) {
        impl_->cap.release();
    }
    
    is_open_ = false;
}

}  // namespace benchmark
}  // namespace rivision
