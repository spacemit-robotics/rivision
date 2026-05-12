// Package search — intent_parser.go implements natural language intent parsing (G8).
// Extracts time ranges, camera names, and semantic search text from user queries.
// Two strategies: regex-based (zero-latency default) and VLM-based (optional).
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// CameraInfo is a minimal camera descriptor used for name matching.
type CameraInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

// ParsedIntent is the structured output of intent parsing.
type ParsedIntent struct {
	SearchText string     `json:"search_text"`           // semantic core for vectorization
	TimeStart  *time.Time `json:"time_start,omitempty"`  // parsed time range start
	TimeEnd    *time.Time `json:"time_end,omitempty"`    // parsed time range end
	CameraIDs  []string   `json:"camera_ids,omitempty"`  // matched camera IDs
	Behavior   string     `json:"behavior,omitempty"`    // extracted behavior tag
	RawQuery   string     `json:"raw_query"`             // original user query
	Method     string     `json:"method"`                // "regex" or "vlm"
}

// IntentParser extracts structured filters from natural language search queries.
type IntentParser struct {
	vlmURL  string       // optional VLM endpoint for advanced parsing
	client  *http.Client
	useVLM  bool
}

// NewIntentParser creates an intent parser. If vlmURL is non-empty and useVLM is true,
// it will attempt VLM-based parsing with regex as fallback.
func NewIntentParser(vlmURL string, useVLM bool) *IntentParser {
	return &IntentParser{
		vlmURL: vlmURL,
		useVLM: useVLM && vlmURL != "",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Parse extracts structured intent from a natural language query.
// cameras provides the available camera list for name matching.
func (p *IntentParser) Parse(query string, cameras []CameraInfo, now time.Time) *ParsedIntent {
	intent := &ParsedIntent{RawQuery: query, Method: "regex"}

	// Step 1: Extract time range (regex)
	remaining := query
	remaining = p.extractTime(remaining, now, intent)

	// Step 2: Match cameras by name/location
	p.matchCameras(remaining, cameras, intent)

	// Step 3: Extract behavior keywords
	p.extractBehavior(remaining, intent)

	// Step 4: Clean remaining text as search_text
	intent.SearchText = cleanSearchText(remaining)
	if intent.SearchText == "" {
		intent.SearchText = query
	}

	return intent
}

// ParseWithVLM attempts VLM-based parsing, falling back to regex on failure.
func (p *IntentParser) ParseWithVLM(ctx context.Context, query string, cameras []CameraInfo, now time.Time) *ParsedIntent {
	if !p.useVLM {
		return p.Parse(query, cameras, now)
	}

	vlmIntent, err := p.vlmParse(ctx, query, cameras, now)
	if err == nil && vlmIntent.SearchText != "" {
		vlmIntent.Method = "vlm"
		return vlmIntent
	}

	// Fallback to regex
	return p.Parse(query, cameras, now)
}

// --- Time extraction (regex) ---

// timePattern represents a regex pattern with a time range extraction function.
type timePattern struct {
	re      *regexp.Regexp
	extract func(now time.Time, matches []string) (start, end time.Time)
}

var timePatterns = []timePattern{
	// "最近N小时" / "最近N分钟"
	{
		re: regexp.MustCompile(`最近(\d+)小时`),
		extract: func(now time.Time, m []string) (time.Time, time.Time) {
			h := parseInt(m[1], 1)
			return now.Add(-time.Duration(h) * time.Hour), now
		},
	},
	{
		re: regexp.MustCompile(`最近(\d+)分钟`),
		extract: func(now time.Time, m []string) (time.Time, time.Time) {
			mins := parseInt(m[1], 30)
			return now.Add(-time.Duration(mins) * time.Minute), now
		},
	},
	// "今天上午" / "今天下午" / "今天晚上"
	{
		re: regexp.MustCompile(`今天上午`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now)
			return d.Add(6 * time.Hour), d.Add(12 * time.Hour)
		},
	},
	{
		re: regexp.MustCompile(`今天下午`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now)
			return d.Add(12 * time.Hour), d.Add(18 * time.Hour)
		},
	},
	{
		re: regexp.MustCompile(`今天晚上`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now)
			return d.Add(18 * time.Hour), d.Add(24 * time.Hour)
		},
	},
	// "昨天上午/下午/晚上"
	{
		re: regexp.MustCompile(`昨天上午`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now).AddDate(0, 0, -1)
			return d.Add(6 * time.Hour), d.Add(12 * time.Hour)
		},
	},
	{
		re: regexp.MustCompile(`昨天下午`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now).AddDate(0, 0, -1)
			return d.Add(12 * time.Hour), d.Add(18 * time.Hour)
		},
	},
	{
		re: regexp.MustCompile(`昨天晚上`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now).AddDate(0, 0, -1)
			return d.Add(18 * time.Hour), d.Add(24 * time.Hour)
		},
	},
	// "今天" (whole day)
	{
		re: regexp.MustCompile(`今天`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			return startOfDay(now), now
		},
	},
	// "昨天"
	{
		re: regexp.MustCompile(`昨天`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now).AddDate(0, 0, -1)
			return d, d.Add(24 * time.Hour)
		},
	},
	// "前天"
	{
		re: regexp.MustCompile(`前天`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now).AddDate(0, 0, -2)
			return d, d.Add(24 * time.Hour)
		},
	},
	// "上午" / "下午" / "晚上" (today implied)
	{
		re: regexp.MustCompile(`上午`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now)
			return d.Add(6 * time.Hour), d.Add(12 * time.Hour)
		},
	},
	{
		re: regexp.MustCompile(`下午`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now)
			return d.Add(12 * time.Hour), d.Add(18 * time.Hour)
		},
	},
	{
		re: regexp.MustCompile(`晚上`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			d := startOfDay(now)
			return d.Add(18 * time.Hour), d.Add(24 * time.Hour)
		},
	},
	// "N点" / "N点到M点"
	{
		re: regexp.MustCompile(`(\d{1,2})点到(\d{1,2})点`),
		extract: func(now time.Time, m []string) (time.Time, time.Time) {
			h1, h2 := parseInt(m[1], 0), parseInt(m[2], 23)
			d := startOfDay(now)
			return d.Add(time.Duration(h1) * time.Hour), d.Add(time.Duration(h2) * time.Hour)
		},
	},
	{
		re: regexp.MustCompile(`(\d{1,2})点`),
		extract: func(now time.Time, m []string) (time.Time, time.Time) {
			h := parseInt(m[1], 0)
			d := startOfDay(now)
			return d.Add(time.Duration(h) * time.Hour), d.Add(time.Duration(h+1) * time.Hour)
		},
	},
	// "这周" / "本周"
	{
		re: regexp.MustCompile(`(这周|本周)`),
		extract: func(now time.Time, _ []string) (time.Time, time.Time) {
			weekday := int(now.Weekday())
			if weekday == 0 { weekday = 7 }
			d := startOfDay(now).AddDate(0, 0, -(weekday - 1))
			return d, now
		},
	},
}

func (p *IntentParser) extractTime(query string, now time.Time, intent *ParsedIntent) string {
	remaining := query
	for _, tp := range timePatterns {
		matches := tp.re.FindStringSubmatch(query)
		if matches != nil {
			start, end := tp.extract(now, matches)
			intent.TimeStart = &start
			intent.TimeEnd = &end
			remaining = tp.re.ReplaceAllString(remaining, "")
			break // first match wins (patterns are ordered by specificity)
		}
	}
	return remaining
}

// --- Camera matching ---

func (p *IntentParser) matchCameras(query string, cameras []CameraInfo, intent *ParsedIntent) {
	q := strings.ToLower(query)
	for _, cam := range cameras {
		if cam.Name != "" && strings.Contains(q, strings.ToLower(cam.Name)) {
			intent.CameraIDs = append(intent.CameraIDs, cam.ID)
			continue
		}
		if cam.Location != "" && strings.Contains(q, strings.ToLower(cam.Location)) {
			intent.CameraIDs = append(intent.CameraIDs, cam.ID)
		}
	}
}

// --- Behavior extraction ---

var behaviorKeywords = map[string]string{
	"徘徊": "loitering", "打架": "fighting", "奔跑": "running",
	"摔倒": "falling", "倒地": "fallen", "闯入": "intrusion",
	"翻越": "climbing", "尾随": "tailgating", "聚集": "gathering",
	"吸烟": "smoking", "打电话": "phone_call", "拥挤": "crowding",
	"遗留物": "abandoned_object", "偷窃": "theft",
}

func (p *IntentParser) extractBehavior(query string, intent *ParsedIntent) {
	for cn, en := range behaviorKeywords {
		if strings.Contains(query, cn) {
			intent.Behavior = en
			return
		}
	}
}

// --- VLM-based parsing (optional) ---

func (p *IntentParser) vlmParse(ctx context.Context, query string, cameras []CameraInfo, now time.Time) (*ParsedIntent, error) {
	camNames := make([]string, 0, len(cameras))
	for _, c := range cameras {
		name := c.Name
		if c.Location != "" {
			name += " (" + c.Location + ")"
		}
		camNames = append(camNames, name)
	}

	prompt := fmt.Sprintf(`你是搜索意图解析助手。当前时间: %s。
可用摄像头: [%s]
请解析以下搜索查询，提取结构化信息。输出纯JSON，不要markdown:
{
  "search_text": "语义搜索的核心关键词",
  "time_start": "2006-01-02T15:04:05Z07:00 或 null",
  "time_end": "2006-01-02T15:04:05Z07:00 或 null",
  "camera_names": ["匹配的摄像头名称"],
  "behavior": "行为标签 或 空"
}

搜索查询: "%s"`,
		now.Format("2006-01-02 15:04"), strings.Join(camNames, ", "), query)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"prompt":     prompt,
		"max_tokens": 200,
		"temperature": 0.1,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.vlmURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("VLM request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VLM HTTP %d", resp.StatusCode)
	}

	// Parse VLM response (expect JSON in description field)
	var vlmResp struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &vlmResp); err != nil {
		return nil, err
	}

	// Try to parse the description as intent JSON
	var parsed struct {
		SearchText  string   `json:"search_text"`
		TimeStart   string   `json:"time_start"`
		TimeEnd     string   `json:"time_end"`
		CameraNames []string `json:"camera_names"`
		Behavior    string   `json:"behavior"`
	}
	if err := json.Unmarshal([]byte(vlmResp.Description), &parsed); err != nil {
		// Try raw body
		if err2 := json.Unmarshal(body, &parsed); err2 != nil {
			return nil, fmt.Errorf("parse VLM intent: %w", err)
		}
	}

	intent := &ParsedIntent{
		SearchText: parsed.SearchText,
		Behavior:   parsed.Behavior,
		RawQuery:   query,
	}

	if parsed.TimeStart != "" && parsed.TimeStart != "null" {
		if t, err := time.Parse(time.RFC3339, parsed.TimeStart); err == nil {
			intent.TimeStart = &t
		}
	}
	if parsed.TimeEnd != "" && parsed.TimeEnd != "null" {
		if t, err := time.Parse(time.RFC3339, parsed.TimeEnd); err == nil {
			intent.TimeEnd = &t
		}
	}

	// Map camera names back to IDs
	for _, name := range parsed.CameraNames {
		for _, cam := range cameras {
			if strings.EqualFold(cam.Name, name) || strings.Contains(strings.ToLower(cam.Name), strings.ToLower(name)) {
				intent.CameraIDs = append(intent.CameraIDs, cam.ID)
				break
			}
		}
	}

	return intent, nil
}

// --- Helpers ---

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func parseInt(s string, defaultVal int) int {
	v := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int(c-'0')
		}
	}
	if v == 0 {
		return defaultVal
	}
	return v
}

// cleanSearchText removes time/location stop words from the query to get the semantic core.
var stopWords = []string{
	"今天", "昨天", "前天", "上午", "下午", "晚上", "本周", "这周",
	"最近", "小时", "分钟", "有没有", "有人", "是否",
	"在", "的", "了", "吗", "呢", "过",
}

func cleanSearchText(s string) string {
	result := strings.TrimSpace(s)
	for _, w := range stopWords {
		result = strings.ReplaceAll(result, w, "")
	}
	// Remove numbers left from time extraction
	result = regexp.MustCompile(`\d+`).ReplaceAllString(result, "")
	result = strings.TrimSpace(result)
	return result
}
