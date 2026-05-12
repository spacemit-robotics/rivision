#pragma once

#include "rivision/types.h"
#include <vector>
#include <string>
#include <memory>
#include <unordered_map>
#include <shared_mutex>
#include <optional>

namespace rivision::storage {

// =============================================================================
// VectorMeta - Metadata associated with a vector
// =============================================================================

struct VectorMeta {
    std::string id;
    Timestamp timestamp = 0;
    CameraId camera_id;
    std::string class_name;
    float confidence = 0.0f;
    std::string thumbnail_path;
};

// =============================================================================
// VectorStore - In-memory vector storage with persistence
// =============================================================================

class VectorStore {
public:
    struct Config {
        int max_vectors = 100000;
        int embedding_dim = 512;
        std::string persist_path;
    };
    
    static std::unique_ptr<VectorStore> create(const Config& cfg);
    
    virtual ~VectorStore() = default;
    
    // Insert vector
    virtual bool insert(const std::string& id,
                       const std::vector<float>& embedding,
                       const VectorMeta& meta) = 0;
    
    // Search
    struct SearchResult {
        std::string id;
        float score;
        VectorMeta meta;
    };
    
    virtual std::vector<SearchResult> search(
        const std::vector<float>& query,
        int top_k,
        float min_score = 0.0f) = 0;
    
    // Get by ID
    virtual std::optional<std::pair<std::vector<float>, VectorMeta>> get(
        const std::string& id) = 0;
    
    // Remove
    virtual bool remove(const std::string& id) = 0;
    
    // Statistics
    virtual size_t size() const = 0;
    virtual size_t capacity() const = 0;
    
    // Persistence
    virtual bool save() = 0;
    virtual bool load() = 0;
};

// =============================================================================
// InMemoryVectorStore - Brute-force implementation
// =============================================================================

class InMemoryVectorStore : public VectorStore {
public:
    explicit InMemoryVectorStore(const Config& cfg);
    ~InMemoryVectorStore() override;
    
    bool insert(const std::string& id,
               const std::vector<float>& embedding,
               const VectorMeta& meta) override;
    
    std::vector<SearchResult> search(
        const std::vector<float>& query,
        int top_k,
        float min_score = 0.0f) override;
    
    std::optional<std::pair<std::vector<float>, VectorMeta>> get(
        const std::string& id) override;
    
    bool remove(const std::string& id) override;
    
    size_t size() const override;
    size_t capacity() const override { return config_.max_vectors; }
    
    bool save() override;
    bool load() override;
    
private:
    Config config_;
    
    struct Entry {
        std::string id;
        std::vector<float> embedding;
        VectorMeta meta;
        Timestamp insert_ts;
    };
    
    std::vector<Entry> vectors_;
    std::unordered_map<std::string, size_t> id_to_index_;
    
    mutable std::shared_mutex mutex_;
    
    // LRU eviction
    void evictIfNeeded();
    
    // Cosine similarity
    float cosineSimilarity(const std::vector<float>& a,
                          const std::vector<float>& b) const;
    
    // Normalize vector in-place
    void normalize(std::vector<float>& v) const;
};

} // namespace rivision::storage
