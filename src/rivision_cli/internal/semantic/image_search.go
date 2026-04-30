// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package semantic

import (
	"log"
	"encoding/json"
	"sort"
	"strings"
)

// BuildImageCaptionQuery Phase-2：以图搜图先走 image->caption->text embedding。
// 这里先做最小实现：将上游 caption 与 tags 合并为文本查询。
func BuildImageCaptionQuery(caption string, tags []string) string {
	parts := make([]string, 0, 2)
	cap := strings.TrimSpace(caption)
	if cap != "" {
		parts = append(parts, cap)
	}
	if len(tags) > 0 {
		parts = append(parts, strings.Join(tags, " "))
	}
	q := strings.TrimSpace(strings.Join(parts, " "))
	if q == "" {
		q = "图像检索"
	}
	return q
}

// HybridSearchInput Phase-3 预留：图文混合召回输入。
type HybridSearchInput struct {
	TextQuery      string    `json:"text_query"`
	ImageEmbedding []float64 `json:"image_embedding"`
	CameraID       string    `json:"camera_id"`
	Limit          int       `json:"limit"`
	TextWeight     float64   `json:"text_weight"`
	ImageWeight    float64   `json:"image_weight"`
}

// HybridSearch Phase-3：图文混合召回 + 加权重排。
func (s *Store) HybridSearch(in HybridSearchInput) ([]SearchResult, error) {
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}

	textQ := strings.TrimSpace(in.TextQuery)
	imgEmb := in.ImageEmbedding
	
	// 根据搜索模式自动调整权重
	isImageOnlySearch := textQ == "" && len(imgEmb) > 0
	isTextOnlySearch := textQ != "" && len(imgEmb) == 0
	
	if isImageOnlySearch {
		// 以图搜索：仅使用图像向量
		in.TextWeight = 0
		in.ImageWeight = 1.0
		textQ = "图像检索" // 占位符，不参与评分
		log.Printf("[HybridSearch] 以图搜索模式: TextWeight=0, ImageWeight=1.0")
	} else if isTextOnlySearch {
		// 纯文字搜索：仅使用文本向量
		in.TextWeight = 1.0
		in.ImageWeight = 0
		log.Printf("[HybridSearch] 文字搜索模式: TextWeight=1.0, ImageWeight=0")
	} else {
		// 混合搜索：图文结合
		if in.TextWeight <= 0 {
			in.TextWeight = 0.6
		}
		if in.ImageWeight <= 0 {
			in.ImageWeight = 0.4
		}
		log.Printf("[HybridSearch] 混合搜索模式: TextWeight=%.2f, ImageWeight=%.2f", in.TextWeight, in.ImageWeight)
	}

	// 使用 runtimeEmbedText 调用外部嵌入服务 (512维)
	var textEmb []float64
	if in.TextWeight > 0 || textQ != "图像检索" {
		var ok bool
		textEmb, ok = runtimeEmbedText(textQ, 512)
		if !ok || len(textEmb) == 0 {
			textEmb = hashEmbed(textQ, 512)
			log.Printf("[HybridSearch] runtimeEmbedText 失败，回退到 hashEmbed")
		}
	}

	var rows []struct {
		TaskID     string
		Ts         int64
		CamID      string
		StreamName string
		Thumb      string
		ResultText string
		NodeID     string
		ProcMs     int64
		Resolution string
		TextJSON   string
		ImageJSON  string
	}

	query := `SELECT task_id, timestamp_unix, camera_id, COALESCE(stream_name,'') as stream_name, thumbnail, result_text, node_id, processing_time_ms, resolution, embedding_json, image_embedding_json FROM vlm_semantic_cache`
	args := []any{}
	if strings.TrimSpace(in.CameraID) != "" && in.CameraID != "all" {
		query += ` WHERE camera_id = ?`
		args = append(args, in.CameraID)
	}
	query += ` ORDER BY timestamp_unix DESC LIMIT 3000`

	r, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for r.Next() {
		var it struct {
			TaskID     string
			Ts         int64
			CamID      string
			StreamName string
			Thumb      string
			ResultText string
			NodeID     string
			ProcMs     int64
			Resolution string
			TextJSON   string
			ImageJSON  string
		}
		if err := r.Scan(&it.TaskID, &it.Ts, &it.CamID, &it.StreamName, &it.Thumb, &it.ResultText, &it.NodeID, &it.ProcMs, &it.Resolution, &it.TextJSON, &it.ImageJSON); err != nil {
			continue
		}
		rows = append(rows, it)
	}

	out := make([]SearchResult, 0, in.Limit)
	for _, row := range rows {
		var te []float64
		if err := json.Unmarshal([]byte(row.TextJSON), &te); err != nil {
			continue
		}
		textScore := cosineSimilarity(textEmb, te)
		imgScore := 0.0
		if len(imgEmb) > 0 && strings.TrimSpace(row.ImageJSON) != "" {
			var ie []float64
			if err := json.Unmarshal([]byte(row.ImageJSON), &ie); err == nil {
				imgScore = cosineSimilarity(imgEmb, ie)
			}
		}
		// 关键词匹配加成
		kwBoost := keywordBoost(row.ResultText, textQ)
		if kwBoost > 0 {
			log.Printf("[HybridSearch] %s kwBoost=%.4f", row.TaskID, kwBoost)
		}
		score := in.TextWeight*textScore + in.ImageWeight*imgScore + 0.3*kwBoost
		out = append(out, SearchResult{
			TaskID:         row.TaskID,
			Timestamp:      parseTimestamp(float64(row.Ts)),
			CameraID:       row.CamID,
			StreamName:     row.StreamName,
			Thumbnail:      row.Thumb,
			Result:         row.ResultText,
			NodeID:         row.NodeID,
			ProcessingTime: row.ProcMs,
			Resolution:     row.Resolution,
			Score:          score,
			TextScore:      textScore,
			ImageScore:     imgScore,
		})
	}

	// 先按分数排序筛选出相关结果，再按时间降序排列
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	// 保留分数最高的结果
	if len(out) > in.Limit*2 {
		out = out[:in.Limit*2]
	}
	// 最终按时间降序排列（最新在前）
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	if len(out) > in.Limit {
		out = out[:in.Limit]
	}
	return out, nil
}


// keywordBoost 计算关键词匹配加成分数 (简化版)
func keywordBoost(text, query string) float64 {
if text == "" || query == "" || query == "图像检索" {
return 0
}

// 核心关键词列表 (从查询中提取)
keywords := []string{}
runes := []rune(query)

// 提取2字和3字词组
for length := 2; length <= 3 && length <= len(runes); length++ {
for i := 0; i <= len(runes)-length; i++ {
kw := string(runes[i : i+length])
// 跳过常见停用词
if kw != "的人" && kw != "一个" && kw != "有一" {
keywords = append(keywords, kw)
}
}
}

if len(keywords) == 0 {
return 0
}

// 计算匹配率
matchCount := 0
for _, kw := range keywords {
if strings.Contains(text, kw) {
matchCount++
}
}

score := float64(matchCount) / float64(len(keywords))
if score > 0 {
log.Printf("[kwBoost] query=%s matches=%d/%d score=%.3f", query, matchCount, len(keywords), score)
}
return score
}
