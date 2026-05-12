#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <vector>
#include <unordered_map>
#include <shared_mutex>
#include <functional>

namespace rivision::core {

class VlmClient;

// =============================================================================
// MatchResult - Result of rule evaluation
// =============================================================================

struct MatchResult {
    RuleId rule_id;
    std::string rule_name;
    AlertLevel level = AlertLevel::INFO;
    std::string trigger_type;
    std::string zone_name;
    std::string direction;
    Detection matched_detection;
    std::optional<Track> matched_track;
    
    // Actions to execute
    std::vector<RuleConfig::Action> actions;
};

// =============================================================================
// RulesEngine - Evaluates rules against detections
// =============================================================================

class RulesEngine {
public:
    RulesEngine();
    ~RulesEngine();
    
    // Set VLM client for verify actions
    void setVlmClient(VlmClient* vlm) { vlm_client_ = vlm; }
    
    // Load rules from config
    void loadRules(const std::vector<RuleConfig>& rules);
    
    // Reload rules
    void reloadRules(const std::vector<RuleConfig>& rules);
    
    // Add/update/remove individual rules
    void addRule(const RuleConfig& rule);
    void updateRule(const RuleConfig& rule);
    void removeRule(const RuleId& id);
    
    // Get all rules
    std::vector<RuleConfig> getRules() const;
    
    // Evaluate rules against detections and tracks
    std::vector<AlertEvent> evaluate(
        const StreamId& stream_id,
        const std::vector<Detection>& detections,
        const std::vector<Track>& tracks
    );
    
    // Clean up stale track state
    void cleanupTrackSigns(const std::vector<TrackId>& active_track_ids);
    
private:
    // Rule evaluation methods
    std::vector<MatchResult> evaluateRule(
        const RuleConfig& rule,
        const StreamId& stream_id,
        const std::vector<Detection>& detections,
        const std::vector<Track>& tracks
    );
    
    bool evaluateZoneTrigger(
        const RuleConfig& rule,
        const Detection& det
    );
    
    bool evaluateLineCross(
        const RuleConfig& rule,
        const Track& track,
        std::string& direction
    );
    
    bool evaluateDwellTime(
        const RuleConfig& rule,
        const Track& track
    );
    
    bool evaluateClassCount(
        const RuleConfig& rule,
        const std::vector<Detection>& detections
    );
    
    bool evaluateClassPresence(
        const RuleConfig& rule,
        const std::vector<Detection>& detections
    );
    
    bool evaluateEmotionNegative(
        const RuleConfig& rule,
        const Detection& det
    );
    
    bool evaluateKnownTarget(
        const RuleConfig& rule,
        const Detection& det
    );
    
    // Check if rule applies to stream
    bool appliesToStream(const RuleConfig& rule, const StreamId& stream_id) const;
    
    // Check if detection class matches rule
    bool matchesClass(const RuleConfig& rule, int class_id) const;
    
    // Create alert event from match result
    AlertEvent createAlert(
        const MatchResult& match,
        const StreamId& stream_id,
        const std::string& camera_id
    );
    
    // Generate alert ID
    std::string generateAlertId() const;
    
    // VLM verify action
    struct VlmVerifyResult {
        bool verified = false;
        float confidence = 0.0f;
        std::string description;
        bool timeout = false;
    };
    VlmVerifyResult executeVlmVerify(const MatchResult& match, const Frame* frame);
    
    // VLM client
    VlmClient* vlm_client_ = nullptr;
    
    // Rules storage
    std::vector<RuleConfig> rules_;
    mutable std::shared_mutex rules_mutex_;
    
    // Track state for stateful triggers
    struct TrackSign {
        TrackId track_id;
        int64_t first_seen_ts = 0;
        int64_t last_seen_ts = 0;
        std::pair<float, float> first_position;
        std::pair<float, float> last_position;
        bool crossed_line = false;
        std::string cross_direction;
        int dwell_sec = 0;
    };
    
    std::unordered_map<TrackId, TrackSign> track_signs_;
    mutable std::mutex track_mutex_;
};

} // namespace rivision::core
