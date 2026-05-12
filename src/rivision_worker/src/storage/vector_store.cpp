#include "vector_store.h"
#include "utils/logger.h"

#include <fstream>
#include <algorithm>
#include <cmath>
#include <numeric>

namespace rivision::storage {

std::unique_ptr<VectorStore> VectorStore::create(const Config& cfg) {
    return std::make_unique<InMemoryVectorStore>(cfg);
}

InMemoryVectorStore::InMemoryVectorStore(const Config& cfg)
    : config_(cfg) {
    vectors_.reserve(cfg.max_vectors);
}

InMemoryVectorStore::~InMemoryVectorStore() = default;

bool InMemoryVectorStore::insert(const std::string& id,
                                 const std::vector<float>& embedding,
                                 const VectorMeta& meta) {
    if (embedding.size() != static_cast<size_t>(config_.embedding_dim)) {
        LOG_WARN("Embedding dimension mismatch: expected {}, got {}", 
                config_.embedding_dim, embedding.size());
        return false;
    }
    
    std::unique_lock lock(mutex_);
    
    // Check if ID exists
    auto it = id_to_index_.find(id);
    if (it != id_to_index_.end()) {
        // Update existing
        vectors_[it->second].embedding = embedding;
        vectors_[it->second].meta = meta;
        vectors_[it->second].insert_ts = nowMs();
        return true;
    }
    
    // Evict if needed
    evictIfNeeded();
    
    // Normalize embedding
    std::vector<float> normalized = embedding;
    normalize(normalized);
    
    // Insert new
    Entry entry;
    entry.id = id;
    entry.embedding = std::move(normalized);
    entry.meta = meta;
    entry.insert_ts = nowMs();
    
    size_t index = vectors_.size();
    vectors_.push_back(std::move(entry));
    id_to_index_[id] = index;
    
    return true;
}

std::vector<VectorStore::SearchResult> InMemoryVectorStore::search(
    const std::vector<float>& query,
    int top_k,
    float min_score) {
    
    if (query.size() != static_cast<size_t>(config_.embedding_dim)) {
        return {};
    }
    
    // Normalize query
    std::vector<float> normalized_query = query;
    normalize(normalized_query);
    
    std::shared_lock lock(mutex_);
    
    // Compute scores for all vectors
    std::vector<std::pair<float, size_t>> scores;
    scores.reserve(vectors_.size());
    
    for (size_t i = 0; i < vectors_.size(); ++i) {
        float score = cosineSimilarity(normalized_query, vectors_[i].embedding);
        if (score >= min_score) {
            scores.emplace_back(score, i);
        }
    }
    
    // Sort by score descending
    std::partial_sort(scores.begin(),
                     scores.begin() + std::min(static_cast<size_t>(top_k), scores.size()),
                     scores.end(),
                     [](const auto& a, const auto& b) { return a.first > b.first; });
    
    // Build results
    std::vector<SearchResult> results;
    results.reserve(std::min(static_cast<size_t>(top_k), scores.size()));
    
    for (size_t i = 0; i < std::min(static_cast<size_t>(top_k), scores.size()); ++i) {
        const auto& entry = vectors_[scores[i].second];
        SearchResult result;
        result.id = entry.id;
        result.score = scores[i].first;
        result.meta = entry.meta;
        results.push_back(std::move(result));
    }
    
    return results;
}

std::optional<std::pair<std::vector<float>, VectorMeta>> InMemoryVectorStore::get(
    const std::string& id) {
    
    std::shared_lock lock(mutex_);
    
    auto it = id_to_index_.find(id);
    if (it == id_to_index_.end()) {
        return std::nullopt;
    }
    
    const auto& entry = vectors_[it->second];
    return std::make_pair(entry.embedding, entry.meta);
}

bool InMemoryVectorStore::remove(const std::string& id) {
    std::unique_lock lock(mutex_);
    
    auto it = id_to_index_.find(id);
    if (it == id_to_index_.end()) {
        return false;
    }
    
    size_t index = it->second;
    
    // Swap with last element and remove
    if (index != vectors_.size() - 1) {
        std::swap(vectors_[index], vectors_.back());
        id_to_index_[vectors_[index].id] = index;
    }
    
    vectors_.pop_back();
    id_to_index_.erase(it);
    
    return true;
}

size_t InMemoryVectorStore::size() const {
    std::shared_lock lock(mutex_);
    return vectors_.size();
}

bool InMemoryVectorStore::save() {
    if (config_.persist_path.empty()) {
        return true;
    }
    
    std::shared_lock lock(mutex_);
    
    std::ofstream file(config_.persist_path, std::ios::binary);
    if (!file.is_open()) {
        LOG_ERROR("Failed to open file for writing: {}", config_.persist_path);
        return false;
    }
    
    // Write header
    uint32_t magic = 0x56454353;  // "VECS"
    uint32_t version = 1;
    uint32_t dim = config_.embedding_dim;
    uint64_t count = vectors_.size();
    
    file.write(reinterpret_cast<const char*>(&magic), sizeof(magic));
    file.write(reinterpret_cast<const char*>(&version), sizeof(version));
    file.write(reinterpret_cast<const char*>(&dim), sizeof(dim));
    file.write(reinterpret_cast<const char*>(&count), sizeof(count));
    
    // Write entries
    for (const auto& entry : vectors_) {
        // ID length + ID
        uint32_t id_len = entry.id.size();
        file.write(reinterpret_cast<const char*>(&id_len), sizeof(id_len));
        file.write(entry.id.data(), id_len);
        
        // Embedding
        file.write(reinterpret_cast<const char*>(entry.embedding.data()),
                  entry.embedding.size() * sizeof(float));
        
        // Metadata
        uint32_t meta_id_len = entry.meta.id.size();
        file.write(reinterpret_cast<const char*>(&meta_id_len), sizeof(meta_id_len));
        file.write(entry.meta.id.data(), meta_id_len);
        
        file.write(reinterpret_cast<const char*>(&entry.meta.timestamp), sizeof(entry.meta.timestamp));
        
        uint32_t camera_len = entry.meta.camera_id.size();
        file.write(reinterpret_cast<const char*>(&camera_len), sizeof(camera_len));
        file.write(entry.meta.camera_id.data(), camera_len);
        
        uint32_t class_len = entry.meta.class_name.size();
        file.write(reinterpret_cast<const char*>(&class_len), sizeof(class_len));
        file.write(entry.meta.class_name.data(), class_len);
        
        file.write(reinterpret_cast<const char*>(&entry.meta.confidence), sizeof(entry.meta.confidence));
    }
    
    LOG_INFO("Saved {} vectors to {}", count, config_.persist_path);
    return true;
}

bool InMemoryVectorStore::load() {
    if (config_.persist_path.empty()) {
        return true;
    }
    
    std::ifstream file(config_.persist_path, std::ios::binary);
    if (!file.is_open()) {
        LOG_WARN("Vector store file not found: {}", config_.persist_path);
        return false;
    }
    
    // Read header
    uint32_t magic, version, dim;
    uint64_t count;
    
    file.read(reinterpret_cast<char*>(&magic), sizeof(magic));
    file.read(reinterpret_cast<char*>(&version), sizeof(version));
    file.read(reinterpret_cast<char*>(&dim), sizeof(dim));
    file.read(reinterpret_cast<char*>(&count), sizeof(count));
    
    if (magic != 0x56454353) {
        LOG_ERROR("Invalid vector store file format");
        return false;
    }
    
    if (dim != static_cast<uint32_t>(config_.embedding_dim)) {
        LOG_ERROR("Embedding dimension mismatch: file has {}, expected {}", dim, config_.embedding_dim);
        return false;
    }
    
    std::unique_lock lock(mutex_);
    vectors_.clear();
    id_to_index_.clear();
    vectors_.reserve(count);
    
    // Read entries
    for (uint64_t i = 0; i < count; ++i) {
        Entry entry;
        
        // ID
        uint32_t id_len;
        file.read(reinterpret_cast<char*>(&id_len), sizeof(id_len));
        entry.id.resize(id_len);
        file.read(entry.id.data(), id_len);
        
        // Embedding
        entry.embedding.resize(dim);
        file.read(reinterpret_cast<char*>(entry.embedding.data()), dim * sizeof(float));
        
        // Metadata
        uint32_t meta_id_len;
        file.read(reinterpret_cast<char*>(&meta_id_len), sizeof(meta_id_len));
        entry.meta.id.resize(meta_id_len);
        file.read(entry.meta.id.data(), meta_id_len);
        
        file.read(reinterpret_cast<char*>(&entry.meta.timestamp), sizeof(entry.meta.timestamp));
        
        uint32_t camera_len;
        file.read(reinterpret_cast<char*>(&camera_len), sizeof(camera_len));
        entry.meta.camera_id.resize(camera_len);
        file.read(entry.meta.camera_id.data(), camera_len);
        
        uint32_t class_len;
        file.read(reinterpret_cast<char*>(&class_len), sizeof(class_len));
        entry.meta.class_name.resize(class_len);
        file.read(entry.meta.class_name.data(), class_len);
        
        file.read(reinterpret_cast<char*>(&entry.meta.confidence), sizeof(entry.meta.confidence));
        
        entry.insert_ts = nowMs();
        
        id_to_index_[entry.id] = vectors_.size();
        vectors_.push_back(std::move(entry));
    }
    
    LOG_INFO("Loaded {} vectors from {}", count, config_.persist_path);
    return true;
}

void InMemoryVectorStore::evictIfNeeded() {
    // Requires lock to be held
    if (vectors_.size() < static_cast<size_t>(config_.max_vectors)) {
        return;
    }
    
    // Find oldest entry
    size_t oldest_idx = 0;
    Timestamp oldest_ts = vectors_[0].insert_ts;
    
    for (size_t i = 1; i < vectors_.size(); ++i) {
        if (vectors_[i].insert_ts < oldest_ts) {
            oldest_ts = vectors_[i].insert_ts;
            oldest_idx = i;
        }
    }
    
    // Remove oldest
    std::string oldest_id = vectors_[oldest_idx].id;
    
    if (oldest_idx != vectors_.size() - 1) {
        std::swap(vectors_[oldest_idx], vectors_.back());
        id_to_index_[vectors_[oldest_idx].id] = oldest_idx;
    }
    
    vectors_.pop_back();
    id_to_index_.erase(oldest_id);
    
    LOG_DEBUG("Evicted vector: {}", oldest_id);
}

float InMemoryVectorStore::cosineSimilarity(const std::vector<float>& a,
                                            const std::vector<float>& b) const {
    // Assumes vectors are normalized
    float dot = 0.0f;
    for (size_t i = 0; i < a.size(); ++i) {
        dot += a[i] * b[i];
    }
    return dot;
}

void InMemoryVectorStore::normalize(std::vector<float>& v) const {
    float norm = 0.0f;
    for (float x : v) {
        norm += x * x;
    }
    norm = std::sqrt(norm);
    
    if (norm > 1e-6f) {
        for (float& x : v) {
            x /= norm;
        }
    }
}

} // namespace rivision::storage
