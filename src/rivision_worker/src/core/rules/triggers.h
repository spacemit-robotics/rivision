#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <vector>
#include <unordered_map>

namespace rivision::core {

class TriggerEvaluator {
public:
    struct TrackState {
        TrackId track_id;
        int64_t first_seen_ts = 0;
        int64_t last_seen_ts = 0;
        std::pair<float, float> first_position;
        std::pair<float, float> last_position;
        bool crossed_line = false;
        std::string cross_direction;
        int dwell_sec = 0;
    };
    
    struct EvalContext {
        const StreamId* stream_id = nullptr;
        const std::vector<Detection>* detections = nullptr;
        const std::vector<Track>* tracks = nullptr;
        std::unordered_map<TrackId, TrackState>* track_states = nullptr;
    };
    
    struct EvalResult {
        bool triggered = false;
        std::string zone_name;
        std::string direction;
        Detection matched_detection;
        std::optional<Track> matched_track;
    };
    
    static EvalResult evaluateZone(
        const RuleConfig& rule,
        const Detection& det
    );
    
    static EvalResult evaluateLineCross(
        const RuleConfig& rule,
        const Track& track,
        TrackState& state
    );
    
    static EvalResult evaluateDwellTime(
        const RuleConfig& rule,
        const Track& track,
        TrackState& state
    );
    
    static EvalResult evaluateClassCount(
        const RuleConfig& rule,
        const std::vector<Detection>& detections
    );
    
    static EvalResult evaluateClassPresence(
        const RuleConfig& rule,
        const std::vector<Detection>& detections
    );
    
    static EvalResult evaluateEmotionNegative(
        const RuleConfig& rule,
        const Detection& det
    );
    
    static EvalResult evaluateKnownTarget(
        const RuleConfig& rule,
        const Detection& det
    );
    
    static bool matchesClass(
        const std::vector<int>& classes,
        int class_id
    );
    
    static bool checkLineCrossing(
        const std::pair<float, float>& p1,
        const std::pair<float, float>& p2,
        const LineConfig& line,
        std::string& direction
    );
    
    static int countMatchingDetections(
        const std::vector<Detection>& detections,
        const std::vector<int>& classes,
        float min_confidence
    );
    
    static bool compareCount(
        int count,
        const std::string& op,
        int value
    );
};

} // namespace rivision::core
