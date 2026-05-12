#include "triggers.h"
#include "rivision/types.h"
#include <cmath>

namespace rivision::core {

TriggerEvaluator::EvalResult TriggerEvaluator::evaluateZone(
    const RuleConfig& rule,
    const Detection& det
) {
    EvalResult result;
    
    if (!rule.trigger.zone) {
        return result;
    }
    
    const auto& zone = *rule.trigger.zone;
    float cx = det.bbox.centerX();
    float cy = det.bbox.centerY();
    
    if (zone.contains(cx, cy)) {
        if (!rule.trigger.classes.empty() && 
            !matchesClass(rule.trigger.classes, det.class_id)) {
            return result;
        }
        if (det.confidence < rule.trigger.min_confidence) {
            return result;
        }
        
        result.triggered = true;
        result.zone_name = zone.name;
        result.matched_detection = det;
    }
    
    return result;
}

TriggerEvaluator::EvalResult TriggerEvaluator::evaluateLineCross(
    const RuleConfig& rule,
    const Track& track,
    TrackState& state
) {
    EvalResult result;
    
    if (!rule.trigger.line) {
        return result;
    }
    
    if (!track.isConfirmed() || track.history.size() < 2) {
        return result;
    }
    
    float cx = track.detection.bbox.centerX();
    float cy = track.detection.bbox.centerY();
    std::pair<float, float> current_pos = {cx, cy};
    
    if (state.first_seen_ts == 0) {
        state.first_seen_ts = nowMs();
        state.first_position = current_pos;
        state.last_position = current_pos;
        return result;
    }
    
    std::string direction;
    if (checkLineCrossing(state.last_position, current_pos, *rule.trigger.line, direction)) {
        if (!state.crossed_line) {
            const auto& line = *rule.trigger.line;
            if (line.direction == "both" || line.direction == direction) {
                result.triggered = true;
                result.direction = direction;
                result.matched_detection = track.detection;
                result.matched_track = track;
                state.crossed_line = true;
                state.cross_direction = direction;
            }
        }
    }
    
    state.last_position = current_pos;
    state.last_seen_ts = nowMs();
    
    return result;
}

TriggerEvaluator::EvalResult TriggerEvaluator::evaluateDwellTime(
    const RuleConfig& rule,
    const Track& track,
    TrackState& state
) {
    EvalResult result;
    
    if (!rule.trigger.zone || rule.trigger.dwell_sec <= 0) {
        return result;
    }
    
    const auto& zone = *rule.trigger.zone;
    float cx = track.detection.bbox.centerX();
    float cy = track.detection.bbox.centerY();
    
    bool in_zone = zone.contains(cx, cy);
    int64_t now = nowMs();
    
    if (in_zone) {
        if (state.first_seen_ts == 0) {
            state.first_seen_ts = now;
        }
        state.last_seen_ts = now;
        
        int elapsed_sec = static_cast<int>((now - state.first_seen_ts) / 1000);
        
        if (elapsed_sec >= rule.trigger.dwell_sec && state.dwell_sec < rule.trigger.dwell_sec) {
            result.triggered = true;
            result.zone_name = zone.name;
            result.matched_detection = track.detection;
            result.matched_track = track;
            state.dwell_sec = elapsed_sec;
        }
    } else {
        state.first_seen_ts = 0;
        state.dwell_sec = 0;
    }
    
    return result;
}

TriggerEvaluator::EvalResult TriggerEvaluator::evaluateClassCount(
    const RuleConfig& rule,
    const std::vector<Detection>& detections
) {
    EvalResult result;
    
    int count = countMatchingDetections(
        detections,
        rule.trigger.classes,
        rule.trigger.min_confidence
    );
    
    if (compareCount(count, rule.trigger.count_operator, rule.trigger.count_value)) {
        result.triggered = true;
        if (!detections.empty()) {
            result.matched_detection = detections[0];
        }
    }
    
    return result;
}

TriggerEvaluator::EvalResult TriggerEvaluator::evaluateClassPresence(
    const RuleConfig& rule,
    const std::vector<Detection>& detections
) {
    EvalResult result;
    
    for (const auto& det : detections) {
        if (matchesClass(rule.trigger.classes, det.class_id) &&
            det.confidence >= rule.trigger.min_confidence) {
            result.triggered = true;
            result.matched_detection = det;
            break;
        }
    }
    
    return result;
}

TriggerEvaluator::EvalResult TriggerEvaluator::evaluateEmotionNegative(
    const RuleConfig& rule,
    const Detection& det
) {
    EvalResult result;
    // Emotion analysis requires VLM - placeholder
    return result;
}

TriggerEvaluator::EvalResult TriggerEvaluator::evaluateKnownTarget(
    const RuleConfig& rule,
    const Detection& det
) {
    EvalResult result;
    // Known target matching requires face embedding comparison - placeholder
    return result;
}

bool TriggerEvaluator::matchesClass(
    const std::vector<int>& classes,
    int class_id
) {
    if (classes.empty()) {
        return true;
    }
    for (int c : classes) {
        if (c == class_id) {
            return true;
        }
    }
    return false;
}

bool TriggerEvaluator::checkLineCrossing(
    const std::pair<float, float>& p1,
    const std::pair<float, float>& p2,
    const LineConfig& line,
    std::string& direction
) {
    float ax = line.start.first;
    float ay = line.start.second;
    float bx = line.end.first;
    float by = line.end.second;
    
    auto cross = [](float x1, float y1, float x2, float y2, float x3, float y3) {
        return (x2 - x1) * (y3 - y1) - (y2 - y1) * (x3 - x1);
    };
    
    float d1 = cross(ax, ay, bx, by, p1.first, p1.second);
    float d2 = cross(ax, ay, bx, by, p2.first, p2.second);
    float d3 = cross(p1.first, p1.second, p2.first, p2.second, ax, ay);
    float d4 = cross(p1.first, p1.second, p2.first, p2.second, bx, by);
    
    if (((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
        ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0))) {
        
        float line_dx = bx - ax;
        float line_dy = by - ay;
        float move_dx = p2.first - p1.first;
        float move_dy = p2.second - p1.second;
        
        float cross_product = line_dx * move_dy - line_dy * move_dx;
        direction = (cross_product > 0) ? "in" : "out";
        
        return true;
    }
    
    return false;
}

int TriggerEvaluator::countMatchingDetections(
    const std::vector<Detection>& detections,
    const std::vector<int>& classes,
    float min_confidence
) {
    int count = 0;
    for (const auto& det : detections) {
        if (matchesClass(classes, det.class_id) && det.confidence >= min_confidence) {
            count++;
        }
    }
    return count;
}

bool TriggerEvaluator::compareCount(
    int count,
    const std::string& op,
    int value
) {
    if (op == ">") return count > value;
    if (op == ">=") return count >= value;
    if (op == "<") return count < value;
    if (op == "<=") return count <= value;
    if (op == "==") return count == value;
    if (op == "!=") return count != value;
    return false;
}

} // namespace rivision::core
