#ifdef USE_OATPP

#include "server.h"
#include "router.h"
#include "core/worker.h"
#include "utils/logger.h"

#include <oatpp/web/server/HttpConnectionHandler.hpp>
#include <oatpp/network/tcp/server/ConnectionProvider.hpp>
#include <oatpp/network/Server.hpp>
#include <oatpp/core/macro/component.hpp>
#include <oatpp/core/base/Environment.hpp>

namespace rivision::api {

// =============================================================================
// AppComponent - oatpp DI components
// =============================================================================

class AppComponent {
public:
    explicit AppComponent(const HttpServer::Config& config, core::Worker* worker)
        : port_(config.port), config_(config), worker_(worker) {}
    
private:
    v_uint16 port_;  // Must be initialized before OATPP_CREATE_COMPONENT uses it
    
public:
    OATPP_CREATE_COMPONENT(std::shared_ptr<oatpp::network::ServerConnectionProvider>, serverConnectionProvider)([this] {
        return oatpp::network::tcp::server::ConnectionProvider::createShared(
            {"0.0.0.0", static_cast<v_uint16>(port_), 
             oatpp::network::Address::IP_4});
    }());
    
    OATPP_CREATE_COMPONENT(std::shared_ptr<oatpp::web::server::HttpRouter>, httpRouter)([] {
        return oatpp::web::server::HttpRouter::createShared();
    }());
    
    OATPP_CREATE_COMPONENT(std::shared_ptr<oatpp::network::ConnectionHandler>, serverConnectionHandler)([] {
        OATPP_COMPONENT(std::shared_ptr<oatpp::web::server::HttpRouter>, router);
        return oatpp::web::server::HttpConnectionHandler::createShared(router);
    }());
    
private:
    HttpServer::Config config_;
    core::Worker* worker_;
};

// =============================================================================
// HttpServer::Impl
// =============================================================================

class HttpServer::Impl {
public:
    Impl(const Config& config, core::Worker* worker)
        : app_component_(config, worker)
        , worker_(worker) {}
    
    void registerRoutes() {
        OATPP_COMPONENT(std::shared_ptr<oatpp::web::server::HttpRouter>, router);
        
        auto api_router = ApiRouter::createShared(worker_);
        
        // Health endpoints
        router->route("GET", "/health", api_router->getHealthHandler());
        router->route("GET", "/api/v1/health", api_router->getHealthHandler());
        
        // Stream endpoints
        router->route("GET", "/api/v1/streams", api_router->getListStreamsHandler());
        router->route("POST", "/api/v1/streams", api_router->getAddStreamHandler());
        router->route("GET", "/api/v1/streams/{id}", api_router->getStreamHandler());
        router->route("DELETE", "/api/v1/streams/{id}", api_router->getRemoveStreamHandler());
        router->route("GET", "/api/v1/streams/{id}/snapshot", api_router->getSnapshotHandler());
        
        // Search endpoints
        router->route("POST", "/api/v1/search", api_router->getSearchHandler());
        router->route("POST", "/api/v1/search/text", api_router->getSearchTextHandler());
        router->route("POST", "/api/v1/search/image", api_router->getSearchImageHandler());
        router->route("POST", "/api/v1/search/hybrid", api_router->getSearchHybridHandler());
        router->route("POST", "/api/v1/search/faces", api_router->getSearchFacesHandler());
        
        // Analytics endpoints
        router->route("GET", "/api/v1/analytics/stats", api_router->getStatsHandler());
        router->route("GET", "/api/v1/analytics/heatmap", api_router->getHeatmapHandler());
        
        // Alerts endpoints
        router->route("GET", "/api/v1/alerts", api_router->getAlertsHandler());
        router->route("GET", "/api/v1/alerts/stats", api_router->getAlertStatsHandler());
        router->route("POST", "/api/v1/alerts/{id}/ack", api_router->getAckAlertHandler());
        
        // Rules endpoints
        router->route("GET", "/api/v1/rules", api_router->getListRulesHandler());
        router->route("POST", "/api/v1/rules", api_router->getAddRuleHandler());
        router->route("PUT", "/api/v1/rules/{id}", api_router->getUpdateRuleHandler());
        router->route("DELETE", "/api/v1/rules/{id}", api_router->getDeleteRuleHandler());
        
        // Config endpoints
        router->route("GET", "/api/v1/config", api_router->getConfigHandler());
        router->route("PUT", "/api/v1/config", api_router->getUpdateConfigHandler());
        
        // Prometheus metrics
        router->route("GET", "/metrics", api_router->getMetricsHandler());
    }
    
    void start() {
        registerRoutes();
        
        OATPP_COMPONENT(std::shared_ptr<oatpp::network::ServerConnectionProvider>, connectionProvider);
        OATPP_COMPONENT(std::shared_ptr<oatpp::network::ConnectionHandler>, connectionHandler);
        
        server_ = std::make_shared<oatpp::network::Server>(connectionProvider, connectionHandler);
        server_->run();
    }
    
    void stop() {
        if (server_) {
            server_->stop();
        }
    }
    
private:
    AppComponent app_component_;
    core::Worker* worker_;
    std::shared_ptr<oatpp::network::Server> server_;
};

// =============================================================================
// HttpServer
// =============================================================================

HttpServer::HttpServer(const Config& config, core::Worker* worker)
    : config_(config)
    , worker_(worker) {
    // Initialize oatpp environment before creating any components
    oatpp::base::Environment::init();
    impl_ = std::make_unique<Impl>(config, worker);
}

HttpServer::~HttpServer() {
    stop();
    oatpp::base::Environment::destroy();
}

bool HttpServer::start() {
    if (running_) {
        return true;
    }
    
    LOG_INFO("Starting HTTP server on {}:{}", config_.host, config_.port);
    
    running_ = true;
    server_thread_ = std::thread(&HttpServer::runLoop, this);
    
    return true;
}

void HttpServer::stop() {
    if (!running_) {
        return;
    }
    
    LOG_INFO("Stopping HTTP server");
    
    running_ = false;
    
    if (impl_) {
        impl_->stop();
    }
    
    if (server_thread_.joinable()) {
        server_thread_.join();
    }
    
    LOG_INFO("HTTP server stopped");
}

void HttpServer::runLoop() {
    try {
        impl_->start();
    } catch (const std::exception& e) {
        LOG_ERROR("HTTP server error: {}", e.what());
    }
}

} // namespace rivision::api

#else // !USE_OATPP

// =============================================================================
// Fallback implementation when oatpp is not available
// =============================================================================

#include "server.h"
#include "utils/logger.h"

namespace rivision::api {

class HttpServer::Impl {
    // Empty - no HTTP server without oatpp
};

HttpServer::HttpServer(const Config& config, core::Worker* worker)
    : config_(config), worker_(worker), impl_(std::make_unique<Impl>()) {
    LOG_WARN("HTTP server compiled without oatpp - API endpoints disabled");
}

HttpServer::~HttpServer() {
    stop();
}

bool HttpServer::start() {
    LOG_WARN("HTTP server disabled (no oatpp). API endpoints not available.");
    running_ = true;  // Mark as "running" to avoid errors
    return true;
}

void HttpServer::stop() {
    running_ = false;
}

void HttpServer::runLoop() {
    // No-op without oatpp
}

} // namespace rivision::api

#endif // USE_OATPP
