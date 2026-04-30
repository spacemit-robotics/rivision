// Package auth 提供节点认证管理
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NodeRegisteredCallback 节点注册/重连成功时的回调
type NodeRegisteredCallback func(node *RegisteredNode)

// NodeAuthManager 节点认证管理器
type NodeAuthManager struct {
	mu                   sync.RWMutex
	registrationTokens   map[string]*RegistrationToken
	registeredNodes      map[string]*RegisteredNode
	ipToNode             map[string]string
	blacklist            map[string]*BlacklistEntry
	failureCount         map[string]int
	dataDir              string
	enablePersistence    bool
	onNodeRegistered     NodeRegisteredCallback // 节点注册/重连成功时的回调
}

// RegistrationToken 注册 Token
type RegistrationToken struct {
	Token       string    `json:"token"`
	NodeID      string    `json:"node_id"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	MaxUses     int       `json:"max_uses"`
	UsedCount   int       `json:"used_count"`
	Description string    `json:"description"`
}

// RegisteredNode 已注册节点
type RegisteredNode struct {
	NodeID        string    `json:"node_id"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	AgentPort     int       `json:"agent_port"`
	Weight        int       `json:"weight"`
	Tags          []string  `json:"tags"`
	RegisteredAt  time.Time `json:"registered_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	CurrentIP     string    `json:"current_ip"`
}

// BlacklistEntry 黑名单条目
type BlacklistEntry struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	ExpireAt  time.Time `json:"expire_at"`
}

// IsValid 检查 Token 是否有效
func (t *RegistrationToken) IsValid() bool {
	if time.Now().After(t.ExpiresAt) {
		return false
	}
	if t.UsedCount >= t.MaxUses {
		return false
	}
	return true
}

// NewNodeAuthManager 创建认证管理器
func NewNodeAuthManager(dataDir string, enablePersistence bool) *NodeAuthManager {
	m := &NodeAuthManager{
		registrationTokens: make(map[string]*RegistrationToken),
		registeredNodes:    make(map[string]*RegisteredNode),
		ipToNode:           make(map[string]string),
		blacklist:          make(map[string]*BlacklistEntry),
		failureCount:       make(map[string]int),
		dataDir:            dataDir,
		enablePersistence:  enablePersistence,
	}

	if enablePersistence {
		m.loadFromDisk()
	}

	return m
}

// SetOnNodeRegistered 设置节点注册/重连成功时的回调
func (m *NodeAuthManager) SetOnNodeRegistered(callback NodeRegisteredCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNodeRegistered = callback
}

// GenerateRegistrationToken 生成注册 Token
// 自动撤销同一节点的旧 Token，确保每个节点只有一个有效 Token
func (m *NodeAuthManager) GenerateRegistrationToken(nodeID string, lifetimeSeconds int, maxUses int, description string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 先撤销同一节点的所有旧 Token
	var revokedCount int
	for tokenStr, token := range m.registrationTokens {
		if token.NodeID == nodeID {
			delete(m.registrationTokens, tokenStr)
			if m.enablePersistence {
				m.deleteTokenFromDisk(tokenStr)
			}
			revokedCount++
		}
	}
	if revokedCount > 0 {
		log.Printf("[NodeAuth] 已撤销节点 %s 的 %d 个旧Token", nodeID, revokedCount)
	}

	// 生成新 Token
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	tokenStr := hex.EncodeToString(tokenBytes)

	token := &RegistrationToken{
		Token:       tokenStr,
		NodeID:      nodeID,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(time.Duration(lifetimeSeconds) * time.Second),
		MaxUses:     maxUses,
		UsedCount:   0,
		Description: description,
	}

	m.registrationTokens[tokenStr] = token

	if m.enablePersistence {
		m.saveTokenToDisk(token)
	}

	log.Printf("[NodeAuth] 为节点 %s 生成Token: %s...", nodeID, tokenStr[:8])
	return tokenStr
}

// ValidateToken 验证 Token
func (m *NodeAuthManager) ValidateToken(tokenStr string, nodeID string) (bool, string) {
	m.mu.RLock()
	token, exists := m.registrationTokens[tokenStr]
	m.mu.RUnlock()

	if !exists {
		return false, "Token 不存在"
	}

	if !token.IsValid() {
		return false, "Token 已过期或已用完"
	}

	if token.NodeID != nodeID {
		return false, "Token 与节点 ID 不匹配"
	}

	return true, ""
}

// RegisterNode 注册节点
func (m *NodeAuthManager) RegisterNode(nodeID, clientIP, tokenStr, host string, port, agentPort, weight int, tags []string) (bool, string) {
	// 检查黑名单
	if m.IsBlacklisted(clientIP) {
		return false, "IP 在黑名单中"
	}

	// 先验证 Token（不持有锁）
	valid, msg := m.ValidateToken(tokenStr, nodeID)

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否为已注册节点重连
	existingNode, isReconnect := m.registeredNodes[nodeID]

	if !valid {
		// 已注册节点重连时，即使 Token 用完也允许（只要 Token 有效期内且匹配）
		if isReconnect {
			token, exists := m.registrationTokens[tokenStr]
			if exists && token.NodeID == nodeID && time.Now().Before(token.ExpiresAt) {
				// Token 匹配且未过期，允许重连，不消耗次数
				existingNode.LastHeartbeat = time.Now()
				existingNode.CurrentIP = clientIP
				existingNode.Host = host
				existingNode.Port = port
				existingNode.AgentPort = agentPort
				m.ipToNode[clientIP] = nodeID
				// 调用回调，通知 Registry 立即注册节点
				if m.onNodeRegistered != nil {
					go m.onNodeRegistered(existingNode)
				}
				log.Printf("[NodeAuth] 节点 %s 重连成功 (IP: %s, 不消耗Token)", nodeID, clientIP)
				return true, "重连成功"
			}
		}
		go m.recordFailure(clientIP) // 异步记录，避免死锁
		return false, msg
	}

	// 新节点注册或已注册节点使用有效 Token 重连
	token := m.registrationTokens[tokenStr]

	// 只有新节点才消耗 Token 使用次数
	if !isReconnect {
		token.UsedCount++
	}

	// 注册/更新节点
	node := &RegisteredNode{
		NodeID:        nodeID,
		Host:          host,
		Port:          port,
		AgentPort:     agentPort,
		Weight:        weight,
		Tags:          tags,
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
		CurrentIP:     clientIP,
	}
	if isReconnect {
		// 保留原始注册时间
		node.RegisteredAt = existingNode.RegisteredAt
	}

	m.registeredNodes[nodeID] = node
	m.ipToNode[clientIP] = nodeID

	if m.enablePersistence {
		m.saveNodeToDisk(node)
	}

	// 调用回调，通知 Registry 立即注册节点
	if m.onNodeRegistered != nil {
		go m.onNodeRegistered(node)
	}

	if isReconnect {
		log.Printf("[NodeAuth] 节点 %s 重连成功 (IP: %s)", nodeID, clientIP)
		return true, "重连成功"
	}
	log.Printf("[NodeAuth] 节点 %s 注册成功 (IP: %s)", nodeID, clientIP)
	return true, "注册成功"
}

// UpdateNodeHeartbeat 更新节点心跳
func (m *NodeAuthManager) UpdateNodeHeartbeat(nodeID, clientIP string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.registeredNodes[nodeID]; ok {
		node.LastHeartbeat = time.Now()
		// 更新 IP 映射
		if node.CurrentIP != clientIP {
			delete(m.ipToNode, node.CurrentIP)
			node.CurrentIP = clientIP
			m.ipToNode[clientIP] = nodeID
		}
	}
}

// IsNodeHeartbeatTimeout 检查节点心跳是否超时
func (m *NodeAuthManager) IsNodeHeartbeatTimeout(nodeID string, timeoutSeconds int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, ok := m.registeredNodes[nodeID]
	if !ok {
		return true
	}

	elapsed := time.Since(node.LastHeartbeat).Seconds()
	return elapsed > float64(timeoutSeconds)
}

// GetRegisteredNode 获取已注册节点
func (m *NodeAuthManager) GetRegisteredNode(nodeID string) (*RegisteredNode, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	node, ok := m.registeredNodes[nodeID]
	return node, ok
}

// GetAllRegisteredNodes 获取所有已注册节点
func (m *NodeAuthManager) GetAllRegisteredNodes() map[string]*RegisteredNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*RegisteredNode, len(m.registeredNodes))
	for k, v := range m.registeredNodes {
		result[k] = v
	}
	return result
}

// UnregisterNode 注销节点
func (m *NodeAuthManager) UnregisterNode(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.registeredNodes[nodeID]; ok {
		delete(m.ipToNode, node.CurrentIP)
		delete(m.registeredNodes, nodeID)

		if m.enablePersistence {
			m.deleteNodeFromDisk(nodeID)
		}

		log.Printf("[NodeAuth] 节点 %s 已注销", nodeID)
	}
}

// IsBlacklisted 检查 IP 是否在黑名单
func (m *NodeAuthManager) IsBlacklisted(ip string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.blacklist[ip]
	if !ok {
		return false
	}

	// 检查是否过期
	if !entry.ExpireAt.IsZero() && time.Now().After(entry.ExpireAt) {
		return false
	}

	return true
}

// AddToBlacklist 添加到黑名单
func (m *NodeAuthManager) AddToBlacklist(ip, reason string, expireHours int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expireAt time.Time
	if expireHours > 0 {
		expireAt = time.Now().Add(time.Duration(expireHours) * time.Hour)
	}

	m.blacklist[ip] = &BlacklistEntry{
		IP:        ip,
		Reason:    reason,
		CreatedAt: time.Now(),
		ExpireAt:  expireAt,
	}

	if m.enablePersistence {
		m.saveBlacklistToDisk()
	}

	log.Printf("[NodeAuth] IP %s 已加入黑名单: %s", ip, reason)
}

// RemoveFromBlacklist 从黑名单移除
func (m *NodeAuthManager) RemoveFromBlacklist(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.blacklist, ip)

	if m.enablePersistence {
		m.saveBlacklistToDisk()
	}

	log.Printf("[NodeAuth] IP %s 已从黑名单移除", ip)
}

// GetBlacklist 获取黑名单
func (m *NodeAuthManager) GetBlacklist() []*BlacklistEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]*BlacklistEntry, 0, len(m.blacklist))
	for _, e := range m.blacklist {
		entries = append(entries, e)
	}
	return entries
}

// recordFailure 记录失败次数
func (m *NodeAuthManager) recordFailure(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failureCount[ip]++

	// 超过阈值自动加入黑名单
	if m.failureCount[ip] >= 5 {
		m.blacklist[ip] = &BlacklistEntry{
			IP:        ip,
			Reason:    "注册失败次数过多",
			CreatedAt: time.Now(),
			ExpireAt:  time.Now().Add(24 * time.Hour),
		}
		log.Printf("[NodeAuth] IP %s 因失败次数过多被自动加入黑名单", ip)
	}
}

// GetTokens 获取所有 Token
func (m *NodeAuthManager) GetTokens() []*RegistrationToken {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tokens := make([]*RegistrationToken, 0, len(m.registrationTokens))
	for _, t := range m.registrationTokens {
		tokens = append(tokens, t)
	}
	return tokens
}

// RevokeToken 撤销 Token
func (m *NodeAuthManager) RevokeToken(tokenStr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.registrationTokens[tokenStr]; !ok {
		return false
	}

	delete(m.registrationTokens, tokenStr)

	if m.enablePersistence {
		m.deleteTokenFromDisk(tokenStr)
	}

	return true
}

// RevokeTokenByNodeID 按节点ID撤销所有Token
func (m *NodeAuthManager) RevokeTokenByNodeID(nodeID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	revokedCount := 0
	for tokenStr, token := range m.registrationTokens {
		if token.NodeID == nodeID {
			delete(m.registrationTokens, tokenStr)
			if m.enablePersistence {
				m.deleteTokenFromDisk(tokenStr)
			}
			revokedCount++
		}
	}

	return revokedCount
}

// CleanupExpiredTokens 清理过期 Token
func (m *NodeAuthManager) CleanupExpiredTokens() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleaned := 0
	for token, t := range m.registrationTokens {
		if !t.IsValid() {
			delete(m.registrationTokens, token)
			if m.enablePersistence {
				m.deleteTokenFromDisk(token)
			}
			cleaned++
		}
	}

	return cleaned
}

// GetStats 获取统计信息
func (m *NodeAuthManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeTokens := 0
	for _, t := range m.registrationTokens {
		if t.IsValid() {
			activeTokens++
		}
	}

	return map[string]interface{}{
		"registered_nodes":  len(m.registeredNodes),
		"active_tokens":     activeTokens,
		"blacklisted_ips":   len(m.blacklist),
		"total_failure_ips": len(m.failureCount),
	}
}

// 持久化方法
func (m *NodeAuthManager) loadFromDisk() {
	// 加载 Token
	tokensFile := filepath.Join(m.dataDir, "tokens.json")
	if data, err := os.ReadFile(tokensFile); err == nil {
		var tokens map[string]*RegistrationToken
		if json.Unmarshal(data, &tokens) == nil {
			m.registrationTokens = tokens
		}
	}

	// 加载节点
	nodesFile := filepath.Join(m.dataDir, "registered_nodes.json")
	if data, err := os.ReadFile(nodesFile); err == nil {
		var nodes map[string]*RegisteredNode
		if json.Unmarshal(data, &nodes) == nil {
			m.registeredNodes = nodes
			for _, node := range nodes {
				m.ipToNode[node.CurrentIP] = node.NodeID
			}
		}
	}

	// 加载黑名单
	blacklistFile := filepath.Join(m.dataDir, "blacklist.json")
	if data, err := os.ReadFile(blacklistFile); err == nil {
		var blacklist map[string]*BlacklistEntry
		if json.Unmarshal(data, &blacklist) == nil {
			m.blacklist = blacklist
		}
	}
}

func (m *NodeAuthManager) saveTokenToDisk(token *RegistrationToken) {
	m.saveTokensToDisk()
}

func (m *NodeAuthManager) deleteTokenFromDisk(tokenStr string) {
	m.saveTokensToDisk()
}

func (m *NodeAuthManager) saveTokensToDisk() {
	os.MkdirAll(m.dataDir, 0755)
	data, _ := json.MarshalIndent(m.registrationTokens, "", "  ")
	os.WriteFile(filepath.Join(m.dataDir, "tokens.json"), data, 0644)
}

func (m *NodeAuthManager) saveNodeToDisk(node *RegisteredNode) {
	m.saveNodesToDisk()
}

func (m *NodeAuthManager) deleteNodeFromDisk(nodeID string) {
	m.saveNodesToDisk()
}

func (m *NodeAuthManager) saveNodesToDisk() {
	os.MkdirAll(m.dataDir, 0755)
	data, _ := json.MarshalIndent(m.registeredNodes, "", "  ")
	os.WriteFile(filepath.Join(m.dataDir, "registered_nodes.json"), data, 0644)
}

func (m *NodeAuthManager) saveBlacklistToDisk() {
	os.MkdirAll(m.dataDir, 0755)
	data, _ := json.MarshalIndent(m.blacklist, "", "  ")
	os.WriteFile(filepath.Join(m.dataDir, "blacklist.json"), data, 0644)
}

// 全局实例
var (
	globalAuthManager *NodeAuthManager
	authManagerOnce   sync.Once
)

// GetNodeAuthManager 获取全局认证管理器
func GetNodeAuthManager() *NodeAuthManager {
	authManagerOnce.Do(func() {
		globalAuthManager = NewNodeAuthManager("./data", true)
	})
	return globalAuthManager
}

// InitNodeAuthManager 初始化全局认证管理器
func InitNodeAuthManager(dataDir string, enablePersistence bool) {
	globalAuthManager = NewNodeAuthManager(dataDir, enablePersistence)
}
