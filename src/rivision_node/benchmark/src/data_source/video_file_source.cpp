#include "data_source.h"
#include <opencv2/opencv.hpp>
#include <filesystem>

namespace rivision {
namespace benchmark {

// ============================================================
// VideoFileSource 实现
// ============================================================
struct VideoFileSource::Impl {
    cv::VideoCapture cap;
    cv::Mat frame_mat;
    std::vector<uchar> jpeg_buffer;
    std::vector<int> jpeg_params = {cv::IMWRITE_JPEG_QUALITY, 90};
};

VideoFileSource::VideoFileSource() : impl_(std::make_unique<Impl>()) {}
VideoFileSource::~VideoFileSource() { close(); }

bool VideoFileSource::open(const std::string& source) {
    if (!std::filesystem::exists(source)) {
        fprintf(stderr, "[VideoFileSource] File not found: %s\n", source.c_str());
        return false;
    }
    
    impl_->cap.open(source);
    if (!impl_->cap.isOpened()) {
        fprintf(stderr, "[VideoFileSource] Failed to open: %s\n", source.c_str());
        return false;
    }
    
    fps_ = static_cast<int>(impl_->cap.get(cv::CAP_PROP_FPS));
    width_ = static_cast<int>(impl_->cap.get(cv::CAP_PROP_FRAME_WIDTH));
    height_ = static_cast<int>(impl_->cap.get(cv::CAP_PROP_FRAME_HEIGHT));
    total_frames_ = static_cast<int64_t>(impl_->cap.get(cv::CAP_PROP_FRAME_COUNT));
    duration_sec_ = total_frames_ > 0 && fps_ > 0 ? static_cast<double>(total_frames_) / fps_ : 0;
    current_frame_ = 0;
    is_open_ = true;
    
    printf("[VideoFileSource] Opened: %s (%dx%d, %d fps, %ld frames, %.1f sec)\n",
           source.c_str(), width_, height_, fps_, total_frames_, duration_sec_);
    
    return true;
}

bool VideoFileSource::read(Frame& frame) {
    if (!is_open_) return false;
    
    if (!impl_->cap.read(impl_->frame_mat)) {
        if (loop_ && total_frames_ > 0) {
            impl_->cap.set(cv::CAP_PROP_POS_FRAMES, 0);
            current_frame_ = 0;
            if (!impl_->cap.read(impl_->frame_mat)) {
                return false;
            }
        } else {
            return false;
        }
    }
    
    // 编码为 JPEG
    cv::imencode(".jpg", impl_->frame_mat, impl_->jpeg_buffer, impl_->jpeg_params);
    
    frame.data = impl_->jpeg_buffer;
    frame.width = impl_->frame_mat.cols;
    frame.height = impl_->frame_mat.rows;
    frame.channels = impl_->frame_mat.channels();
    frame.timestamp_us = now_us();
    frame.frame_id = current_frame_;
    frame.is_jpeg = true;
    
    current_frame_++;
    return true;
}

void VideoFileSource::seek(int64_t frame_num) {
    if (!is_open_) return;
    
    frame_num = std::max(0L, std::min(frame_num, total_frames_ - 1));
    impl_->cap.set(cv::CAP_PROP_POS_FRAMES, static_cast<double>(frame_num));
    current_frame_ = frame_num;
}

void VideoFileSource::close() {
    if (impl_->cap.isOpened()) {
        impl_->cap.release();
    }
    is_open_ = false;
}

// ============================================================
// DataSource 工厂方法
// ============================================================
std::unique_ptr<DataSource> DataSource::create(const std::string& type) {
    if (type == "video_file") {
        return std::make_unique<VideoFileSource>();
    } else if (type == "video_stream") {
        return std::make_unique<VideoStreamSource>();
    } else if (type == "image_folder") {
        return std::make_unique<ImageFolderSource>();
    } else if (type == "single_image") {
        return std::make_unique<SingleImageSource>();
    }
    return nullptr;
}

// ============================================================
// DataSource 默认实现
// ============================================================
bool DataSource::read_batch(std::vector<Frame>& frames, int count) {
    frames.clear();
    frames.reserve(count);
    
    Frame frame;
    for (int i = 0; i < count; i++) {
        if (!read(frame)) break;
        frames.push_back(std::move(frame));
    }
    
    return !frames.empty();
}

}  // namespace benchmark
}  // namespace rivision
