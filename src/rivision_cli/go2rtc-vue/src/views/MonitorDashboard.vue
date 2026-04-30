<template>
  <div class="monitor-dashboard">
    <!-- 顶部标题栏 -->
    <header class="dashboard-header">
      <div class="header-left">
        <span class="logo">🎬</span>
        <h1 class="title">RiVision 分布式智能视频分析</h1>
      </div>
      <div class="header-right">
        <!-- 简化导航 -->
        <nav class="nav-menu">
          <button class="nav-item active">📹 监控</button>
          <button class="nav-item" :class="{ 'gateway-active': showGatewayPanel }" @click="toggleGatewayPanel">
            <span class="gateway-status" :class="{ connected: gatewayStore.isConnected }"></span>
            🔗 网关
          </button>
        </nav>
        <!-- 布局选择器 -->
        <div class="layout-selector">
          <button
            v-for="layout in gridLayouts"
            :key="layout.value"
            class="layout-btn"
            :class="{ active: currentLayout === layout.value }"
            @click="currentLayout = layout.value"
            :title="layout.label"
          >
            {{ layout.label }}
          </button>
        </div>
      </div>
    </header>

    <!-- 主内容区 -->
    <div class="dashboard-content">
      <!-- 左侧边栏 -->
      <aside class="sidebar" :class="{ collapsed: sidebarCollapsed }">
        <!-- 折叠按钮 -->
        <button class="sidebar-toggle" @click="toggleSidebar" :title="sidebarCollapsed ? '展开侧边栏' : '折叠侧边栏'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path v-if="sidebarCollapsed" d="m9 18 6-6-6-6"/>
            <path v-else d="m15 18-6-6 6-6"/>
          </svg>
        </button>
        <CameraList
          v-model="selectedCameraId"
          :vlm-focus-id="vlmFocusId"
          :vlm-enabled-cameras="vlmEnabledCameras"
          :yolo-enabled-cameras="yoloEnabledCameras"
          @select="onCameraSelect"
          @update:vlm-focus-id="onVlmFocusChange"
          @update:vlm-enabled-cameras="onVlmEnabledChange"
          @update:yolo-enabled-cameras="onYoloEnabledChange"
          @open-settings="cameraModalVisible = true"
        />

        <div class="sidebar-divider"></div>

        <AnalysisStatus
          v-show="!sidebarCollapsed"
          :camera-id="vlmFocusId"
          :vlm-active-count="vlmEnabledCameras.size"
          :node-count="nodeStatuses.filter((n: any) => n.healthy !== false).length"
          :total-node-count="nodeStatuses.length"
          :initial-vlm-interval="userVlmInterval"
          :initial-yolo-mode="userYoloMode"
          :initial-confidence-threshold="userConfidenceThreshold"
          @capture="onManualCapture"
          @params-change="onParamsChange"
        />
      </aside>

      <!-- 右侧主区域 -->
      <main class="main-area">
        <!-- 视频网格区 -->
        <div class="video-section">
          <VideoGrid
            ref="videoGridRef"
            :layout="currentLayout"
            :cameras="cameras"
            :selected-cameras="selectedCameras"
            :vlm-focus-id="vlmFocusId"
            :vlm-enabled-cameras="vlmEnabledCameras"
            :is-analyzing="isAnalyzing"
            :yolo-configs="yoloConfigs"
            @update:selected-cameras="onSelectedCamerasChange"
            @video-ready="onVideoReady"
            @video-error="onVideoError"
          />
        </div>

        <!-- 底部区域：Tab（VLM结果 / 视频搜索） -->
        <div class="history-section">
          <div class="bottom-tabs">
            <button
              class="tab-btn"
              :class="{ active: bottomActiveTab === 'vlm' }"
              @click="bottomActiveTab = 'vlm'"
            >
              VLM结果
            </button>
            <button
              class="tab-btn"
              :class="{ active: bottomActiveTab === 'video-search' }"
              @click="bottomActiveTab = 'video-search'"
            >
              视频搜索
            </button>
          </div>

          <div class="tab-content">
            <VlmHistory
              v-show="bottomActiveTab === 'vlm'"
              :items="vlmHistoryItems"
              :camera-id="vlmFocusId"
              :camera-name="vlmFocusCameraName"
              :available-cameras="displayedCameraOptions"
              @view-detail="showDetailModal"
              @item-click="showDetailModal"
              @refresh="fetchHistory"
            />

            <VideoSearchTab
              v-show="bottomActiveTab === 'video-search'"
              :history-items="vlmHistoryItems"
              :available-cameras="displayedCameraOptions"
              @view-detail="showDetailModal"
            />
          </div>
        </div>
      </main>
    </div>

    <CameraManagementModal
      v-model:visible="cameraModalVisible"
      @updated="onCameraManaged"
    />

    <!-- 详情弹窗 -->
    <DetailModal
      v-model="detailModalVisible"
      :data="detailModalData"
      :has-prev="hasNextDetail"
      :has-next="hasPrevDetail"
      @prev="navigateDetail(1)"
      @next="navigateDetail(-1)"
    />

    <!-- 回放弹窗 -->
    <PlaybackModal
      v-model:visible="playbackModalVisible"
      :video-src="playbackVideoSrc"
      :start-time="playbackStartTime"
      :markers="playbackMarkers"
    />

    <!-- 网关面板 -->
    <GatewayPanel
      :visible="showGatewayPanel"
      @close="showGatewayPanel = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, provide } from 'vue'
import CameraList, { type Camera } from '../components/CameraList.vue'
import AnalysisStatus from '../components/AnalysisStatus.vue'
import VideoGrid, { type GridLayout } from '../components/VideoGrid.vue'
import VlmHistory, { type VlmHistoryItem } from '../components/VlmHistory.vue'
import VideoSearchTab from '../components/VideoSearchTab.vue'
import CameraManagementModal from '../components/CameraManagementModal.vue'
import DetailModal, { type DetailData } from '../components/DetailModal.vue'
import PlaybackModal, { type PlaybackMarker } from '../components/PlaybackModal.vue'
import GatewayPanel from '../components/GatewayPanel.vue'
import { getAutoTriggerController } from '../services/autoTriggerController'
import { useGatewayStore } from '../stores/gateway'
import { yoloApi } from '../services/yoloApi'
import { yoloWebSocket } from '../services/yoloWebSocket'
import { playChannel, getOWLStreams } from '../services/owlStreams'
import type { YoloConfig } from '../components/VideoCell.vue'

// Gateway store
const gatewayStore = useGatewayStore()

// ★ B6: 复用帧捕获Canvas（避免每次新建~3.7MB像素缓冲导致OOM）
const _vlmCaptureCanvas = document.createElement('canvas')
const _vlmCaptureCtx = _vlmCaptureCanvas.getContext('2d')!

// 网关面板显示状态
const showGatewayPanel = ref(false)
const cameraModalVisible = ref(false)

function toggleGatewayPanel() {
  showGatewayPanel.value = !showGatewayPanel.value
}

function openGatewayPanel() {
  showGatewayPanel.value = true
}

function openCameraManagement() {
  cameraModalVisible.value = true
}

function focusVlmTab() {
  bottomActiveTab.value = 'vlm'
}

function focusVideoSearchTab() {
  bottomActiveTab.value = 'video-search'
}

// ============ 布局配置 ============
const gridLayouts = [
  { value: '1x1' as GridLayout, label: '1×1' },
  { value: '2x2' as GridLayout, label: '2×2' },
  { value: '3x3' as GridLayout, label: '3×3' },
  { value: '4x4' as GridLayout, label: '4×4' },
]

// ============ 状态 ============
const currentLayout = ref<GridLayout>('2x2')
const selectedCameraId = ref<string>('')
const vlmFocusId = ref<string | null>(null)
const selectedCameras = ref<(string | null)[]>([null, null, null, null])
const cameras = ref<Camera[]>([])
const isAnalyzing = ref(false)

// 多摄像头 VLM/YOLO 开关状态
const vlmEnabledCameras = ref<Set<string>>(new Set())
const yoloEnabledCameras = ref<Set<string>>(new Set())

// ============ 不使用 localStorage，所有状态每次加载从后端获取 ============

function getGridSlotCount(layout: GridLayout): number {
  const [rows, cols] = layout.split('x').map(Number)
  return rows * cols
}


// 侧边栏折叠状态（不持久化，每次默认展开）
const sidebarCollapsed = ref(false)

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

// YOLO 配置 - 根据 yoloEnabledCameras 决定是否启用
const yoloConfigs = computed<Record<string, YoloConfig>>(() => {
  const configs: Record<string, YoloConfig> = {}
  cameras.value.forEach((camera: Camera) => {
    configs[camera.id] = {
      model: 'object',
      confidence: 0.5,
      enabled: yoloEnabledCameras.value.has(camera.id),
    }
  })
  return configs
})

// VLM 历史记录
const vlmHistoryItemsMap = ref<Record<string, VlmHistoryItem[]>>({})
const vlmHistoryItems = computed(() => {
  // 收集所有摄像头的历史记录（下拉菜单通过 VlmHistory 组件内部过滤）
  const allItems: VlmHistoryItem[] = []
  for (const items of Object.values(vlmHistoryItemsMap.value)) {
    allItems.push(...items)
  }
  return allItems.sort((a, b) => b.timestamp - a.timestamp).slice(0, 100)
})

// 摄像头活跃任务数
const cameraActiveTasks = ref<Record<string, number>>({})

// 节点状态
const nodeStatuses = ref<any[]>([])

// 头部统一状态条
const owlHeaderStatus = ref<{ ok: boolean; count: number; message: string }>({ ok: false, count: 0, message: '' })
const semanticHeaderStatus = ref<{ ok: boolean; total: number }>({ ok: false, total: 0 })

// 弹窗状态
const detailModalVisible = ref(false)
const detailModalData = ref<DetailData | undefined>(undefined)
const currentDetailIndex = ref(-1)

const playbackModalVisible = ref(false)
const playbackStartTime = ref<number>(0)
const playbackVideoSrc = ref<string>('')
const playbackMarkers = ref<PlaybackMarker[]>([])

// 底部 Tab（VLM结果 / 视频搜索）
const bottomActiveTab = ref<'vlm' | 'video-search'>('vlm')

// Refs
const videoGridRef = ref<InstanceType<typeof VideoGrid> | null>(null)

// ============ 计算属性 ============
const vlmFocusCameraName = computed(() => {
  if (!vlmFocusId.value) return ''
  const camera = cameras.value.find((c: Camera) => c.id === vlmFocusId.value)
  return camera?.name ?? vlmFocusId.value
})

// 当前显示在视频网格中的摄像头ID列表（去掉null）
const displayedCameraIds = computed(() => {
  return selectedCameras.value.filter((c): c is string => c !== null)
})

// ★ 摄像头选项列表（包含ID和名称）用于VlmHistory下拉菜单
const displayedCameraOptions = computed(() => {
  return displayedCameraIds.value.map(id => {
    const cam = cameras.value.find((c: Camera) => c.id === id)
    return { id, name: cam?.name || id }
  })
})

const hasPrevDetail = computed(() => currentDetailIndex.value > 0)
const hasNextDetail = computed(() => currentDetailIndex.value < vlmHistoryItems.value.length - 1)
const healthyNodeCount = computed(() => nodeStatuses.value.filter((n: any) => n.healthy !== false).length)

// ============ 自动触发控制器 ============
const autoTrigger = getAutoTriggerController({
  enabled: false,  // 默认关闭，用户手动开启VLM后才启用
  motionDetectionEnabled: false,
  maxIdleInterval: 60000,
  minAnalysisInterval: 1000,
  maxQueueSize: 10,
  pipelineMode: true,
  nodeCount: 1,
})

// ============ 摄像头选择 ============
async function onCameraSelect(camera: Camera) {
  selectedCameraId.value = camera.id

  // ★ OWL 集成：播放前先通过 rivision-cli 触发 OWL.Play()
  // 这样 rivision-cli 会：1) 调用 OWL API 2) 自动注册流到 go2rtc
  // 前端通过 go2rtc stream.mp4 播放，零感知接入
  try {
    const played = await playChannel(camera.id)
    if (!played) {
      // 非 OWL / play 不可用时，继续使用 go2rtc 原生流
      console.debug('[Monitor] playChannel returned null, fallback to go2rtc stream')
    }
  } catch (e) {
    // playChannel 在 OWL 未配置时会自动回退，不影响正常播放
    console.debug('[Monitor] playChannel skipped (OWL not configured):', e)
  }
}

function onVlmFocusChange(cameraId: string) {
  console.log('[VLM] Focus changed to:', cameraId)
  vlmFocusId.value = cameraId
  
  // 重新绑定 autoTrigger 到新的视频元素
  rebindAutoTrigger()
}

function onSelectedCamerasChange(newCameras: (string | null)[]) {
  selectedCameras.value = newCameras
  
  // 如果 VLM 焦点摄像头被移除，清除焦点
  if (vlmFocusId.value && !newCameras.includes(vlmFocusId.value)) {
    vlmFocusId.value = null
    autoTrigger.stop()
  }
}

// ============ 视频事件 ============
function onVideoReady(index: number, video: HTMLVideoElement) {
  console.log(`[Video] Cell ${index} ready, readyState=${video.readyState}, vlmFocus=${vlmFocusId.value}`)

  // 如果是 VLM 焦点摄像头，绑定 autoTrigger
  const cameraId = selectedCameras.value[index]
  console.log(`[Video] Cell ${index} camera=${cameraId}, isVlmFocus=${cameraId === vlmFocusId.value}`)
  if (cameraId === vlmFocusId.value) {
    console.log('[Video] Binding autoTrigger to VLM focus camera')
    autoTrigger.bindVideo(video)
    autoTrigger.start()

    // 延迟重试：确保视频有足够数据后再次尝试填充管道
    setTimeout(() => {
      console.log('[Video] Retry pipeline fill after 3s')
      autoTrigger.manualTrigger?.()
    }, 3000)
  }
}

function onVideoError(index: number, error: string) {
  console.error(`[Video] Cell ${index} error:`, error)
  // ★ G3: 视频流错误时，标记该摄像头需要跳过，等待恢复
  const cameraId = selectedCameras.value[index]
  if (cameraId && vlmEnabledCameras.value.has(cameraId)) {
    // 设置较长的重试间隔，避免频繁失败
    multiVlmLastAnalysis.value[cameraId] = Date.now() + 30000
    console.warn(`[Video] ${cameraId}: 流错误，30秒后重试VLM`)
  }
}

// 用户可配置的VLM参数
const userVlmInterval = ref(15) // 秒，用户通过UI配置
const userYoloMode = ref('interval') // 'interval' | 'yolo_trigger' | 'manual'
const userConfidenceThreshold = ref(0.5)

// ★ YOLO→VLM 触发状态
const yoloTriggerCooldown = ref<Record<string, number>>({})
const YOLO_VLM_COOLDOWN_MS = 30000 // YOLO触发VLM的冷却时间(30秒)
let removeYoloGlobalListener: (() => void) | null = null

// 参数变更处理
function onParamsChange(params: { vlmInterval: number; yoloMode: string; confidenceThreshold: number }) {
  // interval最小值保护：K3节点处理约4-6秒/帧，最小间隔 = ceil(6/节点数)
  const healthyCount = Math.max(nodeStatuses.value.filter((n: any) => n.healthy !== false).length, 1)
  const minInterval = Math.max(10, Math.ceil(6 / healthyCount))
  const validatedInterval = Math.max(params.vlmInterval, minInterval)
  
  if (validatedInterval !== params.vlmInterval) {
    console.warn(`[VLM] 间隔 ${params.vlmInterval}s 低于最小值 ${minInterval}s (${healthyCount}节点)，已调整为 ${validatedInterval}s`)
  }
  
  const oldInterval = userVlmInterval.value
  userVlmInterval.value = validatedInterval
  userYoloMode.value = params.yoloMode
  userConfidenceThreshold.value = params.confidenceThreshold
  
  console.log('[VLM] Params changed:', { ...params, vlmInterval: validatedInterval })
  
  // ★ 根据触发模式切换定时/YOLO触发
  if (vlmEnabledCameras.value.size > 0) {
    switchVlmTriggerMode(params.yoloMode, validatedInterval !== oldInterval)
  }
}

// 多摄像头VLM定时分析
let multiVlmTimer: number | null = null
const multiVlmLastAnalysis = ref<Record<string, number>>({})

// ✅ 修复：跟踪MultiVLM当前正在进行的请求数
let _multiVlmActiveCount = 0
// ★ Q3-4修复：轮询索引，确保每个周期从不同摄像头开始，公平分配槽位
let _multiVlmRoundRobinIdx = 0
// ★ 优化：每摄像头当前 inflight 任务数（支持单视频利用多节点）
const _cameraInflight: Record<string, number> = {}

// ★ 核心优化：尝试填充VLM槽位（去除时间间隔限制，最大化节点利用率）
function tryFillVlmSlots() {
  if (!videoGridRef.value) return
  
  const displayCameras = videoGridRef.value.getDisplayCameras?.() || []
  
  // ★ 使用后端实际任务数来限制提交
  const backendRunning = gatewayStore.currentTasks.length
  const llamaCount = gatewayStore.llamaHealthyNodeCount || 1
  const maxConcurrent = llamaCount
  
  if (backendRunning >= maxConcurrent) {
    return // 槽位已满
  }
  
  // ★ 轮询公平分配：跳过焦点摄像头
  const candidates = [...vlmEnabledCameras.value].filter(c => {
    if (c === vlmFocusId.value) return false
    if (!displayCameras.includes(c)) return false
    return true
  })
  
  if (candidates.length === 0) return
  
  // ★ 优化：计算每摄像头可分配的槽位数
  // 视频数 >= 节点数：每摄像头最多1个任务（公平分配）
  // 视频数 < 节点数：允许同一摄像头利用多个节点
  const videoCount = candidates.length
  const maxPerCamera = Math.ceil(maxConcurrent / Math.max(videoCount, 1))
  
  const startIdx = _multiVlmRoundRobinIdx % candidates.length
  _multiVlmRoundRobinIdx++
  
  const availableSlots = maxConcurrent - backendRunning
  let submitted = 0
  
  // ★ 多轮遍历：当视频数<节点数时，同一摄像头可以提交多次
  const rounds = Math.ceil(availableSlots / Math.max(videoCount, 1))
  
  for (let round = 0; round < rounds && submitted < availableSlots; round++) {
    for (let i = 0; i < candidates.length && submitted < availableSlots; i++) {
      const cameraId = candidates[(startIdx + i) % candidates.length]
      
      // ★ 检查该摄像头当前 inflight 数是否已达上限
      const inflight = _cameraInflight[cameraId] || 0
      if (inflight >= maxPerCamera) continue
      
      // ★ 优化：去除时间间隔限制，只要有空槽位就立即提交
      // 这样可以最大化节点利用率
      
      // ★ 递增 inflight 计数
      _cameraInflight[cameraId] = inflight + 1
      submitted++
      
      // ★ 并发提交，完成后立即尝试填充新槽位
      submitCameraVlmAnalysis(cameraId).catch(e => {
        console.error(`[MultiVLM] Error analyzing ${cameraId}:`, e)
      }).finally(() => {
        // ★ 完成时递减 inflight 并立即尝试填充新槽位
        _cameraInflight[cameraId] = Math.max(0, (_cameraInflight[cameraId] || 1) - 1)
        // ★ 核心优化：任务完成后立即尝试填充，不等待轮询周期
        // 30ms延迟：平衡响应速度和状态同步，比100ms节省70ms/任务
        setTimeout(() => tryFillVlmSlots(), 30)
      })
    }
  }
  
  if (submitted > 0) {
    console.debug(`[MultiVLM] 提交 ${submitted} 个任务, 视频=${videoCount}, 节点=${llamaCount}, 每视频上限=${maxPerCamera}`)
  }
}

function startMultiCameraVlmAnalysis() {
  if (multiVlmTimer) return
  
  // ★ 优化：缩短轮询间隔到1秒，配合任务完成回调实现快速填充
  multiVlmTimer = window.setInterval(() => {
    tryFillVlmSlots()
  }, 1000) // ★ 优化：1秒轮询，配合任务完成回调实现快速响应
}

// 单个摄像头VLM分析（独立async，支持多摄像头并发到多节点）
async function submitCameraVlmAnalysis(cameraId: string) {
  const video = videoGridRef.value?.getVideoElementByCameraId?.(cameraId)
  if (!video) {
    console.warn(`[MultiVLM] ${cameraId}: 视频元素不存在`)
    return
  }
  if (video.readyState < 2) {
    console.warn(`[MultiVLM] ${cameraId}: 视频未就绪 readyState=${video.readyState}`)
    return
  }
  
  const vw = video.videoWidth
  const vh = video.videoHeight
  if (!vw || !vh || vw < 100 || vh < 100) {
    console.warn(`[MultiVLM] ${cameraId}: 视频尺寸异常 ${vw}x${vh}`)
    return
  }
  
  // ★ 大图缩放：限制最大维度 800px，减少VLM推理时间和传输量
  // ✅ 优化：1280→800px。VLM场景理解不需要高分辨率：
  //   - MiniCPM-V 自适应切片: 1280px可能触发多slice(K3每slice~1.6-2.2s)
  //   - 800px单slice足够识别人物/车辆/活动，且payload减小~40%
  //   - road_v1 (1920x1012): 1280x675→800x422, 推理时间预估减少30-50%
  const MAX_DIM = 800
  let cw = vw, ch = vh
  if (vw > MAX_DIM || vh > MAX_DIM) {
    const scale = MAX_DIM / Math.max(vw, vh)
    cw = Math.round(vw * scale)
    ch = Math.round(vh * scale)
  }
  // ★ B6: 复用Canvas（避免每次新建~3.7MB像素缓冲）
  if (_vlmCaptureCanvas.width !== cw || _vlmCaptureCanvas.height !== ch) {
    _vlmCaptureCanvas.width = cw
    _vlmCaptureCanvas.height = ch
  }
  _vlmCaptureCtx.drawImage(video, 0, 0, cw, ch)
  
  const imageData = _vlmCaptureCanvas.toDataURL('image/jpeg', 0.75).replace(/^data:image\/\w+;base64,/, '')
  console.debug(`[MultiVLM] ${cameraId}: 帧捕获成功 ${vw}x${vh}→${cw}x${ch}, size=${Math.round(imageData.length * 0.75 / 1024)}KB`)
  
  // ★ 异步模式：Gateway立即返回202，推理在后台执行，结果通过WebSocket推送
  // 解决浏览器6连接限制：4 MSE + 1 YOLO WS + VLM fetch(180s) = 6 → YOLO被堵
  // 异步模式下 fetch <1s 完成 → 连接立即释放 → YOLO不受影响
  // ★ K3推理需要10-20s，设置60s超时避免context canceled
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 60000)  // 60s for K3 VLM inference
  let resp: Response
  try {
    resp = await fetch('/api/v1/inference/analyze?async=true', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: controller.signal,
      body: JSON.stringify({
        image: imageData,
        prompt: '请描述这个监控画面中的内容，包括人物、车辆、活动和异常情况',
        camera_id: cameraId,
        stream_name: cameraId,
        task_type: 'vlm',
        task_id: `cam_${cameraId}_${Date.now().toString(36)}`,
      }),
    })
  } catch (e: any) {
    console.warn(`[MultiVLM] ${cameraId}: 推理请求失败 (${e.name === 'AbortError' ? '60s超时' : e.message})`)
    // 失败时适度重试（60s 后）
    multiVlmLastAnalysis.value[cameraId] = Date.now() - (userVlmInterval.value * 1000) + 60000
    return
  } finally {
    clearTimeout(timeout)
  }
  
  if (!resp.ok && resp.status !== 202) {
    console.warn(`[MultiVLM] ${cameraId}: 推理返回 HTTP ${resp.status}`)
    // ★ Q3-4修复：503重试延迟从30s降至10s，减少饿死窗口
    multiVlmLastAnalysis.value[cameraId] = Date.now() - (userVlmInterval.value * 1000) + 10000
    return
  }
  
  // ★ 异步模式：Gateway返回202 accepted，推理结果通过WebSocket推送
  const result = await resp.json()
  console.debug(`[MultiVLM] ${cameraId}: 请求已提交 (${resp.status}), task=${result.task_id || '?'}`)
}

function stopMultiCameraVlmAnalysis() {
  if (multiVlmTimer) {
    clearInterval(multiVlmTimer)
    multiVlmTimer = null
  }
}

// ★ YOLO→VLM 触发模式：当YOLO检测到高置信度目标时自动触发VLM分析
function startYoloTriggeredVlm() {
  if (removeYoloGlobalListener) return // 已启动
  
  console.log('[YOLO→VLM] 启动YOLO触发VLM模式, 置信度阈值:', userConfidenceThreshold.value)
  
  removeYoloGlobalListener = yoloWebSocket.addGlobalListener((cameraId, detections, inferenceTimeMs) => {
    // 只对已启用VLM的摄像头触发
    if (!vlmEnabledCameras.value.has(cameraId)) return
    if (userYoloMode.value !== 'yolo_trigger') return
    
    // 检查是否有检测结果超过置信度阈值
    const highConfidence = detections.some(d => d.confidence >= userConfidenceThreshold.value)
    if (!highConfidence) return
    
    // 冷却检查：避免频繁触发
    const lastTrigger = yoloTriggerCooldown.value[cameraId] || 0
    const now = Date.now()
    if (now - lastTrigger < YOLO_VLM_COOLDOWN_MS) return
    
    // 触发VLM分析
    yoloTriggerCooldown.value[cameraId] = now
    const detCount = detections.filter(d => d.confidence >= userConfidenceThreshold.value).length
    console.log(`[YOLO→VLM] ${cameraId}: 检测到${detCount}个高置信度目标, 触发VLM分析`)
    
    submitCameraVlmAnalysis(cameraId).catch(e => {
      console.error(`[YOLO→VLM] Error analyzing ${cameraId}:`, e)
    })
  })
}

function stopYoloTriggeredVlm() {
  if (removeYoloGlobalListener) {
    removeYoloGlobalListener()
    removeYoloGlobalListener = null
    console.log('[YOLO→VLM] 已停止YOLO触发VLM模式')
  }
}

// ★ 切换VLM触发模式
function switchVlmTriggerMode(mode: string, intervalChanged: boolean = false) {
  switch (mode) {
    case 'yolo_trigger':
      stopMultiCameraVlmAnalysis()
      startYoloTriggeredVlm()
      break
    case 'manual':
      stopMultiCameraVlmAnalysis()
      stopYoloTriggeredVlm()
      break
    case 'interval':
    default:
      stopYoloTriggeredVlm()
      if (intervalChanged || !multiVlmTimer) {
        stopMultiCameraVlmAnalysis()
        startMultiCameraVlmAnalysis()
      }
      break
  }
}

// 多摄像头 VLM/YOLO 开关回调
function onVlmEnabledChange(cameras: Set<string>) {
  const oldCameras = vlmEnabledCameras.value
  vlmEnabledCameras.value = cameras
  console.log('[VLM] Enabled cameras:', Array.from(cameras))
  
  // 找到新增的摄像头（刚被启用的）
  const newlyEnabled = Array.from(cameras).find(c => !oldCameras.has(c))
  
  // 如果有新启用的摄像头，切换VLM焦点到该摄像头
  if (newlyEnabled) {
    console.log('[VLM] Switching focus to newly enabled camera:', newlyEnabled)
    vlmFocusId.value = newlyEnabled
    rebindAutoTrigger()
  }
  
  // 如果当前焦点摄像头被禁用，切换到其他已启用的摄像头
  if (vlmFocusId.value && !cameras.has(vlmFocusId.value)) {
    const nextCamera = Array.from(cameras)[0] || null
    vlmFocusId.value = nextCamera
    if (nextCamera) {
      rebindAutoTrigger()
    } else {
      autoTrigger.stop()
    }
  }
  
  // 启动或停止VLM分析（根据当前触发模式）
  if (cameras.size > 0) {
    switchVlmTriggerMode(userYoloMode.value)
  } else {
    stopMultiCameraVlmAnalysis()
    stopYoloTriggeredVlm()
  }
}

function onYoloEnabledChange(enabledSet: Set<string>) {
  yoloEnabledCameras.value = enabledSet
  // YOLO 状态由后端持久化（CameraList 的 toggle 已调用 yoloApi.enable/disable）
  console.log('[YOLO] Enabled cameras:', Array.from(enabledSet))
}

// 从后端同步 YOLO 启用状态（页面加载时恢复状态）
async function fetchYoloStatus() {
  try {
    const status = await yoloApi.getStatus()
    if (status.enabled_cameras && status.enabled_cameras.length > 0) {
      yoloEnabledCameras.value = new Set(status.enabled_cameras)
      console.log('[YOLO] Synced enabled cameras from backend:', status.enabled_cameras)
    }
  } catch (e) {
    console.warn('[YOLO] Failed to fetch YOLO status:', e)
  }
}

function rebindAutoTrigger() {
  if (!vlmFocusId.value || !videoGridRef.value) {
    console.warn('[VLM] rebindAutoTrigger: no vlmFocusId or videoGridRef')
    autoTrigger.stop()
    return
  }
  
  const video = videoGridRef.value.getVlmFocusVideoElement()
  if (video) {
    console.log(`[VLM] Rebinding autoTrigger: focus=${vlmFocusId.value}, video=${video.videoWidth}x${video.videoHeight}, readyState=${video.readyState}, src=${video.src?.slice(0,50)}`)
    autoTrigger.bindVideo(video)
    autoTrigger.resetPipeline()
    autoTrigger.start()
  } else {
    console.warn(`[VLM] rebindAutoTrigger: video element not found for ${vlmFocusId.value}`)
  }
}

// ============ 手动截帧 ============
async function onManualCapture() {
  if (!vlmFocusId.value || !videoGridRef.value) return

  const video = videoGridRef.value.getVlmFocusVideoElement()
  if (!video) return

  try {
    // ★ B6: 复用Canvas
    const mcw = video.videoWidth || 1280
    const mch = video.videoHeight || 720
    if (_vlmCaptureCanvas.width !== mcw || _vlmCaptureCanvas.height !== mch) {
      _vlmCaptureCanvas.width = mcw
      _vlmCaptureCanvas.height = mch
    }
    _vlmCaptureCtx.drawImage(video, 0, 0)
    
    const imageData = _vlmCaptureCanvas.toDataURL('image/jpeg', 0.85)
      .replace(/^data:image\/\w+;base64,/, '')

    // ★ 异步模式：立即返回202，结果通过WebSocket推送
    // ★ K3推理需要10-20s，设置60s超时避免context canceled
    const vlmController = new AbortController()
    const vlmTimeout = setTimeout(() => vlmController.abort(), 60000)
    let resp: Response
    try {
      resp = await fetch('/api/v1/inference/analyze?async=true', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        signal: vlmController.signal,
        body: JSON.stringify({
          image: imageData,
          prompt: '请描述这个监控画面中的内容，包括人物、车辆、活动和异常情况',
          camera_id: vlmFocusId.value,
          stream_name: vlmFocusId.value,
          trigger_type: 'manual',
        }),
      })
    } finally {
      clearTimeout(vlmTimeout)
    }

    if (!resp.ok && resp.status !== 202) {
      throw new Error(`HTTP ${resp.status}`)
    }

    // ★ 异步模式：推理结果通过WebSocket推送
    const data = await resp.json()
    console.log(`[Manual] 请求已提交 (${resp.status}), task=${data.task_id || '?'}`)
  } catch (e) {
    console.error('Manual capture failed:', e)
  }
}

// ============ 详情弹窗 ============
function getCameraNameById(cameraId: string): string {
  const cam = displayedCameraOptions.value.find(c => c.id === cameraId)
  return cam?.name || cameraId
}

function showDetailModal(item: VlmHistoryItem) {
  const index = vlmHistoryItems.value.findIndex((t: VlmHistoryItem) => t.id === item.id)
  currentDetailIndex.value = index >= 0 ? index : 0
  
  detailModalData.value = {
    id: item.id,
    timestamp: item.timestamp,
    cameraId: item.cameraId,
    cameraName: getCameraNameById(item.cameraId),
    imageSrc: item.thumbnail,
    result: item.result,
    nodeId: item.nodeId,
    processingTime: item.processingTime,
    resolution: item.resolution,
  }
  detailModalVisible.value = true
}

function navigateDetail(direction: number) {
  const newIndex = currentDetailIndex.value + direction
  if (newIndex >= 0 && newIndex < vlmHistoryItems.value.length) {
    currentDetailIndex.value = newIndex
    const item = vlmHistoryItems.value[newIndex]
    detailModalData.value = {
      id: item.id,
      timestamp: item.timestamp,
      cameraId: item.cameraId,
      cameraName: getCameraNameById(item.cameraId),
      imageSrc: item.thumbnail,
      result: item.result,
      nodeId: item.nodeId,
      processingTime: item.processingTime,
      resolution: item.resolution,
    }
  }
}

// ============ 数据获取 ============
function onCameraManaged() {
  fetchCameras()
}

async function fetchCameras() {
  try {
    // 与 CameraList 一致：优先 OWL /api/owl/streams，否则 go2rtc /api/go2rtc/streams
    const streams = await getOWLStreams()
    cameras.value = streams.map((s) => ({
      id: s.id,
      name: s.name,
      // 对本地 go2rtc 流（proxy:// 或未知）按可用处理，避免误判为离线导致无法选择
      location: s.online ? '在线' : (s.source?.startsWith('proxy://') ? '可用' : '离线'),
      online: s.online,
      isActive: s.online || (s.source?.startsWith('proxy://') ?? false),
    }))
    console.debug('[fetchCameras] loaded:', cameras.value.map((c: Camera) => c.id))
  } catch (e) {
    console.error('Failed to fetch cameras:', e)
  }
}

async function fetchHistory() {
  try {
    const resp = await fetch(`/api/v1/inference/history?limit=100`)
    if (resp.ok) {
      const data = await resp.json()
      const serverItems: VlmHistoryItem[] = (data.items || []).map((item: any) => {
        const camId = item.camera_id || item.stream_name
        // 从 cameras 列表查找友好名称
        const cam = cameras.value.find((c: Camera) => c.id === camId)
        return {
          id: item.task_id,
          timestamp: item.timestamp,
          cameraId: camId,
          cameraName: cam?.name || camId,
          thumbnail: item.thumbnail ? (item.thumbnail.startsWith('data:') ? item.thumbnail : `data:image/jpeg;base64,${item.thumbnail}`) : undefined,
          result: item.result || '',
          nodeId: item.node_id,
          processingTime: item.processing_time,
          resolution: item.resolution,
        }
      })
      
      // 按摄像头分组
      const groupedByCamera: Record<string, VlmHistoryItem[]> = {}
      for (const item of serverItems) {
        if (!item.cameraId) continue
        if (!groupedByCamera[item.cameraId]) groupedByCamera[item.cameraId] = []
        groupedByCamera[item.cameraId].push(item)
      }
      
      // 合并：保留本地已有记录的thumbnail
      const newMap = { ...vlmHistoryItemsMap.value }
      for (const [camId, serverCamItems] of Object.entries(groupedByCamera)) {
        const existingItems = newMap[camId] || []
        const existingMap = new Map(existingItems.map(item => [item.id, item]))
        
        const mergedItems = serverCamItems.map(serverItem => {
          const existing = existingMap.get(serverItem.id)
          if (existing?.thumbnail) {
            return { ...serverItem, thumbnail: existing.thumbnail }
          }
          return serverItem
        })
        
        const serverIds = new Set(serverCamItems.map(item => item.id))
        const localOnlyItems = existingItems.filter(item => !serverIds.has(item.id))
        newMap[camId] = [...localOnlyItems, ...mergedItems].slice(0, 50)
      }
      vlmHistoryItemsMap.value = newMap

      // 同步写入本地 SQLite 语义索引（失败不影响页面）
      fetch('/api/vlm/search/reindex', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          items: serverItems.map((it) => ({
            task_id: it.id,
            timestamp: Math.floor(it.timestamp / 1000),
            camera_id: it.cameraId,
            stream_name: it.cameraName || it.cameraId,
            thumbnail: it.thumbnail || '',
            result: it.result || '',
            node_id: it.nodeId || '',
            processing_time: it.processingTime || 0,
            resolution: it.resolution || '',
          })),
        }),
      }).catch((err) => {
        console.warn('[semantic] reindex failed:', err)
      })
    }
  } catch (e) {
    console.error('Failed to fetch history:', e)
  }
}

// ★ P2修复：跟踪上一次健康节点数，检测节点恢复时主动重启VLM pipeline
let _lastHealthyCount = 0

async function fetchHeaderStatus() {
  // OWL：连接状态与通道数量分开探测，避免 /owl/streams 回退数据导致“假连接”
  try {
    const infoResp = await fetch('/api/owl/server/info')
    if (!infoResp.ok) {
      owlHeaderStatus.value = { ok: false, count: 0, message: `HTTP ${infoResp.status}` }
    } else {
      let count = 0
      let msg = ''
      try {
        const chResp = await fetch('/api/owl/channels?page=1&page_size=1')
        if (chResp.ok) {
          const chData = await chResp.json()
          count = Number(chData?.total || chData?.data?.total || 0)
        } else {
          // channels 不可用不等于 OWL 连接失败，只标记降级
          msg = `channels HTTP ${chResp.status}`
        }
      } catch (e: any) {
        msg = e?.message || 'channels unavailable'
      }
      owlHeaderStatus.value = { ok: true, count, message: msg }
    }
  } catch (e: any) {
    owlHeaderStatus.value = { ok: false, count: 0, message: e?.message || '连接失败' }
  }

  // Semantic index
  try {
    const resp = await fetch('/api/vlm/search/stats')
    if (resp.ok) {
      const data = await resp.json()
      semanticHeaderStatus.value = { ok: true, total: data?.total_indexed ?? 0 }
    } else {
      semanticHeaderStatus.value = { ok: false, total: 0 }
    }
  } catch {
    semanticHeaderStatus.value = { ok: false, total: 0 }
  }
}

async function fetchNodeStatus() {
  try {
    const resp = await fetch('/api/v1/nodes')
    if (resp.ok) {
      const data = await resp.json()
      const nodes = data.nodes || []
      nodeStatuses.value = nodes
      
      const healthyCount = nodes.filter((n: any) => n.healthy !== false).length
      if (healthyCount > 0) {
        // ★ Q3-4修复：MultiVLM有非焦点摄像头时，AutoTrigger pipeline降至1
        // 原因：后端 _async_task_max=4，pipeline=2占满后 MultiVLM的
        // 3路摄像头(road_v1/persion_v1/test_u2026) 至少1路503 → 30s重试延迟 → 饿死
        // 修复：pipeline=1，剩余3槽位给MultiVLM，确保所有摄像头公平分配
        const nonFocusCameras = [...vlmEnabledCameras.value].filter(c => c !== vlmFocusId.value)
        const pipelineDepth = nonFocusCameras.length > 0 ? 1 : healthyCount
        autoTrigger.updateConfig({ 
          nodeCount: pipelineDepth,
          maxQueueSize: healthyCount * 2,
          pipelineMode: true,
        })
        
        // ★ P2修复：节点恢复时主动清除503退避并重置pipeline
        // 场景：node2挂掉→VLM全503→前端退避→node2恢复→但VLM不自动恢复
        // 修复：检测到健康节点数增加时，清除退避状态，重启pipeline
        if (healthyCount > _lastHealthyCount && _lastHealthyCount >= 0) {
          console.log(`[NodeRecovery] 健康节点数恢复: ${_lastHealthyCount} → ${healthyCount}, 重置VLM pipeline`)
          autoTrigger.resetPipeline()
          // 重置MultiVLM的冷却时间戳，允许立即重试
          for (const camId of vlmEnabledCameras.value) {
            multiVlmLastAnalysis.value[camId] = 0
          }
        }
      }
      _lastHealthyCount = healthyCount
    }
  } catch (e) {
    console.error('Failed to fetch node status:', e)
  }
}

// ============ WebSocket ============
let ws: WebSocket | null = null
let refreshTimer: number | null = null

function connectWebSocket() {
  // 连接到Gateway的WebSocket端点（获取带thumbnail的VLM结果推送）
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const gatewayHost = `${window.location.hostname}:8081`
  const wsUrl = `${protocol}//${gatewayHost}/api/v1/ws/live`
  
  console.log(`[VLM WS] Connecting to Gateway WebSocket: ${wsUrl}`)
  ws = new WebSocket(wsUrl)
  
  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data)
      const msgType = msg.type || msg.channel
      
      // 处理任务完成消息
      if ((msgType === 'tasks' && msg.data?.event === 'completed') || msgType === 'inference_result') {
        const taskData = msg.data
        const resultCameraId = taskData.camera_id || taskData.stream_name
        
        if (!resultCameraId) return
        
        // 仅当该摄像头已启用VLM时才接收结果
        if (!vlmEnabledCameras.value.has(resultCameraId)) return
        
        // 更新分析状态
        if (resultCameraId === vlmFocusId.value) {
          isAnalyzing.value = false
        }
        
        autoTrigger.markAnalysisComplete(resultCameraId, vlmFocusId.value || '')
        
        // ★ 修复：如果是错误消息，只更新状态，不添加到历史记录
        if (taskData.error) {
          console.warn(`[VLM] task=${taskData.task_id} 失败: ${taskData.error}`)
          // 更新任务计数（失败也要释放槽位）
          cameraActiveTasks.value = {
            ...cameraActiveTasks.value,
            [resultCameraId]: Math.max(0, (cameraActiveTasks.value[resultCameraId] || 0) - 1),
          }
          return // 失败任务不添加到历史记录
        }
        
        // ★ 验证：必须有 result 字段才是有效的成功结果
        if (!taskData.result) {
          console.warn(`[VLM] task=${taskData.task_id} 无结果，跳过`)
          return
        }
        
        // 处理缩略图
        let thumbnail = taskData.thumbnail
        if (thumbnail && !thumbnail.startsWith('data:')) {
          thumbnail = `data:image/jpeg;base64,${thumbnail}`
        }
        
        // ★ 去重：同一 task_id 可能通过 tasks.completed 和 inference_result 两种消息到达
        const taskId = taskData.task_id
        const currentItems = vlmHistoryItemsMap.value[resultCameraId] || []
        if (taskId && currentItems.some(item => item.id === taskId)) {
          return // 已存在，跳过重复
        }
        
        // 添加到历史记录
        const newItem: VlmHistoryItem = {
          id: taskId,
          timestamp: taskData.timestamp || Date.now(),
          cameraId: resultCameraId,
          thumbnail: thumbnail,
          result: taskData.result || '',
          nodeId: taskData.node_id,
          processingTime: taskData.processing_time,
          resolution: taskData.resolution,
          isNew: true,
        }
        
        // 更新对应摄像头的历史记录
        vlmHistoryItemsMap.value = {
          ...vlmHistoryItemsMap.value,
          [resultCameraId]: [newItem, ...currentItems.slice(0, 49)],
        }
        
        // 更新任务计数
        cameraActiveTasks.value = {
          ...cameraActiveTasks.value,
          [resultCameraId]: Math.max(0, (cameraActiveTasks.value[resultCameraId] || 0) - 1),
        }
        
        // 3秒后移除 isNew 标记
        setTimeout(() => {
          const items = vlmHistoryItemsMap.value[resultCameraId]
          if (items) {
            const idx = items.findIndex((i: VlmHistoryItem) => i.id === newItem.id)
            if (idx !== -1) {
              items[idx] = { ...items[idx], isNew: false }
            }
          }
        }, 3000)
      }
    } catch (e) {
      console.error('WebSocket message error:', e)
    }
  }
  
  ws.onopen = () => {
    console.log('[WS] Connected')
    autoTrigger.resetPipeline()
    cameraActiveTasks.value = {}
  }
  
  ws.onclose = () => {
    console.log('[WS] Connection closed, reconnecting...')
    setTimeout(connectWebSocket, 3000)
  }
  
  ws.onerror = (e) => {
    console.error('[WS] Error:', e)
  }
}

// ============ 自动触发回调 ============
autoTrigger.setTriggerCallback(async (imageData: string, event: { type: string }) => {
  if (!vlmFocusId.value) return
  
  isAnalyzing.value = true
  
  cameraActiveTasks.value = {
    ...cameraActiveTasks.value,
    [vlmFocusId.value]: (cameraActiveTasks.value[vlmFocusId.value] || 0) + 1,
  }
  
  try {
    // ★ 异步模式：立即返回202，结果通过WebSocket推送
    // ★ K3推理需要10-20s，设置60s超时避免context canceled
    const vlmController = new AbortController()
    const vlmTimeout = setTimeout(() => vlmController.abort(), 60000)
    let resp: Response
    try {
      resp = await fetch('/api/v1/inference/analyze?async=true', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        signal: vlmController.signal,
        body: JSON.stringify({
          image: imageData,
          prompt: '请描述这个监控画面中的内容，包括人物、车辆、活动和异常情况',
          camera_id: vlmFocusId.value,
          stream_name: vlmFocusId.value,
          trigger_type: event.type,
        }),
      })
    } finally {
      clearTimeout(vlmTimeout)
    }

    if (!resp.ok && resp.status !== 202) {
      // ★ 503修复：立即释放slot + 全局退避，防止定时器链式累积饿死YOLO
      // 旧方案：setTimeout(markAnalysisComplete, 5000) → 定时器链式累积 → 每秒1-2次无效提交
      // 新方案：立即释放 + set503Backoff → 唯一定时器，退避期间fillPipeline静默跳过
      if (resp.status === 503) {
        console.warn(`[AutoTrigger] Gateway队列已满 (503), 退避5秒`)
        cameraActiveTasks.value = {
          ...cameraActiveTasks.value,
          [vlmFocusId.value!]: Math.max(0, (cameraActiveTasks.value[vlmFocusId.value!] || 0) - 1),
        }
        autoTrigger.set503Backoff(5000)  // ★ 必须在markAnalysisComplete之前：后者同步调用fillPipeline
        autoTrigger.markAnalysisComplete(vlmFocusId.value!, vlmFocusId.value!)
        isAnalyzing.value = false
        return
      }
      throw new Error(`HTTP ${resp.status}`)
    }

    // ★ 异步模式：推理结果通过WebSocket推送
    // ★ B5修复：202只表示Gateway已接受，推理尚未完成
    //   不要在这里 markAnalysisComplete / 递减 cameraActiveTasks
    //   否则 pipeline 认为"任务完成"立即提交新任务 → 每秒30-50个请求洪泛
    //   真正完成在 WebSocket onmessage 中处理（line ~803）
    const result = await resp.json()
    console.debug(`[AutoTrigger] 请求已提交 (${resp.status}), task=${result.task_id || '?'}`)

    // ★ B5: 设置安全超时 — 如果120秒内WebSocket未推送结果，释放pipeline槽位
    const safetyTaskId = result.task_id
    const safetyCamId = vlmFocusId.value!
    setTimeout(() => {
      // 只有当该任务尚未通过WebSocket完成时才释放
      const items = vlmHistoryItemsMap.value[safetyCamId] || []
      const alreadyCompleted = safetyTaskId && items.some((i: VlmHistoryItem) => i.id === safetyTaskId)
      if (!alreadyCompleted) {
        console.warn(`[AutoTrigger] ⏰ 安全超时: task=${safetyTaskId}, 释放pipeline槽位`)
        cameraActiveTasks.value = {
          ...cameraActiveTasks.value,
          [safetyCamId]: Math.max(0, (cameraActiveTasks.value[safetyCamId] || 0) - 1),
        }
        autoTrigger.markAnalysisComplete(safetyCamId, safetyCamId)
      }
    }, 120000)

    isAnalyzing.value = false
  } catch (e) {
    console.error('[Submit] Error:', e)
    isAnalyzing.value = false
    // ★ 错误时释放pipeline槽位（任务不会有WebSocket结果）
    cameraActiveTasks.value = {
      ...cameraActiveTasks.value,
      [vlmFocusId.value!]: Math.max(0, (cameraActiveTasks.value[vlmFocusId.value!] || 0) - 1),
    }
    autoTrigger.markAnalysisComplete(vlmFocusId.value!, vlmFocusId.value!)
  }
})

// ============ Provide ============
provide('cameraActiveTasks', cameraActiveTasks)

// ============ 生命周期 ============
onMounted(async () => {
  await fetchCameras()
  
  // ---- 同步后端 YOLO 状态到前端（显示之前会话的状态）----
  try {
    const status = await yoloApi.getStatus()
    if (status.enabled_cameras && status.enabled_cameras.length > 0) {
      // ★ 同步之前的状态，并在控制台提示用户
      yoloEnabledCameras.value = new Set(status.enabled_cameras)
      console.log('[Init] ⚠️ 检测到之前会话的 YOLO 状态，已恢复:', status.enabled_cameras)
      console.log('[Init] 如需清除，可调用: POST /api/yolo/disable-all')
    } else {
      yoloEnabledCameras.value = new Set()
      console.log('[Init] YOLO 状态为空，等待用户启用')
    }
  } catch (e) {
    console.warn('[Init] Failed to sync backend YOLO state:', e)
    yoloEnabledCameras.value = new Set()
  }
  
  // ---- 网格默认空白，由用户手动选择摄像头 ----
  console.log('[Init] Grid default empty, waiting for user to add cameras')
  
  // ---- VLM/YOLO 默认关闭，等待用户手动开启 ----
  console.log('[Init] VLM/YOLO default OFF, waiting for user to enable')
  
  // ---- 启动 VLM 分析（仅当用户之前已开启）----
  if (vlmEnabledCameras.value.size > 0) {
    startMultiCameraVlmAnalysis()
  }
  
  fetchNodeStatus()
  fetchHeaderStatus()
  if (vlmEnabledCameras.value.size > 0) fetchHistory()
  connectWebSocket()
  
  // 如果初始获取摄像头为空（go2rtc可能还在启动），延迟重试
  if (cameras.value.length === 0) {
    console.warn('[Init] cameras empty, retrying in 3s...')
    setTimeout(fetchCameras, 3000)
  }
  
  // ★ P2修复：节点状态高频轮询(15s)，摄像头/历史低频轮询(60s)
  // 节点故障/恢复需快速检测→15s; 摄像头列表变化少→60s足够
  refreshTimer = window.setInterval(() => {
    fetchNodeStatus()
    fetchHeaderStatus()
  }, 15000)
  // 低频刷新
  window.setInterval(() => {
    if (vlmEnabledCameras.value.size > 0) fetchHistory()
    fetchCameras()  // 定期刷新摄像头列表，确保 yoloConfigs 不会因初始获取失败而永远为空
  }, 60000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  if (ws) ws.close()
  autoTrigger.stop()
  stopMultiCameraVlmAnalysis()
  stopYoloTriggeredVlm()
})

// ============ Watch ============
watch(vlmFocusId, (newId) => {
  if (newId) {
    fetchHistory()
    // 延迟绑定，等待视频元素就绪
    setTimeout(rebindAutoTrigger, 500)
  } else {
    autoTrigger.stop()
  }
})
</script>

<style scoped>
.monitor-dashboard {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--bg-primary, #0f0f1a);
  color: var(--text-primary, #e0e0e0);
}

.dashboard-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 24px;
  background: var(--bg-secondary, #1e1e2e);
  border-bottom: 1px solid var(--border-color, #333);
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo {
  font-size: 24px;
}

.title {
  font-size: 18px;
  font-weight: 600;
  margin: 0;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 20px;
}

.status-strip {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.status-pill {
  border: 1px solid #3f3f52;
  background: #24283a;
  color: #c8cfde;
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 12px;
  line-height: 18px;
  border-width: 1px;
}

.status-pill.clickable {
  cursor: pointer;
  transition: all 0.2s ease;
}

.status-pill.clickable:hover {
  transform: translateY(-1px);
  filter: brightness(1.08);
}

.status-pill.ok {
  border-color: #2f8f61;
  background: #1f3328;
  color: #a6efc8;
}

.status-pill.warn {
  border-color: #8f6a31;
  background: #332919;
  color: #f0d39b;
}

.layout-selector {
  display: flex;
  gap: 4px;
  background: var(--bg-tertiary, #2a2a3e);
  padding: 4px;
  border-radius: 8px;
}

.layout-btn {
  padding: 6px 12px;
  background: transparent;
  border: none;
  border-radius: 6px;
  color: var(--text-secondary, #888);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.layout-btn:hover {
  background: var(--bg-hover, #3a3a4e);
  color: var(--text-primary, #e0e0e0);
}

.layout-btn.active {
  background: var(--accent-primary, #10a37f);
  color: white;
}

.dashboard-content {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.sidebar {
  width: 260px;
  background: var(--bg-secondary, #1e1e2e);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  border-right: 1px solid var(--border-color, #333);
  overflow: hidden;  /* ★ 不让整个侧边栏滚动 */
  flex-shrink: 0;
  transition: width 0.3s ease, padding 0.3s ease;
  position: relative;
}

.sidebar.collapsed {
  width: 48px;
  padding: 16px 8px;
  overflow: hidden;
}

.sidebar.collapsed :deep(.camera-list),
.sidebar.collapsed :deep(.section-title),
.sidebar.collapsed :deep(.camera-items),
.sidebar.collapsed :deep(.no-cameras) {
  display: none;
}

.sidebar-toggle {
  position: absolute;
  top: 12px;
  right: 8px;
  width: 32px;
  height: 32px;
  background: var(--bg-tertiary, #2a2a3e);
  border: 1px solid var(--border-color, #333);
  border-radius: 6px;
  color: var(--text-secondary, #888);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  z-index: 10;
}

.sidebar-toggle svg {
  width: 16px;
  height: 16px;
  stroke-width: 2;
}

.sidebar-toggle:hover {
  background: var(--accent-primary, #10a37f);
  border-color: var(--accent-primary, #10a37f);
  color: white;
}

.sidebar.collapsed .sidebar-toggle {
  position: static;
  margin: 0 auto;
}

.sidebar-divider {
  height: 1px;
  background: var(--border-color, #333);
}

.sidebar.collapsed .sidebar-divider {
  display: none;
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  overflow: hidden;
}

.video-section {
  flex: 1;
  min-height: 0;
}

.history-section {
  height: 320px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.bottom-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}

.tab-btn {
  border: 1px solid var(--border-color, #333);
  background: var(--bg-secondary, #1e1e2e);
  color: var(--text-secondary, #aaa);
  border-radius: 8px;
  padding: 6px 12px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.tab-btn:hover {
  color: var(--text-primary, #fff);
  border-color: var(--accent-primary, #10a37f);
}

.tab-btn.active {
  background: var(--accent-primary, #10a37f);
  color: #fff;
  border-color: var(--accent-primary, #10a37f);
}

.tab-content {
  flex: 1;
  min-height: 0;
}

/* 响应式 */
@media (max-width: 1200px) {
  .sidebar {
    width: 220px;
  }
  
  .history-section {
    height: 240px;
  }
}

@media (max-width: 768px) {
  .dashboard-content {
    flex-direction: column;
  }
  
  .sidebar {
    width: 100%;
    flex-direction: row;
    flex-wrap: wrap;
    border-right: none;
    border-bottom: 1px solid var(--border-color, #333);
  }
  
  .main-area {
    padding: 12px;
  }
  
  .history-section {
    height: 200px;
  }
}

/* 导航菜单样式 */
.nav-menu {
  display: flex;
  gap: 8px;
  margin-right: 16px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: var(--text-secondary, #888);
  text-decoration: none;
  font-size: 13px;
  transition: all 0.2s;
}

.nav-item:hover {
  background: var(--bg-hover, rgba(255,255,255,0.1));
  color: var(--text-primary, #fff);
}

.nav-item.active {
  background: var(--accent-primary, #10a37f);
  color: white;
}

.nav-item.gateway-active {
  background: #409eff;
  color: white;
}

.gateway-status {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #f56c6c;
}

.gateway-status.connected {
  background: #67c23a;
}

@media (max-width: 1200px) {
  .nav-menu {
    display: none;
  }
}
</style>
