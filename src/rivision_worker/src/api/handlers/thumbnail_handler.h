#pragma once

#include <cstdint>
#include <string>
#include <vector>
#include <functional>

namespace rivision::api {

class ThumbnailHandler {
public:
    using GetThumbnailCallback = std::function<std::vector<uint8_t>(const std::string& id)>;
    using GetSnapshotCallback = std::function<std::vector<uint8_t>(const std::string& stream_id)>;
    
    ThumbnailHandler();
    ~ThumbnailHandler();
    
    void setThumbnailsDir(const std::string& dir) { thumbnails_dir_ = dir; }
    void setGetThumbnailCallback(GetThumbnailCallback cb) { get_callback_ = std::move(cb); }
    void setGetSnapshotCallback(GetSnapshotCallback cb) { snapshot_callback_ = std::move(cb); }
    
    std::vector<uint8_t> handleGetThumbnail(const std::string& id);
    
    std::vector<uint8_t> handleGetSnapshot(const std::string& stream_id);
    
    std::string handleGetThumbnailBase64(const std::string& id);
    
private:
    std::string thumbnails_dir_;
    GetThumbnailCallback get_callback_;
    GetSnapshotCallback snapshot_callback_;
};

} // namespace rivision::api
