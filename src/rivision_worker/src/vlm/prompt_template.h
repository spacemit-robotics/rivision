#pragma once

#include <string>
#include <map>
#include <regex>
#include <sstream>

namespace rivision::vlm {

// =============================================================================
// PromptTemplate - VLM prompt templates for various verification tasks
// =============================================================================

class PromptTemplate {
public:
    // Template types
    enum class Type {
        OBJECT_VERIFY,          // 验证检测到的物体
        BEHAVIOR_VERIFY,        // 验证行为（如打架、跌倒）
        EMOTION_CLASSIFY,       // 情绪分类
        SCENE_DESCRIBE,         // 场景描述
        FACE_COMPARE,           // 人脸比对
        ANOMALY_DETECT,         // 异常检测
        CROWD_COUNT,            // 人群计数
        CUSTOM                  // 自定义模板
    };
    
    // Get default template for type
    static std::string getTemplate(Type type) {
        switch (type) {
            case Type::OBJECT_VERIFY:
                return R"(请分析图像中标记区域的物体。
问题: 这个区域是否包含 {class_name}？
请回答 "是" 或 "否"，并简要说明原因。)";
            
            case Type::BEHAVIOR_VERIFY:
                return R"(请分析图像中的场景。
问题: 是否存在 {behavior} 行为？
请回答 "是" 或 "否"，并描述你观察到的情况。)";
            
            case Type::EMOTION_CLASSIFY:
                return R"(请分析图像中人物的表情和情绪。
可能的情绪类别: 高兴、悲伤、愤怒、惊讶、恐惧、厌恶、中性
请返回JSON格式: {"emotion": "情绪", "confidence": 0.0-1.0, "reason": "原因"})";
            
            case Type::SCENE_DESCRIBE:
                return R"(请描述图像中的场景。
包括: 主要物体、人物数量、活动、环境特征。
请用简洁的中文描述，不超过100字。)";
            
            case Type::FACE_COMPARE:
                return R"(请比较图像中的两张人脸是否为同一人。
请回答: "是同一人" 或 "不是同一人"
相似度评分: 0-100)";
            
            case Type::ANOMALY_DETECT:
                return R"(请分析图像中是否存在异常情况。
关注: 可疑物品、异常行为、安全隐患
请回答 "正常" 或 "异常"，如异常请描述具体情况。)";
            
            case Type::CROWD_COUNT:
                return R"(请统计图像中的人数。
请返回JSON格式: {"count": 人数, "confidence": 0.0-1.0, "distribution": "描述"})";
            
            default:
                return "{prompt}";
        }
    }
    
    // Render template with variables
    static std::string render(const std::string& tmpl, 
                             const std::map<std::string, std::string>& vars) {
        std::string result = tmpl;
        
        for (const auto& [key, value] : vars) {
            std::string placeholder = "{" + key + "}";
            size_t pos = 0;
            while ((pos = result.find(placeholder, pos)) != std::string::npos) {
                result.replace(pos, placeholder.length(), value);
                pos += value.length();
            }
        }
        
        return result;
    }
    
    // Render with type and variables
    static std::string render(Type type, const std::map<std::string, std::string>& vars) {
        return render(getTemplate(type), vars);
    }
    
    // Build object verification prompt
    static std::string objectVerify(const std::string& class_name, 
                                    float confidence = 0.0f) {
        std::map<std::string, std::string> vars = {
            {"class_name", class_name},
            {"confidence", std::to_string(confidence)}
        };
        return render(Type::OBJECT_VERIFY, vars);
    }
    
    // Build behavior verification prompt
    static std::string behaviorVerify(const std::string& behavior) {
        return render(Type::BEHAVIOR_VERIFY, {{"behavior", behavior}});
    }
    
    // Build emotion classification prompt
    static std::string emotionClassify() {
        return getTemplate(Type::EMOTION_CLASSIFY);
    }
    
    // Build scene description prompt
    static std::string sceneDescribe() {
        return getTemplate(Type::SCENE_DESCRIBE);
    }
    
    // Build anomaly detection prompt
    static std::string anomalyDetect() {
        return getTemplate(Type::ANOMALY_DETECT);
    }
    
    // Build crowd counting prompt
    static std::string crowdCount() {
        return getTemplate(Type::CROWD_COUNT);
    }
    
    // Build custom prompt with context
    static std::string custom(const std::string& prompt,
                             const std::string& context = "") {
        if (context.empty()) {
            return prompt;
        }
        
        std::ostringstream oss;
        oss << "背景信息:\n" << context << "\n\n";
        oss << "问题:\n" << prompt;
        return oss.str();
    }
};

// =============================================================================
// VLM Response Parser - Parse VLM responses
// =============================================================================

class VlmResponseParser {
public:
    struct VerifyResult {
        bool verified = false;
        float confidence = 0.0f;
        std::string reason;
    };
    
    struct EmotionResult {
        std::string emotion = "neutral";
        float confidence = 0.0f;
        std::string reason;
    };
    
    struct CountResult {
        int count = 0;
        float confidence = 0.0f;
        std::string distribution;
    };
    
    // Parse yes/no verification response
    static VerifyResult parseVerify(const std::string& response) {
        VerifyResult result;
        
        std::string lower = response;
        std::transform(lower.begin(), lower.end(), lower.begin(), ::tolower);
        
        // Check for positive keywords
        if (lower.find("是") != std::string::npos ||
            lower.find("yes") != std::string::npos ||
            lower.find("确认") != std::string::npos ||
            lower.find("存在") != std::string::npos) {
            result.verified = true;
            result.confidence = 0.8f;
        }
        
        // Check for negative keywords
        if (lower.find("否") != std::string::npos ||
            lower.find("no") != std::string::npos ||
            lower.find("不是") != std::string::npos ||
            lower.find("没有") != std::string::npos) {
            result.verified = false;
            result.confidence = 0.8f;
        }
        
        result.reason = response;
        return result;
    }
    
    // Parse emotion classification response (expects JSON)
    static EmotionResult parseEmotion(const std::string& response) {
        EmotionResult result;
        
        // Try to find emotion in response
        static const std::vector<std::pair<std::string, std::string>> emotions = {
            {"高兴", "happy"}, {"happy", "happy"},
            {"悲伤", "sad"}, {"sad", "sad"},
            {"愤怒", "angry"}, {"angry", "angry"},
            {"惊讶", "surprised"}, {"surprised", "surprised"},
            {"恐惧", "fear"}, {"fear", "fear"},
            {"厌恶", "disgust"}, {"disgust", "disgust"},
            {"中性", "neutral"}, {"neutral", "neutral"}
        };
        
        std::string lower = response;
        std::transform(lower.begin(), lower.end(), lower.begin(), ::tolower);
        
        for (const auto& [keyword, emotion] : emotions) {
            if (lower.find(keyword) != std::string::npos) {
                result.emotion = emotion;
                result.confidence = 0.7f;
                break;
            }
        }
        
        result.reason = response;
        return result;
    }
    
    // Parse count response
    static CountResult parseCount(const std::string& response) {
        CountResult result;
        
        // Try to extract number
        std::regex num_regex(R"(\d+)");
        std::smatch match;
        if (std::regex_search(response, match, num_regex)) {
            result.count = std::stoi(match[0].str());
            result.confidence = 0.7f;
        }
        
        result.distribution = response;
        return result;
    }
};

} // namespace rivision::vlm
