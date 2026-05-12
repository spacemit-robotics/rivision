#include "thumbnail_handler.h"
#include <fstream>
#include <filesystem>
#include <sstream>
#include <iomanip>

namespace rivision::api {

namespace fs = std::filesystem;

static const char base64_chars[] =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

static std::string base64_encode(const std::vector<uint8_t>& data) {
    std::string result;
    size_t i = 0;
    uint8_t arr3[3], arr4[4];
    
    for (auto byte : data) {
        arr3[i++] = byte;
        if (i == 3) {
            arr4[0] = (arr3[0] & 0xfc) >> 2;
            arr4[1] = ((arr3[0] & 0x03) << 4) + ((arr3[1] & 0xf0) >> 4);
            arr4[2] = ((arr3[1] & 0x0f) << 2) + ((arr3[2] & 0xc0) >> 6);
            arr4[3] = arr3[2] & 0x3f;
            
            for (int j = 0; j < 4; j++) {
                result += base64_chars[arr4[j]];
            }
            i = 0;
        }
    }
    
    if (i) {
        for (size_t j = i; j < 3; j++) {
            arr3[j] = 0;
        }
        
        arr4[0] = (arr3[0] & 0xfc) >> 2;
        arr4[1] = ((arr3[0] & 0x03) << 4) + ((arr3[1] & 0xf0) >> 4);
        arr4[2] = ((arr3[1] & 0x0f) << 2) + ((arr3[2] & 0xc0) >> 6);
        
        for (size_t j = 0; j < i + 1; j++) {
            result += base64_chars[arr4[j]];
        }
        
        while (i++ < 3) {
            result += '=';
        }
    }
    
    return result;
}

ThumbnailHandler::ThumbnailHandler() = default;
ThumbnailHandler::~ThumbnailHandler() = default;

std::vector<uint8_t> ThumbnailHandler::handleGetThumbnail(const std::string& id) {
    if (get_callback_) {
        return get_callback_(id);
    }
    
    if (thumbnails_dir_.empty()) {
        return {};
    }
    
    fs::path path = fs::path(thumbnails_dir_) / (id + ".jpg");
    if (!fs::exists(path)) {
        return {};
    }
    
    std::ifstream file(path, std::ios::binary);
    if (!file) {
        return {};
    }
    
    file.seekg(0, std::ios::end);
    size_t size = file.tellg();
    file.seekg(0, std::ios::beg);
    
    std::vector<uint8_t> data(size);
    file.read(reinterpret_cast<char*>(data.data()), size);
    
    return data;
}

std::vector<uint8_t> ThumbnailHandler::handleGetSnapshot(const std::string& stream_id) {
    if (snapshot_callback_) {
        return snapshot_callback_(stream_id);
    }
    return {};
}

std::string ThumbnailHandler::handleGetThumbnailBase64(const std::string& id) {
    auto data = handleGetThumbnail(id);
    if (data.empty()) {
        return R"({"error":"thumbnail not found"})";
    }
    
    std::string base64 = base64_encode(data);
    return R"({"data":"data:image/jpeg;base64,)" + base64 + "\"}";
}

} // namespace rivision::api
