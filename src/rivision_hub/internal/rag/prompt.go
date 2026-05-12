// Package rag — prompt.go manages LLM prompt templates for RAG Q&A (N6).
package rag

import (
	"fmt"
	"strings"
)

const systemPromptZH = `你是视频监控知识库助手。基于以下检索到的视频片段信息回答用户问题。
规则:
1. 仅基于提供的信息回答，不要编造信息
2. 如果信息不足以回答，请明确说明
3. 引用来源时使用 [REF-N] 标注
4. 回答简洁准确，使用中文`

const systemPromptEN = `You are a video surveillance knowledge base assistant. Answer user questions based on retrieved video clip information.
Rules:
1. Only answer based on provided information, do not fabricate
2. If information is insufficient, clearly state so
3. Use [REF-N] to cite sources
4. Be concise and accurate`

// PromptLanguage selects prompt language.
type PromptLanguage string

const (
	PromptZH PromptLanguage = "zh"
	PromptEN PromptLanguage = "en"
)

// BuildPrompt assembles the full LLM prompt from system instruction, context blocks, and user query.
func BuildPrompt(query string, blocks []contextBlock) string {
	lang := detectLanguage(query)
	return BuildPromptWithLang(query, blocks, lang)
}

// BuildPromptWithLang assembles the prompt with explicit language selection.
func BuildPromptWithLang(query string, blocks []contextBlock, lang PromptLanguage) string {
	var sb strings.Builder

	// System prompt
	if lang == PromptEN {
		sb.WriteString(systemPromptEN)
	} else {
		sb.WriteString(systemPromptZH)
	}
	sb.WriteString("\n\n")

	// Context section
	if len(blocks) > 0 {
		if lang == PromptEN {
			sb.WriteString("Retrieved information:\n")
		} else {
			sb.WriteString("检索到的信息:\n")
		}
		for _, b := range blocks {
			fmt.Fprintf(&sb, "[REF-%d] %s | %s | %s\n", b.RefID, b.Timestamp, b.CameraID, b.Description)
		}
		sb.WriteString("\n")
	} else {
		if lang == PromptEN {
			sb.WriteString("No relevant video clips found.\n\n")
		} else {
			sb.WriteString("未检索到相关视频片段。\n\n")
		}
	}

	// User question
	if lang == PromptEN {
		fmt.Fprintf(&sb, "User question: %s\n\nAnswer:", query)
	} else {
		fmt.Fprintf(&sb, "用户问题: %s\n\n回答:", query)
	}

	return sb.String()
}

// BuildContextSummary creates a compact context representation for limited-token models.
// Limits each block to maxCharsPerBlock characters.
func BuildContextSummary(blocks []contextBlock, maxCharsPerBlock int) string {
	if maxCharsPerBlock <= 0 {
		maxCharsPerBlock = 100
	}

	var sb strings.Builder
	for _, b := range blocks {
		desc := b.Description
		if len(desc) > maxCharsPerBlock {
			desc = desc[:maxCharsPerBlock] + "..."
		}
		fmt.Fprintf(&sb, "[REF-%d] %s %s: %s\n", b.RefID, b.Timestamp, b.CameraID, desc)
	}
	return sb.String()
}

// EstimateTokens provides a rough token count estimate.
// Chinese: ~1.5 chars per token; English: ~4 chars per token.
func EstimateTokens(text string) int {
	zhCount := 0
	enCount := 0
	for _, r := range text {
		if r > 0x4E00 && r < 0x9FFF {
			zhCount++
		} else {
			enCount++
		}
	}
	return int(float64(zhCount)/1.5) + int(float64(enCount)/4.0)
}

// detectLanguage performs simple language detection based on CJK character ratio.
func detectLanguage(text string) PromptLanguage {
	zhCount := 0
	total := 0
	for _, r := range text {
		total++
		if r >= 0x4E00 && r <= 0x9FFF {
			zhCount++
		}
	}
	if total == 0 {
		return PromptZH
	}
	if float64(zhCount)/float64(total) > 0.3 {
		return PromptZH
	}
	return PromptEN
}
