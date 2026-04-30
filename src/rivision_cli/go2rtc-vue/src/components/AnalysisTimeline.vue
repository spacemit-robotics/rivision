<template>
  <div class="analysis-timeline">
    <div class="timeline-header">
      <div class="title">
        <span class="icon">📊</span>
        <span>分析时间线</span>
      </div>
      <div class="header-right">
        <select v-model="dateFilter" class="date-filter">
          <option value="today">今日</option>
          <option value="yesterday">昨日</option>
          <option value="week">本周</option>
        </select>
      </div>
    </div>

    <div class="timeline-container">
      <!-- 时间轴 -->
      <div class="time-axis">
        <div class="axis-line"></div>
        <div 
          v-for="mark in visibleMinutes" 
          :key="mark.time" 
          class="time-mark"
          :style="{ left: getMarkPosition(mark.time) + '%' }"
        >
          <span class="time-label">{{ mark.label }}</span>
        </div>
        <div class="now-marker" :style="{ left: getNowPosition() + '%' }"></div>
      </div>

      <!-- 瀑布流节点行 -->
      <div class="waterfall-rows">
        <div 
          v-for="(row, rowIndex) in nodeRows" 
          :key="rowIndex" 
          class="node-row"
          :style="{ top: (rowIndex * 70) + 'px' }"
        >
          <div
            v-for="item in row.items"
            :key="item.id"
            class="analysis-point"
            :class="{ 
              processing: item.status === 'processing', 
              selected: item.id === selectedId,
              failed: item.status === 'failed'
            }"
            :style="{ left: getItemPosition(item.timestamp) + '%' }"
            @click="selectItem(item)"
          >
            <div class="point-thumbnail">
              <img v-if="item.thumbnail" :src="item.thumbnail" :alt="item.summary" />
              <span v-else-if="item.status === 'processing'" class="processing-icon">⏳</span>
              <span v-else class="no-thumb">📷</span>
            </div>
            <div class="point-info">
              <span class="point-time">{{ formatTime(item.timestamp) }}</span>
              <span class="point-node" :style="{ backgroundColor: getNodeColor(item.nodeId) }">
                {{ item.nodeId || '节点' + (rowIndex + 1) }}
              </span>
              <span class="point-status">
                <template v-if="item.status === 'completed'">✅</template>
                <template v-else-if="item.status === 'processing'">🔄</template>
                <template v-else>❌</template>
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 节点状态栏 -->
    <div class="node-status-bar" v-if="nodeStatuses.length > 0">
      <div 
        v-for="node in nodeStatuses" 
        :key="node.id" 
        class="node-status"
        :class="{ busy: node.status === 'processing', idle: node.status === 'idle' }"
      >
        <span class="node-dot" :style="{ backgroundColor: getNodeColor(node.id) }"></span>
        <span class="node-name">{{ node.id }}</span>
        <span class="node-state">{{ node.status === 'processing' ? '处理中' : '空闲' }}</span>
        <span v-if="node.remainingTime" class="node-remaining">({{ node.remainingTime }}s)</span>
      </div>
      <div class="queue-info">
        <span>队列: {{ queuePending }}待处理</span>
        <span>完成: {{ queueCompleted }}</span>
      </div>
    </div>

    <div v-if="timelineItems.length === 0" class="no-data">
      <p>暂无分析记录</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'

export interface TimelineItem {
  id: string
  timestamp: number
  cameraId: string
  thumbnail?: string
  summary?: string
  result?: string
  nodeId?: string
  processingTime?: number
  resolution?: string
  status: 'completed' | 'processing' | 'failed'
}

export interface NodeStatus {
  id: string
  status: 'idle' | 'processing'
  remainingTime?: number
  healthy?: boolean
}

const props = defineProps<{
  cameraId?: string
  items?: TimelineItem[]
  processingTime?: number  // 单帧处理时间（秒）
  nodes?: NodeStatus[]     // 节点列表
}>()

const emit = defineEmits<{
  (e: 'select', item: TimelineItem): void
}>()

const dateFilter = ref('today')
const selectedId = ref<string>('')
const timelineItems = ref<TimelineItem[]>([])
const nodeStatuses = ref<NodeStatus[]>([])

// 用于触发时间范围刷新的响应式变量
const nowTick = ref(Date.now())
let tickTimer: number | null = null

// 动态计算节点数量（只计算在线节点）
const nodeCount = computed(() => {
  const healthyNodes = props.nodes?.filter(n => n.healthy !== false).length || 0
  const fromItems = new Set(timelineItems.value.map(i => i.nodeId).filter(Boolean)).size
  return Math.max(healthyNodes, fromItems, 1)
})


// 队列统计
const queuePending = computed(() => 
  timelineItems.value.filter(i => i.status === 'processing').length
)
const queueCompleted = computed(() => 
  timelineItems.value.filter(i => i.status === 'completed').length
)

// 节点颜色映射
const nodeColors: Record<string, string> = {}
const colorPalette = ['#4a9eff', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899']

function getNodeColor(nodeId?: string): string {
  if (!nodeId) return colorPalette[0]
  if (!nodeColors[nodeId]) {
    nodeColors[nodeId] = colorPalette[Object.keys(nodeColors).length % colorPalette.length]
  }
  return nodeColors[nodeId]
}

// 按节点分组的瀑布流行
const nodeRows = computed(() => {
  const nodeMap = new Map<string, TimelineItem[]>()
  
  timelineItems.value.forEach((item, index) => {
    const nodeId = item.nodeId || `节点${(index % nodeCount.value) + 1}`
    if (!nodeMap.has(nodeId)) {
      nodeMap.set(nodeId, [])
    }
    nodeMap.get(nodeId)!.push({ ...item, nodeId })
  })
  
  return Array.from(nodeMap.entries()).map(([nodeId, items]) => ({
    nodeId,
    items: items.sort((a, b) => a.timestamp - b.timestamp)
  }))
})

// 时间范围计算 - 依赖nowTick实现动态更新
const timeRange = computed(() => {
  const now = nowTick.value
  const start = now - 3600000 // 1小时前
  return { start, end: now }
})

// 1小时范围，每10分钟一个刻度
const visibleMinutes = computed(() => {
  const marks: { time: number; label: string }[] = []
  const start = timeRange.value.start
  const end = timeRange.value.end
  
  for (let t = start; t <= end; t += 600000) {
    const date = new Date(t)
    marks.push({
      time: t,
      label: `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`
    })
  }
  return marks
})

function getMarkPosition(timestamp: number): number {
  const range = timeRange.value.end - timeRange.value.start
  return ((timestamp - timeRange.value.start) / range) * 100
}

function getNowPosition(): number {
  const now = Date.now()
  const range = timeRange.value.end - timeRange.value.start
  return Math.min(100, ((now - timeRange.value.start) / range) * 100)
}

function getItemPosition(timestamp: number): number {
  const ms = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const range = timeRange.value.end - timeRange.value.start
  const pos = ((ms - timeRange.value.start) / range) * 100
  return Math.max(0, Math.min(100, pos))
}

function formatTime(timestamp: number): string {
  const ms = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const date = new Date(ms)
  return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`
}

function selectItem(item: TimelineItem) {
  selectedId.value = item.id
  emit('select', item)
}

async function fetchTimeline() {
  try {
    const params = new URLSearchParams()
    if (props.cameraId) params.set('camera_id', props.cameraId)
    params.set('start', timeRange.value.start.toString())
    params.set('end', timeRange.value.end.toString())
    
    const resp = await fetch(`/api/inference/history?${params}`)
    if (resp.ok) {
      const data = await resp.json()
      timelineItems.value = (data.items || []).map((item: any) => ({
        id: item.task_id || item.id,
        timestamp: item.timestamp || Date.now(),
        cameraId: item.camera_id || item.stream_name,
        thumbnail: item.thumbnail,
        summary: item.summary || (item.result ? item.result.slice(0, 20) : ''),
        result: item.result,
        status: item.status || 'completed'
      }))
    }
  } catch (e) {
    console.error('Failed to fetch timeline:', e)
  }
}

// 如果props传入items则使用传入的
watch(() => props.items, (newItems) => {
  if (newItems) {
    timelineItems.value = newItems
  }
}, { immediate: true })

watch(() => props.cameraId, () => {
  if (!props.items) fetchTimeline()
})

watch(dateFilter, () => {
  if (!props.items) fetchTimeline()
})

let refreshTimer: number | null = null

onMounted(() => {
  if (!props.items) {
    fetchTimeline()
    refreshTimer = window.setInterval(fetchTimeline, 30000)
  }
  // 每10秒更新时间轴范围
  tickTimer = window.setInterval(() => {
    nowTick.value = Date.now()
  }, 10000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  if (tickTimer) clearInterval(tickTimer)
})

defineExpose({
  refresh: fetchTimeline,
  items: timelineItems
})
</script>

<style scoped>
.analysis-timeline {
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 12px;
  padding: 16px;
}

.timeline-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  flex-wrap: wrap;
  gap: 8px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.stats-bar {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: var(--text-secondary, #888);
}

.stat-item {
  padding: 4px 8px;
  background: var(--bg-tertiary, #252535);
  border-radius: 4px;
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

.date-filter {
  background: var(--bg-tertiary, #252535);
  border: 1px solid var(--border-color, #333);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--text-primary, #e0e0e0);
  cursor: pointer;
}

.timeline-container {
  position: relative;
  min-height: 250px;
  margin-top: 10px;
}

.time-axis {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 30px;
}

.axis-line {
  position: absolute;
  top: 20px;
  left: 0;
  right: 0;
  height: 2px;
  background: var(--border-color, #333);
}

.time-mark {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
}

.time-label {
  font-size: 10px;
  color: var(--text-secondary, #888);
}

.now-marker {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
}

.now-label {
  font-size: 10px;
  color: var(--primary-color, #4a9eff);
  font-weight: bold;
}

/* 瀑布流布局 */
.waterfall-rows {
  position: absolute;
  top: 35px;
  left: 0;
  right: 0;
  bottom: 0;
}

.node-row {
  position: absolute;
  left: 0;
  right: 0;
  height: 65px;
}

.analysis-point {
  position: absolute;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  cursor: pointer;
  transition: transform 0.2s ease;
}

.analysis-point:hover {
  transform: translateX(-50%) scale(1.08);
  z-index: 10;
}

.analysis-point.selected .point-thumbnail {
  border-color: var(--primary-color, #4a9eff);
  box-shadow: 0 0 8px var(--primary-color, #4a9eff);
}

.analysis-point.processing .point-thumbnail {
  border-color: #f59e0b;
  animation: processing-glow 1.5s infinite;
}

.analysis-point.failed .point-thumbnail {
  border-color: #ef4444;
}

@keyframes processing-glow {
  0%, 100% { box-shadow: 0 0 4px #f59e0b; }
  50% { box-shadow: 0 0 12px #f59e0b; }
}

.point-thumbnail {
  width: 48px;
  height: 32px;
  border-radius: 4px;
  background: var(--bg-tertiary, #252535);
  border: 2px solid var(--border-color, #444);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.point-thumbnail img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.point-info {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
}

.point-time {
  font-size: 9px;
  color: var(--text-secondary, #888);
}

.point-node {
  font-size: 8px;
  padding: 1px 4px;
  border-radius: 2px;
  color: white;
}

.point-status {
  font-size: 10px;
}

.processing-icon {
  font-size: 16px;
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.no-thumb {
  font-size: 16px;
  opacity: 0.5;
}

.point-time {
  font-size: 10px;
  color: var(--text-secondary, #888);
}

.point-summary {
  font-size: 10px;
  color: var(--text-primary, #e0e0e0);
  max-width: 60px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  text-align: center;
}

.no-data {
  text-align: center;
  padding: 30px;
  color: var(--text-secondary, #888);
  font-size: 14px;
}

/* 节点状态栏 */
.node-status-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding: 12px;
  margin-top: 8px;
  background: var(--bg-tertiary, #252535);
  border-radius: 8px;
  align-items: center;
}

.node-status {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 4px;
  font-size: 11px;
}

.node-status.busy {
  border: 1px solid #f59e0b;
}

.node-status.idle {
  border: 1px solid #10b981;
}

.node-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.node-name {
  font-weight: 500;
  color: var(--text-primary, #e0e0e0);
}

.node-state {
  color: var(--text-secondary, #888);
}

.node-remaining {
  color: #f59e0b;
  font-size: 10px;
}

.queue-info {
  margin-left: auto;
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: var(--text-secondary, #888);
}
</style>
