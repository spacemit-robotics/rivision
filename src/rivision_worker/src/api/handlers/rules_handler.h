#pragma once

#include "rivision/config.h"
#include "rivision/types.h"
#include <string>
#include <functional>
#include <vector>

namespace rivision::api {

class RulesHandler {
public:
    using GetRulesCallback = std::function<std::vector<RuleConfig>()>;
    using AddRuleCallback = std::function<bool(const RuleConfig&)>;
    using UpdateRuleCallback = std::function<bool(const RuleConfig&)>;
    using DeleteRuleCallback = std::function<bool(const RuleId&)>;
    
    RulesHandler();
    ~RulesHandler();
    
    void setGetRulesCallback(GetRulesCallback cb) { get_callback_ = std::move(cb); }
    void setAddRuleCallback(AddRuleCallback cb) { add_callback_ = std::move(cb); }
    void setUpdateRuleCallback(UpdateRuleCallback cb) { update_callback_ = std::move(cb); }
    void setDeleteRuleCallback(DeleteRuleCallback cb) { delete_callback_ = std::move(cb); }
    
    std::string handleListRules();
    
    std::string handleGetRule(const std::string& id);
    
    std::string handleAddRule(const std::string& body);
    
    std::string handleUpdateRule(const std::string& id, const std::string& body);
    
    std::string handleDeleteRule(const std::string& id);
    
private:
    RuleConfig parseRule(const std::string& json);
    std::string ruleToJson(const RuleConfig& rule);
    
    GetRulesCallback get_callback_;
    AddRuleCallback add_callback_;
    UpdateRuleCallback update_callback_;
    DeleteRuleCallback delete_callback_;
};

} // namespace rivision::api
