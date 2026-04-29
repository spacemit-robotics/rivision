#pragma once

#include "benchmark_common.h"
#include <memory>
#include <queue>
#include <thread>
#include <condition_variable>

namespace rivision {
namespace benchmark {

// ============================================================
// 数据源基类
// ============================================================
class DataSource {
public:
    virtual ~DataSource() = default;
    
    // 打开数据源
    virtual bool open(const std::string& source) = 0;
    
    // 读取单帧 (阻塞)
    virtual bool read(Frame& frame) = 0;
    
    // 批量读取
    virtual bool read_batch(std::vector<Frame>& frames, int count);
    
    // 源信息
    virtual int fps() const = 0;
    virtual int width() const = 0;
    virtual int height() const = 0;
    virtual int64_t total_frames() const = 0;  // -1 for stream
    
    // 控制
    virtual void seek(int64_t /*frame_num*/) {}
    virtual void close() = 0;
    virtual bool is_open() const = 0;
    virtual bool is_stream() const { return total_frames() < 0; }
    
    // 工厂方法
    static std::unique_ptr<DataSource> create(const std::string& type);
};

// ============================================================
// 视频文件数据源
// ============================================================
class VideoFileSource : public DataSource {
public:
    VideoFileSource();
    ~VideoFileSource() override;
    
    bool open(const std::string& source) override;
    bool read(Frame& frame) override;
    void seek(int64_t frame_num) override;
    void close() override;
    
    int fps() const override { return fps_; }
    int width() const override { return width_; }
    int height() const override { return height_; }
    int64_t total_frames() const override { return total_frames_; }
    bool is_open() const override { return is_open_; }
    
    // 视频特有
    double duration_sec() const { return duration_sec_; }
    int64_t current_frame() const { return current_frame_; }

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    int fps_ = 0;
    int width_ = 0;
    int height_ = 0;
    int64_t total_frames_ = 0;
    int64_t current_frame_ = 0;
    double duration_sec_ = 0;
    bool is_open_ = false;
    bool loop_ = true;
};

// ============================================================
// 视频流数据源 (RTSP/HTTP-FLV/V4L2)
// ============================================================
class VideoStreamSource : public DataSource {
public:
    VideoStreamSource();
    ~VideoStreamSource() override;
    
    bool open(const std::string& source) override;
    bool read(Frame& frame) override;
    void close() override;
    
    int fps() const override { return fps_; }
    int width() const override { return width_; }
    int height() const override { return height_; }
    int64_t total_frames() const override { return -1; }  // 流无总帧数
    bool is_open() const override { return is_open_; }
    bool is_stream() const override { return true; }
    
    // 设置缓冲区大小
    void set_buffer_size(int frames) { buffer_size_ = frames; }

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
    
    int fps_ = 0;
    int width_ = 0;
    int height_ = 0;
    int buffer_size_ = 30;
    bool is_open_ = false;
};

// ============================================================
// 图片文件夹数据源
// ============================================================
class ImageFolderSource : public DataSource {
public:
    ImageFolderSource();
    ~ImageFolderSource() override;
    
    bool open(const std::string& source) override;
    bool read(Frame& frame) override;
    void seek(int64_t frame_num) override;
    void close() override;
    
    int fps() const override { return fps_; }
    int width() const override { return width_; }
    int height() const override { return height_; }
    int64_t total_frames() const override { return static_cast<int64_t>(image_paths_.size()); }
    bool is_open() const override { return is_open_; }
    
    // 设置帧率 (模拟视频)
    void set_fps(int fps) { fps_ = fps; }
    void set_loop(bool loop) { loop_ = loop; }
    void set_shuffle(bool shuffle) { shuffle_ = shuffle; }

private:
    std::vector<std::string> image_paths_;
    int64_t current_index_ = 0;
    int fps_ = 30;
    int width_ = 0;
    int height_ = 0;
    bool is_open_ = false;
    bool loop_ = true;
    bool shuffle_ = false;
};

// ============================================================
// 单图片数据源 (用于单帧测试)
// ============================================================
class SingleImageSource : public DataSource {
public:
    SingleImageSource();
    ~SingleImageSource() override;
    
    bool open(const std::string& source) override;
    bool read(Frame& frame) override;
    void seek(int64_t /*frame_num*/) override { current_read_ = 0; }
    void close() override;
    
    int fps() const override { return 1; }
    int width() const override { return width_; }
    int height() const override { return height_; }
    int64_t total_frames() const override { return repeat_count_; }
    bool is_open() const override { return is_open_; }
    
    // 设置重复次数
    void set_repeat_count(int count) { repeat_count_ = count; }

private:
    Frame cached_frame_;
    int width_ = 0;
    int height_ = 0;
    int repeat_count_ = 1;
    int64_t current_read_ = 0;
    bool is_open_ = false;
};

// ============================================================
// 带预取的数据源包装器
// ============================================================
class PrefetchDataSource : public DataSource {
public:
    PrefetchDataSource(std::unique_ptr<DataSource> source, int prefetch_size = 10);
    ~PrefetchDataSource() override;
    
    bool open(const std::string& source) override;
    bool read(Frame& frame) override;
    void close() override;
    
    int fps() const override { return source_->fps(); }
    int width() const override { return source_->width(); }
    int height() const override { return source_->height(); }
    int64_t total_frames() const override { return source_->total_frames(); }
    bool is_open() const override { return source_->is_open(); }

private:
    void prefetch_thread();
    
    std::unique_ptr<DataSource> source_;
    std::queue<Frame> buffer_;
    std::mutex mutex_;
    std::condition_variable cv_not_empty_;
    std::condition_variable cv_not_full_;
    std::thread prefetch_thread_;
    
    int prefetch_size_;
    std::atomic<bool> running_{false};
    std::atomic<bool> eof_{false};
};

}  // namespace benchmark
}  // namespace rivision
