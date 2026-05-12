package search

import (
	"sort"
	"strings"
)

// AggregatedResult is a single result after global aggregation.
type AggregatedResult struct {
	DetectionID string  `json:"detection_id"`
	CameraID    string  `json:"camera_id"`
	NodeID      string  `json:"node_id"`
	Score       float64 `json:"score"`
	Timestamp   int64   `json:"timestamp"`
	Thumbnail   string  `json:"thumbnail_url,omitempty"`
	ClassName   string  `json:"class_name,omitempty"`
	Description string  `json:"description,omitempty"`
}

// Aggregator performs global sorting, de-duplication, and keyword bonus on
// results collected from multiple Workers (§5.8 aggregator.go).
type Aggregator struct {
	maxResults int
}

// NewAggregator creates a result aggregator.
func NewAggregator(maxResults int) *Aggregator {
	if maxResults <= 0 {
		maxResults = 50
	}
	return &Aggregator{maxResults: maxResults}
}

// Aggregate merges per-Worker results into a globally sorted list.
//   - Sorts by Score descending
//   - Deduplicates by DetectionID
//   - Applies keyword bonus if keywords are non-empty
//   - Truncates to maxResults
func (a *Aggregator) Aggregate(results []AggregatedResult, keywords []string) []AggregatedResult {
	// Deduplicate
	seen := make(map[string]bool)
	deduped := make([]AggregatedResult, 0, len(results))
	for _, r := range results {
		if seen[r.DetectionID] {
			continue
		}
		seen[r.DetectionID] = true

		// Keyword bonus: boost score if description/className contains keywords
		if len(keywords) > 0 {
			bonus := keywordBonus(r, keywords)
			r.Score += bonus
		}

		deduped = append(deduped, r)
	}

	// Sort by score descending
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Score > deduped[j].Score
	})

	// Truncate
	if len(deduped) > a.maxResults {
		deduped = deduped[:a.maxResults]
	}

	return deduped
}

// keywordBonus adds a small score boost for keyword matches in description/class.
func keywordBonus(r AggregatedResult, keywords []string) float64 {
	bonus := 0.0
	text := strings.ToLower(r.Description + " " + r.ClassName)
	for _, kw := range keywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			bonus += 0.05 // 5% per keyword match
		}
	}
	if bonus > 0.15 {
		bonus = 0.15 // cap at 15%
	}
	return bonus
}
