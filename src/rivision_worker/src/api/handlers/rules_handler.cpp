#include "rules_handler.h"
#include <nlohmann/json.hpp>

namespace rivision::api {

using json = nlohmann::json;

RulesHandler::RulesHandler() = default;
RulesHandler::~RulesHandler() = default;

std::string RulesHandler::handleListRules() {
    if (!get_callback_) {
        return R"({"error":"rules handler not configured"})";
    }
    
    auto rules = get_callback_();
    
    json arr = json::array();
    for (const auto& rule : rules) {
        arr.push_back(json::parse(ruleToJson(rule)));
    }
    
    json result;
    result["rules"] = arr;
    result["count"] = rules.size();
    
    return result.dump();
}

std::string RulesHandler::handleGetRule(const std::string& id) {
    if (!get_callback_) {
        return R"({"error":"rules handler not configured"})";
    }
    
    auto rules = get_callback_();
    for (const auto& rule : rules) {
        if (rule.id == id) {
            return ruleToJson(rule);
        }
    }
    
    return R"({"error":"rule not found"})";
}

std::string RulesHandler::handleAddRule(const std::string& body) {
    if (!add_callback_) {
        return R"({"error":"add rule handler not configured"})";
    }
    
    try {
        auto rule = parseRule(body);
        if (add_callback_(rule)) {
            return R"({"success":true,"rule_id":")" + rule.id + "\"}";
        } else {
            return R"({"error":"failed to add rule"})";
        }
    } catch (const std::exception& e) {
        return R"({"error":")" + std::string(e.what()) + "\"}";
    }
}

std::string RulesHandler::handleUpdateRule(const std::string& id, const std::string& body) {
    if (!update_callback_) {
        return R"({"error":"update rule handler not configured"})";
    }
    
    try {
        auto rule = parseRule(body);
        rule.id = id;
        if (update_callback_(rule)) {
            return R"({"success":true})";
        } else {
            return R"({"error":"failed to update rule"})";
        }
    } catch (const std::exception& e) {
        return R"({"error":")" + std::string(e.what()) + "\"}";
    }
}

std::string RulesHandler::handleDeleteRule(const std::string& id) {
    if (!delete_callback_) {
        return R"({"error":"delete rule handler not configured"})";
    }
    
    if (delete_callback_(id)) {
        return R"({"success":true})";
    } else {
        return R"({"error":"failed to delete rule"})";
    }
}

RuleConfig RulesHandler::parseRule(const std::string& jsonStr) {
    json j = json::parse(jsonStr);
    RuleConfig rule;
    
    rule.id = j.value("id", "");
    rule.name = j.value("name", "");
    rule.enabled = j.value("enabled", true);
    
    if (j.contains("streams")) {
        rule.streams = j["streams"].get<std::vector<std::string>>();
    }
    
    if (j.contains("trigger")) {
        auto& t = j["trigger"];
        rule.trigger.type = t.value("type", "");
        rule.trigger.dwell_sec = t.value("dwell_sec", 0);
        rule.trigger.count_operator = t.value("count_operator", "");
        rule.trigger.count_value = t.value("count_value", 0);
        rule.trigger.min_confidence = t.value("min_confidence", 0.5f);
        
        if (t.contains("classes")) {
            rule.trigger.classes = t["classes"].get<std::vector<int>>();
        }
    }
    
    if (j.contains("actions")) {
        for (const auto& a : j["actions"]) {
            RuleConfig::Action action;
            action.type = a.value("type", "");
            action.level = a.value("level", "warning");
            action.prompt = a.value("prompt", "");
            action.pre_sec = a.value("pre_sec", 5);
            action.post_sec = a.value("post_sec", 10);
            if (a.contains("channels")) {
                action.channels = a["channels"].get<std::vector<std::string>>();
            }
            rule.actions.push_back(action);
        }
    }
    
    return rule;
}

std::string RulesHandler::ruleToJson(const RuleConfig& rule) {
    json j;
    j["id"] = rule.id;
    j["name"] = rule.name;
    j["enabled"] = rule.enabled;
    j["streams"] = rule.streams;
    
    j["trigger"] = {
        {"type", rule.trigger.type},
        {"dwell_sec", rule.trigger.dwell_sec},
        {"count_operator", rule.trigger.count_operator},
        {"count_value", rule.trigger.count_value},
        {"min_confidence", rule.trigger.min_confidence},
        {"classes", rule.trigger.classes}
    };
    
    json actions = json::array();
    for (const auto& a : rule.actions) {
        actions.push_back({
            {"type", a.type},
            {"level", a.level},
            {"prompt", a.prompt},
            {"pre_sec", a.pre_sec},
            {"post_sec", a.post_sec},
            {"channels", a.channels}
        });
    }
    j["actions"] = actions;
    
    return j.dump();
}

} // namespace rivision::api
