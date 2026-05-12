package rag

import (
	"strings"
	"testing"
)

func TestBuildPromptZH(t *testing.T) {
	blocks := []contextBlock{
		{RefID: 1, Timestamp: "2026-05-06T14:30:00", CameraID: "cam_entrance", Description: "一名穿红色外套的男子走入大厅"},
		{RefID: 2, Timestamp: "2026-05-06T14:32:00", CameraID: "cam_cashier", Description: "顾客正在排队结账"},
	}
	prompt := BuildPrompt("今天入口有什么异常", blocks)

	if !strings.Contains(prompt, "视频监控知识库助手") {
		t.Error("missing system prompt")
	}
	if !strings.Contains(prompt, "[REF-1]") {
		t.Error("missing REF-1")
	}
	if !strings.Contains(prompt, "[REF-2]") {
		t.Error("missing REF-2")
	}
	if !strings.Contains(prompt, "今天入口有什么异常") {
		t.Error("missing user query")
	}
}

func TestBuildPromptEN(t *testing.T) {
	blocks := []contextBlock{
		{RefID: 1, Timestamp: "2026-05-06T14:30:00", CameraID: "cam_entrance", Description: "A man in red entered the hall"},
	}
	prompt := BuildPromptWithLang("What happened at the entrance?", blocks, PromptEN)

	if !strings.Contains(prompt, "video surveillance knowledge base") {
		t.Error("missing English system prompt")
	}
	if !strings.Contains(prompt, "[REF-1]") {
		t.Error("missing REF-1")
	}
}

func TestBuildPromptEmpty(t *testing.T) {
	prompt := BuildPrompt("查找异常", nil)
	if !strings.Contains(prompt, "未检索到") {
		t.Error("missing no-results message")
	}
}

func TestResolveCitations(t *testing.T) {
	refs := []Reference{
		{RefID: 1, DetectionID: "det_001"},
		{RefID: 2, DetectionID: "det_002"},
	}
	answer := "根据[REF-1]的记录和[REF-2]的信息"
	resolved := ResolveCitations(answer, refs)
	if !strings.Contains(resolved, "[REF-1]") {
		t.Error("REF-1 should be preserved")
	}
	if strings.Contains(resolved, "无效引用") {
		t.Error("should not contain invalid reference")
	}
}

func TestResolveCitationsInvalid(t *testing.T) {
	refs := []Reference{
		{RefID: 1, DetectionID: "det_001"},
	}
	answer := "根据[REF-1]和[REF-5]的信息"
	resolved := ResolveCitations(answer, refs)
	if !strings.Contains(resolved, "[REF-5][无效引用]") {
		t.Errorf("should mark REF-5 as invalid, got: %s", resolved)
	}
}

func TestExtractCitedRefIDs(t *testing.T) {
	answer := "参考[REF-1]和[REF-3]，另外[REF-1]也提到"
	ids := ExtractCitedRefIDs(answer)
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique refs, got %d", len(ids))
	}
}

func TestFilterUnusedReferences(t *testing.T) {
	refs := []Reference{
		{RefID: 1}, {RefID: 2}, {RefID: 3},
	}
	answer := "根据[REF-1]和[REF-3]的信息"
	filtered := FilterUnusedReferences(answer, refs)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered refs, got %d", len(filtered))
	}
}

func TestEstimateTokens(t *testing.T) {
	zh := "这是一段中文测试文本"
	tokens := EstimateTokens(zh)
	if tokens < 5 {
		t.Errorf("expected at least 5 tokens for Chinese text, got %d", tokens)
	}

	en := "This is an English test text for token estimation"
	tokensEn := EstimateTokens(en)
	if tokensEn < 8 {
		t.Errorf("expected at least 8 tokens for English text, got %d", tokensEn)
	}
}

func TestDetectLanguage(t *testing.T) {
	if detectLanguage("今天入口有人吗") != PromptZH {
		t.Error("expected ZH")
	}
	if detectLanguage("What happened at entrance?") != PromptEN {
		t.Error("expected EN")
	}
}

func TestBuildContextSummary(t *testing.T) {
	blocks := []contextBlock{
		{RefID: 1, Timestamp: "2026-05-06T14:30:00", CameraID: "cam_1", Description: strings.Repeat("很长的描述", 50)},
	}
	summary := BuildContextSummary(blocks, 20)
	if len(summary) > 200 {
		t.Errorf("summary too long: %d chars", len(summary))
	}
	if !strings.Contains(summary, "...") {
		t.Error("expected truncation marker")
	}
}

func TestFormatReferencesMarkdown(t *testing.T) {
	refs := []Reference{
		{RefID: 1, Timestamp: "2026-05-06T14:30", CameraID: "cam_1", Description: "test", Thumbnail: "http://thumb/1.jpg"},
	}
	md := FormatReferencesMarkdown(refs)
	if !strings.Contains(md, "**[REF-1]**") {
		t.Error("missing ref marker")
	}
	if !strings.Contains(md, "[缩略图]") {
		t.Error("missing thumbnail link")
	}
}

func TestBuildFallbackAnswer(t *testing.T) {
	e := &Engine{}
	blocks := []contextBlock{
		{RefID: 1, Timestamp: "2026-05-06T14:30:00", CameraID: "cam_1", Description: "有人走过"},
	}
	answer := e.buildFallbackAnswer(blocks)
	if !strings.Contains(answer, "[REF-1]") {
		t.Error("missing REF-1")
	}
	if !strings.Contains(answer, "检索到") {
		t.Error("missing header")
	}

	empty := e.buildFallbackAnswer(nil)
	if !strings.Contains(empty, "未找到") {
		t.Error("missing no-result message")
	}
}
