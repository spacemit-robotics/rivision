// Snapshot Manager - JPEG encoding and thumbnail generation
#pragma once

#include "rivision/types.h"
#include <string>
#include <vector>
#include <memory>
#include <mutex>
#include <functional>

namespace rivision::pipeline {

using Frame = rivision::Frame;

struct SnapshotConfig {
    int quality = 85;           // JPEG quality (1-100)
    int thumbnail_width = 320;  // Thumbnail width
    int thumbnail_height = 180; // Thumbnail height
    std::string save_path = "/opt/rivision/rivision_worker/snapshots";
};

struct SnapshotResult {
    bool success = false;
    std::string file_path;
    std::string thumbnail_path;
    std::vector<uint8_t> jpeg_data;
    std::vector<uint8_t> thumbnail_data;
    int64_t timestamp_ms = 0;
    std::string error;
};

class SnapshotManager {
public:
    explicit SnapshotManager(const SnapshotConfig& cfg);
    ~SnapshotManager();
    
    // Capture snapshot from frame
    SnapshotResult capture(const Frame& frame, const std::string& camera_id);
    
    // Encode frame to JPEG
    bool encodeJpeg(const Frame& frame, std::vector<uint8_t>& output, int quality = -1);
    
    // Create thumbnail from frame
    bool createThumbnail(const Frame& frame, std::vector<uint8_t>& output);
    
    // Resize frame
    bool resize(const Frame& src, Frame& dst, int width, int height);
    
    // Save to file
    bool saveToFile(const std::vector<uint8_t>& data, const std::string& path);
    
    // Base64 encode
    std::string toBase64(const std::vector<uint8_t>& data);
    
    // Get snapshot by ID
    SnapshotResult getSnapshot(const std::string& snapshot_id);
    
    // Delete old snapshots
    int cleanup(int max_age_hours = 24);
    
private:
    SnapshotConfig config_;
    std::mutex mutex_;
    
    std::string generatePath(const std::string& camera_id, const std::string& suffix);
};

} // namespace rivision::pipeline
