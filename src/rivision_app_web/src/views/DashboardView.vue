<template>
  <div class="dashboard">
    <!-- 顶部统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon cameras">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/>
            <circle cx="12" cy="13" r="4"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ stats.cameras?.online || 0 }}</span>
          <span class="stat-label">在线摄像头</span>
        </div>
        <span class="stat-trend up" v-if="stats.cameras?.total">
          / {{ stats.cameras.total }}
        </span>
      </div>

      <div class="stat-card">
        <div class="stat-icon nodes">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
            <line x1="8" y1="21" x2="16" y2="21"/>
            <line x1="12" y1="17" x2="12" y2="21"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ stats.nodes?.active || 0 }}</span>
          <span class="stat-label">活跃节点</span>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon alerts">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
            <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ stats.alerts?.today || 0 }}</span>
          <span class="stat-label">今日告警</span>
        </div>
        <span class="stat-trend" :class="stats.alerts?.trend > 0 ? 'up' : 'down'">
          {{ stats.alerts?.trend > 0 ? '+' : '' }}{{ stats.alerts?.trend || 0 }}%
        </span>
      </div>

      <div class="stat-card">
        <div class="stat-icon events">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ formatNumber(stats.events?.total || 0) }}</span>
          <span class="stat-label">事件总数</span>
        </div>
      </div>
    </div>

    <!-- 主内容区 -->
    <div class="main-content">
      <!-- 左侧：摄像头网格 -->
      <div class="camera-section">
        <div class="section-header">
          <h2>实时监控</h2>
          <div class="view-toggle">
            <button :class="{ active: gridSize === 4 }" @click="gridSize = 4">2×2</button>
            <button :class="{ active: gridSize === 9 }" @click="gridSize = 9">3×3</button>
            <button :class="{ active: gridSize === 16 }" @click="gridSize = 16">4×4</button>
          </div>
        </div>
        
        <div class="camera-grid" :class="`grid-${gridSize}`">
          <div 
            v-for="cam in displayCameras" 
            :key="cam.id"
            class="camera-cell"
            :class="{ selected: selectedCamera === cam.id }"
            @click="selectCamera(cam)"
          >
            <div class="video-wrapper">
              <img 
                v-if="cam.snapshot" 
                :src="cam.snapshot" 
                :alt="cam.name"
              />
              <div v-else class="placeholder">
                <svg viewBox="0 0 24 24" class="placeholder-icon">
                  <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" fill="none" stroke="currentColor" stroke-width="1.5"/>
                </svg>
              </div>
              <div class="camera-overlay">
                <span class="camera-name">{{ cam.name }}</span>
                <span class="camera-status" :class="cam.status">
                  {{ cam.status === 'online' ? '在线' : '离线' }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧：信息面板 -->
      <div class="info-panel">
        <!-- 实时告警 -->
        <div class="panel-section alerts-panel">
          <h3>实时告警</h3>
          <div class="alerts-list">
            <div 
              v-for="alert in recentAlerts" 
              :key="alert.id"
              class="alert-item"
              :class="alert.level"
            >
              <div class="alert-icon">
                <svg viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 2L1 21h22L12 2zm0 3.5L19.5 19h-15L12 5.5z"/>
                  <path d="M11 10h2v4h-2zm0 6h2v2h-2z"/>
                </svg>
              </div>
              <div class="alert-content">
                <p class="alert-message">{{ alert.message }}</p>
                <span class="alert-meta">
                  {{ alert.camera }} · {{ formatTime(alert.time) }}
                </span>
              </div>
            </div>
            <div v-if="recentAlerts.length === 0" class="no-alerts">
              暂无告警
            </div>
          </div>
        </div>

        <!-- 事件统计 -->
        <div class="panel-section events-panel">
          <h3>事件统计</h3>
          <div class="event-chart">
            <div 
              v-for="(count, type) in eventStats" 
              :key="type"
              class="event-bar"
            >
              <span class="event-label">{{ eventLabels[type] || type }}</span>
              <div class="bar-container">
                <div 
                  class="bar-fill" 
                  :style="{ width: getBarWidth(count) }"
                ></div>
              </div>
              <span class="event-count">{{ count }}</span>
            </div>
          </div>
        </div>

        <!-- 系统状态 -->
        <div class="panel-section system-panel">
          <h3>系统状态</h3>
          <div class="system-stats">
            <div class="sys-item">
              <span class="sys-label">CPU</span>
              <div class="progress-bar">
                <div class="progress" :style="{ width: systemStats.cpu + '%' }"></div>
              </div>
              <span class="sys-value">{{ systemStats.cpu }}%</span>
            </div>
            <div class="sys-item">
              <span class="sys-label">内存</span>
              <div class="progress-bar">
                <div class="progress" :style="{ width: systemStats.memory + '%' }"></div>
              </div>
              <span class="sys-value">{{ systemStats.memory }}%</span>
            </div>
            <div class="sys-item">
              <span class="sys-label">磁盘</span>
              <div class="progress-bar">
                <div class="progress" :style="{ width: systemStats.disk + '%' }"></div>
              </div>
              <span class="sys-value">{{ systemStats.disk }}%</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'

// 状态
const stats = ref({
  cameras: { online: 0, total: 0 },
  nodes: { active: 0 },
  alerts: { today: 0, trend: 0 },
  events: { total: 0 }
})

const cameras = ref([])
const selectedCamera = ref(null)
const gridSize = ref(9)
const recentAlerts = ref([])
const eventStats = ref({})
const systemStats = ref({ cpu: 0, memory: 0, disk: 0 })

let ws = null
let refreshTimer = null

const eventLabels = {
  'person': '人员检测',
  'vehicle': '车辆检测',
  'intrusion': '越界入侵',
  'loiter': '人员徘徊',
  'fall': '跌倒检测',
  'fight': '打架检测'
}

// 计算属性
const displayCameras = computed(() => {
  return cameras.value.slice(0, gridSize.value)
})

// 方法
const formatNumber = (num) => {
  if (num >= 10000) return (num / 10000).toFixed(1) + 'w'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'k'
  return num.toString()
}

const formatTime = (time) => {
  const date = new Date(time)
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

const getBarWidth = (count) => {
  const max = Math.max(...Object.values(eventStats.value), 1)
  return (count / max * 100) + '%'
}

const selectCamera = (cam) => {
  selectedCamera.value = cam.id === selectedCamera.value ? null : cam.id
}

// 数据加载
const loadStats = async () => {
  try {
    const res = await fetch('/api/v1/stats/overview')
    const data = await res.json()
    stats.value = data
  } catch (err) {
    console.error('Failed to load stats:', err)
  }
}

const loadCameras = async () => {
  try {
    const res = await fetch('/api/v1/cameras')
    const data = await res.json()
    cameras.value = data.cameras || []
    
    // 更新摄像头统计
    stats.value.cameras = {
      online: cameras.value.filter(c => c.status === 'online').length,
      total: cameras.value.length
    }
  } catch (err) {
    console.error('Failed to load cameras:', err)
  }
}

const loadAlerts = async () => {
  try {
    const res = await fetch('/api/v1/alerts?limit=10')
    const data = await res.json()
    recentAlerts.value = data.alerts || []
  } catch (err) {
    console.error('Failed to load alerts:', err)
  }
}

const loadEventStats = async () => {
  try {
    const res = await fetch('/api/v1/stats/events?period=24h')
    const data = await res.json()
    eventStats.value = data.by_type || {}
  } catch (err) {
    console.error('Failed to load event stats:', err)
  }
}

const loadSystemStats = async () => {
  try {
    const res = await fetch('/api/v1/system/status')
    const data = await res.json()
    systemStats.value = {
      cpu: Math.round(data.cpu_percent || 0),
      memory: Math.round(data.memory_percent || 0),
      disk: Math.round(data.disk_percent || 0)
    }
  } catch (err) {
    console.error('Failed to load system stats:', err)
  }
}

// WebSocket实时更新
const connectWebSocket = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  ws = new WebSocket(`${protocol}//${window.location.host}/ws/dashboard`)
  
  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data)
      handleWSMessage(msg)
    } catch (err) {
      console.error('WS message parse error:', err)
    }
  }
  
  ws.onclose = () => {
    setTimeout(connectWebSocket, 3000)
  }
}

const handleWSMessage = (msg) => {
  switch (msg.type) {
    case 'alert':
      recentAlerts.value.unshift(msg.data)
      if (recentAlerts.value.length > 10) {
        recentAlerts.value.pop()
      }
      stats.value.alerts.today++
      break
    case 'camera_status':
      const idx = cameras.value.findIndex(c => c.id === msg.data.id)
      if (idx >= 0) {
        cameras.value[idx].status = msg.data.status
      }
      break
    case 'system_stats':
      systemStats.value = msg.data
      break
  }
}

// 生命周期
onMounted(() => {
  loadStats()
  loadCameras()
  loadAlerts()
  loadEventStats()
  loadSystemStats()
  
  connectWebSocket()
  refreshTimer = setInterval(() => {
    loadSystemStats()
    loadEventStats()
  }, 30000)
})

onUnmounted(() => {
  if (ws) ws.close()
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.dashboard {
  padding: 20px;
  background: #f5f7fa;
  min-height: 100vh;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 20px;
  margin-bottom: 24px;
}

.stat-card {
  background: white;
  border-radius: 12px;
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.08);
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.stat-icon svg {
  width: 24px;
  height: 24px;
}

.stat-icon.cameras { background: #e0f2fe; color: #0284c7; }
.stat-icon.nodes { background: #d1fae5; color: #059669; }
.stat-icon.alerts { background: #fee2e2; color: #dc2626; }
.stat-icon.events { background: #fef3c7; color: #d97706; }

.stat-content {
  flex: 1;
}

.stat-value {
  display: block;
  font-size: 1.75rem;
  font-weight: 600;
  color: #1f2937;
}

.stat-label {
  font-size: 0.85rem;
  color: #6b7280;
}

.stat-trend {
  font-size: 0.85rem;
  font-weight: 500;
}

.stat-trend.up { color: #059669; }
.stat-trend.down { color: #dc2626; }

.main-content {
  display: grid;
  grid-template-columns: 1fr 360px;
  gap: 24px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.section-header h2 {
  font-size: 1.1rem;
  font-weight: 600;
  color: #1f2937;
}

.view-toggle {
  display: flex;
  gap: 4px;
}

.view-toggle button {
  padding: 6px 12px;
  border: 1px solid #e5e7eb;
  background: white;
  border-radius: 6px;
  font-size: 0.8rem;
  cursor: pointer;
}

.view-toggle button.active {
  background: #3b82f6;
  border-color: #3b82f6;
  color: white;
}

.camera-grid {
  display: grid;
  gap: 12px;
}

.camera-grid.grid-4 { grid-template-columns: repeat(2, 1fr); }
.camera-grid.grid-9 { grid-template-columns: repeat(3, 1fr); }
.camera-grid.grid-16 { grid-template-columns: repeat(4, 1fr); }

.camera-cell {
  background: #1f2937;
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.2s;
}

.camera-cell:hover {
  transform: scale(1.02);
}

.camera-cell.selected {
  outline: 3px solid #3b82f6;
  outline-offset: -3px;
}

.video-wrapper {
  position: relative;
  aspect-ratio: 16/9;
}

.video-wrapper img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #374151;
}

.placeholder-icon {
  width: 40px;
  height: 40px;
  opacity: 0.3;
  color: white;
}

.camera-overlay {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 8px 10px;
  background: linear-gradient(transparent, rgba(0,0,0,0.7));
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.camera-name {
  color: white;
  font-size: 0.8rem;
  font-weight: 500;
}

.camera-status {
  font-size: 0.7rem;
  padding: 2px 6px;
  border-radius: 4px;
}

.camera-status.online { background: #059669; color: white; }
.camera-status.offline { background: #6b7280; color: white; }

.info-panel {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.panel-section {
  background: white;
  border-radius: 12px;
  padding: 16px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.08);
}

.panel-section h3 {
  font-size: 0.95rem;
  font-weight: 600;
  color: #1f2937;
  margin-bottom: 12px;
}

.alerts-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 240px;
  overflow-y: auto;
}

.alert-item {
  display: flex;
  gap: 10px;
  padding: 10px;
  border-radius: 8px;
  background: #fef2f2;
}

.alert-item.warning { background: #fef3c7; }
.alert-item.info { background: #e0f2fe; }

.alert-icon {
  width: 20px;
  height: 20px;
  color: #dc2626;
}

.alert-item.warning .alert-icon { color: #d97706; }
.alert-item.info .alert-icon { color: #0284c7; }

.alert-content {
  flex: 1;
}

.alert-message {
  font-size: 0.85rem;
  color: #1f2937;
  margin-bottom: 4px;
}

.alert-meta {
  font-size: 0.75rem;
  color: #6b7280;
}

.no-alerts {
  text-align: center;
  padding: 20px;
  color: #9ca3af;
  font-size: 0.9rem;
}

.event-chart {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.event-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.event-label {
  width: 70px;
  font-size: 0.8rem;
  color: #6b7280;
}

.bar-container {
  flex: 1;
  height: 8px;
  background: #e5e7eb;
  border-radius: 4px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #3b82f6, #60a5fa);
  border-radius: 4px;
  transition: width 0.3s;
}

.event-count {
  width: 40px;
  text-align: right;
  font-size: 0.8rem;
  font-weight: 500;
  color: #1f2937;
}

.system-stats {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.sys-item {
  display: flex;
  align-items: center;
  gap: 10px;
}

.sys-label {
  width: 40px;
  font-size: 0.8rem;
  color: #6b7280;
}

.progress-bar {
  flex: 1;
  height: 6px;
  background: #e5e7eb;
  border-radius: 3px;
  overflow: hidden;
}

.progress {
  height: 100%;
  background: #3b82f6;
  border-radius: 3px;
  transition: width 0.3s;
}

.sys-value {
  width: 45px;
  text-align: right;
  font-size: 0.8rem;
  font-weight: 500;
  color: #1f2937;
}

@media (max-width: 1200px) {
  .main-content {
    grid-template-columns: 1fr;
  }
  
  .info-panel {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 768px) {
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
  
  .info-panel {
    grid-template-columns: 1fr;
  }
}
</style>
