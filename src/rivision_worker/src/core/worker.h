#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <memory>
#include <atomic>
#include <thread>
#include <chrono>
#include <mutex>

namespace rivision::core {

// Forward declarations
class StreamManager;
class ConfigManager;
class RulesEngine;
class AnalyticsCollector;
class SearchEngine;
class NotifyDispatcher;
class VlmClient;

} // namespace rivision::core

namespace rivision::storage {
class MetaDB;
class EventCache;
class VectorStore;
class StreamStore;
}

namespace rivision::hub {
class HubClient;
class EventPublisher;
}

namespace rivision::api {
class HttpServer;
}

namespace rivision::pipeline {
class PipelineManager;
class YoloService;
}

namespace rivision::core {

// =============================================================================
// Worker - Main application class
// =============================================================================

class Worker {
public:
    Worker();
    ~Worker();
    
    // Non-copyable
    Worker(const Worker&) = delete;
    Worker& operator=(const Worker&) = delete;
    
    // Initialize with configuration
    bool init(const WorkerConfig& config);
    
    // Start all services
    bool start();
    
    // Stop all services (graceful shutdown)
    void stop();
    
    // Check if running
    bool isRunning() const { return running_.load(); }
    
    // Get health status
    HealthStatus getHealth() const;
    
    // Get uptime in seconds
    int64_t getUptimeSeconds() const;
    
    // Get configuration
    const WorkerConfig& config() const { return config_; }
    
    // Access components (for API handlers)
    StreamManager* streamManager() { return stream_mgr_.get(); }
    RulesEngine* rulesEngine() { return rules_engine_.get(); }
    AnalyticsCollector* analytics() { return analytics_.get(); }
    SearchEngine* searchEngine() { return search_engine_.get(); }
    storage::MetaDB* metaDB() { return meta_db_.get(); }
    storage::VectorStore* vectorStore() { return vector_store_.get(); }
    hub::EventPublisher* eventPublisher() { return event_publisher_.get(); }
    pipeline::YoloService* yoloService() { return yolo_service_.get(); }
    VlmClient* vlmClient() { return vlm_client_.get(); }
    
private:
    // Initialization steps
    bool initStorage();
    bool initHAL();
    bool initYoloService();
    bool initPipeline();
    bool initBusinessModules();
    bool initVlmClient();
    bool initHttpServer();
    bool initHubClient();
    
    // Background tasks
    void runCleanupTask();
    void runStatsAggregationTask();
    
    // Detection callback (from pipeline)
    void onDetection(const StreamId& stream_id,
                    const std::vector<Detection>& detections,
                    const std::vector<Track>& tracks);
    
    // Configuration
    WorkerConfig config_;
    YoloConfig yolo_config_;
    NotifyConfig notify_config_;
    std::vector<RuleConfig> rules_config_;
    
    // Helper to check embed service health (const because it updates mutable cache)
    void checkEmbedHealth() const;
    
    // State
    std::atomic<bool> running_{false};
    std::chrono::steady_clock::time_point start_time_;
    int degradation_level_ = 0;
    std::string degradation_reason_;
    
    // Embed service status (cached, mutable for const getHealth)
    struct EmbedStatus {
        bool connected = false;
        std::string model;
        std::chrono::steady_clock::time_point last_check;
    };
    mutable EmbedStatus embed_status_;
    mutable std::mutex embed_mutex_;
    
    // Components (owned)
    std::unique_ptr<storage::MetaDB> meta_db_;
    std::unique_ptr<storage::EventCache> event_cache_;
    std::unique_ptr<storage::VectorStore> vector_store_;
    std::unique_ptr<storage::StreamStore> stream_store_;
    
    std::unique_ptr<pipeline::YoloService> yolo_service_;
    std::unique_ptr<pipeline::PipelineManager> pipeline_mgr_;
    
    std::unique_ptr<StreamManager> stream_mgr_;
    std::unique_ptr<ConfigManager> config_mgr_;
    std::unique_ptr<RulesEngine> rules_engine_;
    std::unique_ptr<AnalyticsCollector> analytics_;
    std::unique_ptr<SearchEngine> search_engine_;
    std::unique_ptr<NotifyDispatcher> notify_dispatcher_;
    std::unique_ptr<VlmClient> vlm_client_;
    
    std::unique_ptr<hub::HubClient> hub_client_;
    std::unique_ptr<hub::EventPublisher> event_publisher_;
    
    std::unique_ptr<api::HttpServer> http_server_;
    
    // Background threads
    std::thread cleanup_thread_;
    std::thread stats_thread_;
    std::atomic<bool> stop_background_{false};
};

} // namespace rivision::core
