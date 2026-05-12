#include "engine.h"
#include "storage/vector_store.h"
#include "storage/meta_db.h"
#include "pipeline/inference/embedder.h"
#include "utils/logger.h"

#include <algorithm>
#include <sstream>
#include <regex>

namespace rivision::core {

SearchEngine::SearchEngine(storage::VectorStore* vector_store, storage::MetaDB* meta_db)
    : vector_store_(vector_store)
    , meta_db_(meta_db) {
}

SearchEngine::~SearchEngine() = default;

void SearchEngine::setEmbedder(pipeline::Embedder* embedder) {
    embedder_ = embedder;
}

std::vector<SearchResult> SearchEngine::searchByText(
    const std::string& query,
    int top_k,
    const std::vector<CameraId>& camera_filter,
    Timestamp start_ts,
    Timestamp end_ts) {
    
    if (!vector_store_) {
        LOG_WARN("Search engine: vector store not initialized");
        return {};
    }
    
    // Embed text query using embedder service
    std::vector<float> query_embedding;
    
    if (embedder_) {
        if (embedder_->embedText(query, query_embedding)) {
            LOG_DEBUG("Text embedding generated: {} dimensions", query_embedding.size());
        } else {
            LOG_WARN("Text embedding failed: {}", embedder_->getLastError());
        }
    }
    
    // If embedding failed, fall back to keyword search only
    if (query_embedding.empty()) {
        LOG_DEBUG("Using keyword-only search for query: {}", query);
        
        // Get all recent vectors and filter by metadata
        auto raw_results = vector_store_->search(std::vector<float>(512, 0.0f), top_k * 5, 0.0f);
        
        std::vector<SearchResult> results;
        for (const auto& r : raw_results) {
            SearchResult sr;
            sr.id = r.id;
            sr.score = 0.0f;  // Will be boosted by keywords
            sr.timestamp = r.meta.timestamp;
            sr.camera_id = r.meta.camera_id;
            sr.class_name = r.meta.class_name;
            sr.thumbnail_path = r.meta.thumbnail_path;
            results.push_back(std::move(sr));
        }
        
        // Apply filters
        results = applyFilters(results, camera_filter, start_ts, end_ts);
        
        // Boost by keywords (this becomes the primary ranking)
        boostByKeywords(results, query);
        
        // Sort and limit
        std::sort(results.begin(), results.end(),
            [](const SearchResult& a, const SearchResult& b) {
                return a.score > b.score;
            });
        
        // Only return results with keyword matches
        results.erase(
            std::remove_if(results.begin(), results.end(),
                [](const SearchResult& r) { return r.score < 0.01f; }),
            results.end());
        
        if (results.size() > static_cast<size_t>(top_k)) {
            results.resize(top_k);
        }
        
        return results;
    }
    
    // Search vectors with embedding
    auto raw_results = vector_store_->search(query_embedding, top_k * 2);
    
    // Convert to SearchResult
    std::vector<SearchResult> results;
    for (const auto& r : raw_results) {
        SearchResult sr;
        sr.id = r.id;
        sr.score = r.score;
        sr.timestamp = r.meta.timestamp;
        sr.camera_id = r.meta.camera_id;
        sr.class_name = r.meta.class_name;
        sr.thumbnail_path = r.meta.thumbnail_path;
        results.push_back(std::move(sr));
    }
    
    // Apply filters
    results = applyFilters(results, camera_filter, start_ts, end_ts);
    
    // Boost by keywords
    boostByKeywords(results, query);
    
    // Sort by score and limit
    std::sort(results.begin(), results.end(),
        [](const SearchResult& a, const SearchResult& b) {
            return a.score > b.score;
        });
    
    if (results.size() > static_cast<size_t>(top_k)) {
        results.resize(top_k);
    }
    
    return results;
}

std::vector<SearchResult> SearchEngine::searchByImage(
    const std::vector<float>& image_embedding,
    int top_k,
    const std::vector<CameraId>& camera_filter,
    Timestamp start_ts,
    Timestamp end_ts) {
    
    if (!vector_store_) {
        return {};
    }
    
    // Search vectors
    auto raw_results = vector_store_->search(image_embedding, top_k * 2);
    
    // Convert to SearchResult
    std::vector<SearchResult> results;
    for (const auto& r : raw_results) {
        SearchResult sr;
        sr.id = r.id;
        sr.score = r.score;
        sr.timestamp = r.meta.timestamp;
        sr.camera_id = r.meta.camera_id;
        sr.class_name = r.meta.class_name;
        sr.thumbnail_path = r.meta.thumbnail_path;
        results.push_back(std::move(sr));
    }
    
    // Apply filters
    results = applyFilters(results, camera_filter, start_ts, end_ts);
    
    // Limit
    if (results.size() > static_cast<size_t>(top_k)) {
        results.resize(top_k);
    }
    
    return results;
}

std::vector<SearchResult> SearchEngine::hybridSearch(
    const std::string& text_query,
    const std::vector<float>& image_embedding,
    float text_weight,
    float image_weight,
    int top_k,
    const std::vector<CameraId>& camera_filter,
    Timestamp start_ts,
    Timestamp end_ts) {
    
    // Get image search results
    auto image_results = searchByImage(image_embedding, top_k * 2, 
                                       camera_filter, start_ts, end_ts);
    
    // Build score map
    std::unordered_map<std::string, float> scores;
    for (const auto& r : image_results) {
        scores[r.id] = r.score * image_weight;
    }
    
    // Apply keyword boost (simplified text search)
    boostByKeywords(image_results, text_query);
    
    // Update scores with text boost
    for (const auto& r : image_results) {
        scores[r.id] = r.score;  // Already boosted
    }
    
    // Sort and return
    std::vector<SearchResult> results = image_results;
    
    std::sort(results.begin(), results.end(),
        [&scores](const SearchResult& a, const SearchResult& b) {
            return scores.at(a.id) > scores.at(b.id);
        });
    
    if (results.size() > static_cast<size_t>(top_k)) {
        results.resize(top_k);
    }
    
    return results;
}

std::vector<SearchResult> SearchEngine::searchFaces(
    const std::vector<float>& face_embedding,
    int top_k,
    float min_similarity) {
    
    if (!vector_store_) {
        return {};
    }
    
    auto raw_results = vector_store_->search(face_embedding, top_k, min_similarity);
    
    std::vector<SearchResult> results;
    for (const auto& r : raw_results) {
        SearchResult sr;
        sr.id = r.id;
        sr.score = r.score;
        sr.timestamp = r.meta.timestamp;
        sr.camera_id = r.meta.camera_id;
        sr.class_name = r.meta.class_name;
        sr.thumbnail_path = r.meta.thumbnail_path;
        results.push_back(std::move(sr));
    }
    
    return results;
}

bool SearchEngine::indexDetection(const Detection& det,
                                  const CameraId& camera_id,
                                  const std::string& thumbnail_path) {
    if (!vector_store_ || !det.embedding) {
        return false;
    }
    
    storage::VectorMeta meta;
    meta.id = det.class_name + "-" + std::to_string(nowMs());
    meta.timestamp = nowMs();
    meta.camera_id = camera_id;
    meta.class_name = det.class_name;
    meta.confidence = det.confidence;
    meta.thumbnail_path = thumbnail_path;
    
    return vector_store_->insert(meta.id, *det.embedding, meta);
}

std::vector<SearchResult> SearchEngine::applyFilters(
    const std::vector<SearchResult>& results,
    const std::vector<CameraId>& camera_filter,
    Timestamp start_ts,
    Timestamp end_ts) {
    
    std::vector<SearchResult> filtered;
    
    for (const auto& r : results) {
        // Camera filter
        if (!camera_filter.empty()) {
            bool found = false;
            for (const auto& cam : camera_filter) {
                if (r.camera_id == cam) {
                    found = true;
                    break;
                }
            }
            if (!found) continue;
        }
        
        // Time filter
        if (start_ts > 0 && r.timestamp < start_ts) continue;
        if (end_ts > 0 && r.timestamp > end_ts) continue;
        
        filtered.push_back(r);
    }
    
    return filtered;
}

void SearchEngine::boostByKeywords(std::vector<SearchResult>& results,
                                   const std::string& query_text) {
    if (query_text.empty()) return;
    
    // Extract keywords (handle CJK and spaces)
    std::vector<std::string> keywords;
    std::string current;
    
    for (char c : query_text) {
        if (c == ' ' || c == '\t' || c == '\n') {
            if (!current.empty()) {
                keywords.push_back(current);
                current.clear();
            }
        } else {
            current += c;
        }
    }
    if (!current.empty()) {
        keywords.push_back(current);
    }
    
    // Boost scores for keyword matches
    for (auto& result : results) {
        float boost = 0.0f;
        
        for (const auto& keyword : keywords) {
            // Check class name match
            if (result.class_name.find(keyword) != std::string::npos) {
                boost += 0.05f;
            }
            // Check camera ID match
            if (result.camera_id.find(keyword) != std::string::npos) {
                boost += 0.05f;
            }
        }
        
        // Cap boost at 0.15
        boost = std::min(boost, 0.15f);
        result.score += boost;
    }
}

} // namespace rivision::core
