#include <gtest/gtest.h>
#include "core/rules/engine.h"
#include "rivision/types.h"
#include "rivision/config.h"

using namespace rivision;
using namespace rivision::core;

class RulesEngineTest : public ::testing::Test {
protected:
    void SetUp() override {
        engine_ = std::make_unique<RulesEngine>();
    }
    
    void TearDown() override {
        engine_.reset();
    }
    
    std::unique_ptr<RulesEngine> engine_;
};

TEST_F(RulesEngineTest, EmptyRules) {
    std::vector<Detection> dets;
    std::vector<Track> tracks;
    
    auto alerts = engine_->evaluate("stream-1", dets, tracks);
    
    EXPECT_TRUE(alerts.empty());
}

TEST_F(RulesEngineTest, ZoneTrigger) {
    // Create rule with zone trigger
    RuleConfig rule;
    rule.id = "zone-1";
    rule.name = "Test Zone";
    rule.enabled = true;
    rule.streams = {"*"};
    rule.trigger.type = "zone";
    rule.trigger.zone = ZoneConfig{
        "test_zone",
        {{0.0f, 0.0f}, {1.0f, 0.0f}, {1.0f, 1.0f}, {0.0f, 1.0f}}
    };
    rule.trigger.classes = {0};  // person
    
    RuleConfig::Action alert_action;
    alert_action.type = "alert";
    alert_action.level = "warning";
    rule.actions.push_back(alert_action);
    
    engine_->loadRules({rule});
    
    // Create detection inside zone
    Detection det;
    det.bbox = {0.4f, 0.4f, 0.6f, 0.6f};  // center at 0.5, 0.5
    det.class_id = 0;
    det.confidence = 0.8f;
    
    std::vector<Detection> dets = {det};
    std::vector<Track> tracks;
    
    auto alerts = engine_->evaluate("stream-1", dets, tracks);
    
    EXPECT_EQ(alerts.size(), 1);
    EXPECT_EQ(alerts[0].rule_id, "zone-1");
    EXPECT_EQ(alerts[0].trigger_type, "zone");
    EXPECT_EQ(alerts[0].level, AlertLevel::WARNING);
}

TEST_F(RulesEngineTest, ClassCountTrigger) {
    RuleConfig rule;
    rule.id = "count-1";
    rule.name = "Crowding";
    rule.enabled = true;
    rule.trigger.type = "class_count";
    rule.trigger.classes = {0};
    rule.trigger.count_operator = ">";
    rule.trigger.count_value = 2;
    
    RuleConfig::Action alert_action;
    alert_action.type = "alert";
    alert_action.level = "warning";
    rule.actions.push_back(alert_action);
    
    engine_->loadRules({rule});
    
    // 3 detections - should trigger
    std::vector<Detection> dets(3);
    for (int i = 0; i < 3; ++i) {
        dets[i].class_id = 0;
        dets[i].confidence = 0.8f;
        dets[i].bbox = {0.1f * i, 0.1f, 0.1f * i + 0.1f, 0.2f};
    }
    
    std::vector<Track> tracks;
    auto alerts = engine_->evaluate("stream-1", dets, tracks);
    
    EXPECT_EQ(alerts.size(), 1);
    EXPECT_EQ(alerts[0].trigger_type, "class_count");
}

TEST_F(RulesEngineTest, ClassCountNotTriggered) {
    RuleConfig rule;
    rule.id = "count-1";
    rule.name = "Crowding";
    rule.enabled = true;
    rule.trigger.type = "class_count";
    rule.trigger.classes = {0};
    rule.trigger.count_operator = ">";
    rule.trigger.count_value = 5;
    
    RuleConfig::Action alert_action;
    alert_action.type = "alert";
    alert_action.level = "warning";
    rule.actions.push_back(alert_action);
    
    engine_->loadRules({rule});
    
    // Only 3 detections - should NOT trigger (needs > 5)
    std::vector<Detection> dets(3);
    for (int i = 0; i < 3; ++i) {
        dets[i].class_id = 0;
        dets[i].confidence = 0.8f;
    }
    
    std::vector<Track> tracks;
    auto alerts = engine_->evaluate("stream-1", dets, tracks);
    
    EXPECT_TRUE(alerts.empty());
}

TEST_F(RulesEngineTest, DisabledRule) {
    RuleConfig rule;
    rule.id = "disabled-1";
    rule.name = "Disabled Rule";
    rule.enabled = false;  // Disabled
    rule.trigger.type = "class_presence";
    rule.trigger.classes = {0};
    
    engine_->loadRules({rule});
    
    Detection det;
    det.class_id = 0;
    det.confidence = 0.8f;
    
    std::vector<Detection> dets = {det};
    std::vector<Track> tracks;
    
    auto alerts = engine_->evaluate("stream-1", dets, tracks);
    
    EXPECT_TRUE(alerts.empty());
}

TEST_F(RulesEngineTest, StreamFilter) {
    RuleConfig rule;
    rule.id = "stream-filter-1";
    rule.name = "Stream Filter Test";
    rule.enabled = true;
    rule.streams = {"stream-1"};  // Only stream-1
    rule.trigger.type = "class_presence";
    rule.trigger.classes = {0};
    
    RuleConfig::Action alert_action;
    alert_action.type = "alert";
    alert_action.level = "info";
    rule.actions.push_back(alert_action);
    
    engine_->loadRules({rule});
    
    Detection det;
    det.class_id = 0;
    det.confidence = 0.8f;
    
    std::vector<Detection> dets = {det};
    std::vector<Track> tracks;
    
    // Should trigger on stream-1
    auto alerts1 = engine_->evaluate("stream-1", dets, tracks);
    EXPECT_EQ(alerts1.size(), 1);
    
    // Should NOT trigger on stream-2
    auto alerts2 = engine_->evaluate("stream-2", dets, tracks);
    EXPECT_TRUE(alerts2.empty());
}

TEST_F(RulesEngineTest, ZoneContains) {
    ZoneConfig zone;
    zone.name = "test";
    zone.polygon = {
        {0.2f, 0.2f},
        {0.8f, 0.2f},
        {0.8f, 0.8f},
        {0.2f, 0.8f}
    };
    
    // Point inside
    EXPECT_TRUE(zone.contains(0.5f, 0.5f));
    
    // Points outside
    EXPECT_FALSE(zone.contains(0.0f, 0.0f));
    EXPECT_FALSE(zone.contains(0.1f, 0.5f));
    EXPECT_FALSE(zone.contains(0.5f, 0.9f));
}

int main(int argc, char** argv) {
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}
