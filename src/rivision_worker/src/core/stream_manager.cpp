#include "stream_manager.h"
#include "storage/stream_store.h"
#include "pipeline/pipeline_manager.h"
#include "pipeline/stream_context.h"
#include "utils/logger.h"

#include <random>
#include <sstream>
#include <iomanip>

namespace rivision::core {

StreamManager::StreamManager(pipeline::PipelineManager* pipeline_mgr)
    : pipeline_mgr_(pipeline_mgr) {
}

StreamManager::~StreamManager() {
    stopAll();
}

Result<StreamId> StreamManager::addStream(const StreamConfig& config) {
    return addStreamInternal(config, true);  // persist = true
}

Result<StreamId> StreamManager::addStreamInternal(const StreamConfig& config, bool persist) {
    std::unique_lock lock(mutex_);
    
    // Generate ID if not provided
    std::string id = config.id.empty() ? generateId() : config.id;
    
    // Check if already exists
    if (streams_.find(id) != streams_.end()) {
        return Result<StreamId>::failure("Stream already exists: " + id);
    }
    
    LOG_INFO("Adding stream: id={}, url={}", id, config.rtsp_url);
    
    // Create stream entry
    StreamEntry entry;
    entry.config = config;
    entry.config.id = id;
    entry.state = StreamState::CREATED;
    
    // Create stream context
    try {
        entry.context = pipeline_mgr_->createContext(entry.config);
        if (!entry.context) {
            return Result<StreamId>::failure("Failed to create stream context");
        }
    } catch (const std::exception& e) {
        return Result<StreamId>::failure(std::string("Exception: ") + e.what());
    }
    
    // Start the stream
    entry.state = StreamState::CONNECTING;
    if (!entry.context->start()) {
        entry.state = StreamState::ERROR;
        entry.error_msg = entry.context->getLastError();
        LOG_ERROR("Failed to start stream {}: {}", id, entry.error_msg);
        return Result<StreamId>::failure("Failed to start stream: " + entry.error_msg);
    }
    
    entry.state = StreamState::RUNNING;
    streams_[id] = std::move(entry);
    
    // Persist to store if enabled
    if (persist && store_) {
        store_->save(streams_[id].config);
    }
    
    LOG_INFO("Stream added successfully: {}", id);
    return Result<StreamId>(id);
}

bool StreamManager::removeStream(const StreamId& id) {
    std::unique_lock lock(mutex_);
    
    auto it = streams_.find(id);
    if (it == streams_.end()) {
        LOG_WARN("Stream not found: {}", id);
        return false;
    }
    
    LOG_INFO("Removing stream: {}", id);
    
    // Stop the context
    if (it->second.context) {
        it->second.context->stop();
    }
    
    // Remove from persistent store
    if (store_) {
        store_->remove(id);
    }
    
    streams_.erase(it);
    
    LOG_INFO("Stream removed: {}", id);
    return true;
}

StreamStatus StreamManager::getStatus(const StreamId& id) const {
    std::shared_lock lock(mutex_);
    
    auto it = streams_.find(id);
    if (it == streams_.end()) {
        StreamStatus status;
        status.state = StreamState::STOPPED;
        status.error_msg = "Stream not found";
        return status;
    }
    
    StreamStatus status;
    status.state = it->second.state;
    status.error_msg = it->second.error_msg;
    
    if (it->second.context) {
        auto ctx_status = it->second.context->getStatus();
        status.started_at = ctx_status.started_at;
        status.frames_processed = ctx_status.frames_processed;
        status.detections_count = ctx_status.detections_count;
        status.fps = ctx_status.fps;
        status.reconnect_count = ctx_status.reconnect_count;
    }
    
    return status;
}

std::vector<StreamConfig> StreamManager::listStreams() const {
    std::shared_lock lock(mutex_);
    
    std::vector<StreamConfig> result;
    result.reserve(streams_.size());
    
    for (const auto& [id, entry] : streams_) {
        result.push_back(entry.config);
    }
    
    return result;
}

std::optional<StreamConfig> StreamManager::getConfig(const StreamId& id) const {
    std::shared_lock lock(mutex_);
    
    auto it = streams_.find(id);
    if (it == streams_.end()) {
        return std::nullopt;
    }
    
    return it->second.config;
}

std::optional<StreamManager::StreamInfo> StreamManager::getInfo(const StreamId& id) const {
    std::shared_lock lock(mutex_);
    
    auto it = streams_.find(id);
    if (it == streams_.end()) {
        return std::nullopt;
    }
    
    StreamInfo info;
    info.config = it->second.config;
    info.status = getStatus(id);
    
    return info;
}

bool StreamManager::pauseStream(const StreamId& id) {
    std::unique_lock lock(mutex_);
    
    auto it = streams_.find(id);
    if (it == streams_.end()) {
        return false;
    }
    
    if (it->second.context && it->second.state == StreamState::RUNNING) {
        it->second.context->pause();
        it->second.state = StreamState::PAUSED;
        LOG_INFO("Stream paused: {}", id);
        return true;
    }
    
    return false;
}

bool StreamManager::resumeStream(const StreamId& id) {
    std::unique_lock lock(mutex_);
    
    auto it = streams_.find(id);
    if (it == streams_.end()) {
        return false;
    }
    
    if (it->second.context && it->second.state == StreamState::PAUSED) {
        it->second.context->resume();
        it->second.state = StreamState::RUNNING;
        LOG_INFO("Stream resumed: {}", id);
        return true;
    }
    
    return false;
}

void StreamManager::stopAll() {
    std::unique_lock lock(mutex_);
    
    LOG_INFO("Stopping all streams ({} total)", streams_.size());
    
    for (auto& [id, entry] : streams_) {
        if (entry.context) {
            entry.context->stop();
        }
        entry.state = StreamState::STOPPED;
    }
    
    streams_.clear();
    LOG_INFO("All streams stopped");
}

bool StreamManager::getFrame(const StreamId& id, Frame& frame) {
    std::shared_lock lock(mutex_);
    
    auto it = streams_.find(id);
    if (it == streams_.end() || !it->second.context) {
        return false;
    }
    
    return it->second.context->getLatestFrame(frame);
}

int StreamManager::activeCount() const {
    std::shared_lock lock(mutex_);
    
    int count = 0;
    for (const auto& [id, entry] : streams_) {
        if (entry.state == StreamState::RUNNING) {
            count++;
        }
    }
    
    return count;
}

bool StreamManager::exists(const StreamId& id) const {
    std::shared_lock lock(mutex_);
    return streams_.find(id) != streams_.end();
}

std::string StreamManager::generateId() const {
    static std::random_device rd;
    static std::mt19937 gen(rd());
    static std::uniform_int_distribution<> dis(0, 15);
    
    std::stringstream ss;
    ss << "stream-";
    for (int i = 0; i < 8; ++i) {
        ss << std::hex << dis(gen);
    }
    
    return ss.str();
}

void StreamManager::setStore(storage::StreamStore* store) {
    store_ = store;
    LOG_INFO("Stream persistence enabled");
}

int StreamManager::restoreStreams() {
    if (!store_) {
        LOG_DEBUG("No stream store configured, skipping restore");
        return 0;
    }
    
    auto configs = store_->loadAll();
    if (configs.empty()) {
        LOG_INFO("No saved streams to restore");
        return 0;
    }
    
    LOG_INFO("Restoring {} streams from persistent store", configs.size());
    
    int restored = 0;
    for (const auto& config : configs) {
        auto result = addStreamInternal(config, false);  // persist = false (already in store)
        if (result.ok()) {
            restored++;
            LOG_INFO("Restored stream: {}", config.id);
        } else {
            LOG_WARN("Failed to restore stream {}: {}", config.id, result.error());
        }
    }
    
    LOG_INFO("Restored {}/{} streams", restored, configs.size());
    return restored;
}

} // namespace rivision::core
