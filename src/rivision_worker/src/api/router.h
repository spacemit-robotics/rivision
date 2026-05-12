#pragma once

#include <oatpp/web/server/api/ApiController.hpp>
#include <oatpp/core/macro/codegen.hpp>
#include <oatpp/core/macro/component.hpp>
#include <memory>

namespace rivision::core {
class Worker;
}

namespace rivision::api {

// =============================================================================
// ApiRouter - Route handler factory
// =============================================================================

class ApiRouter : public oatpp::web::server::api::ApiController {
public:
    using ApiController::ApiController;
    
    static std::shared_ptr<ApiRouter> createShared(core::Worker* worker);
    
    // Health
    std::shared_ptr<OutgoingResponse> handleHealth();
    
    // Streams
    std::shared_ptr<OutgoingResponse> handleListStreams();
    std::shared_ptr<OutgoingResponse> handleAddStream(const oatpp::String& body);
    std::shared_ptr<OutgoingResponse> handleGetStream(const oatpp::String& id);
    std::shared_ptr<OutgoingResponse> handleRemoveStream(const oatpp::String& id);
    std::shared_ptr<OutgoingResponse> handleSnapshot(const oatpp::String& id);
    
    // Search
    std::shared_ptr<OutgoingResponse> handleSearch(const oatpp::String& body);
    std::shared_ptr<OutgoingResponse> handleSearchText(const oatpp::String& body);
    std::shared_ptr<OutgoingResponse> handleSearchImage(const oatpp::String& body);
    std::shared_ptr<OutgoingResponse> handleSearchHybrid(const oatpp::String& body);
    std::shared_ptr<OutgoingResponse> handleSearchFaces(const oatpp::String& body);
    
    // Analytics
    std::shared_ptr<OutgoingResponse> handleStats();
    std::shared_ptr<OutgoingResponse> handleHeatmap();
    
    // Alerts
    std::shared_ptr<OutgoingResponse> handleListAlerts();
    std::shared_ptr<OutgoingResponse> handleAlertStats();
    std::shared_ptr<OutgoingResponse> handleAckAlert(const oatpp::String& id);
    
    // Rules
    std::shared_ptr<OutgoingResponse> handleListRules();
    std::shared_ptr<OutgoingResponse> handleAddRule(const oatpp::String& body);
    std::shared_ptr<OutgoingResponse> handleUpdateRule(const oatpp::String& id, const oatpp::String& body);
    std::shared_ptr<OutgoingResponse> handleDeleteRule(const oatpp::String& id);
    
    // Config
    std::shared_ptr<OutgoingResponse> handleGetConfig();
    std::shared_ptr<OutgoingResponse> handleUpdateConfig(const oatpp::String& body);
    
    // Metrics
    std::shared_ptr<OutgoingResponse> handleMetrics();
    
    // Handler getters (for server.cpp)
    auto getHealthHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleHealth>(this); 
    }
    auto getListStreamsHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleListStreams>(this); 
    }
    auto getAddStreamHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleAddStream>(this); 
    }
    auto getStreamHandler() { 
        return createPathHandler<ApiRouter, &ApiRouter::handleGetStream>(this, "id"); 
    }
    auto getRemoveStreamHandler() { 
        return createPathHandler<ApiRouter, &ApiRouter::handleRemoveStream>(this, "id"); 
    }
    auto getSnapshotHandler() { 
        return createPathHandler<ApiRouter, &ApiRouter::handleSnapshot>(this, "id"); 
    }
    auto getSearchHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleSearch>(this); 
    }
    auto getSearchTextHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleSearchText>(this); 
    }
    auto getSearchImageHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleSearchImage>(this); 
    }
    auto getSearchHybridHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleSearchHybrid>(this); 
    }
    auto getSearchFacesHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleSearchFaces>(this); 
    }
    auto getStatsHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleStats>(this); 
    }
    auto getHeatmapHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleHeatmap>(this); 
    }
    auto getAlertsHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleListAlerts>(this); 
    }
    auto getAlertStatsHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleAlertStats>(this); 
    }
    auto getAckAlertHandler() { 
        return createPathHandler<ApiRouter, &ApiRouter::handleAckAlert>(this, "id"); 
    }
    auto getListRulesHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleListRules>(this); 
    }
    auto getAddRuleHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleAddRule>(this); 
    }
    auto getUpdateRuleHandler() { 
        return createPathBodyHandler<ApiRouter, &ApiRouter::handleUpdateRule>(this, "id"); 
    }
    auto getDeleteRuleHandler() { 
        return createPathHandler<ApiRouter, &ApiRouter::handleDeleteRule>(this, "id"); 
    }
    auto getConfigHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleGetConfig>(this); 
    }
    auto getUpdateConfigHandler() { 
        return createBodyHandler<ApiRouter, &ApiRouter::handleUpdateConfig>(this); 
    }
    auto getMetricsHandler() { 
        return createHandler<ApiRouter, &ApiRouter::handleMetrics>(this); 
    }
    
private:
    // Handler implementations - must be defined before use
    class LambdaHandler : public oatpp::web::server::HttpRequestHandler {
    public:
        using HandlerFunc = std::function<std::shared_ptr<OutgoingResponse>()>;
        LambdaHandler(HandlerFunc func) : func_(std::move(func)) {}
        std::shared_ptr<OutgoingResponse> handle(const std::shared_ptr<IncomingRequest>& request) override {
            (void)request;
            return func_();
        }
    private:
        HandlerFunc func_;
    };
    
    template<typename T, std::shared_ptr<OutgoingResponse> (T::*Method)(const oatpp::String&)>
    class BodyHandler : public oatpp::web::server::HttpRequestHandler {
    public:
        BodyHandler(T* self) : self_(self) {}
        std::shared_ptr<OutgoingResponse> handle(const std::shared_ptr<IncomingRequest>& request) override {
            auto body = request->readBodyToString();
            return (self_->*Method)(body);
        }
    private:
        T* self_;
    };
    
    template<typename T, std::shared_ptr<OutgoingResponse> (T::*Method)(const oatpp::String&)>
    class PathHandler : public oatpp::web::server::HttpRequestHandler {
    public:
        PathHandler(T* self, const char* param) : self_(self), param_(param) {}
        std::shared_ptr<OutgoingResponse> handle(const std::shared_ptr<IncomingRequest>& request) override {
            auto value = request->getPathVariable(param_.c_str());
            return (self_->*Method)(value);
        }
    private:
        T* self_;
        std::string param_;
    };
    
    template<typename T, std::shared_ptr<OutgoingResponse> (T::*Method)(const oatpp::String&, const oatpp::String&)>
    class PathBodyHandler : public oatpp::web::server::HttpRequestHandler {
    public:
        PathBodyHandler(T* self, const char* param) : self_(self), param_(param) {}
        std::shared_ptr<OutgoingResponse> handle(const std::shared_ptr<IncomingRequest>& request) override {
            auto value = request->getPathVariable(param_.c_str());
            auto body = request->readBodyToString();
            return (self_->*Method)(value, body);
        }
    private:
        T* self_;
        std::string param_;
    };

    // Helper to create handlers
    template<typename T, std::shared_ptr<OutgoingResponse> (T::*Method)()>
    std::shared_ptr<oatpp::web::server::HttpRequestHandler> createHandler(T* self) {
        return std::make_shared<LambdaHandler>([self]() { return (self->*Method)(); });
    }
    
    template<typename T, std::shared_ptr<OutgoingResponse> (T::*Method)(const oatpp::String&)>
    std::shared_ptr<oatpp::web::server::HttpRequestHandler> createBodyHandler(T* self) {
        return std::make_shared<BodyHandler<T, Method>>(self);
    }
    
    template<typename T, std::shared_ptr<OutgoingResponse> (T::*Method)(const oatpp::String&)>
    std::shared_ptr<oatpp::web::server::HttpRequestHandler> createPathHandler(T* self, const char* param) {
        return std::make_shared<PathHandler<T, Method>>(self, param);
    }
    
    template<typename T, std::shared_ptr<OutgoingResponse> (T::*Method)(const oatpp::String&, const oatpp::String&)>
    std::shared_ptr<oatpp::web::server::HttpRequestHandler> createPathBodyHandler(T* self, const char* param) {
        return std::make_shared<PathBodyHandler<T, Method>>(self, param);
    }
    
    core::Worker* worker_ = nullptr;
};

} // namespace rivision::api
