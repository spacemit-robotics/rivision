#include <gtest/gtest.h>
#include "storage/vector_store.h"

using namespace rivision;
using namespace rivision::storage;

class VectorStoreTest : public ::testing::Test {
protected:
    void SetUp() override {
        VectorStore::Config cfg;
        cfg.max_vectors = 1000;
        cfg.embedding_dim = 512;
        cfg.persist_path = "";  // No persistence for tests
        
        store_ = VectorStore::create(cfg);
    }
    
    std::unique_ptr<VectorStore> store_;
    
    std::vector<float> randomVector(int dim) {
        std::vector<float> v(dim);
        for (int i = 0; i < dim; ++i) {
            v[i] = static_cast<float>(rand()) / RAND_MAX;
        }
        return v;
    }
};

TEST_F(VectorStoreTest, InsertAndSearch) {
    VectorMeta meta;
    meta.id = "vec-1";
    meta.camera_id = "cam-1";
    meta.class_name = "person";
    
    auto embedding = randomVector(512);
    
    EXPECT_TRUE(store_->insert("vec-1", embedding, meta));
    EXPECT_EQ(store_->size(), 1);
    
    // Search with same vector should return it
    auto results = store_->search(embedding, 1);
    EXPECT_EQ(results.size(), 1);
    EXPECT_EQ(results[0].id, "vec-1");
    EXPECT_GT(results[0].score, 0.99f);  // Should be very similar
}

TEST_F(VectorStoreTest, MultipleInserts) {
    for (int i = 0; i < 10; ++i) {
        VectorMeta meta;
        meta.id = "vec-" + std::to_string(i);
        meta.camera_id = "cam-1";
        
        auto embedding = randomVector(512);
        store_->insert(meta.id, embedding, meta);
    }
    
    EXPECT_EQ(store_->size(), 10);
}

TEST_F(VectorStoreTest, SearchTopK) {
    // Insert 100 vectors
    std::vector<std::vector<float>> embeddings;
    for (int i = 0; i < 100; ++i) {
        VectorMeta meta;
        meta.id = "vec-" + std::to_string(i);
        
        auto embedding = randomVector(512);
        embeddings.push_back(embedding);
        store_->insert(meta.id, embedding, meta);
    }
    
    // Search for top 5
    auto results = store_->search(embeddings[0], 5);
    EXPECT_EQ(results.size(), 5);
    
    // First result should be the query vector itself
    EXPECT_EQ(results[0].id, "vec-0");
}

TEST_F(VectorStoreTest, MinScoreFilter) {
    VectorMeta meta;
    meta.id = "vec-1";
    
    auto embedding1 = randomVector(512);
    store_->insert("vec-1", embedding1, meta);
    
    // Search with different vector, require high min score
    auto different_embedding = randomVector(512);
    auto results = store_->search(different_embedding, 10, 0.99f);
    
    // Should be empty or very few since random vectors are unlikely to match
    // (this is probabilistic, but with 512 dims, cosine similarity should be low)
    EXPECT_LE(results.size(), 1);
}

TEST_F(VectorStoreTest, Remove) {
    VectorMeta meta;
    meta.id = "vec-1";
    
    auto embedding = randomVector(512);
    store_->insert("vec-1", embedding, meta);
    EXPECT_EQ(store_->size(), 1);
    
    EXPECT_TRUE(store_->remove("vec-1"));
    EXPECT_EQ(store_->size(), 0);
    
    // Remove non-existent
    EXPECT_FALSE(store_->remove("vec-999"));
}

TEST_F(VectorStoreTest, Get) {
    VectorMeta meta;
    meta.id = "vec-1";
    meta.camera_id = "cam-1";
    meta.class_name = "person";
    meta.confidence = 0.95f;
    
    auto embedding = randomVector(512);
    store_->insert("vec-1", embedding, meta);
    
    auto result = store_->get("vec-1");
    EXPECT_TRUE(result.has_value());
    EXPECT_EQ(result->second.camera_id, "cam-1");
    EXPECT_EQ(result->second.class_name, "person");
    
    // Get non-existent
    auto result2 = store_->get("vec-999");
    EXPECT_FALSE(result2.has_value());
}

TEST_F(VectorStoreTest, Capacity) {
    EXPECT_EQ(store_->capacity(), 1000);
}

TEST_F(VectorStoreTest, LRUEviction) {
    // Create store with small capacity
    VectorStore::Config cfg;
    cfg.max_vectors = 5;
    cfg.embedding_dim = 512;
    auto small_store = VectorStore::create(cfg);
    
    // Insert 7 vectors (should evict 2)
    for (int i = 0; i < 7; ++i) {
        VectorMeta meta;
        meta.id = "vec-" + std::to_string(i);
        auto embedding = randomVector(512);
        small_store->insert(meta.id, embedding, meta);
    }
    
    EXPECT_EQ(small_store->size(), 5);
    
    // Oldest vectors should be evicted
    EXPECT_FALSE(small_store->get("vec-0").has_value());
    EXPECT_FALSE(small_store->get("vec-1").has_value());
    EXPECT_TRUE(small_store->get("vec-6").has_value());
}

TEST_F(VectorStoreTest, DimensionMismatch) {
    VectorMeta meta;
    meta.id = "vec-1";
    
    // Wrong dimension
    auto bad_embedding = randomVector(256);
    EXPECT_FALSE(store_->insert("vec-1", bad_embedding, meta));
    EXPECT_EQ(store_->size(), 0);
}

int main(int argc, char** argv) {
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}
