<template>
  <div class="event-timeline">
    <div class="timeline-header">
      <div class="title">
        <span class="icon">�</span>
        <span>事件时间线</span>
      </div>
      <span class="item-count" v-if="resultList.length">共 {{ resultList.length }} 个事件</span>
    </div>

    <div class="event-list" v-if="resultList.length > 0">
      <div
        v-for="item in resultList"
        :key="item.id"
        class="event-item"
        @click="selectItem(item)"
      >
        <div class="event-time-marker">
          <span class="time-dot"></span>
          <span class="time-label">{{ formatTime(item.timestamp) }}</span>
        </div>
        <div class="event-content">
          <div class="event-thumbnail">
            <img v-if="item.thumbnail" :src="item.thumbnail" alt="截图" />
            <span v-else class="no-thumb">🖼</span>
            <span v-if="item.nodeId" class="node-badge" :style="{ background: getNodeColor(item.nodeId) }">{{ item.nodeId }}</span>
          </div>
          <div class="event-text">
            <p class="event-result">{{ truncateText(item.text, 150) }}</p>
            <div class="event-actions">
              <button class="action-btn primary" @click.stop="viewDetailFor(item)">查看详情</button>
              <button class="action-btn" @click.stop="playbackFor(item)">视频回放</button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="no-result" v-else>
      <div class="placeholder-icon">🤖</div>
      <p>等待分析结果...</p>
      <p class="hint">系统将自动采样分析</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import type { TimelineItem } from './AnalysisTimeline.vue'

export interface AnalysisResult {
  id: string
  timestamp: number
  cameraId: string
  thumbnail?: string
  text: string
  videoSegment?: string
  nodeId?: string
  processingTime?: number
  resolution?: string
}

const props = defineProps<{
  cameraId?: string
  result?: AnalysisResult
  items?: TimelineItem[]  // 完整的事件列表
}>()

const emit = defineEmits<{
  (e: 'view-detail', result: AnalysisResult): void
  (e: 'playback', result: AnalysisResult): void
}>()

const result = ref<AnalysisResult | null>(props.result || null)
const resultList = ref<AnalysisResult[]>([])

// 节点颜色映射（与AnalysisTimeline保持一致）
const nodeColors: Record<string, string> = {}
const colorPalette = ['#4a9eff', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899']

function getNodeColor(nodeId?: string): string {
  if (!nodeId) return colorPalette[0]
  if (!nodeColors[nodeId]) {
    nodeColors[nodeId] = colorPalette[Object.keys(nodeColors).length % colorPalette.length]
  }
  return nodeColors[nodeId]
}

// 监听props.items变化（完整列表）
watch(() => props.items, (newItems) => {
  if (newItems && newItems.length > 0) {
    // 转换TimelineItemInput为AnalysisResult格式
    resultList.value = newItems.map(item => ({
      id: item.id,
      timestamp: item.timestamp,
      cameraId: item.cameraId,
      thumbnail: item.thumbnail,
      text: item.result || item.summary || '',
      nodeId: item.nodeId,
      processingTime: item.processingTime,
      resolution: item.resolution,
    }))
  }
}, { immediate: true, deep: true })

// 监听props.result变化（单个新结果）
watch(() => props.result, (newVal) => {
  if (newVal) {
    result.value = newVal
    // 添加到列表顶部（去重）
    const exists = resultList.value.find(r => r.id === newVal.id)
    if (!exists) {
      resultList.value.unshift(newVal)
    }
  }
}, { immediate: true })

function truncateText(text: string, maxLen: number): string {
  return text.length > maxLen ? text.slice(0, maxLen) + '...' : text
}

function selectItem(item: AnalysisResult) {
  result.value = item
}

function viewDetailFor(item: AnalysisResult) {
  emit('view-detail', item)
}

function playbackFor(item: AnalysisResult) {
  emit('playback', item)
}

function formatTime(timestamp: number): string {
  // 后端返回秒级时间戳，转换为毫秒级
  const ms = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const date = new Date(ms)
  return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}:${date.getSeconds().toString().padStart(2, '0')}`
}

async function fetchLatest() {
  if (props.result) return // 如果传入了result则不请求
  
  try {
    const params = new URLSearchParams()
    if (props.cameraId) params.set('camera_id', props.cameraId)
    params.set('limit', '1')
    
    const resp = await fetch(`/api/inference/history?${params}`)
    if (resp.ok) {
      const data = await resp.json()
      if (data.items && data.items.length > 0) {
        const item = data.items[0]
        const newResult = {
          id: item.task_id || item.id,
          timestamp: item.timestamp || Date.now(),
          cameraId: item.camera_id || item.stream_name,
          thumbnail: item.thumbnail,
          text: item.result || item.summary || '',
          videoSegment: item.video_segment
        }
        result.value = newResult
        // 添加到列表
        const exists = resultList.value.find(r => r.id === newResult.id)
        if (!exists) {
          resultList.value.unshift(newResult)
        }
      }
    }
  } catch (e) {
    console.error('Failed to fetch latest result:', e)
  }
}

// WebSocket监听新结果
let ws: WebSocket | null = null

function connectWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}/ws/inference`
  
  ws = new WebSocket(wsUrl)
  
  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      if (data.type === 'result' && (!props.cameraId || data.camera_id === props.cameraId)) {
        result.value = {
          id: data.task_id,
          timestamp: data.timestamp || Date.now(),
          cameraId: data.camera_id,
          thumbnail: data.thumbnail,
          text: data.result,
          videoSegment: data.video_segment
        }
      }
    } catch (e) {
      // 忽略解析错误
    }
  }
  
  ws.onclose = () => {
    // 5秒后重连
    setTimeout(connectWebSocket, 5000)
  }
}

let refreshTimer: number | null = null

onMounted(() => {
  fetchLatest()
  connectWebSocket()
  
  // 备用轮询
  refreshTimer = window.setInterval(fetchLatest, 60000)
})

onUnmounted(() => {
  if (ws) ws.close()
  if (refreshTimer) clearInterval(refreshTimer)
})

defineExpose({
  refresh: fetchLatest,
  setResult: (r: AnalysisResult) => { result.value = r }
})
</script>

<style scoped>
.event-timeline {
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 12px;
  padding: 16px;
}

.timeline-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
}

.title .icon {
  font-size: 16px;
}

.item-count {
  font-size: 12px;
  color: var(--text-secondary, #888);
}

.event-list {
  max-height: 500px;
  overflow-y: auto;
  padding-right: 8px;
}

.event-item {
  display: flex;
  flex-direction: column;
  padding: 12px 0;
  border-bottom: 1px solid var(--border-color, #333);
  cursor: pointer;
  transition: background 0.2s ease;
}

.event-item:hover {
  background: var(--bg-tertiary, #252535);
  border-radius: 8px;
  margin: 0 -8px;
  padding: 12px 8px;
}

.event-item:last-child {
  border-bottom: none;
}

.event-time-marker {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.time-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--primary-color, #4a9eff);
}

.time-label {
  font-size: 12px;
  color: var(--primary-color, #4a9eff);
  font-weight: 500;
}

.event-content {
  display: flex;
  gap: 12px;
  padding-left: 16px;
}

.event-thumbnail {
  position: relative;
  width: 80px;
  height: 60px;
  border-radius: 6px;
  background: var(--bg-tertiary, #252535);
  border: 1px solid var(--border-color, #444);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.node-badge {
  position: absolute;
  bottom: 2px;
  left: 2px;
  color: white;
  font-size: 9px;
  padding: 1px 4px;
  border-radius: 3px;
  font-weight: 500;
}

.event-thumbnail img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.no-thumb {
  font-size: 20px;
  opacity: 0.5;
}

.event-text {
  flex: 1;
  min-width: 0;
}

.event-result {
  font-size: 13px;
  color: var(--text-primary, #e0e0e0);
  line-height: 1.5;
  margin: 0 0 8px 0;
  word-break: break-word;
}

.event-actions {
  display: flex;
  gap: 8px;
}

.action-btn {
  padding: 4px 12px;
  font-size: 12px;
  border-radius: 4px;
  border: 1px solid var(--border-color, #444);
  cursor: pointer;
  background: var(--bg-tertiary, #252535);
  color: var(--text-primary, #e0e0e0);
  transition: all 0.2s ease;
}

.action-btn:hover {
  border-color: var(--primary-color, #4a9eff);
}

.action-btn.primary {
  background: var(--primary-color, #4a9eff);
  border-color: var(--primary-color, #4a9eff);
  color: white;
}

.action-btn.primary:hover {
  opacity: 0.8;
}

.no-result {
  text-align: center;
  padding: 40px 20px;
  color: var(--text-secondary, #888);
}

.placeholder-icon {
  font-size: 48px;
  margin-bottom: 12px;
  opacity: 0.5;
}

.no-result p {
  margin: 4px 0;
}

.no-result .hint {
  font-size: 12px;
  opacity: 0.7;
}
</style>
