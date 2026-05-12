<template>
  <div class="gateway-panel">
    <!-- 头部 -->
    <div class="panel-header">
      <h3>节点状态</h3>
      <div class="status-summary">
        <span class="online-count">
          <span class="status-dot online"></span>
          {{ onlineCount }} 在线
        </span>
        <span class="divider">/</span>
        <span class="total-count">{{ nodes.length }} 总计</span>
      </div>
    </div>

    <!-- 节点列表 -->
    <div class="nodes-list">
      <div v-if="loading" class="loading-state">
        <div class="loading-spinner"></div>
      </div>
      
      <div v-else-if="nodes.length === 0" class="empty-state">
        <div class="empty-icon">🖥️</div>
        <div class="empty-text">暂无节点</div>
      </div>
      
      <div 
        v-for="node in nodes" 
        :key="node.id"
        class="node-card"
        :class="{ offline: node.status !== 'online', expanded: expandedId === node.id }"
        @click="toggleExpand(node.id)"
      >
        <div class="node-main">
          <!-- 状态指示器 -->
          <div class="node-status">
            <span class="status-dot" :class="node.status"></span>
          </div>
          
          <!-- 节点信息 -->
          <div class="node-info">
            <div class="node-name">{{ node.name || node.id }}</div>
            <div class="node-meta">
              <span class="node-type">{{ getNodeTypeLabel(node.type) }}</span>
              <span class="node-ip">{{ node.ip }}</span>
            </div>
          </div>
          
          <!-- 快速指标 -->
          <div class="node-metrics-quick">
            <div class="metric-item" :class="getCpuClass(getNodeCpu(node))">
              <span class="metric-value">{{ getNodeCpu(node) }}%</span>
              <span class="metric-label">CPU</span>
            </div>
            <div class="metric-item" :class="getMemClass(getNodeMem(node))">
              <span class="metric-value">{{ getNodeMem(node) }}%</span>
              <span class="metric-label">内存</span>
            </div>
            <div class="metric-item">
              <span class="metric-value">{{ node.camera_count || node.cameras || 0 }}</span>
              <span class="metric-label">摄像头</span>
            </div>
          </div>
          
          <!-- 展开指示 -->
          <div class="expand-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline :points="expandedId === node.id ? '18,15 12,9 6,15' : '6,9 12,15 18,9'"/>
            </svg>
          </div>
        </div>
        
        <!-- 展开详情 -->
        <div v-if="expandedId === node.id" class="node-details">
          <!-- 资源使用 -->
          <div class="detail-section">
            <h5>资源使用</h5>
            <div class="metrics-grid">
              <div class="metric-bar">
                <div class="bar-label">
                  <span>CPU</span>
                  <span>{{ getNodeCpu(node) }}%</span>
                </div>
                <div class="bar-track">
                  <div class="bar-fill" :class="getCpuClass(getNodeCpu(node))" :style="{ width: `${getNodeCpu(node)}%` }"></div>
                </div>
              </div>
              <div class="metric-bar">
                <div class="bar-label">
                  <span>内存</span>
                  <span>{{ getNodeMem(node) }}%</span>
                </div>
                <div class="bar-track">
                  <div class="bar-fill" :class="getMemClass(getNodeMem(node))" :style="{ width: `${getNodeMem(node)}%` }"></div>
                </div>
              </div>
              <div class="metric-bar">
                <div class="bar-label">
                  <span>磁盘</span>
                  <span>{{ getNodeDisk(node) }}%</span>
                </div>
                <div class="bar-track">
                  <div class="bar-fill" :style="{ width: `${getNodeDisk(node)}%` }"></div>
                </div>
              </div>
            </div>
          </div>
          
          <!-- 运行信息 -->
          <div class="detail-section">
            <h5>运行信息</h5>
            <div class="info-grid">
              <div class="info-item">
                <span class="info-label">运行时间</span>
                <span class="info-value">{{ formatUptime(node.uptime) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">版本</span>
                <span class="info-value">{{ node.version || '-' }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">最后心跳</span>
                <span class="info-value">{{ formatTime(node.lastHeartbeat) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">缓存事件</span>
                <span class="info-value">{{ node.cachedEvents || 0 }}</span>
              </div>
            </div>
          </div>
          
          <!-- 摄像头列表 -->
          <div v-if="node.cameraList && node.cameraList.length > 0" class="detail-section">
            <h5>分配的摄像头</h5>
            <div class="camera-list">
              <div 
                v-for="cam in node.cameraList" 
                :key="cam.id"
                class="camera-item"
              >
                <span class="camera-status" :class="cam.status"></span>
                <span class="camera-name">{{ cam.name }}</span>
              </div>
            </div>
          </div>
          
          <!-- 操作按钮 -->
          <div class="detail-actions">
            <button class="action-btn" @click.stop="viewDetails(node)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="3"/>
                <path d="M12 1v2m0 18v2M4.22 4.22l1.42 1.42m12.72 12.72 1.42 1.42M1 12h2m18 0h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/>
              </svg>
              详情
            </button>
            <button class="action-btn" @click.stop="restartNode(node)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/>
                <path d="M21 3v5h-5"/>
              </svg>
              重启
            </button>
            <button class="action-btn danger" @click.stop="removeNode(node)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              </svg>
              移除
            </button>
          </div>
        </div>
      </div>
    </div>
    
    <!-- 刷新按钮 -->
    <div class="panel-footer">
      <button class="refresh-btn" @click="fetchNodes" :disabled="loading">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" :class="{ spinning: loading }">
          <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/>
          <path d="M21 3v5h-5"/>
        </svg>
        {{ loading ? '刷新中...' : '刷新' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import axios from 'axios'

// ============ 类型定义 ============
export interface NodeCamera {
  id: string
  name: string
  status: 'online' | 'offline'
}

export interface Node {
  id: string
  name?: string
  type: 'hub' | 'worker'
  status: 'online' | 'offline'
  ip?: string
  port?: number
  cpu?: number
  cpu_percent?: number
  memory?: number
  memory_percent?: number
  disk?: number
  disk_percent?: number
  camera_count?: number
  uptime?: number
  version?: string
  lastHeartbeat?: string
  cameras?: number
  cachedEvents?: number
  cameraList?: NodeCamera[]
}

// ============ Emits ============
const emit = defineEmits<{
  (e: 'view-details', node: Node): void
  (e: 'restart', node: Node): void
  (e: 'remove', node: Node): void
}>()

// ============ 状态 ============
const nodes = ref<Node[]>([])
const loading = ref(false)
const expandedId = ref<string | null>(null)
let refreshInterval: number | null = null

// ============ 计算属性 ============
const onlineCount = computed(() => {
  return nodes.value.filter(n => n.status === 'online').length
})

// ============ 方法 ============
async function fetchNodes() {
  loading.value = true
  
  try {
    const res = await axios.get('/api/v1/nodes')
    nodes.value = res.data.nodes || []
  } catch (e) {
    console.error('获取节点失败', e)
  } finally {
    loading.value = false
  }
}

function toggleExpand(id: string) {
  expandedId.value = expandedId.value === id ? null : id
}

function getNodeTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    hub: 'Hub',
    worker: 'Worker',
  }
  return labels[type] || type
}

// 兼容后端字段名 (cpu_percent / memory_percent)
function getNodeCpu(node: any): number {
  return Math.round(node.cpu_percent || node.cpu || 0)
}

function getNodeMem(node: any): number {
  return Math.round(node.memory_percent || node.memory || 0)
}

function getNodeDisk(node: any): number {
  return Math.round(node.disk_percent || node.disk || 0)
}

function getCpuClass(cpu: number = 0): string {
  if (cpu > 80) return 'critical'
  if (cpu > 60) return 'warning'
  return 'normal'
}

function getMemClass(mem: number = 0): string {
  if (mem > 85) return 'critical'
  if (mem > 70) return 'warning'
  return 'normal'
}

function formatUptime(seconds: number = 0): string {
  if (!seconds) return '-'
  
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  
  if (days > 0) return `${days}天 ${hours}小时`
  if (hours > 0) return `${hours}小时 ${minutes}分钟`
  return `${minutes}分钟`
}

function formatTime(dateStr?: string): string {
  if (!dateStr) return '-'
  
  const date = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  
  return date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

function viewDetails(node: Node) {
  emit('view-details', node)
}

function restartNode(node: Node) {
  if (confirm(`确定要重启节点 "${node.name || node.id}" 吗？`)) {
    emit('restart', node)
  }
}

function removeNode(node: Node) {
  if (confirm(`确定要移除节点 "${node.name || node.id}" 吗？`)) {
    emit('remove', node)
  }
}

// ============ 生命周期 ============
onMounted(() => {
  fetchNodes()
  refreshInterval = window.setInterval(fetchNodes, 30000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})
</script>

<style scoped>
.gateway-panel {
  display: flex;
  flex-direction: column;
  background: #fff;
  border-radius: 12px;
  overflow: hidden;
  height: 100%;
}

/* 头部 */
.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
}

.panel-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #1a1a2e;
}

.status-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #6b7280;
}

.online-count {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #10b981;
  font-weight: 500;
}

.divider {
  color: #d1d5db;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #9ca3af;
}

.status-dot.online {
  background: #10b981;
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
}

.status-dot.offline {
  background: #ef4444;
}

/* 节点列表 */
.nodes-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.loading-state {
  display: flex;
  justify-content: center;
  padding: 40px;
}

.loading-spinner {
  width: 24px;
  height: 24px;
  border: 2px solid #e5e7eb;
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px;
  color: #6b7280;
}

.empty-icon {
  font-size: 36px;
  margin-bottom: 8px;
}

.empty-text {
  font-size: 14px;
}

/* 节点卡片 */
.node-card {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  margin-bottom: 10px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.node-card:hover {
  border-color: #d1d5db;
}

.node-card.expanded {
  border-color: #3b82f6;
  background: #f0f9ff;
}

.node-card.offline {
  opacity: 0.7;
}

.node-main {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
}

.node-status {
  flex-shrink: 0;
}

.node-info {
  flex: 1;
  min-width: 0;
}

.node-name {
  font-size: 14px;
  font-weight: 600;
  color: #1a1a2e;
  margin-bottom: 2px;
}

.node-meta {
  display: flex;
  gap: 8px;
  font-size: 12px;
  color: #6b7280;
}

.node-type {
  padding: 1px 6px;
  background: #e5e7eb;
  border-radius: 4px;
}

.node-metrics-quick {
  display: flex;
  gap: 12px;
}

.metric-item {
  text-align: center;
  min-width: 44px;
}

.metric-value {
  display: block;
  font-size: 14px;
  font-weight: 600;
  color: #1a1a2e;
}

.metric-item.warning .metric-value {
  color: #f59e0b;
}

.metric-item.critical .metric-value {
  color: #ef4444;
}

.metric-label {
  font-size: 10px;
  color: #9ca3af;
  text-transform: uppercase;
}

.expand-icon {
  color: #9ca3af;
  flex-shrink: 0;
}

.expand-icon svg {
  width: 16px;
  height: 16px;
}

/* 展开详情 */
.node-details {
  padding: 0 14px 14px;
  border-top: 1px solid #e5e7eb;
  margin-top: 0;
}

.detail-section {
  margin-top: 14px;
}

.detail-section h5 {
  margin: 0 0 10px;
  font-size: 11px;
  font-weight: 600;
  color: #6b7280;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.metrics-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.metric-bar {
}

.bar-label {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: #374151;
  margin-bottom: 4px;
}

.bar-track {
  height: 6px;
  background: #e5e7eb;
  border-radius: 3px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  background: #3b82f6;
  border-radius: 3px;
  transition: width 0.3s ease;
}

.bar-fill.warning {
  background: #f59e0b;
}

.bar-fill.critical {
  background: #ef4444;
}

.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.info-label {
  font-size: 11px;
  color: #6b7280;
}

.info-value {
  font-size: 13px;
  font-weight: 500;
  color: #1a1a2e;
}

.camera-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.camera-item {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  font-size: 12px;
}

.camera-status {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #9ca3af;
}

.camera-status.online {
  background: #10b981;
}

.detail-actions {
  display: flex;
  gap: 8px;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid #e5e7eb;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 10px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fff;
  font-size: 12px;
  color: #374151;
  cursor: pointer;
  transition: all 0.2s ease;
}

.action-btn:hover {
  border-color: #3b82f6;
  color: #3b82f6;
}

.action-btn.danger:hover {
  border-color: #ef4444;
  color: #ef4444;
}

.action-btn svg {
  width: 14px;
  height: 14px;
}

/* 底部 */
.panel-footer {
  padding: 12px;
  border-top: 1px solid #e5e7eb;
}

.refresh-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 100%;
  padding: 10px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  font-size: 13px;
  color: #374151;
  cursor: pointer;
  transition: all 0.2s ease;
}

.refresh-btn:hover:not(:disabled) {
  background: #f3f4f6;
}

.refresh-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.refresh-btn svg {
  width: 16px;
  height: 16px;
}

.refresh-btn svg.spinning {
  animation: spin 1s linear infinite;
}
</style>
