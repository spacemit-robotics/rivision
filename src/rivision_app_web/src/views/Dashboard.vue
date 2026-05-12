<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { useNodesStore } from '../stores/nodes'
import axios from 'axios'
import VideoGrid from '../components/VideoGrid.vue'
import GatewayPanel from '../components/GatewayPanel.vue'
import CameraManagementModal from '../components/CameraManagementModal.vue'
import type { GridLayout, Camera as GridCamera } from '../components/VideoGrid.vue'
import type { Camera } from '../components/CameraManagementModal.vue'

const nodesStore = useNodesStore()

// 视频网格状态 - 从 localStorage 恢复
const STORAGE_KEY = 'dashboard_selected_cameras'
const LAYOUT_KEY = 'dashboard_grid_layout'

function loadSelectedCameras(): (string | null)[] {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved) return JSON.parse(saved)
  } catch (e) {}
  return [null, null, null, null]
}

function saveSelectedCameras(cameras: (string | null)[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(cameras))
}

const gridLayout = ref<GridLayout>(
  (localStorage.getItem(LAYOUT_KEY) as GridLayout) || '2x2'
)
const cameras = ref<GridCamera[]>([])
const selectedCameras = ref<(string | null)[]>(loadSelectedCameras())
const vlmFocusId = ref<string | null>(null)

// AI 功能启用状态
const yoloEnabledCameras = ref<Set<string>>(new Set())
const vlmEnabledCameras = ref<Set<string>>(new Set())

// 监听布局和摄像头变化，持久化
watch(gridLayout, (val) => localStorage.setItem(LAYOUT_KEY, val))
watch(selectedCameras, (val) => saveSelectedCameras(val), { deep: true })

// 摄像头管理弹窗
const showCameraModal = ref(false)
const editingCamera = ref<any>(null)

// 统计数据
const stats = ref({
  totalNodes: 0,
  onlineNodes: 0,
  totalCameras: 0,
  activeCameras: 0,
  todayEvents: 0,
  todayAlerts: 0,
})

const recentEvents = ref<any[]>([])
let refreshInterval: ReturnType<typeof setInterval>

// 视图模式
const viewMode = ref<'grid' | 'stats'>('grid')

// 统计卡片配置
const statCards = [
  { key: 'totalNodes', label: '总节点数', icon: '🖥️', color: '#3b82f6' },
  { key: 'onlineNodes', label: '在线节点', icon: '✅', color: '#10b981' },
  { key: 'totalCameras', label: '摄像头总数', icon: '📹', color: '#8b5cf6' },
  { key: 'activeCameras', label: '活跃摄像头', icon: '🟢', color: '#06b6d4' },
  { key: 'todayEvents', label: '今日事件', icon: '📊', color: '#f59e0b' },
  { key: 'todayAlerts', label: '今日告警', icon: '🔔', color: '#ef4444' },
]

// 布局选项
const layoutOptions: { value: GridLayout; label: string }[] = [
  { value: '1x1', label: '1x1' },
  { value: '2x2', label: '2x2' },
  { value: '3x3', label: '3x3' },
  { value: '4x4', label: '4x4' },
]

// 节点列表 (用于摄像头分配)
const nodes = computed(() => nodesStore.nodes)

// 获取统计数据
async function fetchStats() {
  try {
    const [nodesRes, statsRes] = await Promise.all([
      axios.get('/api/v1/nodes'),
      axios.get('/api/v1/system/stats').catch(() => ({ data: {} }))
    ])
    
    const nodeList = nodesRes.data.nodes || []
    stats.value.totalNodes = nodeList.length
    stats.value.onlineNodes = nodeList.filter((n: any) => n.status === 'online').length
    
    if (statsRes.data) {
      stats.value.totalCameras = statsRes.data.total_cameras || 0
      stats.value.activeCameras = statsRes.data.active_cameras || 0
      stats.value.todayEvents = statsRes.data.today_events || 0
      stats.value.todayAlerts = statsRes.data.today_alerts || 0
    }
  } catch (e) {
    console.error('获取统计数据失败', e)
  }
}

// 获取摄像头列表
async function fetchCameras() {
  try {
    const res = await axios.get('/api/v1/cameras')
    const cameraList = res.data.cameras || []
    cameras.value = cameraList.map((c: any) => ({
      id: c.id,
      name: c.name || c.id,
      url: c.url,
      status: c.status || 'offline',
    }))
    
    // 初始化 AI 功能启用状态
    const yoloSet = new Set<string>()
    const vlmSet = new Set<string>()
    for (const c of cameraList) {
      if (c.config?.yolo_enabled) yoloSet.add(c.id)
      if (c.config?.vlm_enabled) vlmSet.add(c.id)
    }
    yoloEnabledCameras.value = yoloSet
    vlmEnabledCameras.value = vlmSet
  } catch (e) {
    console.error('获取摄像头失败', e)
  }
}

// 获取最近事件
async function fetchRecentEvents() {
  try {
    const res = await axios.get('/api/v1/events?limit=5')
    recentEvents.value = res.data.events || []
  } catch (e) {
    console.error('获取事件失败', e)
  }
}

// 添加摄像头
function handleAddCamera() {
  editingCamera.value = null
  showCameraModal.value = true
}

// 保存摄像头
async function handleSaveCamera(camera: any) {
  try {
    if (camera.id) {
      await axios.put(`/api/v1/cameras/${camera.id}`, camera)
    } else {
      await axios.post('/api/v1/cameras', camera)
    }
    await fetchCameras()
  } catch (e) {
    console.error('保存摄像头失败', e)
  }
}

// 视频就绪回调
function handleVideoReady(index: number, video: HTMLVideoElement) {
  console.log(`视频 ${index} 就绪`, video)
}

// 视频错误回调
function handleVideoError(index: number, error: string) {
  console.error(`视频 ${index} 错误:`, error)
}

// YOLO/VLM 开关切换
async function handleToggleYolo(cameraId: string, enabled: boolean) {
  try {
    // 更新本地状态
    const newSet = new Set(yoloEnabledCameras.value)
    if (enabled) {
      newSet.add(cameraId)
    } else {
      newSet.delete(cameraId)
    }
    yoloEnabledCameras.value = newSet
    
    // 调用 API 更新摄像头配置
    await axios.patch(`/api/v1/cameras/${cameraId}/config`, {
      yolo_enabled: enabled
    })
    console.log(`[Dashboard] YOLO ${enabled ? '启用' : '禁用'}: ${cameraId}`)
  } catch (e) {
    console.error('切换 YOLO 失败', e)
    // 回滚状态
    const rollbackSet = new Set(yoloEnabledCameras.value)
    if (enabled) {
      rollbackSet.delete(cameraId)
    } else {
      rollbackSet.add(cameraId)
    }
    yoloEnabledCameras.value = rollbackSet
  }
}

async function handleToggleVlm(cameraId: string, enabled: boolean) {
  try {
    const newSet = new Set(vlmEnabledCameras.value)
    if (enabled) {
      newSet.add(cameraId)
    } else {
      newSet.delete(cameraId)
    }
    vlmEnabledCameras.value = newSet
    
    await axios.patch(`/api/v1/cameras/${cameraId}/config`, {
      vlm_enabled: enabled
    })
    console.log(`[Dashboard] VLM ${enabled ? '启用' : '禁用'}: ${cameraId}`)
  } catch (e) {
    console.error('切换 VLM 失败', e)
    const rollbackSet = new Set(vlmEnabledCameras.value)
    if (enabled) {
      rollbackSet.delete(cameraId)
    } else {
      rollbackSet.add(cameraId)
    }
    vlmEnabledCameras.value = rollbackSet
  }
}

// ============ WebSocket 检测事件 ============
const videoGridRef = ref<any>(null)
let eventsWs: WebSocket | null = null

function connectEventsWebSocket() {
  const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${wsProtocol}//${location.host}/ws/events`
  
  eventsWs = new WebSocket(wsUrl)
  
  eventsWs.onopen = () => {
    console.log('[Dashboard] WebSocket 已连接')
  }
  
  eventsWs.onmessage = (event) => {
    // Handle both single JSON and newline-separated multi-JSON frames
    const parts = event.data.split('\n')
    for (const part of parts) {
      if (!part) continue
      try {
        const msg = JSON.parse(part)
        if (msg.type === 'detection' && msg.data) {
          handleDetectionEvent(msg.data)
        }
      } catch (e) {
        // 忽略解析错误
      }
    }
  }
  
  eventsWs.onclose = () => {
    console.log('[Dashboard] WebSocket 断开，5秒后重连')
    setTimeout(connectEventsWebSocket, 5000)
  }
  
  eventsWs.onerror = (e) => {
    console.error('[Dashboard] WebSocket 错误', e)
  }
}

function handleDetectionEvent(data: any) {
  const cameraId = data.camera_id
  if (!cameraId || !videoGridRef.value) return
  
  const displayCameras = videoGridRef.value.getDisplayCameras()
  const index = displayCameras.indexOf(cameraId)
  if (index >= 0) {
    const cell = videoGridRef.value.getCellRef(index)
    if (cell && data.detections) {
      // ★ 传递 Worker 帧尺寸，让 VideoCell 进行坐标转换
      cell.setDetections(data.detections, data.frame_width || 640, data.frame_height || 360)
    }
  }
}

// 生命周期
onMounted(() => {
  fetchStats()
  fetchCameras()
  fetchRecentEvents()
  connectEventsWebSocket()
  refreshInterval = setInterval(() => {
    fetchStats()
    fetchRecentEvents()
  }, 30000)
})

onUnmounted(() => {
  clearInterval(refreshInterval)
  if (eventsWs) {
    eventsWs.close()
    eventsWs = null
  }
})
</script>

<template>
  <div class="dashboard">
    <!-- 顶部工具栏 -->
    <div class="dashboard-toolbar">
      <div class="toolbar-left">
        <div class="view-switcher">
          <button 
            :class="['view-btn', { active: viewMode === 'grid' }]"
            @click="viewMode = 'grid'"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="7" height="7"/>
              <rect x="14" y="3" width="7" height="7"/>
              <rect x="3" y="14" width="7" height="7"/>
              <rect x="14" y="14" width="7" height="7"/>
            </svg>
            视频网格
          </button>
          <button 
            :class="['view-btn', { active: viewMode === 'stats' }]"
            @click="viewMode = 'stats'"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 3v18h18"/>
              <path d="m19 9-5 5-4-4-3 3"/>
            </svg>
            统计概览
          </button>
        </div>
        
        <div v-if="viewMode === 'grid'" class="layout-selector">
          <span class="selector-label">布局:</span>
          <select v-model="gridLayout" class="layout-select">
            <option v-for="opt in layoutOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
        </div>
      </div>
      
      <div class="toolbar-right">
        <button class="btn btn-primary" @click="handleAddCamera">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="12" y1="5" x2="12" y2="19"/>
            <line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
          添加摄像头
        </button>
      </div>
    </div>

    <!-- 视频网格视图 -->
    <div v-if="viewMode === 'grid'" class="grid-view">
      <div class="main-content">
        <VideoGrid
          ref="videoGridRef"
          :layout="gridLayout"
          :cameras="cameras"
          v-model:selected-cameras="selectedCameras"
          :vlm-focus-id="vlmFocusId"
          :vlm-enabled-cameras="vlmEnabledCameras"
          :yolo-enabled-cameras="yoloEnabledCameras"
          :stream-base-url="'/api/v1/streams'"
          @video-ready="handleVideoReady"
          @video-error="handleVideoError"
          @toggle-yolo="handleToggleYolo"
          @toggle-vlm="handleToggleVlm"
        />
      </div>
      
      <div class="side-panel">
        <GatewayPanel />
      </div>
    </div>

    <!-- 统计概览视图 -->
    <div v-else class="stats-view">
      <!-- 统计卡片 -->
      <div class="stats-grid">
        <div 
          v-for="card in statCards" 
          :key="card.key"
          class="stat-card"
          :style="{ borderTopColor: card.color }"
        >
          <div class="stat-icon">{{ card.icon }}</div>
          <div class="stat-info">
            <div class="stat-value">{{ stats[card.key as keyof typeof stats] }}</div>
            <div class="stat-label">{{ card.label }}</div>
          </div>
        </div>
      </div>

      <!-- 节点状态和最近事件 -->
      <div class="dashboard-grid">
        <!-- 节点状态 -->
        <div class="card">
          <div class="card-title">节点状态</div>
          <div class="nodes-list">
            <div 
              v-for="node in nodesStore.nodes.slice(0, 5)" 
              :key="node.id"
              class="node-item"
            >
              <div class="node-info">
                <span class="node-name">{{ node.name || node.id }}</span>
                <span class="node-type">{{ node.type }}</span>
              </div>
              <span 
                :class="['status-badge', node.status === 'online' ? 'status-online' : 'status-offline']"
              >
                {{ node.status === 'online' ? '在线' : '离线' }}
              </span>
            </div>
            <div v-if="nodesStore.nodes.length === 0" class="empty-state">
              暂无节点数据
            </div>
          </div>
        </div>

        <!-- 最近事件 -->
        <div class="card">
          <div class="card-title">最近事件</div>
          <div class="events-list">
            <div 
              v-for="event in recentEvents" 
              :key="event.id"
              class="event-item"
            >
              <div class="event-icon">
                {{ event.type === 'alert' ? '🔔' : '📊' }}
              </div>
              <div class="event-info">
                <div class="event-title">{{ event.title || event.type }}</div>
                <div class="event-time">{{ event.created_at }}</div>
              </div>
            </div>
            <div v-if="recentEvents.length === 0" class="empty-state">
              暂无事件数据
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 摄像头管理弹窗 -->
    <CameraManagementModal
      v-model:visible="showCameraModal"
      :camera="editingCamera"
      :nodes="nodes"
      @save="handleSaveCamera"
    />
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

/* 工具栏 */
.dashboard-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 0;
  flex-shrink: 0;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 20px;
}

.view-switcher {
  display: flex;
  background: #f3f4f6;
  border-radius: 10px;
  padding: 4px;
}

.view-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border: none;
  background: transparent;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  color: #6b7280;
  cursor: pointer;
  transition: all 0.2s ease;
}

.view-btn svg {
  width: 16px;
  height: 16px;
}

.view-btn:hover {
  color: #374151;
}

.view-btn.active {
  background: #fff;
  color: #1a1a2e;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.layout-selector {
  display: flex;
  align-items: center;
  gap: 8px;
}

.selector-label {
  font-size: 13px;
  color: #6b7280;
}

.layout-select {
  padding: 8px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  font-size: 13px;
  color: #374151;
  cursor: pointer;
}

.layout-select:focus {
  outline: none;
  border-color: #3b82f6;
}

.toolbar-right {
  display: flex;
  gap: 10px;
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 10px 16px;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn svg {
  width: 16px;
  height: 16px;
}

.btn-primary {
  background: #3b82f6;
  color: #fff;
}

.btn-primary:hover {
  background: #2563eb;
}

/* 视频网格视图 */
.grid-view {
  flex: 1;
  display: flex;
  gap: 20px;
  overflow: hidden;
}

.main-content {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.side-panel {
  width: 320px;
  flex-shrink: 0;
  overflow: hidden;
}

@media (max-width: 1200px) {
  .side-panel {
    display: none;
  }
}

/* 统计视图 */
.stats-view {
  flex: 1;
  overflow-y: auto;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

@media (max-width: 1200px) {
  .stats-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

.stat-card {
  background: white;
  border-radius: 12px;
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  border-top: 3px solid;
}

.stat-icon {
  font-size: 32px;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: #1a1a2e;
}

.stat-label {
  font-size: 13px;
  color: #6b7280;
}

.dashboard-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
}

@media (max-width: 768px) {
  .dashboard-grid {
    grid-template-columns: 1fr;
  }
}

.card {
  background: #fff;
  border-radius: 12px;
  padding: 20px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #1a1a2e;
  margin-bottom: 16px;
}

.nodes-list, .events-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.node-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: #f9fafb;
  border-radius: 8px;
}

.node-info {
  display: flex;
  align-items: center;
}

.node-name {
  font-weight: 500;
  color: #1a1a2e;
}

.node-type {
  font-size: 12px;
  color: #6b7280;
  margin-left: 8px;
  padding: 2px 6px;
  background: #e5e7eb;
  border-radius: 4px;
}

.status-badge {
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
}

.status-online {
  background: #d1fae5;
  color: #059669;
}

.status-offline {
  background: #fee2e2;
  color: #dc2626;
}

.event-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: #f9fafb;
  border-radius: 8px;
}

.event-icon {
  font-size: 24px;
}

.event-info {
  flex: 1;
}

.event-title {
  font-weight: 500;
  color: #1a1a2e;
}

.event-time {
  font-size: 12px;
  color: #6b7280;
  margin-top: 2px;
}

.empty-state {
  text-align: center;
  padding: 40px;
  color: #9ca3af;
}
</style>
