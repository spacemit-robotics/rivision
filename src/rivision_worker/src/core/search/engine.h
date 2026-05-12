#pragma once

#include "rivision/types.h"
#include <memory>
#include <string>
#include <vector>

namespace rivision::storage {
class VectorStore;
class MetaDB;
}

namespace rivision::pipeline {
class Embedder;
}

namespace rivision::core {

// =============================================================================
// SearchEngine - Vector similarity search
// =============================================================================

class SearchEngine {
public:
    SearchEngine(storage::VectorStore* vector_store, storage::MetaDB* meta_db);
    ~SearchEngine();
    
    // Set embedder for text/image encoding
    void setEmbedder(pipeline::Embedder* embedder);
    
    // Search by text query
    std::vector<SearchResult> searchByText(
        const std::string& query,
        int top_k,
        const std::vector<CameraId>& camera_filter = {},
        Timestamp start_ts = 0,
        Timestamp end_ts = 0);
    
    // Search by image
    std::vector<SearchResult> searchByImage(
        const std::vector<float>& image_embedding,
        int top_k,
        const std::vector<CameraId>& camera_filter = {},
        Timestamp start_ts = 0,
        Timestamp end_ts = 0);
    
    // Hybrid search (text + image)
    std::vector<SearchResult> hybridSearch(
        const std::string& text_query,
        const std::vector<float>& image_embedding,
        float text_weight,
        float image_weight,
        int top_k,
        const std::vector<CameraId>& camera_filter = {},
        Timestamp start_ts = 0,
        Timestamp end_ts = 0);
    
    // Face search (G7 feature)
    std::vector<SearchResult> searchFaces(
        const std::vector<float>& face_embedding,
        int top_k,
        float min_similarity = 0.85f);
    
    // Add detection to index
    bool indexDetection(const Detection& det, 
                       const CameraId& camera_id,
                       const std::string& thumbnail_path);
    
private:
    // Apply filters
    std::vector<SearchResult> applyFilters(
        const std::vector<SearchResult>& results,
        const std::vector<CameraId>& camera_filter,
        Timestamp start_ts,
        Timestamp end_ts);
    
    // Boost by keywords
    void boostByKeywords(std::vector<SearchResult>& results,
                        const std::string& query_text);
    
    storage::VectorStore* vector_store_;
    storage::MetaDB* meta_db_;
    pipeline::Embedder* embedder_ = nullptr;
};

} // namespace rivision::core
