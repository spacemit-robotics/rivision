// Package knowledge 实现知识库同步
package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rivision/rivision-hub/pkg/models"
)

// Syncer 知识库同步器
type Syncer struct {
	mu            sync.RWMutex
	store         *Store
	nodeVersions  map[string]int // nodeID -> version
	syncInterval  time.Duration
	client        *http.Client
	stopChan      chan struct{}
}

// NodeInfo 节点信息 (Port = Worker HTTP API 端口)
type NodeInfo struct {
	ID   string
	Host string
	Port int
}

// NewSyncer 创建同步器
func NewSyncer(store *Store, syncInterval time.Duration) *Syncer {
	if syncInterval == 0 {
		syncInterval = time.Minute
	}
	return &Syncer{
		store:        store,
		nodeVersions: make(map[string]int),
		syncInterval: syncInterval,
		client:       &http.Client{Timeout: 30 * time.Second},
		stopChan:     make(chan struct{}),
	}
}

// Start 启动同步循环
func (s *Syncer) Start(ctx context.Context, getNodes func() []NodeInfo) {
	log.Printf("[KnowledgeSync] 启动同步器，间隔: %v", s.syncInterval)
	
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			nodes := getNodes()
			s.syncAll(nodes)
		}
	}
}

// Stop 停止同步
func (s *Syncer) Stop() {
	close(s.stopChan)
}

// syncAll 同步到所有节点
func (s *Syncer) syncAll(nodes []NodeInfo) {
	currentVersion := s.store.GetVersion()

	for _, node := range nodes {
		// 检查节点版本
		s.mu.RLock()
		nodeVersion := s.nodeVersions[node.ID]
		s.mu.RUnlock()

		if nodeVersion >= currentVersion {
			continue
		}

		// 需要同步
		go s.syncToNode(node, currentVersion)
	}
}

// syncToNode 同步到单个节点
func (s *Syncer) syncToNode(node NodeInfo, version int) error {
	// 获取知识库
	kb, err := s.store.GetKnowledgeBase()
	if err != nil {
		log.Printf("[KnowledgeSync] 获取知识库失败: %v", err)
		return err
	}

	// 构建同步请求
	req := models.KnowledgeSyncRequest{
		Version: kb.Version,
		Rules:   kb.Rules,
		VLMPrompts: make(map[string]string),
	}
	for _, p := range kb.Prompts {
		req.VLMPrompts[p.Scenario] = p.Template
	}

	body, _ := json.Marshal(req)

	// 发送同步请求
	url := fmt.Sprintf("http://%s:%d/api/v1/knowledge/sync", node.Host, node.Port)
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[KnowledgeSync] 同步到 %s 失败: %v", node.ID, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[KnowledgeSync] 同步到 %s 失败: HTTP %d", node.ID, resp.StatusCode)
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 解析响应
	var syncResp models.KnowledgeSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err == nil && syncResp.Success {
		s.mu.Lock()
		s.nodeVersions[node.ID] = version
		s.mu.Unlock()
		log.Printf("[KnowledgeSync] 同步到 %s 成功，版本: %d", node.ID, version)
	}

	return nil
}

// SyncNow 立即同步到指定节点
func (s *Syncer) SyncNow(node NodeInfo) (*models.KnowledgeSyncResponse, error) {
	kb, err := s.store.GetKnowledgeBase()
	if err != nil {
		return nil, err
	}

	req := models.KnowledgeSyncRequest{
		Version: kb.Version,
		Rules:   kb.Rules,
		VLMPrompts: make(map[string]string),
	}
	for _, p := range kb.Prompts {
		req.VLMPrompts[p.Scenario] = p.Template
	}

	body, _ := json.Marshal(req)
	url := fmt.Sprintf("http://%s:%d/api/v1/knowledge/sync", node.Host, node.Port)
	
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var syncResp models.KnowledgeSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return nil, err
	}

	if syncResp.Success {
		s.mu.Lock()
		s.nodeVersions[node.ID] = kb.Version
		s.mu.Unlock()
	}

	return &syncResp, nil
}

// GetNodeVersions 获取各节点版本
func (s *Syncer) GetNodeVersions() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]int)
	for k, v := range s.nodeVersions {
		result[k] = v
	}
	return result
}

// SetNodeVersion 设置节点版本（用于节点主动上报）
func (s *Syncer) SetNodeVersion(nodeID string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodeVersions[nodeID] = version
}
