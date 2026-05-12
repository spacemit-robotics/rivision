#include "worker.h"
#include "stream_manager.h"
#include "config_manager.h"
#include "notify_dispatcher.h"
#include "rules/engine.h"
#include "analytics/collector.h"
#include "search/engine.h"

#include "storage/meta_db.h"
#include "storage/event_cache.h"
#include "storage/vector_store.h"
#include "storage/stream_store.h"

#include "pipeline/pipeline_manager.h"
#include "pipeline/inference/yolo_service.h"

#include "hub/hub_client.h"
#include "hub/event_publisher.h"

#include "vlm/vlm_client.h"

#include "api/server.h"
#include "utils/logger.h"
#include "utils/system_monitor.h"
#include "rivision/export.h"

#include <filesystem>
#include <regex>
#include <mutex>
#include <ifaddrs.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <curl/curl.h>

namespace fs = std::filesystem;

namespace rivision::core {

// Get local IP that can reach the Hub
static std::string getLocalIpForHub() {
    struct ifaddrs *ifaddr, *ifa;
    std::string result = "127.0.0.1";  // fallback
    
    if (getifaddrs(&ifaddr) == -1) {
        return result;
    }
    
    for (ifa = ifaddr; ifa != nullptr; ifa = ifa->ifa_next) {
        if (ifa->ifa_addr == nullptr) continue;
        
        // Only IPv4
        if (ifa->ifa_addr->sa_family != AF_INET) continue;
        
        // Skip loopback
        std::string name = ifa->ifa_name;
        if (name == "lo") continue;
        
        auto *addr = reinterpret_cast<struct sockaddr_in*>(ifa->ifa_addr);
        char ip[INET_ADDRSTRLEN];
        inet_ntop(AF_INET, &addr->sin_addr, ip, INET_ADDRSTRLEN);
        
        // Prefer non-docker/virtual interfaces
        if (name.find("docker") == std::string::npos && 
            name.find("veth") == std::string::npos &&
            name.find("br-") == std::string::npos) {
            result = ip;
            break;  // Take first non-virtual interface
        }
        
        // Keep as fallback if we haven't found a better one
        if (result == "127.0.0.1") {
            result = ip;
        }
    }
    
    freeifaddrs(ifaddr);
    LOG_INFO("Auto-detected local IP: {}", result);
    return result;
}

Worker::Worker() = default;

Worker::~Worker() {
    if (running_) {
        stop();
    }
}

bool Worker::init(const WorkerConfig& config) {
    config_ = config;
    
    LOG_INFO("Initializing worker, node_id={}", config_.node_id);
    
    // Step 1: Create data directories
    try {
        fs::create_directories(config_.storage.data_dir);
        fs::create_directories(config_.storage.recordings_dir);
        fs::create_directories(config_.storage.thumbnails_dir);
        fs::create_directories(config_.storage.vectors_dir);
    } catch (const std::exception& e) {
        LOG_ERROR("Failed to create directories: {}", e.what());
        return false;
    }
    
    // Step 2: Initialize storage
    if (!initStorage()) {
        LOG_ERROR("Failed to initialize storage");
        return false;
    }
    
    // Step 3: Load YOLO config
    LOG_INFO("Loading YOLO config from: {}", config_.inference.yolo_config);
    utils::Logger::flush();
    try {
        yolo_config_ = YoloConfig::load(config_.inference.yolo_config);
        LOG_INFO("YOLO model path resolved to: {}", yolo_config_.model.path);
        utils::Logger::flush();
        yolo_config_.initDefaultClasses();
    } catch (const std::exception& e) {
        LOG_WARN("Failed to load YOLO config, using defaults: {}", e.what());
        yolo_config_.initDefaultClasses();
    }
    
    // Step 4: Initialize HAL (platform detection)
    if (!initHAL()) {
        LOG_ERROR("Failed to initialize HAL");
        return false;
    }
    
    // Step 5: Initialize YoloService
    if (!initYoloService()) {
        LOG_ERROR("Failed to initialize YoloService");
        return false;
    }
    
    // Step 6: Initialize Pipeline
    if (!initPipeline()) {
        LOG_ERROR("Failed to initialize Pipeline");
        return false;
    }
    
    // Step 7: Initialize business modules
    if (!initBusinessModules()) {
        LOG_ERROR("Failed to initialize business modules");
        return false;
    }
    
    // Step 8: Initialize VLM client (optional)
    if (config_.vlm.enabled) {
        if (!initVlmClient()) {
            LOG_WARN("VLM client initialization failed, continuing without VLM");
        }
    }
    
    // Step 9: Initialize HTTP server
    if (!initHttpServer()) {
        LOG_ERROR("Failed to initialize HTTP server");
        return false;
    }
    
    // Step 10: Initialize Hub client
    if (!config_.hub.url.empty()) {
        if (!initHubClient()) {
            LOG_WARN("Hub client initialization failed, running in standalone mode");
        }
    }
    
    LOG_INFO("Worker initialization complete");
    return true;
}

bool Worker::initStorage() {
    LOG_DEBUG("Initializing storage...");
    
    // MetaDB
    meta_db_ = std::make_unique<storage::MetaDB>();
    if (!meta_db_->open(config_.storage.meta_db)) {
        LOG_ERROR("Failed to open MetaDB: {}", config_.storage.meta_db);
        return false;
    }
    
    // EventCache
    event_cache_ = std::make_unique<storage::EventCache>();
    if (!event_cache_->open(config_.storage.event_cache_db)) {
        LOG_ERROR("Failed to open EventCache: {}", config_.storage.event_cache_db);
        return false;
    }
    
    // VectorStore
    storage::VectorStore::Config vs_config;
    vs_config.max_vectors = config_.search.max_vectors;
    vs_config.embedding_dim = config_.search.embedding_dim;
    vs_config.persist_path = config_.search.persist_path;
    
    vector_store_ = storage::VectorStore::create(vs_config);
    if (!vector_store_) {
        LOG_ERROR("Failed to create VectorStore");
        return false;
    }
    
    // Try to load persisted vectors
    if (fs::exists(vs_config.persist_path)) {
        if (vector_store_->load()) {
            LOG_INFO("Loaded {} vectors from disk", vector_store_->size());
        }
    }
    
    LOG_DEBUG("Storage initialized");
    return true;
}

bool Worker::initHAL() {
    LOG_INFO("Initializing HAL...");
    utils::Logger::flush();
    return true;
}

bool Worker::initYoloService() {
    LOG_INFO("Initializing YoloService with model: {}", yolo_config_.model.path);
    utils::Logger::flush();
    
    pipeline::YoloService::Options opts;
    opts.model_path = yolo_config_.model.path;
    opts.input_width = yolo_config_.model.input_width;
    opts.input_height = yolo_config_.model.input_height;
    opts.conf_threshold = yolo_config_.detection.confidence_threshold;
    opts.nms_threshold = yolo_config_.detection.nms_threshold;
    opts.device = config_.inference.device;
    opts.class_names = yolo_config_.class_names;
    
    yolo_service_ = pipeline::YoloService::create(opts);
    if (!yolo_service_) {
        LOG_ERROR("Failed to create YoloService");
        return false;
    }
    
    if (!yolo_service_->init()) {
        LOG_ERROR("Failed to initialize YoloService");
        return false;
    }
    
    // Warmup
    LOG_INFO("Warming up YoloService...");
    utils::Logger::flush();
    yolo_service_->warmup(5);
    LOG_INFO("YoloService initialized, avg latency: {:.1f}ms", 
             yolo_service_->getAvgLatencyMs());
    utils::Logger::flush();
    
    return true;
}

bool Worker::initPipeline() {
    LOG_INFO("Initializing Pipeline...");
    utils::Logger::flush();
    
    pipeline_mgr_ = std::make_unique<pipeline::PipelineManager>();
    pipeline_mgr_->setYoloService(yolo_service_.get());
    
    // Set detection callback
    pipeline_mgr_->setDetectionCallback(
        [this](const StreamId& id, const auto& dets, const auto& tracks) {
            onDetection(id, dets, tracks);
        }
    );
    
    LOG_INFO("Pipeline initialized");
    utils::Logger::flush();
    return true;
}

bool Worker::initBusinessModules() {
    LOG_INFO("Initializing business modules...");
    utils::Logger::flush();
    
    // StreamStore (persistent stream configurations)
    LOG_INFO("Creating StreamStore...");
    utils::Logger::flush();
    stream_store_ = std::make_unique<storage::StreamStore>();
    std::string stream_store_path = config_.storage.data_dir + "/streams.db";
    if (!stream_store_->open(stream_store_path)) {
        LOG_WARN("Failed to open stream store: {}, streams will not persist", stream_store_path);
        stream_store_.reset();
    }
    
    // StreamManager
    LOG_INFO("Creating StreamManager...");
    utils::Logger::flush();
    stream_mgr_ = std::make_unique<StreamManager>(pipeline_mgr_.get());
    
    // Enable stream persistence
    if (stream_store_) {
        stream_mgr_->setStore(stream_store_.get());
    }
    
    // RulesEngine
    LOG_INFO("Creating RulesEngine...");
    utils::Logger::flush();
    rules_engine_ = std::make_unique<RulesEngine>();
    
    // Load rules from config
    try {
        rules_config_ = RuleConfig::loadAll("config/rules.yaml");
        rules_engine_->loadRules(rules_config_);
        LOG_INFO("Loaded {} rules", rules_config_.size());
    } catch (const std::exception& e) {
        LOG_WARN("Failed to load rules: {}", e.what());
    }
    utils::Logger::flush();
    
    // AnalyticsCollector
    LOG_INFO("Creating AnalyticsCollector...");
    utils::Logger::flush();
    analytics_ = std::make_unique<AnalyticsCollector>(meta_db_.get());
    
    // SearchEngine
    LOG_INFO("Creating SearchEngine...");
    utils::Logger::flush();
    search_engine_ = std::make_unique<SearchEngine>(
        vector_store_.get(), 
        meta_db_.get()
    );
    
    // NotifyDispatcher
    LOG_INFO("Creating NotifyDispatcher...");
    utils::Logger::flush();
    try {
        notify_config_ = NotifyConfig::load("config/notify.yaml");
    } catch (const std::exception& e) {
        LOG_WARN("Failed to load notify config: {}", e.what());
    }
    notify_dispatcher_ = std::make_unique<NotifyDispatcher>(notify_config_);
    
    LOG_INFO("Business modules initialized");
    utils::Logger::flush();
    return true;
}

bool Worker::initVlmClient() {
    LOG_DEBUG("Initializing VLM client...");
    
    vlm_client_ = std::make_unique<VlmClient>(config_.vlm.endpoint);
    vlm_client_->setTimeout(config_.vlm.timeout_ms);
    
    // Test connection
    if (!vlm_client_->checkHealth()) {
        LOG_WARN("VLM service not available at {}", config_.vlm.endpoint);
        vlm_client_.reset();
        return false;
    }
    
    LOG_INFO("VLM client connected to {}", config_.vlm.endpoint);
    return true;
}

bool Worker::initHttpServer() {
    LOG_INFO("Initializing HTTP server on {}:{}...", config_.server.host, config_.server.http_port);
    utils::Logger::flush();
    
    api::HttpServer::Config srv_config;
    srv_config.host = config_.server.host;
    srv_config.port = config_.server.http_port;
    srv_config.max_connections = config_.server.max_connections;
    
    http_server_ = std::make_unique<api::HttpServer>(srv_config, this);
    
    LOG_INFO("HTTP server initialized");
    utils::Logger::flush();
    return true;
}

bool Worker::initHubClient() {
    LOG_INFO("Initializing Hub client for url={}", config_.hub.url);
    utils::Logger::flush();
    
    hub_client_ = std::make_unique<hub::HubClient>(
        config_.hub.url,
        config_.node_id,
        config_.hub.token
    );
    hub_client_->setReconnectMs(config_.hub.reconnect_ms);
    hub_client_->setHeartbeatMs(config_.hub.heartbeat_ms);
    
    // Determine advertise host: use config if set, otherwise auto-detect
    std::string advertise_host = config_.server.advertise_host;
    if (advertise_host.empty()) {
        // Auto-detect: extract host from Hub URL and use our route to it
        advertise_host = getLocalIpForHub();
    }
    hub_client_->setHost(advertise_host);
    hub_client_->setHttpPort(config_.server.http_port);
    
    // Set YOLO/VLM status and capabilities
    hub_client_->setYoloEnabled(yolo_service_ != nullptr);
    hub_client_->setVlmEnabled(vlm_client_ && vlm_client_->isConnected());
    hub_client_->setVersion(RIVISION_VERSION_STRING);
    // Set model names from config (populated by ConfigManager::extractModelNames)
    hub_client_->setVlmModel(config_.models.vlm);
    
    // Check embed service health and get model name
    checkEmbedHealth();
    {
        std::lock_guard<std::mutex> lock(embed_mutex_);
        if (!embed_status_.model.empty()) {
            hub_client_->setEmbedModel(embed_status_.model);
            LOG_INFO("Embed model from service: {}", embed_status_.model);
        } else if (!config_.models.embed.empty()) {
            hub_client_->setEmbedModel(config_.models.embed);
        }
    }
    hub_client_->setCapabilities(yolo_config_.model.path);
    
    // Set pre-heartbeat callback to update embed status before each heartbeat
    hub_client_->setPreHeartbeatCallback([this]() {
        checkEmbedHealth();
        std::lock_guard<std::mutex> lock(embed_mutex_);
        if (!embed_status_.model.empty()) {
            hub_client_->setEmbedModel(embed_status_.model);
        }
    });
    
    LOG_INFO("Creating EventPublisher...");
    utils::Logger::flush();
    event_publisher_ = std::make_unique<hub::EventPublisher>(
        hub_client_.get(),
        event_cache_.get()
    );
    
    LOG_INFO("Hub client initialized");
    utils::Logger::flush();
    return true;
}

bool Worker::start() {
    if (running_) {
        LOG_WARN("Worker already running");
        return true;
    }
    
    LOG_INFO("Starting worker...");
    start_time_ = std::chrono::steady_clock::now();
    running_ = true;
    stop_background_ = false;
    
    // Start HTTP server
    if (!http_server_->start()) {
        LOG_ERROR("Failed to start HTTP server");
        running_ = false;
        return false;
    }
    
    // Restore saved streams from persistent store
    if (stream_mgr_) {
        int restored = stream_mgr_->restoreStreams();
        if (restored > 0) {
            LOG_INFO("Auto-restored {} streams from persistent store", restored);
        }
    }
    
    // Connect to Hub
    if (hub_client_) {
        hub_client_->connect();
    }
    
    // Start background tasks
    cleanup_thread_ = std::thread(&Worker::runCleanupTask, this);
    stats_thread_ = std::thread(&Worker::runStatsAggregationTask, this);
    
    LOG_INFO("Worker started successfully");
    return true;
}

void Worker::stop() {
    if (!running_) {
        return;
    }
    
    LOG_INFO("Stopping worker...");
    running_ = false;
    stop_background_ = true;
    
    // Stop background tasks
    if (cleanup_thread_.joinable()) {
        cleanup_thread_.join();
    }
    if (stats_thread_.joinable()) {
        stats_thread_.join();
    }
    
    // Stop all streams
    if (stream_mgr_) {
        stream_mgr_->stopAll();
    }
    
    // Stop HTTP server
    if (http_server_) {
        http_server_->stop();
    }
    
    // Disconnect from Hub
    if (hub_client_) {
        hub_client_->disconnect();
    }
    
    // Persist vectors
    if (vector_store_) {
        vector_store_->save();
    }
    
    LOG_INFO("Worker stopped");
}

HealthStatus Worker::getHealth() const {
    HealthStatus status;
    
    // Determine overall status
    if (!running_) {
        status.status = HealthLevel::UNHEALTHY;
    } else if (degradation_level_ > 0) {
        status.status = HealthLevel::DEGRADED;
    } else {
        status.status = HealthLevel::HEALTHY;
    }
    
    status.uptime_s = getUptimeSeconds();
    status.degradation_level = degradation_level_;
    status.degradation_reason = degradation_reason_;
    
    // Streams
    if (stream_mgr_) {
        status.streams.active = stream_mgr_->activeCount();
        status.streams.max = 9;  // K3 limit
    }
    
    // Inference
    if (yolo_service_) {
        status.inference.connected = true;
        status.inference.latency_ms = static_cast<int>(yolo_service_->getAvgLatencyMs());
    }
    
    // Vector store
    if (vector_store_) {
        status.vector_store.vectors = vector_store_->size();
        status.vector_store.max = vector_store_->capacity();
    }
    
    // Hub
    if (hub_client_) {
        status.hub.connected = hub_client_->isConnected();
        status.hub.last_heartbeat = hub_client_->lastHeartbeat();
    }
    
    // Embed service (check and cache)
    checkEmbedHealth();
    {
        std::lock_guard<std::mutex> lock(embed_mutex_);
        status.embed.connected = embed_status_.connected;
        // Update hub_client with embed model if found (for heartbeat)
        if (hub_client_ && !embed_status_.model.empty()) {
            hub_client_->setEmbedModel(embed_status_.model);
        }
    }
    
    // System metrics
    auto sys = utils::SystemMonitor::getMetrics();
    status.system.cpu_pct = sys.cpu_percent;
    status.system.mem_pct = sys.mem_percent;
    status.system.disk_pct = sys.disk_percent;
    status.system.load_1 = sys.load_1;
    
    return status;
}

int64_t Worker::getUptimeSeconds() const {
    if (!running_) return 0;
    
    auto now = std::chrono::steady_clock::now();
    return std::chrono::duration_cast<std::chrono::seconds>(
        now - start_time_
    ).count();
}

// Helper for CURL response
static size_t WriteCallback(void* contents, size_t size, size_t nmemb, std::string* s) {
    size_t newLength = size * nmemb;
    s->append((char*)contents, newLength);
    return newLength;
}

void Worker::checkEmbedHealth() const {
    // Only check every 30 seconds
    auto now = std::chrono::steady_clock::now();
    {
        std::lock_guard<std::mutex> lock(embed_mutex_);
        auto elapsed = std::chrono::duration_cast<std::chrono::seconds>(
            now - embed_status_.last_check).count();
        if (elapsed < 30 && embed_status_.last_check.time_since_epoch().count() > 0) {
            return;  // Use cached status
        }
    }
    
    // Get embed URL from config
    std::string embed_url = config_.embed.url;
    if (embed_url.empty()) {
        embed_url = "http://localhost:18081";
    }
    
    std::string health_url = embed_url + "/health";
    
    CURL* curl = curl_easy_init();
    if (!curl) {
        return;
    }
    
    std::string response;
    curl_easy_setopt(curl, CURLOPT_URL, health_url.c_str());
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteCallback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, 2L);  // 2 second timeout
    curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT, 1L);
    
    CURLcode res = curl_easy_perform(curl);
    curl_easy_cleanup(curl);
    
    std::lock_guard<std::mutex> lock(embed_mutex_);
    embed_status_.last_check = now;
    
    if (res == CURLE_OK) {
        embed_status_.connected = true;
        // Parse model name from JSON: {"model":"chinese-clip-vit-b-16",...}
        size_t pos = response.find("\"model\"");
        if (pos != std::string::npos) {
            size_t start = response.find(':', pos) + 2;  // Skip :"
            size_t end = response.find('"', start);
            if (start < end && end != std::string::npos) {
                embed_status_.model = response.substr(start, end - start);
            }
        }
    } else {
        embed_status_.connected = false;
        embed_status_.model.clear();
    }
}

void Worker::onDetection(const StreamId& stream_id,
                        const std::vector<Detection>& detections,
                        const std::vector<Track>& tracks) {
    // Evaluate rules
    auto alerts = rules_engine_->evaluate(stream_id, detections, tracks);
    
    // Process alerts
    for (auto& alert : alerts) {
        // VLM verification
        if (vlm_client_ && alert.vlm_result.has_value() == false) {
            // Check if any action requires VLM verify
            // (simplified - full implementation would check rule actions)
        }
        
        // Send notifications
        if (notify_dispatcher_) {
            notify_dispatcher_->sendAlert(alert);
        }
        
        // Publish to Hub
        if (event_publisher_) {
            event_publisher_->publishAlert(alert);
        }
        
        // Store in MetaDB
        if (meta_db_) {
            meta_db_->insertAlert(alert);
        }
    }
    
    // Update analytics
    if (analytics_) {
        for (const auto& det : detections) {
            analytics_->recordDetection(stream_id, det);
        }
        
        for (const auto& track : tracks) {
            if (track.isConfirmed()) {
                analytics_->recordPresence(stream_id, track.id);
            }
        }
    }
    
    // Publish detection event to Hub
    if (event_publisher_ && !detections.empty()) {
        event_publisher_->publishDetection(stream_id, detections, tracks);
    }
}

void Worker::runCleanupTask() {
    LOG_DEBUG("Cleanup task started");
    
    while (!stop_background_) {
        std::this_thread::sleep_for(
            std::chrono::seconds(config_.cleanup.cleanup_interval_sec)
        );
        
        if (stop_background_) break;
        
        LOG_DEBUG("Running cleanup...");
        
        // Check disk usage
        auto sys = utils::SystemMonitor::getMetrics();
        
        if (sys.disk_percent >= config_.cleanup.disk_critical_watermark_pct) {
            LOG_WARN("Disk usage critical: {:.1f}%", sys.disk_percent);
            degradation_level_ = 3;
            degradation_reason_ = "Disk usage critical";
            
            // Aggressive cleanup
            if (meta_db_) {
                meta_db_->cleanupOldData(1);  // Keep only 1 day
            }
        } else if (sys.disk_percent >= config_.cleanup.disk_high_watermark_pct) {
            LOG_WARN("Disk usage high: {:.1f}%", sys.disk_percent);
            
            // Normal cleanup
            if (meta_db_) {
                meta_db_->cleanupOldData(config_.cleanup.detection_retain_days);
            }
        }
    }
    
    LOG_DEBUG("Cleanup task stopped");
}

void Worker::runStatsAggregationTask() {
    LOG_DEBUG("Stats aggregation task started");
    
    while (!stop_background_) {
        // Aggregate every hour
        std::this_thread::sleep_for(std::chrono::minutes(5));
        
        if (stop_background_) break;
        
        // Flush analytics to database
        if (analytics_ && meta_db_) {
            analytics_->flushToDB();
        }
    }
    
    LOG_DEBUG("Stats aggregation task stopped");
}

} // namespace rivision::core
