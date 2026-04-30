package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/rivision/rivision-cli/internal/gateway"
)

func saveResult(result *gateway.AnalysisResult, outputPath string) error {
	return saveJSON(result, outputPath)
}

func saveSearchResults(results []gateway.SearchResult, outputPath string) error {
	return saveJSON(results, outputPath)
}

func saveSummary(summary *gateway.VideoSummary, outputPath string) error {
	return saveJSON(summary, outputPath)
}

func saveJSON(data interface{}, outputPath string) error {
	ext := strings.ToLower(filepath.Ext(outputPath))
	
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if ext == ".json" {
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		return encoder.Encode(data)
	}

	// 默认也用JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
