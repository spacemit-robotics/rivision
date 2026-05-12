// Snapshot Manager implementation
#include "snapshot.h"
#include "pipeline/stream_context.h"
#include "utils/logger.h"

#include <opencv2/opencv.hpp>
#include <filesystem>
#include <fstream>
#include <chrono>
#include <sstream>
#include <iomanip>
#include <cstring>

namespace rivision::pipeline {

namespace fs = std::filesystem;

SnapshotManager::SnapshotManager(const SnapshotConfig& cfg)
    : config_(cfg) {
    // Ensure save path exists
    try {
        fs::create_directories(config_.save_path);
    } catch (const std::exception& e) {
        LOG_ERROR("Failed to create snapshot directory {}: {}", config_.save_path, e.what());
    }
}

SnapshotManager::~SnapshotManager() = default;

SnapshotResult SnapshotManager::capture(const Frame& frame, const std::string& camera_id) {
    SnapshotResult result;
    result.timestamp_ms = std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::system_clock::now().time_since_epoch()
    ).count();
    
    // Encode full image
    if (!encodeJpeg(frame, result.jpeg_data)) {
        result.error = "Failed to encode JPEG";
        return result;
    }
    
    // Create thumbnail
    if (!createThumbnail(frame, result.thumbnail_data)) {
        result.error = "Failed to create thumbnail";
        return result;
    }
    
    // Save files
    result.file_path = generatePath(camera_id, ".jpg");
    result.thumbnail_path = generatePath(camera_id, "_thumb.jpg");
    
    if (!saveToFile(result.jpeg_data, result.file_path)) {
        result.error = "Failed to save snapshot";
        return result;
    }
    
    if (!saveToFile(result.thumbnail_data, result.thumbnail_path)) {
        result.error = "Failed to save thumbnail";
        return result;
    }
    
    result.success = true;
    LOG_DEBUG("Snapshot captured: {}", result.file_path);
    return result;
}

bool SnapshotManager::encodeJpeg(const Frame& frame, std::vector<uint8_t>& output, int quality) {
    if (quality < 0) quality = config_.quality;
    
    try {
        cv::Mat img;
        
        // Convert frame to cv::Mat based on format
        if (frame.format == PixelFormat::RGB24) {
            img = cv::Mat(frame.height, frame.width, CV_8UC3, 
                         const_cast<uint8_t*>(frame.cpuData()));
            cv::cvtColor(img, img, cv::COLOR_RGB2BGR);
        } else if (frame.format == PixelFormat::BGR24) {
            img = cv::Mat(frame.height, frame.width, CV_8UC3,
                         const_cast<uint8_t*>(frame.cpuData()));
        } else if (frame.format == PixelFormat::NV12) {
            cv::Mat yuv(frame.height * 3 / 2, frame.width, CV_8UC1,
                       const_cast<uint8_t*>(frame.cpuData()));
            cv::cvtColor(yuv, img, cv::COLOR_YUV2BGR_NV12);
        } else {
            LOG_ERROR("Unsupported pixel format for JPEG encoding");
            return false;
        }
        
        std::vector<int> params = {cv::IMWRITE_JPEG_QUALITY, quality};
        return cv::imencode(".jpg", img, output, params);
        
    } catch (const cv::Exception& e) {
        LOG_ERROR("OpenCV error encoding JPEG: {}", e.what());
        return false;
    }
}

bool SnapshotManager::createThumbnail(const Frame& frame, std::vector<uint8_t>& output) {
    try {
        cv::Mat img;
        
        // Convert frame to BGR
        if (frame.format == PixelFormat::RGB24) {
            img = cv::Mat(frame.height, frame.width, CV_8UC3,
                         const_cast<uint8_t*>(frame.cpuData()));
            cv::cvtColor(img, img, cv::COLOR_RGB2BGR);
        } else if (frame.format == PixelFormat::BGR24) {
            img = cv::Mat(frame.height, frame.width, CV_8UC3,
                         const_cast<uint8_t*>(frame.cpuData()));
        } else if (frame.format == PixelFormat::NV12) {
            cv::Mat yuv(frame.height * 3 / 2, frame.width, CV_8UC1,
                       const_cast<uint8_t*>(frame.cpuData()));
            cv::cvtColor(yuv, img, cv::COLOR_YUV2BGR_NV12);
        } else {
            return false;
        }
        
        // Resize to thumbnail
        cv::Mat thumb;
        cv::resize(img, thumb, cv::Size(config_.thumbnail_width, config_.thumbnail_height));
        
        // Encode
        std::vector<int> params = {cv::IMWRITE_JPEG_QUALITY, 75};
        return cv::imencode(".jpg", thumb, output, params);
        
    } catch (const cv::Exception& e) {
        LOG_ERROR("Error creating thumbnail: {}", e.what());
        return false;
    }
}

bool SnapshotManager::resize(const Frame& src, Frame& dst, int width, int height) {
    try {
        cv::Mat src_mat(src.height, src.width, CV_8UC3,
                       const_cast<uint8_t*>(src.cpuData()));
        cv::Mat dst_mat;
        cv::resize(src_mat, dst_mat, cv::Size(width, height));
        
        dst.width = width;
        dst.height = height;
        dst.format = src.format;
        auto& dst_vec = dst.data.emplace<std::vector<uint8_t>>();
        dst_vec.resize(dst_mat.total() * dst_mat.elemSize());
        std::memcpy(dst_vec.data(), dst_mat.data, dst_vec.size());
        
        return true;
    } catch (const cv::Exception& e) {
        LOG_ERROR("Error resizing frame: {}", e.what());
        return false;
    }
}

bool SnapshotManager::saveToFile(const std::vector<uint8_t>& data, const std::string& path) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    try {
        // Ensure directory exists
        fs::create_directories(fs::path(path).parent_path());
        
        std::ofstream file(path, std::ios::binary);
        if (!file) {
            LOG_ERROR("Cannot open file for writing: {}", path);
            return false;
        }
        
        file.write(reinterpret_cast<const char*>(data.data()), data.size());
        return file.good();
        
    } catch (const std::exception& e) {
        LOG_ERROR("Error saving file {}: {}", path, e.what());
        return false;
    }
}

std::string SnapshotManager::toBase64(const std::vector<uint8_t>& data) {
    static const char* base64_chars =
        "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    
    std::string result;
    result.reserve(((data.size() + 2) / 3) * 4);
    
    size_t i = 0;
    while (i < data.size()) {
        uint32_t octet_a = i < data.size() ? data[i++] : 0;
        uint32_t octet_b = i < data.size() ? data[i++] : 0;
        uint32_t octet_c = i < data.size() ? data[i++] : 0;
        
        uint32_t triple = (octet_a << 16) + (octet_b << 8) + octet_c;
        
        result.push_back(base64_chars[(triple >> 18) & 0x3F]);
        result.push_back(base64_chars[(triple >> 12) & 0x3F]);
        result.push_back(base64_chars[(triple >> 6) & 0x3F]);
        result.push_back(base64_chars[triple & 0x3F]);
    }
    
    // Padding
    size_t mod = data.size() % 3;
    if (mod == 1) {
        result[result.size() - 1] = '=';
        result[result.size() - 2] = '=';
    } else if (mod == 2) {
        result[result.size() - 1] = '=';
    }
    
    return result;
}

SnapshotResult SnapshotManager::getSnapshot(const std::string& snapshot_id) {
    SnapshotResult result;
    std::string path = config_.save_path + "/" + snapshot_id + ".jpg";
    
    try {
        if (!fs::exists(path)) {
            result.error = "Snapshot not found";
            return result;
        }
        
        std::ifstream file(path, std::ios::binary);
        result.jpeg_data = std::vector<uint8_t>(
            std::istreambuf_iterator<char>(file),
            std::istreambuf_iterator<char>()
        );
        
        result.file_path = path;
        result.success = true;
        
    } catch (const std::exception& e) {
        result.error = e.what();
    }
    
    return result;
}

int SnapshotManager::cleanup(int max_age_hours) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    int deleted = 0;
    auto cutoff = std::chrono::system_clock::now() - std::chrono::hours(max_age_hours);
    
    try {
        for (const auto& entry : fs::directory_iterator(config_.save_path)) {
            if (!entry.is_regular_file()) continue;
            
            auto mtime = fs::last_write_time(entry);
            auto mtime_sys = std::chrono::time_point_cast<std::chrono::system_clock::duration>(
                mtime - fs::file_time_type::clock::now() + std::chrono::system_clock::now()
            );
            
            if (mtime_sys < cutoff) {
                fs::remove(entry);
                deleted++;
            }
        }
    } catch (const std::exception& e) {
        LOG_ERROR("Error during cleanup: {}", e.what());
    }
    
    LOG_INFO("Cleaned up {} old snapshots", deleted);
    return deleted;
}

std::string SnapshotManager::generatePath(const std::string& camera_id, const std::string& suffix) {
    auto now = std::chrono::system_clock::now();
    auto time_t = std::chrono::system_clock::to_time_t(now);
    auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
        now.time_since_epoch()
    ).count() % 1000;
    
    std::ostringstream oss;
    oss << config_.save_path << "/"
        << camera_id << "_"
        << std::put_time(std::localtime(&time_t), "%Y%m%d_%H%M%S")
        << "_" << std::setfill('0') << std::setw(3) << ms
        << suffix;
    
    return oss.str();
}

} // namespace rivision::pipeline
