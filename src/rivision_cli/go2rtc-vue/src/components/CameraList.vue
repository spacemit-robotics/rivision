<template>
  <div class="camera-list">
    <div class="section-title">
      <span class="icon">📹</span>
      <span>摄像头</span>
      <div class="batch-controls">
        <button 
          class="batch-btn vlm" 
          :class="{ active: isAllVlmEnabled }" 
          @click="toggleAllVlm"
          title="全部VLM开关"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
          </svg>
        </button>
        <button 
          class="batch-btn yolo" 
          :class="{ active: isAllYoloEnabled }" 
          @click="toggleAllYolo"
          title="全部YOLO开关"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <circle cx="12" cy="12" r="10"/>
            <circle cx="12" cy="12" r="6"/>
            <circle cx="12" cy="12" r="2"/>
          </svg>
        </button>
        <button 
          class="batch-btn settings" 
          @click="$emit('open-settings')"
          title="摄像头管理"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <circle cx="12" cy="12" r="3"/>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
          </svg>
        </button>
      </div>
    </div>
    
    <div class="camera-filters">
      <input
        v-model="searchText"
        class="search-input"
        type="text"
        placeholder="搜索摄像头名称/ID"
      />
      <div class="filter-row">
        <select v-model="statusFilter" class="filter-select">
          <option value="all">全部状态</option>
          <option value="online">仅在线</option>
          <option value="offline">仅离线</option>
        </select>
        <select v-model="sortMode" class="filter-select">
          <option value="name_asc">名称 A-Z</option>
          <option value="name_desc">名称 Z-A</option>
          <option value="online_first">在线优先</option>
        </select>
      </div>
    </div>

    <div class="camera-items">
      <div
        v-for="camera in filteredCameras"
        :key="camera.id"
        class="camera-item"
        :class="{ 
          selected: camera.id === selectedId,
          offline: !camera.online,
          'is-vlm-focus': camera.id === vlmFocusId
        }"
        @click="selectCamera(camera)"
      >
        <span 
          class="status-dot" 
          :class="camera.online ? 'online' : 'offline'"
        ></span>
        <div class="camera-info">
          <div class="camera-name">
            {{ camera.name }}
            <span v-if="camera.type" :class="['type-tag', camera.type.toLowerCase()]">{{ camera.type }}</span>
          </div>
          <div class="camera-location">{{ camera.location }}</div>
        </div>
        <!-- 摄像头独立开关 -->
        <div class="camera-toggles" v-if="camera.online" @click.stop>
          <button 
            class="toggle-btn vlm" 
            :class="{ active: vlmEnabledCameras.has(camera.id) }" 
            @click="toggleCameraVlm(camera.id)"
            :title="vlmEnabledCameras.has(camera.id) ? '关闭VLM分析' : '开启VLM分析'"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
            </svg>
          </button>
          <button 
            class="toggle-btn yolo" 
            :class="{ active: yoloEnabledCameras.has(camera.id) }" 
            @click="toggleCameraYolo(camera.id)"
            :title="yoloEnabledCameras.has(camera.id) ? '关闭YOLO检测' : '开启YOLO检测'"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <circle cx="12" cy="12" r="10"/>
              <circle cx="12" cy="12" r="6"/>
              <circle cx="12" cy="12" r="2"/>
            </svg>
          </button>
        </div>
        <!-- 任务徽章（已移除，VLM/YOLO开关已提供状态指示） -->
      </div>
    </div>

    <div v-if="cameras.length === 0" class="no-cameras">
      <p>暂无摄像头</p>
      <p class="hint">请检查 go2rtc / OWL 配置</p>
    </div>

    <div v-else-if="filteredCameras.length === 0" class="no-cameras">
      <p>未匹配到摄像头</p>
      <p class="hint">请调整搜索条件或筛选项</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, inject, watch, type Ref } from 'vue'
import { yoloApi } from '../services/yoloApi'
import { getOWLStreams } from '../services/owlStreams'

export interface Camera {
  id: string
  name: string
  location: string
  online: boolean
  streamUrl?: string
  type?: string  // 视频源类型: ONVIF | GB28181 | RTSP | RTMP
}

const props = defineProps<{
  modelValue?: string
  vlmFocusId?: string | null
  vlmEnabledCameras?: Set<string>
  yoloEnabledCameras?: Set<string>
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', id: string): void
  (e: 'select', camera: Camera): void
  (e: 'update:vlmFocusId', id: string): void
  (e: 'update:vlmEnabledCameras', cameras: Set<string>): void
  (e: 'update:yoloEnabledCameras', cameras: Set<string>): void
  (e: 'open-settings'): void
}>()

// 本地状态
const vlmEnabledCameras = ref<Set<string>>(new Set(props.vlmEnabledCameras || []))
const yoloEnabledCameras = ref<Set<string>>(new Set(props.yoloEnabledCameras || []))

// 同步父组件的 prop 变化（页面加载后从后端同步状态时触发）
watch(() => props.yoloEnabledCameras, (newVal) => {
  if (newVal) {
    yoloEnabledCameras.value = new Set(newVal)
  }
})

watch(() => props.vlmEnabledCameras, (newVal) => {
  if (newVal) {
    vlmEnabledCameras.value = new Set(newVal)
  }
})

// 计算属性：是否全部启用
const isAllVlmEnabled = computed(() => {
  const onlineCameras = cameras.value.filter(c => c.online)
  return onlineCameras.length > 0 && onlineCameras.every(c => vlmEnabledCameras.value.has(c.id))
})

const isAllYoloEnabled = computed(() => {
  const onlineCameras = cameras.value.filter(c => c.online)
  return onlineCameras.length > 0 && onlineCameras.every(c => yoloEnabledCameras.value.has(c.id))
})

// 批量开关
function toggleAllVlm() {
  const onlineCameras = cameras.value.filter(c => c.online)
  if (isAllVlmEnabled.value) {
    vlmEnabledCameras.value = new Set()
  } else {
    vlmEnabledCameras.value = new Set(onlineCameras.map(c => c.id))
  }
  emit('update:vlmEnabledCameras', vlmEnabledCameras.value)
}

async function toggleAllYolo() {
  const onlineCameras = cameras.value.filter(c => c.online)
  const enableAll = !isAllYoloEnabled.value
  const previousSet = new Set(yoloEnabledCameras.value)
  
  // ★ 乐观更新：先同步更新前端状态，再异步调用后端API
  if (enableAll) {
    yoloEnabledCameras.value = new Set(onlineCameras.map(c => c.id))
  } else {
    yoloEnabledCameras.value = new Set()
  }
  emit('update:yoloEnabledCameras', yoloEnabledCameras.value)
  
  try {
    // 后台批量调用后端API
    for (const camera of onlineCameras) {
      if (enableAll) {
        await yoloApi.enable(camera.id)
      } else {
        await yoloApi.disable(camera.id)
      }
    }
  } catch (error) {
    console.error('[CameraList] Batch YOLO toggle failed, rolling back:', error)
    // 回滚到之前的状态
    yoloEnabledCameras.value = previousSet
    emit('update:yoloEnabledCameras', previousSet)
  }
}

// 单个摄像头开关
function toggleCameraVlm(cameraId: string) {
  const newSet = new Set(vlmEnabledCameras.value)
  if (newSet.has(cameraId)) {
    newSet.delete(cameraId)
  } else {
    newSet.add(cameraId)
  }
  vlmEnabledCameras.value = newSet
  emit('update:vlmEnabledCameras', newSet)
}

async function toggleCameraYolo(cameraId: string) {
  const wasEnabled = yoloEnabledCameras.value.has(cameraId)
  
  // ★ 乐观更新：先同步更新前端状态，避免快速点击多个摄像头时的竞态条件
  // 旧逻辑在 await 前捕获快照，多个并发 toggle 的快照互相覆盖导致只有最后一个生效
  const newSet = new Set(yoloEnabledCameras.value)
  if (wasEnabled) {
    newSet.delete(cameraId)
  } else {
    newSet.add(cameraId)
  }
  yoloEnabledCameras.value = newSet
  emit('update:yoloEnabledCameras', newSet)
  
  try {
    // 后台调用后端API
    if (wasEnabled) {
      await yoloApi.disable(cameraId)
    } else {
      await yoloApi.enable(cameraId)
    }
  } catch (error) {
    console.error(`[CameraList] YOLO toggle failed for ${cameraId}, rolling back:`, error)
    // 回滚：恢复之前的状态
    const rollbackSet = new Set(yoloEnabledCameras.value)
    if (wasEnabled) {
      rollbackSet.add(cameraId)
    } else {
      rollbackSet.delete(cameraId)
    }
    yoloEnabledCameras.value = rollbackSet
    emit('update:yoloEnabledCameras', rollbackSet)
  }
}

const cameras = ref<Camera[]>([])
const selectedId = ref<string>(props.modelValue || '')
const searchText = ref('')
const cameraHints = ref<Record<string, string>>({})
let refreshTimer: number | undefined
const statusFilter = ref<'all' | 'online' | 'offline'>('all')
const sortMode = ref<'name_asc' | 'name_desc' | 'online_first'>('online_first')

const filteredCameras = computed(() => {
  let list = [...cameras.value]

  const keyword = searchText.value.trim().toLowerCase()
  if (keyword) {
    list = list.filter((c) =>
      c.id.toLowerCase().includes(keyword) ||
      c.name.toLowerCase().includes(keyword) ||
      c.location.toLowerCase().includes(keyword)
    )
  }

  if (statusFilter.value === 'online') {
    list = list.filter((c) => c.online)
  } else if (statusFilter.value === 'offline') {
    list = list.filter((c) => !c.online)
  }

  switch (sortMode.value) {
    case 'name_asc':
      list.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
      break
    case 'name_desc':
      list.sort((a, b) => b.name.localeCompare(a.name, 'zh-CN'))
      break
    case 'online_first':
    default:
      list.sort((a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name, 'zh-CN'))
      break
  }

  return list
})

// 注入摄像头活跃任务数
const cameraActiveTasks = inject<Ref<Record<string, number>>>('cameraActiveTasks', ref({}))

// 获取指定摄像头的任务数
function getTaskCount(cameraId: string): number {
  return cameraActiveTasks.value?.[cameraId] || 0
}

async function fetchCameras() {
  try {
    // ★ OWL 优先：使用统一流服务，自动回退到 go2rtc
    const streams = await getOWLStreams()
    cameraHints.value = {}
    cameras.value = streams.map(s => {
      const hint = s.online ? '在线' : (s.source?.startsWith('proxy://') ? '可用(待拉流)' : '离线')
      cameraHints.value[s.id] = hint
      return {
        id: s.id,
        name: (s.name && s.name.trim()) || s.id,
        location: hint,
        online: s.online || (s.source?.startsWith('proxy://') ?? false),
        streamUrl: `/api/go2rtc/stream.mp4?src=${encodeURIComponent(s.id)}`,
        type: s.type && s.type !== 'unknown' ? s.type.toUpperCase() : undefined
      }
    })
  } catch (e) {
    console.error('Failed to fetch cameras:', e)
  }
}

function selectCamera(camera: Camera) {
  selectedId.value = camera.id
  emit('update:modelValue', camera.id)
  emit('select', camera)
}

function setVlmFocus(cameraId: string) {
  emit('update:vlmFocusId', cameraId)
}

onMounted(() => {
  fetchCameras()
  // 每30秒刷新一次
  refreshTimer = window.setInterval(fetchCameras, 30000)
})

onUnmounted(() => {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
  }
})

defineExpose({
  refresh: fetchCameras,
  cameras
})
</script>

<style scoped>
.camera-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  /* ★ 填充可用空间并限制高度 */
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
  padding-bottom: 8px;
  padding-right: 32px;
  border-bottom: 1px solid var(--border-color, #333);
}

.section-title .icon {
  font-size: 16px;
}

.batch-controls {
  display: flex;
  gap: 4px;
  margin-left: auto;
}

.batch-btn {
  width: 26px;
  height: 26px;
  padding: 5px;
  border: 1px solid var(--border-color, #444);
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary, #888);
  cursor: pointer;
  transition: all 0.2s ease;
  display: flex;
  align-items: center;
  justify-content: center;
}

.batch-btn svg {
  width: 14px;
  height: 14px;
  stroke-width: 2;
}

.batch-btn:hover {
  border-color: var(--primary-color, #4a9eff);
  color: var(--primary-color, #4a9eff);
}

.batch-btn.active {
  background: var(--primary-color, #4a9eff);
  border-color: var(--primary-color, #4a9eff);
  color: white;
}

.camera-toggles {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}

.toggle-btn {
  width: 24px;
  height: 24px;
  padding: 4px;
  border: 1px solid var(--border-color, #444);
  border-radius: 6px;
  background: transparent;
  color: var(--text-muted, #666);
  cursor: pointer;
  transition: all 0.2s ease;
  display: flex;
  align-items: center;
  justify-content: center;
}

.toggle-btn svg {
  width: 14px;
  height: 14px;
  stroke-width: 2;
}

.toggle-btn:hover {
  border-color: var(--text-secondary, #888);
  color: var(--text-primary, #e0e0e0);
}

.toggle-btn.vlm.active {
  background: var(--accent-primary, #10a37f);
  border-color: var(--accent-primary, #10a37f);
  color: white;
}

.toggle-btn.yolo.active {
  background: #e6a23c;
  border-color: #e6a23c;
  color: white;
}

.yolo-tag {
  display: inline-flex;
  align-items: center;
  padding: 2px 6px;
  margin-left: 4px;
  background: #e6a23c;
  color: white;
  font-size: 10px;
  font-weight: 700;
  border-radius: 4px;
  vertical-align: middle;
}

.camera-filters {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 8px;
}

.search-input {
  width: 100%;
  height: 32px;
  border: 1px solid var(--border-color, #444);
  border-radius: 8px;
  background: var(--bg-tertiary, #252535);
  color: var(--text-primary, #e0e0e0);
  padding: 0 10px;
  font-size: 12px;
  outline: none;
}

.search-input:focus {
  border-color: var(--primary-color, #4a9eff);
}

.filter-row {
  display: flex;
  gap: 8px;
}

.filter-select {
  flex: 1;
  height: 30px;
  border: 1px solid var(--border-color, #444);
  border-radius: 6px;
  background: var(--bg-tertiary, #252535);
  color: var(--text-secondary, #bbb);
  padding: 0 8px;
  font-size: 12px;
}

.camera-items {
  display: flex;
  flex-direction: column;
  gap: 8px;
  /* ★ 填充剩余空间并独立滚动 */
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: 4px;
}

/* 自定义滚动条样式 */
.camera-items::-webkit-scrollbar {
  width: 6px;
}
.camera-items::-webkit-scrollbar-track {
  background: transparent;
}
.camera-items::-webkit-scrollbar-thumb {
  background: var(--border-color, #444);
  border-radius: 3px;
}
.camera-items::-webkit-scrollbar-thumb:hover {
  background: var(--text-muted, #666);
}

.camera-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px;
  background: var(--bg-tertiary, #252535);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
  border: 2px solid transparent;
}

.camera-item:hover {
  background: var(--bg-hover, #2a2a3a);
}

.camera-item.selected {
  border-color: var(--primary-color, #4a9eff);
  background: var(--bg-selected, #1a2a4a);
}

.camera-item.offline {
  opacity: 0.6;
}

.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-dot.online {
  background: #4ade80;
  box-shadow: 0 0 6px #4ade80;
}

.status-dot.offline {
  background: #f87171;
}

.camera-info {
  flex: 1;
  min-width: 0;
}

.camera-name {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary, #e0e0e0);
}

.camera-location {
  font-size: 12px;
  color: var(--text-secondary, #888);
  margin-top: 2px;
}

.check-mark {
  color: var(--primary-color, #4a9eff);
  font-weight: bold;
}

.status-dot.processing {
  background: #4a9eff;
  box-shadow: 0 0 6px #4a9eff;
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.no-cameras {
  text-align: center;
  padding: 20px;
  color: var(--text-secondary, #888);
}

.no-cameras .hint {
  font-size: 12px;
  margin-top: 4px;
  opacity: 0.7;
}

/* VLM 标签样式 */
.vlm-tag {
  display: inline-flex;
  align-items: center;
  padding: 2px 6px;
  margin-left: 4px;
  background: var(--accent-primary, #10a37f);
  color: white;
  font-size: 10px;
  font-weight: 700;
  border-radius: 4px;
  vertical-align: middle;
}

/* 视频源类型标签样式 */
.type-tag {
  display: inline-flex;
  padding: 2px 6px;
  margin-left: 6px;
  font-size: 10px;
  font-weight: 600;
  border-radius: 4px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  background: #6b7280;
  color: white;
}
.type-tag.onvif {
  background: #06b6d4;
}
.type-tag.gb28181 {
  background: #8b5cf6;
}
.type-tag.rtsp {
  background: #f59e0b;
}
.type-tag.rtmp {
  background: #ef4444;
}
</style>
