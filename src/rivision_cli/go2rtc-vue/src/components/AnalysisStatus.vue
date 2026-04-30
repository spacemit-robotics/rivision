<template>
  <div class="analysis-status">
    <div class="section-title">
      <span class="icon">📊</span>
      <span>分析状态</span>
    </div>

    <div class="status-items">
      <div class="status-item">
        <span class="label">触发方式</span>
        <span class="value" v-if="yoloMode === 'interval'">定时 {{ samplingInterval }}s</span>
        <span class="value" v-else-if="yoloMode === 'yolo_trigger'">YOLO触发</span>
        <span class="value" v-else>手动</span>
      </div>

      <div class="status-item">
        <span class="label">节点</span>
        <span class="value">{{ healthyNodes }}/{{ totalNodes }}个</span>
      </div>

      <div class="status-item">
        <span class="label">效率</span>
        <span class="value highlight" v-if="vlmActiveCount > 0 && yoloMode === 'interval'">{{ framesPerHour }}帧/h ({{ vlmActiveCount }}路)</span>
        <span class="value highlight" v-else-if="vlmActiveCount > 0 && yoloMode === 'yolo_trigger'">按需 ({{ vlmActiveCount }}路)</span>
        <span class="value" style="color: #888" v-else>未启动</span>
      </div>

      <div class="status-item gateway">
        <span class="label">分布式推理网关</span>
        <span class="value" :class="gatewayStatus">
          <span class="status-icon">{{ gatewayStatus === 'online' ? '✅' : '❌' }}</span>
          {{ gatewayStatus === 'online' ? '在线' : '离线' }}
        </span>
      </div>
    </div>

    <!-- 参数设置区域 -->
    <div class="section-title params-title">
      <span class="icon">⚙️</span>
      <span>参数设置</span>
    </div>
    
    <div class="params-items">
      <div class="param-item">
        <span class="label">VLM触发模式</span>
        <div class="param-input">
          <select v-model="yoloMode" @change="onParamsChange">
            <option value="interval">定时触发</option>
            <option value="yolo_trigger">YOLO触发</option>
            <option value="manual">仅手动</option>
          </select>
        </div>
      </div>
      
      <div class="param-item" v-if="yoloMode === 'interval'">
        <span class="label">VLM 采样间隔</span>
        <div class="param-input">
          <input 
            type="number" 
            v-model.number="vlmInterval" 
            min="30" 
            max="600" 
            @change="onParamsChange"
          />
          <span class="unit">秒</span>
        </div>
      </div>
      
      <div class="param-item" v-if="yoloMode === 'yolo_trigger'">
        <span class="label">置信度阈值</span>
        <div class="param-input">
          <input 
            type="number" 
            v-model.number="confidenceThreshold" 
            min="0.1" 
            max="1.0" 
            step="0.1"
            @change="onParamsChange"
          />
        </div>
      </div>
      
      <div class="param-hint" v-if="yoloMode === 'yolo_trigger'">
        YOLO检测到高置信度目标时自动触发VLM分析，冷却时间30秒
      </div>
      <div class="param-hint" v-else-if="yoloMode === 'manual'">
        仅通过手动截帧按钮触发VLM分析
      </div>
      
      <!-- ★ YOLO 帧率设置 -->
      <div class="param-item">
        <span class="label">YOLO帧率</span>
        <div class="param-input">
          <input 
            type="number" 
            v-model.number="yoloMaxFps" 
            min="1" 
            max="60" 
            step="5"
            @change="onYoloFpsChange"
          />
          <span class="unit">fps</span>
        </div>
      </div>
      <div class="param-hint">
        YOLO检测最大帧率，实际帧率根据节点吞吐量自适应
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'

const props = defineProps<{
  cameraId?: string | null
  vlmActiveCount?: number
  nodeCount?: number
  totalNodeCount?: number
  // ★ 从父组件接收初始值
  initialVlmInterval?: number
  initialYoloMode?: string
  initialConfidenceThreshold?: number
}>()

const emit = defineEmits<{
  (e: 'paramsChange', params: { vlmInterval: number; yoloMode: string; confidenceThreshold: number }): void
  (e: 'capture'): void
}>()

const vlmActiveCount = computed(() => props.vlmActiveCount ?? 0)

// 状态数据
const gatewayStatus = ref<'online' | 'offline'>('offline')
const healthyNodes = computed(() => props.nodeCount ?? 0)
const totalNodes = computed(() => props.totalNodeCount ?? 0)

// 参数设置（从 props 获取初始值，避免重复定义默认值）
const vlmInterval = ref(props.initialVlmInterval ?? 15)
const yoloMode = ref(props.initialYoloMode ?? 'interval')
const confidenceThreshold = ref(props.initialConfidenceThreshold ?? 0.5)

// ★ YOLO 帧率设置（默认 30fps）
const yoloMaxFps = ref(30)

// 加载 YOLO 帧率设置
async function loadYoloFps() {
  try {
    const resp = await fetch('/api/yolo/status')
    if (resp.ok) {
      const data = await resp.json()
      if (data.sync_frame_rate) {
        yoloMaxFps.value = data.sync_frame_rate
      }
    }
  } catch (e) {
    console.warn('[AnalysisStatus] 加载 YOLO 帧率失败:', e)
  }
}

// YOLO 帧率变更处理
async function onYoloFpsChange() {
  try {
    const resp = await fetch('/api/yolo/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sync_frame_rate: yoloMaxFps.value })
    })
    if (!resp.ok) {
      console.error('[AnalysisStatus] 设置 YOLO 帧率失败')
    }
  } catch (e) {
    console.error('[AnalysisStatus] 设置 YOLO 帧率失败:', e)
  }
}

// 同步父组件 props 变化
watch(() => props.initialVlmInterval, (val) => { if (val !== undefined) vlmInterval.value = val })
watch(() => props.initialYoloMode, (val) => { if (val !== undefined) yoloMode.value = val })
watch(() => props.initialConfidenceThreshold, (val) => { if (val !== undefined) confidenceThreshold.value = val })

function onParamsChange() {
  emit('paramsChange', {
    vlmInterval: vlmInterval.value,
    yoloMode: yoloMode.value,
    confidenceThreshold: confidenceThreshold.value
  })
}

// 采样间隔 = 用户配置的 VLM 间隔（多节点并行时按节点数均摊）
const samplingInterval = computed(() => {
  const nodes = Math.max(healthyNodes.value, 1)
  // 多节点pipeline模式下，实际间隔 = vlmInterval / nodes（并行处理）
  return Math.max(Math.round(vlmInterval.value / nodes), 1)
})

// 每小时处理帧数（每路摄像头）
const framesPerHour = computed(() => {
  return Math.round(3600 / samplingInterval.value)
})

async function fetchStatus() {
  try {
    // 获取Gateway状态 (通过 rivision-cli 的 /api/status)
    const statusResp = await fetch('/api/status')
    if (statusResp.ok) {
      const data = await statusResp.json()
      gatewayStatus.value = data.gateway?.status === 'healthy' ? 'online' : 'offline'
    }

    // 节点状态现在由父组件通过 props 传入，不再重复获取
  } catch (e) {
    gatewayStatus.value = 'offline'
  }
}

let statusTimer: number | null = null

onMounted(() => {
  fetchStatus()
  loadYoloFps()  // ★ 加载 YOLO 帧率设置
  statusTimer = window.setInterval(fetchStatus, 5000)
})

onUnmounted(() => {
  if (statusTimer) clearInterval(statusTimer)
})

defineExpose({
  refresh: fetchStatus
})
</script>

<style scoped>
.analysis-status {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border-color, #333);
}

.section-title .icon {
  font-size: 16px;
}

.status-items {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.status-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
}

.status-item .label {
  color: var(--text-secondary, #888);
}

.status-item .value {
  color: var(--text-primary, #e0e0e0);
  font-weight: 500;
}

.status-item .value.highlight {
  color: var(--primary-color, #4a9eff);
  font-weight: 600;
}

.status-item.gateway .value.online {
  color: #4ade80;
}

.status-item.gateway .value.offline {
  color: #f87171;
}

.status-icon {
  margin-right: 4px;
}

.queue-bar {
  position: relative;
  width: 80px;
  height: 18px;
  background: var(--bg-tertiary, #252535);
  border-radius: 4px;
  overflow: hidden;
}

.queue-fill {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: linear-gradient(90deg, var(--primary-color, #4a9eff), #6366f1);
  transition: width 0.3s ease;
}

.queue-text {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  font-size: 11px;
  color: var(--text-primary, #e0e0e0);
  font-weight: 500;
  text-shadow: 0 0 4px rgba(0, 0, 0, 0.5);
}

.manual-capture-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 16px;
  margin-top: 8px;
  background: linear-gradient(135deg, var(--primary-color, #4a9eff), #6366f1);
  border: none;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.manual-capture-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(74, 158, 255, 0.4);
}

.manual-capture-btn:active:not(:disabled) {
  transform: translateY(0);
}

.manual-capture-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-icon {
  font-size: 24px;
}

.btn-text {
  font-size: 14px;
  font-weight: 600;
  color: white;
}

.btn-hint {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.7);
}

/* 参数设置样式 */
.params-title {
  margin-top: 16px;
}

.params-items {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.param-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
}

.param-item .label {
  color: var(--text-secondary, #888);
}

.param-input {
  display: flex;
  align-items: center;
  gap: 4px;
}

.param-input input {
  width: 60px;
  padding: 4px 8px;
  border: 1px solid var(--border-color, #444);
  border-radius: 4px;
  background: var(--bg-tertiary, #252535);
  color: var(--text-primary, #e0e0e0);
  font-size: 12px;
  text-align: right;
}

.param-input input:focus {
  outline: none;
  border-color: var(--primary-color, #4a9eff);
}

.param-input .unit {
  color: var(--text-secondary, #888);
  font-size: 12px;
}

.param-item select {
  padding: 4px 8px;
  border: 1px solid var(--border-color, #444);
  border-radius: 4px;
  background: var(--bg-tertiary, #252535);
  color: var(--text-primary, #e0e0e0);
  font-size: 12px;
  cursor: pointer;
}

.param-item select:focus {
  outline: none;
  border-color: var(--primary-color, #4a9eff);
}

.param-hint {
  font-size: 11px;
  color: var(--text-secondary, #666);
  padding: 4px 0;
  line-height: 1.4;
}
</style>
