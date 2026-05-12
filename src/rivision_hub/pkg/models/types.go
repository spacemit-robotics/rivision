// Package models 定义共享数据结构: Detection, Alert, Camera, Rule (v6_design §2.1.1)
package models

import (
	"time"
)

// ========================================
// Alert 相关
// ========================================

// AlertLevel 告警级别
type AlertLevel string

const (
	AlertLevelLow      AlertLevel = "low"
	AlertLevelMedium   AlertLevel = "medium"
	AlertLevelHigh     AlertLevel = "high"
	AlertLevelCritical AlertLevel = "critical"
)

// AlertStatus 告警状态
type AlertStatus string

const (
	AlertStatusPending  AlertStatus = "pending"
	AlertStatusAcked    AlertStatus = "acked"
	AlertStatusResolved AlertStatus = "resolved"
)

// AlertType 告警类型
type AlertType string

const (
	AlertTypeIntrusion        AlertType = "intrusion"
	AlertTypeLoitering        AlertType = "loitering"
	AlertTypeCrowd            AlertType = "crowd"
	AlertTypeFire             AlertType = "fire"
	AlertTypeFall             AlertType = "fall"
	AlertTypeCustomerVIP      AlertType = "customer_vip"
	AlertTypeCustomerInterest AlertType = "customer_interest"
	AlertTypeQueueLong        AlertType = "queue_long"
)

// Alert 告警实体
type Alert struct {
	ID          string      `json:"id" db:"id"`
	NodeID      string      `json:"node_id" db:"node_id"`
	CameraID    string      `json:"camera_id" db:"camera_id"`
	Type        AlertType   `json:"type" db:"type"`
	Level       AlertLevel  `json:"level" db:"level"`
	Status      AlertStatus `json:"status" db:"status"`
	Title       string      `json:"title" db:"title"`
	Description string      `json:"description" db:"description"`
	Thumbnail   string      `json:"thumbnail,omitempty" db:"thumbnail"`
	VLMResult   string      `json:"vlm_result,omitempty" db:"vlm_result"`
	Detections  []Detection `json:"detections,omitempty" db:"-"`
	Metadata    Metadata    `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	AckedAt     *time.Time  `json:"acked_at,omitempty" db:"acked_at"`
	ResolvedAt  *time.Time  `json:"resolved_at,omitempty" db:"resolved_at"`
	AckedBy     string      `json:"acked_by,omitempty" db:"acked_by"`
}

// Detection 检测目标
type Detection struct {
	Class      string    `json:"class"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
}

// Metadata 元数据
type Metadata map[string]interface{}

// AlertEvent 告警事件（从Worker上报）
type AlertEvent struct {
	Type      string      `json:"type"`
	NodeID    string      `json:"node_id"`
	CameraID  string      `json:"camera_id"`
	Event     EventDetail `json:"event"`
	Timestamp time.Time   `json:"timestamp"`
}

// EventDetail 事件详情
type EventDetail struct {
	Type        AlertType   `json:"type"`
	RiskLevel   AlertLevel  `json:"risk_level"`
	Description string      `json:"vlm_description,omitempty"`
	Thumbnail   string      `json:"thumbnail,omitempty"`
	Detections  []Detection `json:"detections,omitempty"`
}

// AlertStats 告警统计
type AlertStats struct {
	Total    int64            `json:"total"`
	Pending  int64            `json:"pending"`
	Acked    int64            `json:"acked"`
	Resolved int64            `json:"resolved"`
	ByLevel  map[string]int64 `json:"by_level"`
	ByType   map[string]int64 `json:"by_type"`
	Last24h  int64            `json:"last_24h"`
	LastHour int64            `json:"last_hour"`
}

// AlertQuery 告警查询参数
type AlertQuery struct {
	NodeID    string      `form:"node_id"`
	CameraID  string      `form:"camera_id"`
	Type      AlertType   `form:"type"`
	Level     AlertLevel  `form:"level"`
	Status    AlertStatus `form:"status"`
	StartTime *time.Time  `form:"start_time"`
	EndTime   *time.Time  `form:"end_time"`
	Limit     int         `form:"limit"`
	Offset    int         `form:"offset"`
}

// GetDefaultAlertLevel 根据告警类型获取默认级别
func GetDefaultAlertLevel(t AlertType) AlertLevel {
	switch t {
	case AlertTypeFire:
		return AlertLevelCritical
	case AlertTypeIntrusion, AlertTypeFall:
		return AlertLevelHigh
	case AlertTypeLoitering, AlertTypeCrowd, AlertTypeQueueLong:
		return AlertLevelMedium
	case AlertTypeCustomerVIP, AlertTypeCustomerInterest:
		return AlertLevelLow
	default:
		return AlertLevelMedium
	}
}

// GetAlertTitle 根据告警类型获取标题
func GetAlertTitle(t AlertType) string {
	titles := map[AlertType]string{
		AlertTypeIntrusion:        "入侵检测告警",
		AlertTypeLoitering:        "徘徊检测告警",
		AlertTypeCrowd:            "人群聚集告警",
		AlertTypeFire:             "火灾检测告警",
		AlertTypeFall:             "跌倒检测告警",
		AlertTypeCustomerVIP:      "VIP客户到店",
		AlertTypeCustomerInterest: "客户关注商品",
		AlertTypeQueueLong:        "排队过长告警",
	}
	if title, ok := titles[t]; ok {
		return title
	}
	return "未知告警"
}

// CameraStatusEvent 摄像头状态事件
type CameraStatusEvent struct {
	CameraID  string                 `json:"camera_id"`
	NodeID    string                 `json:"node_id"`
	Status    string                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Stream    map[string]interface{} `json:"stream,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// ========================================
// Rule 相关
// ========================================

// RuleType 规则类型
type RuleType string

const (
	RuleTypeDetection RuleType = "detection"
	RuleTypeBehavior  RuleType = "behavior"
	RuleTypeScene     RuleType = "scene"
	RuleTypeCustom    RuleType = "custom"
)

// RuleAction 规则动作
type RuleAction string

const (
	RuleActionAlert   RuleAction = "alert"
	RuleActionVLM     RuleAction = "vlm"
	RuleActionWebhook RuleAction = "webhook"
	RuleActionRecord  RuleAction = "record"
	RuleActionNotify  RuleAction = "notify"
)

// Rule 规则定义
type Rule struct {
	ID          string      `json:"id" db:"id"`
	Name        string      `json:"name" db:"name"`
	Description string      `json:"description" db:"description"`
	Type        RuleType    `json:"type" db:"type"`
	Enabled     bool        `json:"enabled" db:"enabled"`
	Priority    int         `json:"priority" db:"priority"`
	Conditions  []Condition `json:"conditions" db:"-"`
	Actions     []ActionDef `json:"actions" db:"-"`
	VLMPrompt   string      `json:"vlm_prompt,omitempty" db:"vlm_prompt"`
	CameraIDs   []string    `json:"camera_ids,omitempty" db:"-"`
	Schedule    *Schedule   `json:"schedule,omitempty" db:"-"`
	Version     int         `json:"version" db:"version"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
}

// Condition 规则条件
type Condition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Logic    string      `json:"logic,omitempty"`
}

// ActionDef 动作定义
type ActionDef struct {
	Type   RuleAction             `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// Schedule 规则生效时间
type Schedule struct {
	TimeRanges []TimeRange `json:"time_ranges,omitempty"`
	Weekdays   []int       `json:"weekdays,omitempty"`
}

// TimeRange 时间范围
type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// VLMPromptTemplate VLM提示词模板
type VLMPromptTemplate struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Scenario    string    `json:"scenario" db:"scenario"`
	Template    string    `json:"template" db:"template"`
	Variables   []string  `json:"variables" db:"-"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// KnowledgeBase 知识库
type KnowledgeBase struct {
	Version   int                 `json:"version"`
	Rules     []Rule              `json:"rules"`
	Prompts   []VLMPromptTemplate `json:"prompts"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// KnowledgeSyncRequest 知识库同步请求
type KnowledgeSyncRequest struct {
	Version    int               `json:"version"`
	Rules      []Rule            `json:"rules"`
	VLMPrompts map[string]string `json:"vlm_prompts"`
}

// KnowledgeSyncResponse 知识库同步响应
type KnowledgeSyncResponse struct {
	Success bool   `json:"success"`
	NodeID  string `json:"node_id"`
	Version int    `json:"version"`
	Message string `json:"message,omitempty"`
}

// CreateRuleRequest 创建规则请求
type CreateRuleRequest struct {
	Name        string      `json:"name" binding:"required"`
	Description string      `json:"description"`
	Type        RuleType    `json:"type" binding:"required"`
	Conditions  []Condition `json:"conditions" binding:"required"`
	Actions     []ActionDef `json:"actions" binding:"required"`
	VLMPrompt   string      `json:"vlm_prompt"`
	CameraIDs   []string    `json:"camera_ids"`
	Schedule    *Schedule   `json:"schedule"`
	Priority    int         `json:"priority"`
}

// UpdateRuleRequest 更新规则请求
type UpdateRuleRequest struct {
	Name        *string     `json:"name"`
	Description *string     `json:"description"`
	Enabled     *bool       `json:"enabled"`
	Conditions  []Condition `json:"conditions"`
	Actions     []ActionDef `json:"actions"`
	VLMPrompt   *string     `json:"vlm_prompt"`
	CameraIDs   []string    `json:"camera_ids"`
	Schedule    *Schedule   `json:"schedule"`
	Priority    *int        `json:"priority"`
}

// DefaultPromptTemplates 默认提示词模板
var DefaultPromptTemplates = map[string]string{
	"security_intrusion": `分析这张监控画面，判断是否存在入侵行为。
如果检测到可疑人员，请描述：
1. 人员数量和位置
2. 行为特征（是否在徘徊、翻越、试探等）
3. 风险等级评估（低/中/高/极高）
4. 建议处置措施`,

	"security_loitering": `分析这张监控画面，判断是否存在徘徊行为。
请描述：
1. 目标人员特征
2. 停留时间估计
3. 行为是否异常
4. 风险等级评估`,

	"retail_customer": `分析这张零售场景画面，识别客户行为。
请描述：
1. 客户数量和分布
2. 正在关注的商品区域
3. 停留时间和兴趣程度
4. 是否需要店员协助`,

	"retail_vip": `这是VIP客户到店场景，请分析：
1. 客户当前位置
2. 可能的购物意向
3. 推荐的服务方式
4. 历史偏好参考`,
}

// ========================================
// Target 相关 (知识库特征目标)
// ========================================

// Target 特征目标（人员、车辆等识别目标）
type Target struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Type        string    `json:"type" db:"type"`               // person, vehicle, object
	Description string    `json:"description" db:"description"`
	Tags        []string  `json:"tags" db:"-"`
	ImageURL    string    `json:"image_url,omitempty" db:"image_url"`
	Features    []float64 `json:"features,omitempty" db:"-"`    // embedding vector
	Metadata    Metadata  `json:"metadata,omitempty" db:"metadata"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateTargetRequest 创建目标请求
type CreateTargetRequest struct {
	Name        string   `json:"name" form:"name" binding:"required"`
	Type        string   `json:"type" form:"type" binding:"required"`
	Description string   `json:"description" form:"description"`
	Tags        []string `json:"tags" form:"tags"`
}

// ========================================
// Camera 相关
// ========================================

// Camera 摄像头
type Camera struct {
	ID       string `json:"id" db:"id"`
	Name     string `json:"name" db:"name"`
	URL      string `json:"url" db:"url"`
	Protocol string `json:"protocol,omitempty" db:"protocol"`
	GroupID  string `json:"group_id,omitempty" db:"group_id"`
	NodeID   string `json:"node_id,omitempty" db:"node_id"`
	Status   string `json:"status" db:"status"`
	Enabled  bool   `json:"enabled" db:"enabled"`
}
