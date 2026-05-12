#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <functional>
#include <memory>

namespace rivision::core {

class VlmClient;

class ActionExecutor {
public:
    struct VlmVerifyResult {
        bool verified = false;
        float confidence = 0.0f;
        std::string description;
        bool timeout = false;
    };
    
    struct RecordRequest {
        StreamId stream_id;
        int pre_sec = 5;
        int post_sec = 10;
        std::string output_path;
    };
    
    struct NotifyRequest {
        AlertEvent alert;
        std::vector<std::string> channels;
        std::string message;
    };
    
    using RecordCallback = std::function<void(const RecordRequest&)>;
    using NotifyCallback = std::function<void(const NotifyRequest&)>;
    
    ActionExecutor();
    ~ActionExecutor();
    
    void setVlmClient(VlmClient* vlm) { vlm_client_ = vlm; }
    void setRecordCallback(RecordCallback cb) { record_callback_ = std::move(cb); }
    void setNotifyCallback(NotifyCallback cb) { notify_callback_ = std::move(cb); }
    
    VlmVerifyResult executeVlmVerify(
        const RuleConfig::Action& action,
        const Frame& frame,
        const Detection& detection
    );
    
    void executeRecord(
        const RuleConfig::Action& action,
        const StreamId& stream_id,
        const AlertEvent& alert
    );
    
    void executeNotify(
        const RuleConfig::Action& action,
        const AlertEvent& alert
    );
    
    std::string renderMessage(
        const std::string& template_str,
        const AlertEvent& alert
    );
    
private:
    VlmClient* vlm_client_ = nullptr;
    RecordCallback record_callback_;
    NotifyCallback notify_callback_;
};

} // namespace rivision::core
