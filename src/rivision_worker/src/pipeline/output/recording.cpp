#include "recording.h"
#include "hal/interface/i_mux.h"
#include "hal/interface/i_encoder.h"
#include "hal/factory.h"
#include "utils/logger.h"

#include <filesystem>
#include <fstream>
#include <sstream>
#include <iomanip>
#include <random>

namespace fs = std::filesystem;

namespace rivision::pipeline {

// =============================================================================
// FrameRingBuffer
// =============================================================================

FrameRingBuffer::FrameRingBuffer(size_t capacity, int fps)
    : buffer_(capacity)
    , fps_(fps) {
}

void FrameRingBuffer::push(const Frame& frame) {
    buffer_.push(frame);
}

std::vector<Frame> FrameRingBuffer::getRange(Timestamp start_ts, Timestamp end_ts) const {
    auto all = buffer_.getAll();
    
    std::vector<Frame> result;
    for (const auto& f : all) {
        if (f.timestamp >= start_ts && f.timestamp <= end_ts) {
            result.push_back(f);
        }
    }
    
    return result;
}

std::vector<Frame> FrameRingBuffer::getLastSeconds(int seconds) const {
    size_t n = seconds * fps_;
    return buffer_.getLast(n);
}

bool FrameRingBuffer::getLatest(Frame& frame) const {
    auto last = buffer_.getLast(1);
    if (last.empty()) {
        return false;
    }
    frame = last[0];
    return true;
}

void FrameRingBuffer::clear() {
    buffer_.clear();
}

size_t FrameRingBuffer::size() const {
    return buffer_.size();
}

// =============================================================================
// RecordingManager
// =============================================================================

RecordingManager::RecordingManager(const Config& config)
    : config_(config) {
    
    // Create output directory
    fs::create_directories(config_.output_dir);
}

RecordingManager::~RecordingManager() {
    stop();
}

bool RecordingManager::start() {
    if (running_) {
        return true;
    }
    
    running_ = true;
    worker_thread_ = std::thread(&RecordingManager::recordLoop, this);
    
    LOG_INFO("RecordingManager started");
    return true;
}

void RecordingManager::stop() {
    if (!running_) {
        return;
    }
    
    running_ = false;
    jobs_cv_.notify_all();
    
    if (worker_thread_.joinable()) {
        worker_thread_.join();
    }
    
    LOG_INFO("RecordingManager stopped");
}

void RecordingManager::pushFrame(const StreamId& stream_id, const Frame& frame) {
    std::lock_guard<std::mutex> lock(buffers_mutex_);
    
    auto it = buffers_.find(stream_id);
    if (it == buffers_.end()) {
        // Create new buffer for this stream
        size_t capacity = config_.fps * config_.pre_record_sec;
        buffers_[stream_id] = std::make_unique<FrameRingBuffer>(capacity, config_.fps);
        it = buffers_.find(stream_id);
    }
    
    it->second->push(frame);
}

std::string RecordingManager::triggerRecording(const RecordingRequest& req) {
    // Generate recording ID
    static std::atomic<int> counter{0};
    std::string id = "rec-" + std::to_string(nowMs()) + "-" + std::to_string(counter++);
    
    // Get pre-event frames
    std::vector<Frame> pre_frames;
    {
        std::lock_guard<std::mutex> lock(buffers_mutex_);
        auto it = buffers_.find(req.stream_id);
        if (it != buffers_.end()) {
            pre_frames = it->second->getLastSeconds(req.pre_sec);
        }
    }
    
    // Create job
    RecordingJob job;
    job.id = id;
    job.path = generatePath(req.stream_id, req.alert_id);
    job.stream_id = req.stream_id;
    job.alert_id = req.alert_id;
    job.pre_frames = std::move(pre_frames);
    job.post_sec = req.post_sec;
    job.start_ts = nowMs();
    
    // Add to queue
    {
        std::lock_guard<std::mutex> lock(jobs_mutex_);
        jobs_.push(std::move(job));
    }
    jobs_cv_.notify_one();
    
    LOG_INFO("Recording triggered: {} -> {}", id, job.path);
    return id;
}

std::string RecordingManager::getRecordingPath(const std::string& recording_id) const {
    std::lock_guard<std::mutex> lock(active_mutex_);
    auto it = active_recordings_.find(recording_id);
    if (it != active_recordings_.end()) {
        return it->second;
    }
    return "";
}

int RecordingManager::activeCount() const {
    std::lock_guard<std::mutex> lock(active_mutex_);
    return active_recordings_.size();
}

void RecordingManager::recordLoop() {
    LOG_DEBUG("Recording loop started");
    
    while (running_) {
        RecordingJob job;
        
        // Wait for job
        {
            std::unique_lock<std::mutex> lock(jobs_mutex_);
            jobs_cv_.wait(lock, [this] {
                return !jobs_.empty() || !running_;
            });
            
            if (!running_ && jobs_.empty()) {
                break;
            }
            
            if (jobs_.empty()) {
                continue;
            }
            
            job = std::move(jobs_.front());
            jobs_.pop();
        }
        
        // Track active recording
        {
            std::lock_guard<std::mutex> lock(active_mutex_);
            active_recordings_[job.id] = job.path;
        }
        
        LOG_DEBUG("Processing recording: {}", job.id);
        
        // TODO: Actually encode and write the video file
        // For now, just simulate the recording
        
        // Write pre-frames
        // ... encoder->encode(frame) ...
        
        // Wait for post-event duration
        std::this_thread::sleep_for(std::chrono::seconds(job.post_sec));
        
        // Remove from active
        {
            std::lock_guard<std::mutex> lock(active_mutex_);
            active_recordings_.erase(job.id);
        }
        
        LOG_INFO("Recording complete: {} ({} pre-frames)", job.id, job.pre_frames.size());
    }
    
    LOG_DEBUG("Recording loop ended");
}

std::string RecordingManager::generatePath(const StreamId& stream_id, const AlertId& alert_id) {
    auto now = std::chrono::system_clock::now();
    auto time = std::chrono::system_clock::to_time_t(now);
    std::tm tm = *std::localtime(&time);
    
    std::ostringstream oss;
    oss << config_.output_dir << "/"
        << std::put_time(&tm, "%Y%m%d") << "/"
        << stream_id << "_"
        << std::put_time(&tm, "%H%M%S") << "_"
        << alert_id << ".mp4";
    
    // Create date directory
    fs::create_directories(fs::path(oss.str()).parent_path());
    
    return oss.str();
}

} // namespace rivision::pipeline
