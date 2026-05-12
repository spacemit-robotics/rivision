#include "engine.h"
#include "vlm/vlm_client.h"
#include "utils/logger.h"

#include <random>
#include <sstream>
#include <iomanip>
#include <cmath>
#include <algorithm>

namespace rivision::core {

RulesEngine::RulesEngine() = default;
RulesEngine::~RulesEngine() = default;

void RulesEngine::loadRules(const std::vector<RuleConfig>& rules) {
    std::unique_lock lock(rules_mutex_);
    rules_ = rules;
    LOG_INFO("Loaded {} rules", rules_.size());
}

void RulesEngine::reloadRules(const std::vector<RuleConfig>& rules) {
    loadRules(rules);
}

void RulesEngine::addRule(const RuleConfig& rule) {
    std::unique_lock lock(rules_mutex_);
    
    // Check for duplicate
    auto it = std::find_if(rules_.begin(), rules_.end(),
        [&](const RuleConfig& r) { return r.id == rule.id; });
    
    if (it != rules_.end()) {
        *it = rule;
        LOG_INFO("Updated rule: {}", rule.id);
    } else {
        rules_.push_back(rule);
        LOG_INFO("Added rule: {}", rule.id);
    }
}

void RulesEngine::updateRule(const RuleConfig& rule) {
    addRule(rule);
}

void RulesEngine::removeRule(const RuleId& id) {
    std::unique_lock lock(rules_mutex_);
    
    auto it = std::remove_if(rules_.begin(), rules_.end(),
        [&](const RuleConfig& r) { return r.id == id; });
    
    if (it != rules_.end()) {
        rules_.erase(it, rules_.end());
        LOG_INFO("Removed rule: {}", id);
    }
}

std::vector<RuleConfig> RulesEngine::getRules() const {
    std::shared_lock lock(rules_mutex_);
    return rules_;
}

std::vector<AlertEvent> RulesEngine::evaluate(
    const StreamId& stream_id,
    const std::vector<Detection>& detections,
    const std::vector<Track>& tracks) {
    
    std::vector<AlertEvent> alerts;
    std::shared_lock rules_lock(rules_mutex_);
    
    // Update track signs
    {
        std::lock_guard<std::mutex> track_lock(track_mutex_);
        int64_t now = nowMs();
        
        for (const auto& track : tracks) {
            auto& sign = track_signs_[track.id];
            
            if (sign.first_seen_ts == 0) {
                sign.track_id = track.id;
                sign.first_seen_ts = now;
                sign.first_position = {track.detection.bbox.centerX(), 
                                      track.detection.bbox.centerY()};
            }
            
            sign.last_seen_ts = now;
            sign.last_position = {track.detection.bbox.centerX(),
                                 track.detection.bbox.centerY()};
            sign.dwell_sec = (now - sign.first_seen_ts) / 1000;
        }
    }
    
    // Evaluate each rule
    for (const auto& rule : rules_) {
        if (!rule.enabled) continue;
        if (!appliesToStream(rule, stream_id)) continue;
        
        auto matches = evaluateRule(rule, stream_id, detections, tracks);
        
        for (auto& match : matches) {
            // Get camera_id from stream (simplified - would come from stream config)
            std::string camera_id = stream_id;
            
            auto alert = createAlert(match, stream_id, camera_id);
            alerts.push_back(std::move(alert));
        }
    }
    
    // Cleanup track signs
    std::vector<TrackId> active_ids;
    for (const auto& track : tracks) {
        active_ids.push_back(track.id);
    }
    cleanupTrackSigns(active_ids);
    
    return alerts;
}

void RulesEngine::cleanupTrackSigns(const std::vector<TrackId>& active_track_ids) {
    std::lock_guard<std::mutex> lock(track_mutex_);
    
    int64_t now = nowMs();
    int64_t timeout_ms = 30000;  // 30 seconds
    
    for (auto it = track_signs_.begin(); it != track_signs_.end(); ) {
        bool active = std::find(active_track_ids.begin(), active_track_ids.end(),
                               it->first) != active_track_ids.end();
        bool expired = (now - it->second.last_seen_ts) > timeout_ms;
        
        if (!active && expired) {
            it = track_signs_.erase(it);
        } else {
            ++it;
        }
    }
}

std::vector<MatchResult> RulesEngine::evaluateRule(
    const RuleConfig& rule,
    const StreamId& stream_id,
    const std::vector<Detection>& detections,
    const std::vector<Track>& tracks) {
    
    std::vector<MatchResult> results;
    const auto& trigger = rule.trigger;
    
    if (trigger.type == "zone") {
        for (const auto& det : detections) {
            if (!matchesClass(rule, det.class_id)) continue;
            if (det.confidence < trigger.min_confidence) continue;
            
            if (evaluateZoneTrigger(rule, det)) {
                MatchResult match;
                match.rule_id = rule.id;
                match.rule_name = rule.name;
                match.trigger_type = "zone";
                match.zone_name = trigger.zone ? trigger.zone->name : "";
                match.matched_detection = det;
                match.actions = rule.actions;
                
                // Get level from actions
                for (const auto& action : rule.actions) {
                    if (action.type == "alert") {
                        match.level = alertLevelFromString(action.level);
                        break;
                    }
                }
                
                results.push_back(std::move(match));
            }
        }
    }
    else if (trigger.type == "line_cross") {
        for (const auto& track : tracks) {
            if (!track.isConfirmed()) continue;
            if (!matchesClass(rule, track.detection.class_id)) continue;
            
            std::string direction;
            if (evaluateLineCross(rule, track, direction)) {
                MatchResult match;
                match.rule_id = rule.id;
                match.rule_name = rule.name;
                match.trigger_type = "line_cross";
                match.direction = direction;
                match.matched_detection = track.detection;
                match.matched_track = track;
                match.actions = rule.actions;
                
                for (const auto& action : rule.actions) {
                    if (action.type == "alert") {
                        match.level = alertLevelFromString(action.level);
                        break;
                    }
                }
                
                results.push_back(std::move(match));
            }
        }
    }
    else if (trigger.type == "dwell_time") {
        for (const auto& track : tracks) {
            if (!track.isConfirmed()) continue;
            if (!matchesClass(rule, track.detection.class_id)) continue;
            
            if (evaluateDwellTime(rule, track)) {
                MatchResult match;
                match.rule_id = rule.id;
                match.rule_name = rule.name;
                match.trigger_type = "dwell_time";
                match.matched_detection = track.detection;
                match.matched_track = track;
                match.actions = rule.actions;
                
                for (const auto& action : rule.actions) {
                    if (action.type == "alert") {
                        match.level = alertLevelFromString(action.level);
                        break;
                    }
                }
                
                results.push_back(std::move(match));
            }
        }
    }
    else if (trigger.type == "class_count") {
        if (evaluateClassCount(rule, detections)) {
            MatchResult match;
            match.rule_id = rule.id;
            match.rule_name = rule.name;
            match.trigger_type = "class_count";
            match.actions = rule.actions;
            
            // Use first matching detection
            for (const auto& det : detections) {
                if (matchesClass(rule, det.class_id)) {
                    match.matched_detection = det;
                    break;
                }
            }
            
            for (const auto& action : rule.actions) {
                if (action.type == "alert") {
                    match.level = alertLevelFromString(action.level);
                    break;
                }
            }
            
            results.push_back(std::move(match));
        }
    }
    else if (trigger.type == "class_presence") {
        if (evaluateClassPresence(rule, detections)) {
            MatchResult match;
            match.rule_id = rule.id;
            match.rule_name = rule.name;
            match.trigger_type = "class_presence";
            match.actions = rule.actions;
            
            for (const auto& det : detections) {
                if (matchesClass(rule, det.class_id)) {
                    match.matched_detection = det;
                    break;
                }
            }
            
            for (const auto& action : rule.actions) {
                if (action.type == "alert") {
                    match.level = alertLevelFromString(action.level);
                    break;
                }
            }
            
            results.push_back(std::move(match));
        }
    }
    
    return results;
}

bool RulesEngine::evaluateZoneTrigger(const RuleConfig& rule, const Detection& det) {
    if (!rule.trigger.zone) return false;
    
    const auto& zone = *rule.trigger.zone;
    float cx = det.bbox.centerX();
    float cy = det.bbox.centerY();
    
    return zone.contains(cx, cy);
}

bool RulesEngine::evaluateLineCross(const RuleConfig& rule, const Track& track,
                                    std::string& direction) {
    if (!rule.trigger.line) return false;
    
    std::lock_guard<std::mutex> lock(track_mutex_);
    
    auto it = track_signs_.find(track.id);
    if (it == track_signs_.end()) return false;
    
    auto& sign = it->second;
    if (sign.crossed_line) return false;  // Already triggered
    
    const auto& line = *rule.trigger.line;
    float x1 = line.start.first, y1 = line.start.second;
    float x2 = line.end.first, y2 = line.end.second;
    
    // Current and previous positions
    float px1 = sign.first_position.first, py1 = sign.first_position.second;
    float px2 = sign.last_position.first, py2 = sign.last_position.second;
    
    // Check line segment intersection
    auto sign_func = [](float x1, float y1, float x2, float y2, float px, float py) {
        return (x2 - x1) * (py - y1) - (y2 - y1) * (px - x1);
    };
    
    float d1 = sign_func(x1, y1, x2, y2, px1, py1);
    float d2 = sign_func(x1, y1, x2, y2, px2, py2);
    float d3 = sign_func(px1, py1, px2, py2, x1, y1);
    float d4 = sign_func(px1, py1, px2, py2, x2, y2);
    
    bool crossed = ((d1 > 0) != (d2 > 0)) && ((d3 > 0) != (d4 > 0));
    
    if (crossed) {
        // Determine direction
        direction = (d1 > 0) ? "in" : "out";
        
        // Check if direction matches rule
        if (line.direction != "both" && line.direction != direction) {
            return false;
        }
        
        sign.crossed_line = true;
        sign.cross_direction = direction;
        return true;
    }
    
    return false;
}

bool RulesEngine::evaluateDwellTime(const RuleConfig& rule, const Track& track) {
    if (rule.trigger.dwell_sec <= 0) return false;
    
    std::lock_guard<std::mutex> lock(track_mutex_);
    
    auto it = track_signs_.find(track.id);
    if (it == track_signs_.end()) return false;
    
    return it->second.dwell_sec >= rule.trigger.dwell_sec;
}

bool RulesEngine::evaluateClassCount(const RuleConfig& rule,
                                     const std::vector<Detection>& detections) {
    int count = 0;
    for (const auto& det : detections) {
        if (matchesClass(rule, det.class_id) && 
            det.confidence >= rule.trigger.min_confidence) {
            count++;
        }
    }
    
    const auto& op = rule.trigger.count_operator;
    int val = rule.trigger.count_value;
    
    if (op == ">") return count > val;
    if (op == ">=") return count >= val;
    if (op == "<") return count < val;
    if (op == "<=") return count <= val;
    if (op == "==") return count == val;
    
    return false;
}

bool RulesEngine::evaluateClassPresence(const RuleConfig& rule,
                                        const std::vector<Detection>& detections) {
    for (const auto& det : detections) {
        if (matchesClass(rule, det.class_id) &&
            det.confidence >= rule.trigger.min_confidence) {
            return true;
        }
    }
    return false;
}

bool RulesEngine::evaluateEmotionNegative(const RuleConfig& rule, const Detection& det) {
    // Requires emotion detection integration
    // Placeholder implementation
    return false;
}

bool RulesEngine::evaluateKnownTarget(const RuleConfig& rule, const Detection& det) {
    // Requires feature library integration
    // Placeholder implementation
    return false;
}

bool RulesEngine::appliesToStream(const RuleConfig& rule, const StreamId& stream_id) const {
    if (rule.streams.empty()) return true;
    
    for (const auto& s : rule.streams) {
        if (s == "*" || s == stream_id) {
            return true;
        }
    }
    
    return false;
}

bool RulesEngine::matchesClass(const RuleConfig& rule, int class_id) const {
    if (rule.trigger.classes.empty()) return true;
    
    return std::find(rule.trigger.classes.begin(), rule.trigger.classes.end(),
                    class_id) != rule.trigger.classes.end();
}

AlertEvent RulesEngine::createAlert(const MatchResult& match,
                                    const StreamId& stream_id,
                                    const std::string& camera_id) {
    AlertEvent alert;
    alert.id = generateAlertId();
    alert.rule_id = match.rule_id;
    alert.rule_name = match.rule_name;
    alert.stream_id = stream_id;
    alert.camera_id = camera_id;
    alert.level = match.level;
    alert.trigger_type = match.trigger_type;
    alert.zone_name = match.zone_name;
    alert.direction = match.direction;
    alert.timestamp = nowMs();
    
    // Check for record action
    for (const auto& action : match.actions) {
        if (action.type == "record") {
            alert.record.enabled = true;
            alert.record.pre_sec = action.pre_sec;
            alert.record.post_sec = action.post_sec;
            break;
        }
    }
    
    return alert;
}

std::string RulesEngine::generateAlertId() const {
    static std::random_device rd;
    static std::mt19937 gen(rd());
    static std::uniform_int_distribution<> dis(0, 15);
    
    std::stringstream ss;
    ss << "alert-";
    for (int i = 0; i < 12; ++i) {
        ss << std::hex << dis(gen);
    }
    
    return ss.str();
}

RulesEngine::VlmVerifyResult RulesEngine::executeVlmVerify(
    const MatchResult& match,
    const Frame* frame) {
    
    VlmVerifyResult result;
    
    if (!vlm_client_ || !frame) {
        LOG_DEBUG("VLM verify skipped: no VLM client or frame");
        return result;
    }
    
    // Find VLM verify action
    std::string prompt;
    for (const auto& action : match.actions) {
        if (action.type == "vlm_verify") {
            prompt = action.prompt;
            break;
        }
    }
    
    if (prompt.empty()) {
        return result;  // No VLM verify action
    }
    
    LOG_DEBUG("Executing VLM verify for rule: {}", match.rule_name);
    
    // Call VLM service
    auto vlm_result = vlm_client_->verify(*frame, prompt);
    
    result.verified = vlm_result.verified;
    result.confidence = vlm_result.confidence;
    result.description = vlm_result.description;
    result.timeout = vlm_result.timeout;
    
    if (vlm_result.timeout) {
        LOG_WARN("VLM verify timeout for rule: {}", match.rule_name);
    } else {
        LOG_DEBUG("VLM verify result: verified={}, confidence={:.2f}", 
                 result.verified, result.confidence);
    }
    
    return result;
}

} // namespace rivision::core
