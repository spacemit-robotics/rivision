#pragma once

#include "rivision/config.h"
#include "rivision/types.h"
#include <string>
#include <vector>
#include <memory>

namespace rivision::core {

// =============================================================================
// NotifyDispatcher - 告警通知分发器
// 支持微信/钉钉/邮件/Webhook等渠道
// =============================================================================

class NotifyDispatcher {
public:
    explicit NotifyDispatcher(const NotifyConfig& config);
    ~NotifyDispatcher();
    
    // Send alert notification (uses AlertEvent from types.h)
    void sendAlert(const AlertEvent& alert);
    
    // Check if any channel is enabled
    bool isEnabled() const;
    
private:
    void sendWebhook(const AlertEvent& alert);
    void sendWeChat(const AlertEvent& alert);
    void sendDingTalk(const AlertEvent& alert);
    void sendEmail(const AlertEvent& alert);
    
    std::string renderMessage(const AlertEvent& alert) const;
    std::string levelToString(AlertLevel level) const;
    
    NotifyConfig config_;
};

} // namespace rivision::core
