#include "notify_dispatcher.h"
#include "utils/logger.h"
#include <sstream>
#include <chrono>
#include <ctime>

namespace rivision::core {

NotifyDispatcher::NotifyDispatcher(const NotifyConfig& config)
    : config_(config) {
    LOG_INFO("NotifyDispatcher initialized with {} channels", 
             (config_.webhook.enabled ? 1 : 0) +
             (config_.wechat.enabled ? 1 : 0) +
             (config_.dingtalk.enabled ? 1 : 0) +
             (config_.email.enabled ? 1 : 0));
}

NotifyDispatcher::~NotifyDispatcher() = default;

bool NotifyDispatcher::isEnabled() const {
    return config_.webhook.enabled || 
           config_.wechat.enabled || 
           config_.dingtalk.enabled || 
           config_.email.enabled;
}

std::string NotifyDispatcher::levelToString(AlertLevel level) const {
    switch (level) {
        case AlertLevel::CRITICAL: return "critical";
        case AlertLevel::WARNING: return "warning";
        case AlertLevel::INFO: return "info";
        default: return "unknown";
    }
}

void NotifyDispatcher::sendAlert(const AlertEvent& alert) {
    if (!isEnabled()) {
        return;
    }
    
    std::string levelStr = levelToString(alert.level);
    LOG_DEBUG("Sending alert notification: {} ({})", alert.rule_name, levelStr);
    
    // Check level against channel-specific levels
    auto matchLevel = [&](const std::vector<std::string>& levels) -> bool {
        if (levels.empty()) return true;  // No filter = accept all
        for (const auto& l : levels) {
            if (l == levelStr || l == "all") return true;
        }
        return false;
    };
    
    // Send to each enabled channel (respecting level filters)
    if (config_.webhook.enabled && matchLevel(config_.webhook.levels)) {
        sendWebhook(alert);
    }
    if (config_.wechat.enabled && matchLevel(config_.wechat.levels)) {
        sendWeChat(alert);
    }
    if (config_.dingtalk.enabled && matchLevel(config_.dingtalk.levels)) {
        sendDingTalk(alert);
    }
    if (config_.email.enabled && matchLevel(config_.email.levels)) {
        sendEmail(alert);
    }
}

std::string NotifyDispatcher::renderMessage(const AlertEvent& alert) const {
    std::ostringstream oss;
    
    // Format timestamp
    auto ts = std::chrono::system_clock::from_time_t(alert.timestamp / 1000);
    auto time = std::chrono::system_clock::to_time_t(ts);
    char time_buf[32];
    std::strftime(time_buf, sizeof(time_buf), "%Y-%m-%d %H:%M:%S", std::localtime(&time));
    
    oss << "[" << levelToString(alert.level) << "] " << alert.rule_name << "\n"
        << "时间: " << time_buf << "\n"
        << "摄像头: " << alert.camera_id << "\n"
        << "区域: " << (alert.zone_name.empty() ? "-" : alert.zone_name) << "\n";
    
    if (!alert.direction.empty()) {
        oss << "方向: " << alert.direction << "\n";
    }
    
    oss << "触发: " << alert.trigger_type;
    
    if (alert.vlm_result && alert.vlm_result->timeout) {
        oss << " (VLM超时)";
    }
    
    return oss.str();
}

void NotifyDispatcher::sendWebhook(const AlertEvent& alert) {
    LOG_INFO("Webhook notification: {} -> {}", alert.rule_name, config_.webhook.url);
}

void NotifyDispatcher::sendWeChat(const AlertEvent& alert) {
    LOG_INFO("WeChat notification: {} -> {}", alert.rule_name, config_.wechat.webhook_url);
}

void NotifyDispatcher::sendDingTalk(const AlertEvent& alert) {
    LOG_INFO("DingTalk notification: {} -> {}", alert.rule_name, config_.dingtalk.webhook_url);
}

void NotifyDispatcher::sendEmail(const AlertEvent& alert) {
    LOG_INFO("Email notification: {} -> {}", alert.rule_name, 
             config_.email.to_addrs.empty() ? "no recipients" : config_.email.to_addrs[0]);
}

} // namespace rivision::core
