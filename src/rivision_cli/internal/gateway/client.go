package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

type Client struct {
	baseURL       string
	httpClient    *http.Client
	nodesCache    []Node
	nodesCacheTTL time.Time
	cacheMu       sync.RWMutex
}

func NewClient(baseURL string) *Client {
	timeout := getHTTPTimeout()
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		nodesCacheTTL: time.Now(),
	}
}

func getHTTPTimeout() time.Duration {
	if runtime.GOARCH == "riscv64" {
		return 30 * time.Second
	}
	return 120 * time.Second
}

func (c *Client) HealthCheck() bool {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// AnalyzeImage 图像分析
func (c *Client) AnalyzeImage(imagePath, query string) (*AnalysisResult, error) {
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("读取图片失败: %w", err)
	}
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := map[string]interface{}{
		"image":  imageBase64,
		"prompt": query,
	}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/inference/analyze", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}

	var result AnalysisResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &result, nil
}

// AnalyzeImageBytes 从字节数组分析图像
func (c *Client) AnalyzeImageBytes(imageData []byte, prompt string) (*AnalysisResult, error) {
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)
	reqBody := map[string]interface{}{
		"image":  imageBase64,
		"prompt": prompt,
	}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/inference/analyze", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}

	var result AnalysisResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &result, nil
}

// GetNodes 获取节点列表（带缓存，优先快照 API）
func (c *Client) GetNodes() ([]Node, error) {
	c.cacheMu.RLock()
	if time.Now().Before(c.nodesCacheTTL) && len(c.nodesCache) > 0 {
		defer c.cacheMu.RUnlock()
		return c.nodesCache, nil
	}
	c.cacheMu.RUnlock()

	// 优先尝试快照 API（更快）
	nodes, err := c.getNodesFromSnapshot()
	if err == nil {
		c.cacheMu.Lock()
		c.nodesCache = nodes
		c.nodesCacheTTL = time.Now().Add(10 * time.Second)
		c.cacheMu.Unlock()
		return nodes, nil
	}

	// 降级到完整 API
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/nodes")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Nodes []Node `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	c.cacheMu.Lock()
	c.nodesCache = result.Nodes
	c.nodesCacheTTL = time.Now().Add(10 * time.Second)
	c.cacheMu.Unlock()

	return result.Nodes, nil
}

func (c *Client) getNodesFromSnapshot() ([]Node, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/nodes/snapshot")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Nodes []Node `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Nodes, nil
}

// GetNodeInfo 获取单个节点信息
func (c *Client) GetNodeInfo(nodeID string) (*Node, error) {
	nodes, err := c.GetNodes()
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		if n.ID == nodeID {
			return &n, nil
		}
	}
	return nil, nil
}

// GenerateToken 生成节点注册Token
func (c *Client) GenerateToken(nodeID string, hours, maxUses int, description string) (*TokenResult, error) {
	reqBody := map[string]interface{}{
		"node_id":        nodeID,
		"lifetime_hours": hours,
		"max_uses":       maxUses,
		"description":    description,
	}
	jsonData, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/admin/node-tokens/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	var result TokenResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListTokens 获取Token列表
func (c *Client) ListTokens() ([]TokenInfo, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/admin/node-tokens/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	var result struct {
		Data []TokenInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// RevokeToken 撤销Token（按token前缀）
func (c *Client) RevokeToken(tokenPrefix string) error {
	req, _ := http.NewRequest("DELETE", c.baseURL+"/api/v1/admin/node-tokens/revoke/"+tokenPrefix, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// RevokeTokenByNodeID 撤销Token（按节点ID）
func (c *Client) RevokeTokenByNodeID(nodeID string) error {
	req, _ := http.NewRequest("DELETE", c.baseURL+"/api/v1/admin/node-tokens/revoke-by-node/"+nodeID, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// CleanupTokens 清理过期Token（按节点ID）
func (c *Client) CleanupTokens(nodeID string) error {
	req, _ := http.NewRequest("DELETE", c.baseURL+"/api/v1/admin/node-tokens/cleanup-by-node/"+nodeID, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// AddToBlacklist 添加IP到黑名单
func (c *Client) AddToBlacklist(ip, reason string) error {
	return c.ManageBlacklist(ip, "add", reason)
}

// RemoveFromBlacklist 从黑名单移除IP
func (c *Client) RemoveFromBlacklist(ip string) error {
	return c.ManageBlacklist(ip, "remove", "")
}

// SearchVideo 视频搜索（逐帧分析）
func (c *Client) SearchVideo(frames []Frame, query string, confidence float64) ([]SearchResult, error) {
	var results []SearchResult
	for _, frame := range frames {
		reqBody := map[string]interface{}{
			"image":  frame.Base64,
			"prompt": query,
		}
		jsonData, _ := json.Marshal(reqBody)
		resp, err := c.httpClient.Post(c.baseURL+"/api/v1/inference/analyze", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			continue
		}
		var ar AnalysisResult
		json.NewDecoder(resp.Body).Decode(&ar)
		resp.Body.Close()
		results = append(results, SearchResult{
			Timestamp:   frame.Timestamp,
			FrameIndex:  frame.Index,
			Description: ar.Content,
			Confidence:  confidence,
			Matched:     len(ar.Content) > 0,
		})
	}
	return results, nil
}

// SummarizeVideo 视频摘要
func (c *Client) SummarizeVideo(frames []Frame) (string, error) {
	reqBody := map[string]interface{}{
		"frames": frames,
		"prompt": "请对这些视频帧进行摘要",
	}
	jsonData, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/inference/analyze", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	var result AnalysisResult
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Content, nil
}

// GenerateSummary 生成视频摘要（逐帧分析后汇总）
func (c *Client) GenerateSummary(frames []Frame, info *VideoInfo) (*VideoSummary, error) {
	var scenes []KeyScene

	// 逐帧分析
	for i, frame := range frames {
		reqBody := map[string]interface{}{
			"image":  frame.Base64,
			"prompt": "请描述这个视频帧中的场景、人物和活动。用中文简洁回答。",
		}
		jsonData, _ := json.Marshal(reqBody)

		resp, err := c.httpClient.Post(c.baseURL+"/api/v1/inference/analyze", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var ar AnalysisResult
			json.NewDecoder(resp.Body).Decode(&ar)
			if ar.Content != "" {
				scenes = append(scenes, KeyScene{
					Timestamp:   frame.Timestamp,
					Description: ar.Content,
				})
			}
		}
		resp.Body.Close()

		// 打印进度
		fmt.Printf("\r  分析帧 %d/%d...", i+1, len(frames))
	}
	fmt.Println()

	// 汇总摘要
	overall := ""
	if len(scenes) > 0 {
		// 用最后一帧请求整体摘要
		var descriptions string
		for i, s := range scenes {
			descriptions += fmt.Sprintf("帧%d (%.1fs): %s\n", i+1, s.Timestamp, s.Description)
		}
		summaryReq := map[string]interface{}{
			"image":  frames[0].Base64,
			"prompt": fmt.Sprintf("以下是视频各帧的描述，请生成一段整体摘要：\n%s", descriptions),
		}
		jsonData, _ := json.Marshal(summaryReq)
		resp, err := c.httpClient.Post(c.baseURL+"/api/v1/inference/analyze", "application/json", bytes.NewBuffer(jsonData))
		if err == nil && resp.StatusCode == http.StatusOK {
			var ar AnalysisResult
			json.NewDecoder(resp.Body).Decode(&ar)
			overall = ar.Content
			resp.Body.Close()
		}
	}

	return &VideoSummary{
		TotalDuration:  info.Duration,
		SegmentCount:   len(scenes),
		OverallSummary: overall,
		KeyScenes:      scenes,
	}, nil
}

// GetRegistrationStats 获取注册统计
func (c *Client) GetRegistrationStats() (*RegistrationStats, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/admin/node-tokens/stats")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	var result RegistrationStats
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}

// ListBlacklist 获取黑名单列表
func (c *Client) ListBlacklist() ([]BlacklistItem, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/admin/node-tokens/blacklist/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		BlacklistedIPs []BlacklistItem `json:"blacklisted_ips"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.BlacklistedIPs, nil
}

// ManageBlacklist 管理黑名单（添加/移除）
func (c *Client) ManageBlacklist(ip, action, reason string) error {
	reqBody := map[string]string{
		"ip":     ip,
		"action": action,
		"reason": reason,
	}
	jsonData, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/admin/node-tokens/blacklist/manage", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API错误 (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// DetectYOLO YOLO目标检测（从文件路径）
func (c *Client) DetectYOLO(imagePath string) (*YOLODetectResult, error) {
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("读取图片失败: %w", err)
	}
	return c.DetectYOLOBase64(base64.StdEncoding.EncodeToString(imageData))
}

// DetectYOLOBase64 YOLO目标检测（从base64数据）
func (c *Client) DetectYOLOBase64(imageBase64 string) (*YOLODetectResult, error) {
	reqBody := map[string]string{"image": imageBase64}
	jsonData, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/yolo/detect", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("YOLO API错误 (%d): %s", resp.StatusCode, string(body))
	}
	var result YOLODetectResult
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}
