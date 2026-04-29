#include "data_source.h"
#include <opencv2/opencv.hpp>
#include <filesystem>
#include <algorithm>
#include <random>
#include <fstream>

namespace rivision {
namespace benchmark {

namespace fs = std::filesystem;

// ============================================================
// ImageFolderSource 实现
// ============================================================
ImageFolderSource::ImageFolderSource() = default;
ImageFolderSource::~ImageFolderSource() { close(); }

bool ImageFolderSource::open(const std::string& source) {
    if (!fs::exists(source)) {
        fprintf(stderr, "[ImageFolderSource] Path not found: %s\n", source.c_str());
        return false;
    }
    
    // 收集所有图片文件
    image_paths_.clear();
    
    std::vector<std::string> extensions = {".jpg", ".jpeg", ".png", ".bmp", ".webp"};
    
    for (const auto& entry : fs::directory_iterator(source)) {
        if (!entry.is_regular_file()) continue;
        
        std::string ext = entry.path().extension().string();
        std::transform(ext.begin(), ext.end(), ext.begin(), ::tolower);
        
        if (std::find(extensions.begin(), extensions.end(), ext) != extensions.end()) {
            image_paths_.push_back(entry.path().string());
        }
    }
    
    if (image_paths_.empty()) {
        fprintf(stderr, "[ImageFolderSource] No images found in: %s\n", source.c_str());
        return false;
    }
    
    // 排序
    std::sort(image_paths_.begin(), image_paths_.end());
    
    // 打乱顺序 (可选)
    if (shuffle_) {
        std::random_device rd;
        std::mt19937 g(rd());
        std::shuffle(image_paths_.begin(), image_paths_.end(), g);
    }
    
    // 读取第一张图片获取尺寸
    cv::Mat first = cv::imread(image_paths_[0]);
    if (first.empty()) {
        fprintf(stderr, "[ImageFolderSource] Failed to read first image\n");
        return false;
    }
    
    width_ = first.cols;
    height_ = first.rows;
    current_index_ = 0;
    is_open_ = true;
    
    printf("[ImageFolderSource] Opened: %s (%zu images, %dx%d)\n",
           source.c_str(), image_paths_.size(), width_, height_);
    
    return true;
}

bool ImageFolderSource::read(Frame& frame) {
    if (!is_open_) return false;
    
    if (current_index_ >= static_cast<int64_t>(image_paths_.size())) {
        if (loop_) {
            current_index_ = 0;
        } else {
            return false;
        }
    }
    
    // 读取图片文件为原始字节 (JPEG)
    const std::string& path = image_paths_[current_index_];
    
    std::ifstream file(path, std::ios::binary | std::ios::ate);
    if (!file) {
        fprintf(stderr, "[ImageFolderSource] Failed to read: %s\n", path.c_str());
        current_index_++;
        return read(frame);  // 跳过损坏文件
    }
    
    std::streamsize size = file.tellg();
    file.seekg(0, std::ios::beg);
    
    frame.data.resize(size);
    if (!file.read(reinterpret_cast<char*>(frame.data.data()), size)) {
        fprintf(stderr, "[ImageFolderSource] Read error: %s\n", path.c_str());
        current_index_++;
        return read(frame);
    }
    
    frame.width = width_;
    frame.height = height_;
    frame.channels = 3;
    frame.timestamp_us = now_us();
    frame.frame_id = current_index_;
    frame.is_jpeg = true;
    
    current_index_++;
    return true;
}

void ImageFolderSource::seek(int64_t frame_num) {
    current_index_ = std::max(0L, std::min(frame_num, 
        static_cast<int64_t>(image_paths_.size()) - 1));
}

void ImageFolderSource::close() {
    image_paths_.clear();
    current_index_ = 0;
    is_open_ = false;
}

// ============================================================
// SingleImageSource 实现
// ============================================================
SingleImageSource::SingleImageSource() = default;
SingleImageSource::~SingleImageSource() { close(); }

bool SingleImageSource::open(const std::string& source) {
    if (!fs::exists(source)) {
        fprintf(stderr, "[SingleImageSource] File not found: %s\n", source.c_str());
        return false;
    }
    
    // 读取图片文件
    std::ifstream file(source, std::ios::binary | std::ios::ate);
    if (!file) {
        fprintf(stderr, "[SingleImageSource] Failed to open: %s\n", source.c_str());
        return false;
    }
    
    std::streamsize size = file.tellg();
    file.seekg(0, std::ios::beg);
    
    cached_frame_.data.resize(size);
    if (!file.read(reinterpret_cast<char*>(cached_frame_.data.data()), size)) {
        fprintf(stderr, "[SingleImageSource] Read error: %s\n", source.c_str());
        return false;
    }
    
    // 使用 OpenCV 获取尺寸
    cv::Mat img = cv::imread(source);
    if (img.empty()) {
        fprintf(stderr, "[SingleImageSource] Failed to decode: %s\n", source.c_str());
        return false;
    }
    
    width_ = img.cols;
    height_ = img.rows;
    cached_frame_.width = width_;
    cached_frame_.height = height_;
    cached_frame_.channels = img.channels();
    cached_frame_.is_jpeg = true;
    
    current_read_ = 0;
    is_open_ = true;
    
    // 静默打开
    return true;
}

bool SingleImageSource::read(Frame& frame) {
    if (!is_open_) return false;
    
    if (current_read_ >= repeat_count_) {
        return false;
    }
    
    frame = cached_frame_;
    frame.timestamp_us = now_us();
    frame.frame_id = current_read_;
    
    current_read_++;
    return true;
}

void SingleImageSource::close() {
    cached_frame_.data.clear();
    current_read_ = 0;
    is_open_ = false;
}

// ============================================================
// PrefetchDataSource 实现
// ============================================================
PrefetchDataSource::PrefetchDataSource(std::unique_ptr<DataSource> source, int prefetch_size)
    : source_(std::move(source)), prefetch_size_(prefetch_size) {}

PrefetchDataSource::~PrefetchDataSource() {
    close();
}

bool PrefetchDataSource::open(const std::string& source) {
    if (!source_->open(source)) {
        return false;
    }
    
    running_ = true;
    prefetch_thread_ = std::thread(&PrefetchDataSource::prefetch_thread, this);
    
    return true;
}

bool PrefetchDataSource::read(Frame& frame) {
    std::unique_lock<std::mutex> lock(mutex_);
    
    cv_not_empty_.wait(lock, [this]() {
        return !buffer_.empty() || eof_ || !running_;
    });
    
    if (buffer_.empty()) {
        return false;
    }
    
    frame = std::move(buffer_.front());
    buffer_.pop();
    
    cv_not_full_.notify_one();
    return true;
}

void PrefetchDataSource::close() {
    running_ = false;
    cv_not_full_.notify_all();
    cv_not_empty_.notify_all();
    
    if (prefetch_thread_.joinable()) {
        prefetch_thread_.join();
    }
    
    source_->close();
}

void PrefetchDataSource::prefetch_thread() {
    Frame frame;
    
    while (running_) {
        if (!source_->read(frame)) {
            eof_ = true;
            cv_not_empty_.notify_all();
            break;
        }
        
        {
            std::unique_lock<std::mutex> lock(mutex_);
            
            cv_not_full_.wait(lock, [this]() {
                return buffer_.size() < static_cast<size_t>(prefetch_size_) || !running_;
            });
            
            if (!running_) break;
            
            buffer_.push(std::move(frame));
            cv_not_empty_.notify_one();
        }
    }
}

}  // namespace benchmark
}  // namespace rivision
