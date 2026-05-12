// Package rag — citation.go resolves [REF-N] markers in LLM output to actual references (N6).
package rag

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var refPattern = regexp.MustCompile(`\[REF-(\d+)\]`)

// ResolveCitations enriches [REF-N] markers in the answer with additional context.
// Currently keeps markers as-is but validates they refer to valid references.
// Invalid references are annotated with "[invalid reference]".
func ResolveCitations(answer string, refs []Reference) string {
	return refPattern.ReplaceAllStringFunc(answer, func(match string) string {
		sub := refPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		n, err := strconv.Atoi(sub[1])
		if err != nil || n < 1 || n > len(refs) {
			return match + "[无效引用]"
		}
		return match // valid reference, keep as-is
	})
}

// ExtractCitedRefIDs extracts all unique reference IDs cited in the answer.
func ExtractCitedRefIDs(answer string) []int {
	matches := refPattern.FindAllStringSubmatch(answer, -1)
	seen := make(map[int]bool)
	var ids []int
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if !seen[n] {
			seen[n] = true
			ids = append(ids, n)
		}
	}
	return ids
}

// FilterUnusedReferences returns only the references that are actually cited in the answer.
func FilterUnusedReferences(answer string, refs []Reference) []Reference {
	cited := make(map[int]bool)
	for _, id := range ExtractCitedRefIDs(answer) {
		cited[id] = true
	}
	var filtered []Reference
	for _, r := range refs {
		if cited[r.RefID] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// FormatReferencesMarkdown formats references as a Markdown list for display.
func FormatReferencesMarkdown(refs []Reference) string {
	if len(refs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n---\n**参考来源:**\n")
	for _, r := range refs {
		fmt.Fprintf(&sb, "- **[REF-%d]** %s | 摄像头: %s | %s",
			r.RefID, r.Timestamp, r.CameraID, r.Description)
		if r.Thumbnail != "" {
			fmt.Fprintf(&sb, " | [缩略图](%s)", r.Thumbnail)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
