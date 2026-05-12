package knowledge

// RuleTemplate provides predefined rule templates per §5.10 scene presets.
type RuleTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Scene       string `json:"scene"`       // retail, warehouse, traffic, high-precision, low-power
	Description string `json:"description"`
	RuleYAML    string `json:"rule_yaml"`   // YAML DSL template
}

// DefaultTemplates returns the 5 built-in scene templates from §5.10.
func DefaultTemplates() []RuleTemplate {
	return []RuleTemplate{
		{
			ID: "tpl_retail", Name: "零售场景", Scene: "retail",
			Description: "客流统计 + 异常行为检测",
			RuleYAML: `
trigger:
  class: person
  action: enter_zone
  zone: checkout_area
alert:
  severity: info
  message: "客流计数 +1"
`,
		},
		{
			ID: "tpl_warehouse", Name: "仓储场景", Scene: "warehouse",
			Description: "入侵检测 + 安全帽检测",
			RuleYAML: `
trigger:
  class: person
  action: enter_zone
  zone: restricted_area
  schedule: "22:00-06:00"
alert:
  severity: critical
  message: "限制区域入侵"
  vlm_verify: true
`,
		},
		{
			ID: "tpl_traffic", Name: "交通场景", Scene: "traffic",
			Description: "车辆计数 + 违停检测",
			RuleYAML: `
trigger:
  class: [car, truck, bus]
  action: dwell
  dwell_seconds: 300
  zone: no_parking
alert:
  severity: warning
  message: "违停检测"
`,
		},
		{
			ID: "tpl_high_precision", Name: "高精度场景", Scene: "high-precision",
			Description: "VLM 二次验证, 低漏报",
			RuleYAML: `
trigger:
  class: person
  confidence: 0.3
alert:
  severity: warning
  vlm_verify: true
  vlm_prompt: "请确认画面中是否有人员"
`,
		},
		{
			ID: "tpl_low_power", Name: "低功耗场景", Scene: "low-power",
			Description: "降低采样率, 减少推理负载",
			RuleYAML: `
trigger:
  class: person
  confidence: 0.5
sampler:
  min_interval_ms: 2000
  motion_threshold: 30
`,
		},
	}
}
