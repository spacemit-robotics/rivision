#include "search_handler.h"
#include <nlohmann/json.hpp>
#include <sstream>

namespace rivision::api {

using json = nlohmann::json;

SearchHandler::SearchHandler() = default;
SearchHandler::~SearchHandler() = default;

std::string SearchHandler::handleSearch(const std::string& body) {
    if (!search_callback_) {
        return R"({"error":"search handler not configured"})";
    }
    
    try {
        auto request = parseRequest(body);
        auto results = search_callback_(request);
        return resultsToJson(results);
        
    } catch (const std::exception& e) {
        return R"({"error":")" + std::string(e.what()) + "\"}";
    }
}

std::string SearchHandler::handleTextSearch(const std::string& body) {
    if (!text_search_callback_) {
        return R"({"error":"text search not configured"})";
    }
    
    try {
        json j = json::parse(body);
        std::string query = j.value("query", "");
        int top_k = j.value("top_k", 10);
        
        auto results = text_search_callback_(query, top_k);
        return resultsToJson(results);
        
    } catch (const std::exception& e) {
        return R"({"error":")" + std::string(e.what()) + "\"}";
    }
}

std::string SearchHandler::handleImageSearch(const std::string& body) {
    if (!image_search_callback_) {
        return R"({"error":"image search not configured"})";
    }
    
    try {
        json j = json::parse(body);
        auto embedding = j.value("embedding", std::vector<float>());
        int top_k = j.value("top_k", 10);
        
        auto results = image_search_callback_(embedding, top_k);
        return resultsToJson(results);
        
    } catch (const std::exception& e) {
        return R"({"error":")" + std::string(e.what()) + "\"}";
    }
}

std::string SearchHandler::handleHybridSearch(const std::string& body) {
    return R"({"error":"hybrid search not implemented"})";
}

SearchRequest SearchHandler::parseRequest(const std::string& jsonStr) {
    json j = json::parse(jsonStr);
    SearchRequest req;
    
    req.query = j.value("query", "");
    req.image_base64 = j.value("image_base64", "");
    req.top_k = j.value("top_k", 10);
    req.min_score = j.value("min_score", 0.5f);
    req.start_ts = j.value("start_ts", 0);
    req.end_ts = j.value("end_ts", 0);
    
    if (j.contains("camera_ids")) {
        req.camera_ids = j["camera_ids"].get<std::vector<std::string>>();
    }
    
    if (j.contains("embedding")) {
        req.embedding = j["embedding"].get<std::vector<float>>();
    }
    
    return req;
}

std::string SearchHandler::resultsToJson(const std::vector<SearchResult>& results) {
    json arr = json::array();
    
    for (const auto& r : results) {
        json item;
        item["id"] = r.id;
        item["score"] = r.score;
        item["timestamp"] = r.timestamp;
        item["camera_id"] = r.camera_id;
        item["thumbnail_path"] = r.thumbnail_path;
        item["class_name"] = r.class_name;
        item["bbox"] = {
            {"x1", r.bbox.x1},
            {"y1", r.bbox.y1},
            {"x2", r.bbox.x2},
            {"y2", r.bbox.y2}
        };
        arr.push_back(item);
    }
    
    json result;
    result["results"] = arr;
    result["count"] = results.size();
    
    return result.dump();
}

} // namespace rivision::api
