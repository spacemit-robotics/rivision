#pragma once

#include "rivision/types.h"
#include <string>
#include <functional>
#include <vector>

namespace rivision::api {

struct SearchRequest {
    std::string query;
    std::string image_base64;
    std::vector<float> embedding;
    int top_k = 10;
    float min_score = 0.5f;
    std::vector<std::string> camera_ids;
    Timestamp start_ts = 0;
    Timestamp end_ts = 0;
};

class SearchHandler {
public:
    using SearchCallback = std::function<std::vector<SearchResult>(const SearchRequest&)>;
    using TextSearchCallback = std::function<std::vector<SearchResult>(const std::string&, int)>;
    using ImageSearchCallback = std::function<std::vector<SearchResult>(const std::vector<float>&, int)>;
    
    SearchHandler();
    ~SearchHandler();
    
    void setSearchCallback(SearchCallback cb) { search_callback_ = std::move(cb); }
    void setTextSearchCallback(TextSearchCallback cb) { text_search_callback_ = std::move(cb); }
    void setImageSearchCallback(ImageSearchCallback cb) { image_search_callback_ = std::move(cb); }
    
    std::string handleSearch(const std::string& body);
    
    std::string handleTextSearch(const std::string& body);
    
    std::string handleImageSearch(const std::string& body);
    
    std::string handleHybridSearch(const std::string& body);
    
private:
    SearchRequest parseRequest(const std::string& json);
    std::string resultsToJson(const std::vector<SearchResult>& results);
    
    SearchCallback search_callback_;
    TextSearchCallback text_search_callback_;
    ImageSearchCallback image_search_callback_;
};

} // namespace rivision::api
