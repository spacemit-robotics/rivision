// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div 
    ref="videoCellRef"
    class="video-cell" 
    :class="{ 
      'is-vlm-focus': isVlmFocus,
      'is-empty': !cameraId,
      'is-analyzing': isVlmFocus && isVlmEnabled && isAnalyzing,
      'is-fullscreen': isFullscreen
    }"
    @click="handleClick"
  >
    <!-- 空位状态 -->
    <div v-if="!cameraId" class="empty-slot">
      <div class="empty-content">
        <div class="empty-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path d="M12 5v14m-7-7h14"/>
          </svg>
        </div>
        <div class="empty-text">点击添加摄像头</div>
      </div>
      <div class="slot-indicator">{{ slotIndex + 1 }}</div>
    </div>

    <!-- 视频播放状态 -->
    <template v-else>
      <!-- 同步模式：使用检测帧图像（仅当有帧数据时显示） -->
      <img
        v-if="yoloSyncMode && yoloEnabled && yoloFrameSrc"
        ref="yoloFrameRef"
        class="video-player yolo-sync-frame"
        :src="yoloFrameSrc"
        alt="YOLO 检测帧"
      />
      <!-- 同步模式加载中（后端已不发送二进制帧，此块不再显示） -->
      <!-- 普通模式：使用视频流（始终渲染，同步模式下隐藏但保持播放用于VLM帧捕获） -->
      <video
        ref="videoRef"
        class="video-player"
        :class="{}"
        autoplay
        muted
        playsinline
      ></video>

      <!-- YOLO Canvas 叠加层 -->
      <canvas 
        ref="yoloCanvasRef" 
        class="yolo-overlay"
      ></canvas>

      <!-- 视频信息叠加层 -->
      <div class="video-overlay">
        <!-- 顶部信息栏 -->
        <div class="overlay-top">
          <div class="camera-info">
            <span class="live-badge">● LIVE</span>
            <span class="camera-name">{{ cameraName }}</span>
          </div>
          <div class="overlay-actions">
            <button 
              v-if="isVlmEnabled"
              class="vlm-badge"
              :class="{ 'vlm-focus': isVlmFocus }"
              :title="isVlmFocus ? 'VLM 推理焦点' : 'VLM 已启用'"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
              </svg>
              VLM
            </button>
            <button 
              class="action-btn fullscreen-btn" 
              @click.stop="toggleFullscreen"
              :title="isFullscreen ? '退出全屏' : '全屏'"
            >
              <svg v-if="!isFullscreen" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor">
                <path d="M8 3v3a2 2 0 0 1-2 2H3m18 0h-3a2 2 0 0 1-2-2V3m0 18v-3a2 2 0 0 1 2-2h3M3 16h3a2 2 0 0 1 2 2v3"/>
              </svg>
            </button>
            <button 
              class="action-btn remove-btn" 
              @click.stop="handleRemove"
              title="移除摄像头"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                <line x1="18" y1="6" x2="6" y2="18"/>
                <line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>
        </div>

        <!-- 底部状态栏 -->
        <div class="overlay-bottom">
          <!-- YOLO 检测结果 (预留) -->
          <div v-if="yoloEnabled" class="yolo-stats">
            <span class="yolo-badge">YOLO</span>
            <span v-if="yoloDetections.length > 0" class="detection-count">{{ yoloDetections.length }} 目标</span>
            <button 
              class="mode-toggle-btn"
              @click.stop="toggleYoloMode"
              :title="yoloDisplayMode === 'sync' ? '当前：同步模式（点击切换到流畅模式）' : '当前：流畅模式（点击切换到同步模式）'"
            >
              {{ yoloDisplayMode === 'sync' ? '同步' : '流畅' }}
            </button>
          </div>
          
          <!-- VLM 分析状态 -->
          <div v-if="isVlmFocus && isVlmEnabled && isAnalyzing" class="analyzing-indicator">
            <span class="analyzing-dot"></span>
            <span>AI 分析中...</span>
          </div>
        </div>
      </div>

      <!-- 加载状态（YOLO同步模式下不显示，因为同步模式有自己的加载指示器） -->
      <div v-if="isLoading && !(yoloSyncMode && yoloEnabled)" class="loading-overlay">
        <div class="loading-spinner"></div>
        <div class="loading-text">连接中...</div>
      </div>

      <!-- 错误状态 -->
      <div v-if="errorMessage" class="error-overlay">
        <div class="error-icon">⚠️</div>
        <div class="error-text">{{ errorMessage }}</div>
        <div class="error-actions">
          <button class="retry-btn" @click="reconnect">重试</button>
          <button class="remove-btn" @click.stop="removeCamera">移除</button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { yoloWebSocket, type YoloDetection as YoloDetectionWS } from '@/services/yoloWebSocket'
import { yoloDirectDetector } from '@/services/yoloDirectDetector'
import { playChannel } from '@/services/owlStreams'

// ============ 类型定义 ============
export interface YoloDetection {
  class: string
  confidence: number
  bbox: [number, number, number, number]  // [x1, y1, x2, y2] 归一化坐标
  track_id?: number
  velocity_x?: number  // 归一化速度
  velocity_y?: number
}

export interface YoloConfig {
  model: 'person' | 'vehicle' | 'object' | 'helmet' | 'custom'
  confidence: number
  enabled: boolean
}

// ============ Props ============
interface Props {
  cameraId: string | null
  cameraName?: string
  slotIndex: number
  isVlmFocus?: boolean
  isVlmEnabled?: boolean
  isAnalyzing?: boolean
  yoloConfig?: YoloConfig | null
}

const props = withDefaults(defineProps<Props>(), {
  cameraName: '',
  isVlmFocus: false,
  isVlmEnabled: false,
  isAnalyzing: false,
  yoloConfig: null,
})

// ============ Emits ============
const emit = defineEmits<{
  (e: 'select-slot', index: number): void
  (e: 'remove', index: number): void
  (e: 'video-ready', video: HTMLVideoElement): void
  (e: 'video-error', error: string): void
}>()

// ============ Refs ============
const videoRef = ref<HTMLVideoElement | null>(null)
const yoloCanvasRef = ref<HTMLCanvasElement | null>(null)
const yoloFrameRef = ref<HTMLImageElement | null>(null)

// ============ 状态 ============
const isLoading = ref(false)
const errorMessage = ref<string | null>(null)
const canvasWidth = ref(640)
const canvasHeight = ref(360)
const isFullscreen = ref(false)
const videoCellRef = ref<HTMLDivElement | null>(null)

// YOLO 相关
const yoloEnabled = computed(() => {
  const val = props.yoloConfig?.enabled ?? false
  return val
})  // 默认不启用YOLO
const yoloDetections = ref<YoloDetection[]>([])
const yoloConnected = ref(false)
const yoloInferenceTime = ref(0)

// ★ 平滑插值：存储前一帧检测用于动画
let prevDetections: YoloDetection[] = []
let detectionTimestamp = 0
const INTERPOLATION_DURATION = 80   // ★ K3 检测间隔 ~67ms (15fps)，插值略长于检测间隔

// YOLO 原始图像尺寸（用于坐标转换）
const yoloImageWidth = ref(1920)
const yoloImageHeight = ref(1080)

// YOLO 显示模式
// - 'sync': 同步模式，显示后端绘制好边界框的JPEG帧，完美同步但帧率低(~20FPS)，依赖网关和节点
// - 'smooth': 流畅模式，WebRTC流+Canvas叠加，流畅但可能略有延迟，不依赖网关
const yoloDisplayMode = ref<'sync' | 'smooth'>('smooth')  // 默认流畅模式（不依赖网关）
const yoloSyncMode = computed(() => yoloDisplayMode.value === 'sync')
const yoloFrameSrc = ref<string>('')  // 检测帧的 Data URL

// 前端跟踪状态
interface TrackedBox {
  track_id: number
  bbox: [number, number, number, number]
  velocity_x: number  // 归一化速度
  velocity_y: number
  last_update: number
  class: string
  confidence: number
}
const trackedBoxes = ref<Map<number, TrackedBox>>(new Map())
const lastFrameTime = ref(0)

// YOLO 同步模式超时回退和帧率监控
let yoloSyncTimeoutTimer: number | null = null
let yoloFrameRateTimer: number | null = null
let lastYoloFrameTime = 0
const yoloSyncFallbackTriggered = ref(false)
const YOLO_FRAME_TIMEOUT_MS = 3000  // 3秒无新帧则回退

// YOLO 流畅模式动画循环
let yoloAnimationId: number | null = null
let lastAnimationTime = 0
let yoloDirty = false  // ★ 脏标记：仅在数据变化时重绘，避免无意义的canvas操作
let cachedDisplayRect: { width: number; height: number } | null = null  // ★ 缓存布局尺寸
const ANIMATION_THROTTLE_MS = 33  // ~30fps节流（需高于YOLO检测帧率15fps以实现平滑速度预测插值）

function startYoloSyncTimeout() {
  if (yoloSyncTimeoutTimer) return
  // 5秒内没有收到 YOLO 帧，自动回退到普通视频流模式
  yoloSyncTimeoutTimer = window.setTimeout(() => {
    if (yoloSyncMode.value && !yoloFrameSrc.value && !yoloSyncFallbackTriggered.value) {
      console.log(`[VideoCell] YOLO sync initial timeout, falling back to video stream`)
      triggerSyncFallback()
    }
  }, 5000)
}

function startFrameRateMonitor() {
  if (yoloFrameRateTimer) return
  lastYoloFrameTime = Date.now()
  // 每秒检查一次帧率
  yoloFrameRateTimer = window.setInterval(() => {
    const elapsed = Date.now() - lastYoloFrameTime
    if (elapsed > YOLO_FRAME_TIMEOUT_MS && yoloSyncMode.value && !yoloSyncFallbackTriggered.value) {
      console.log(`[VideoCell] YOLO frame rate too low (${elapsed}ms since last frame), falling back`)
      triggerSyncFallback()
    }
  }, 1000)
}

function updateLastFrameTime() {
  lastYoloFrameTime = Date.now()
}

function triggerSyncFallback() {
  yoloSyncFallbackTriggered.value = true
  clearYoloSyncTimeout()
  clearFrameRateMonitor()
  yoloDisplayMode.value = 'smooth'
  nextTick(() => startVideoStream())
}

function clearYoloSyncTimeout() {
  if (yoloSyncTimeoutTimer) {
    clearTimeout(yoloSyncTimeoutTimer)
    yoloSyncTimeoutTimer = null
  }
}

function clearFrameRateMonitor() {
  if (yoloFrameRateTimer) {
    clearInterval(yoloFrameRateTimer)
    yoloFrameRateTimer = null
  }
}

// YOLO 流畅模式动画循环 - 使用requestAnimationFrame实现平滑绘制
// ★ 优化：持续绘制 + 平滑插值，让 5fps 检测看起来像 30fps
let animationFrameCount = 0
function startYoloAnimation() {
  if (yoloAnimationId !== null) return
  
  const animate = (timestamp: number) => {
    // ★ 检查组件是否仍然活跃
    if (!yoloEnabled.value) {
      yoloAnimationId = null
      return
    }
    
    // 节流：至少间隔ANIMATION_THROTTLE_MS毫秒 (~30fps)
    if (timestamp - lastAnimationTime >= ANIMATION_THROTTLE_MS) {
      lastAnimationTime = timestamp
      animationFrameCount++
      
      // ★ 持续绘制：每帧都绘制（用插值实现平滑动画）
      if (yoloDetections.value.length > 0 || prevDetections.length > 0) {
        drawYoloDetections()
      }
      
      // 重置脏标记
      if (yoloDirty) yoloDirty = false
    }
    yoloAnimationId = requestAnimationFrame(animate)
  }
  
  yoloAnimationId = requestAnimationFrame(animate)
}

function stopYoloAnimation() {
  if (yoloAnimationId !== null) {
    cancelAnimationFrame(yoloAnimationId)
    yoloAnimationId = null
  }
}

// YOLO 回调函数（提取为命名函数以便统一 watch 和 health check 复用）
const yoloCallback = (detections: YoloDetectionWS[], imageWidth?: number, imageHeight?: number, frameBase64?: string, frameTime?: number) => {
    // 收到数据，更新帧时间并启动帧率监控
    updateLastFrameTime()
    clearYoloSyncTimeout()
    startFrameRateMonitor()
    // 更新原始图像尺寸
    if (imageWidth) yoloImageWidth.value = imageWidth
    if (imageHeight) yoloImageHeight.value = imageHeight
    
    // 同步模式：使用检测帧
    if (yoloSyncMode.value && frameBase64) {
      yoloFrameSrc.value = 'data:image/jpeg;base64,' + frameBase64
    }
    
    // 更新帧时间戳
    if (frameTime) {
      lastFrameTime.value = frameTime
    }
    
    // ★ 修复：yoloDirectDetector 不提供 track_id，清空 trackedBoxes 避免累积
    // 只有 WebSocket 模式才使用 trackedBoxes（但 WebSocket 已禁用）
    trackedBoxes.value.clear()
    
    // ★ 调试：打印异常情况
    if (detections.length > 20) {
      console.warn(`[YOLO] ⚠️ 异常：收到过多检测 ${detections.length} 个，可能是后端问题`)
    }
    
    // ★ 严格过滤：防止满屏框
    const validDetections = detections.filter(d => {
      // 检查 bbox 格式
      if (!d.bbox || !Array.isArray(d.bbox) || d.bbox.length !== 4) return false
      
      const [x1, y1, x2, y2] = d.bbox
      
      // 检查是否为有效数字
      if (!Number.isFinite(x1) || !Number.isFinite(y1) || 
          !Number.isFinite(x2) || !Number.isFinite(y2)) return false
      
      // ★ 严格范围检查：必须在 0-1 之间
      if (x1 < 0 || y1 < 0 || x2 > 1 || y2 > 1) return false
      
      // ★ 检查框大小：必须是合理大小 (最小 1%，最大 95%)
      const width = x2 - x1
      const height = y2 - y1
      if (width < 0.01 || height < 0.01) return false  // 太小
      if (width > 0.95 || height > 0.95) return false  // 太大（满屏）
      
      // ★ 检查置信度
      if (!d.confidence || d.confidence < 0.3) return false
      
      return true
    })
    
    // ★ 如果过滤后超过 15 个检测，警告并截断（正常场景很少超过 15 个目标）
    if (validDetections.length > 15) {
      console.warn(`[YOLO] ⚠️ 检测数量过多 (${validDetections.length})，截断为 15`)
    }
    
    // ★ 平滑插值：保存前一帧用于动画
    prevDetections = [...yoloDetections.value]
    detectionTimestamp = performance.now()
    
    yoloDetections.value = validDetections.slice(0, 15).map(d => ({  // ★ 限制最大 15 个
      class: d.class_name,
      confidence: d.confidence,
      bbox: [
        Math.max(0, Math.min(1, d.bbox[0])),
        Math.max(0, Math.min(1, d.bbox[1])),
        Math.max(0, Math.min(1, d.bbox[2])),
        Math.max(0, Math.min(1, d.bbox[3]))
      ] as [number, number, number, number],
      track_id: d.track_id,
      velocity_x: d.velocity_x || 0,
      velocity_y: d.velocity_y || 0
    }))
    
    // ★ 更新推理时间：前端直接检测 或 后端 WebSocket
    if (yoloDirectDetector.enabled) {
      const dStats = yoloDirectDetector.getStats()
      yoloConnected.value = true
      yoloInferenceTime.value = dStats.avgLatencyMs
    } else {
      const state = yoloWebSocket.getState(props.cameraId!)
      if (state) {
        yoloConnected.value = state.connected
        yoloInferenceTime.value = state.inferenceTimeMs
      }
    }
    
    // ★ 标记脏数据，触发动画循环重绘
    yoloDirty = true
    
    // ★ 两种模式都使用 Canvas 动画循环绘制检测框
    startYoloAnimation()
}

function disconnectYoloWebSocket() {
  if (props.cameraId) {
    yoloWebSocket.disconnect(props.cameraId)
    yoloDirectDetector.stop(props.cameraId)
    // 清理全部 YOLO 状态，确保重新启用时为干净状态
    yoloDetections.value = []
    trackedBoxes.value.clear()
    yoloFrameSrc.value = ''
    stopYoloAnimation()
    yoloConnected.value = false
    clearYoloSyncTimeout()
    clearFrameRateMonitor()
    // ★ 清除 canvas 上残留的边界框（停止动画后不会再自动清除）
    clearYoloCanvas()
  }
}

function clearYoloCanvas() {
  if (!yoloCanvasRef.value) return
  const ctx = yoloCanvasRef.value.getContext('2d')
  if (ctx) {
    ctx.clearRect(0, 0, yoloCanvasRef.value.width, yoloCanvasRef.value.height)
  }
}

function toggleYoloMode() {
  yoloDisplayMode.value = yoloDisplayMode.value === 'sync' ? 'smooth' : 'sync'
  
  // 切换到smooth模式时需要启动视频流和动画循环
  if (yoloDisplayMode.value === 'smooth') {
    yoloFrameSrc.value = ''  // 清空同步帧
    nextTick(() => startVideoStream())
    startYoloAnimation()
  } else {
    // ★ Sync模式也保持视频流（后端不再发送二进制JPEG帧）
    // 两种模式统一: 视频流 + Canvas叠加检测框
    startYoloAnimation()
  }
}

// ============ 视频流管理 ============
// ★ 原生 video.src (stream.mp4) — 浏览器内置 MP4 解复用器
// 正确处理 B 帧重排序，消除 MSE 模式的 "1-2-3, 2-3-4" 解码崩溃循环
let reconnectTimer: number | null = null
let videoStopping = false  // 防止 stopVideoStream 期间 onerror 触发重连
let reconnectAttempts = 0  // ★ 重连次数计数器（防止无限重连导致内存溢出）
const MAX_RECONNECT_ATTEMPTS = 10  // 最大重连次数
const RECONNECT_RESET_INTERVAL = 60000  // 60秒后重置重连计数

async function startVideoStream() {
  if (!videoRef.value || !props.cameraId) return

  isLoading.value = true
  errorMessage.value = null

  try {
    // ★ 完全停止旧流并等待资源释放（参考 Jessibuca 销毁重建模式）
    stopVideoStream()
    await new Promise(resolve => setTimeout(resolve, 100))  // 等待浏览器释放连接

    const cameraId = props.cameraId
    console.log(`[VideoCell ${props.slotIndex}] Starting native stream for: ${cameraId}`)

    // ★ OWL 集成：先调用 playChannel 确保流已注册到 go2rtc
    // 对于 OWL 通道，这会触发 OWL.Play() → bridge.EnsureStream() → go2rtc.AddStream()
    // 对于原生 go2rtc 流，playChannel 会返回 null 但不影响播放
    try {
      await playChannel(cameraId)
    } catch (e) {
      // playChannel 失败不阻止播放尝试（可能是原生 go2rtc 流）
      console.debug(`[VideoCell ${props.slotIndex}] playChannel skipped:`, e)
    }

    // ★ 原生 video.src — 浏览器自行处理 B 帧重排序
    // go2rtc /api/stream.mp4 提供持续 fMP4 HTTP 流
    // 添加时间戳避免浏览器缓存导致的重连问题
    const timestamp = Date.now()
    videoRef.value.src = `/api/go2rtc/stream.mp4?src=${encodeURIComponent(cameraId)}&_t=${timestamp}`

    await videoRef.value.play().catch((e) => {
      if (e.name !== 'AbortError') throw e
    })

    isLoading.value = false
    reconnectAttempts = 0  // ★ 连接成功，重置重连计数器
    emit('video-ready', videoRef.value)

  } catch (e) {
    console.error(`[VideoCell ${props.slotIndex}] Stream error:`, e)
    isLoading.value = false
    errorMessage.value = '视频流连接失败'
    emit('video-error', errorMessage.value)
  }
}

function setupVideoEvents() {
  if (!videoRef.value) return

  videoRef.value.onended = () => {
    console.log(`[VideoCell ${props.slotIndex}] Stream ended, reconnecting...`)
    scheduleReconnect(1000)
  }

  videoRef.value.onerror = () => {
    // ★ 防止 stopVideoStream() 中 load() 触发的 error 事件导致意外重连
    if (videoStopping) return
    console.log(`[VideoCell ${props.slotIndex}] Stream error, reconnecting...`)
    // ★ 修复：视频错误时清理 YOLO 数据，防止旧数据导致满屏框
    yoloDetections.value = []
    trackedBoxes.value.clear()
    clearYoloCanvas()
    scheduleReconnect(2000)
  }

  // ★ onpause: 原生模式需要恢复播放
  videoRef.value.onpause = () => {
    if (videoStopping) return
    if (videoRef.value && !document.hidden && props.cameraId) {
      videoRef.value.play().catch(() => {})
    }
  }

  videoRef.value.oncanplay = () => {
    isLoading.value = false
    updateCanvasSize()
    // ★ 视频就绪后启动前端直接检测（绕过 go2rtc frame.jpeg 5s 瓶颈）
    if (yoloEnabled.value && props.cameraId && videoRef.value) {
      yoloDirectDetector.start(props.cameraId, videoRef.value, yoloCallback)
    }
  }

  videoRef.value.onresize = () => {
    updateCanvasSize()
  }

  // ★ 修复：监控视频卡顿/缓冲事件
  videoRef.value.onwaiting = () => {
    console.log(`[VideoCell ${props.slotIndex}] Video buffering...`)
  }
  
  videoRef.value.onstalled = () => {
    console.log(`[VideoCell ${props.slotIndex}] Video stalled, attempting recovery...`)
    // 视频卡顿时暂停 YOLO 检测，避免累积
    if (props.cameraId) {
      yoloDirectDetector.stop(props.cameraId)
    }
  }
  
  videoRef.value.onplaying = () => {
    // 视频恢复播放时重启 YOLO 检测
    if (yoloEnabled.value && props.cameraId && videoRef.value) {
      yoloDirectDetector.start(props.cameraId, videoRef.value, yoloCallback)
    }
  }

  // ★ 不做 playbackRate 调整 — 变速会触发解码器 re-seek 导致帧重复
}

function scheduleReconnect(delay: number) {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
  }
  
  // ★ 防止无限重连导致内存溢出
  reconnectAttempts++
  if (reconnectAttempts > MAX_RECONNECT_ATTEMPTS) {
    console.warn(`[VideoCell ${props.slotIndex}] Max reconnect attempts (${MAX_RECONNECT_ATTEMPTS}) reached, stopping`)
    errorMessage.value = '连接失败，请检查摄像头或点击移除'
    isLoading.value = false
    // ★ 优化：不再自动重置，需要用户主动操作（重试或移除）
    return
  }
  
  // 指数退避：延迟随重连次数增加
  const backoffDelay = Math.min(delay * Math.pow(1.5, reconnectAttempts - 1), 30000)
  
  reconnectTimer = window.setTimeout(() => {
    if (props.cameraId) {
      startVideoStream()
    }
  }, backoffDelay)
}

function reconnect() {
  errorMessage.value = null
  reconnectAttempts = 0  // ★ 手动重试时重置计数器
  startVideoStream()
}

// ★ 移除摄像头：停止重试并通知父组件移除
function removeCamera() {
  stopVideoStream()
  errorMessage.value = null
  reconnectAttempts = 0
  emit('remove', props.slotIndex)
}

function stopVideoStream() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  if (videoRef.value) {
    // ★ 设置标记防止 load() 触发 onerror → scheduleReconnect 循环
    videoStopping = true
    videoRef.value.pause()
    videoRef.value.removeAttribute('src')
    videoRef.value.load()
    // 异步重置标记（等当前事件循环中的 error 事件处理完）
    setTimeout(() => { videoStopping = false }, 0)
  }
}

// ============ Canvas 尺寸管理 ============
function updateCanvasSize() {
  // ★ 清除缓存的显示尺寸，下次drawYoloDetections会重新获取
  cachedDisplayRect = null
  
  // ★ Canvas 尺寸跟随 video 元素
  if (videoRef.value) {
    canvasWidth.value = videoRef.value.videoWidth || 640
    canvasHeight.value = videoRef.value.videoHeight || 360
  }
}

// ============ YOLO 绘制 ============
function drawYoloDetections() {
  if (!yoloCanvasRef.value) return

  const ctx = yoloCanvasRef.value.getContext('2d')
  if (!ctx) return

  // ★ 使用缓存的显示尺寸避免getBoundingClientRect()强制重排
  // 仅在canvas尺寸未初始化或resize事件后才重新获取
  if (!cachedDisplayRect || cachedDisplayRect.width === 0) {
    const rect = yoloCanvasRef.value.getBoundingClientRect()
    cachedDisplayRect = { width: Math.round(rect.width), height: Math.round(rect.height) }
  }
  const displayWidth = cachedDisplayRect.width
  const displayHeight = cachedDisplayRect.height
  
  // 设置 Canvas 像素尺寸与显示尺寸一致（避免每帧重设导致闪烁）
  if (yoloCanvasRef.value.width !== displayWidth || yoloCanvasRef.value.height !== displayHeight) {
    yoloCanvasRef.value.width = displayWidth
    yoloCanvasRef.value.height = displayHeight
  }

  // 清空画布
  ctx.clearRect(0, 0, displayWidth, displayHeight)

  if (!yoloEnabled.value) return
  
  // 快速退出：无检测数据
  if (yoloDetections.value.length === 0) return
  
  // ★ 平滑插值：计算当前帧的插值位置
  const now = performance.now()
  const elapsed = now - detectionTimestamp
  const t = Math.min(1, elapsed / INTERPOLATION_DURATION)  // 0~1 插值因子
  
  // ★ 使用缓动函数让动画更自然 (ease-out)
  const easeOut = (x: number) => 1 - Math.pow(1 - x, 2)
  const smoothT = easeOut(t)
  
  // ★ 构建插值后的检测数据
  const detectionsToDraw = yoloDetections.value.map((det, i) => {
    // 尝试找到前一帧中相同位置的检测（简单匹配：按索引或最近距离）
    const prev = prevDetections[i]
    if (prev && smoothT < 1) {
      // 线性插值 bbox
      return {
        ...det,
        bbox: [
          prev.bbox[0] + (det.bbox[0] - prev.bbox[0]) * smoothT,
          prev.bbox[1] + (det.bbox[1] - prev.bbox[1]) * smoothT,
          prev.bbox[2] + (det.bbox[2] - prev.bbox[2]) * smoothT,
          prev.bbox[3] + (det.bbox[3] - prev.bbox[3]) * smoothT
        ] as [number, number, number, number]
      }
    }
    return det
  })

  // 计算视频在容器中的实际显示区域（考虑 object-fit: contain）
  let videoDisplayWidth = displayWidth
  let videoDisplayHeight = displayHeight
  let offsetX = 0
  let offsetY = 0
  
  // 获取视频原始纵横比
  const videoAspect = yoloImageWidth.value / yoloImageHeight.value
  const containerAspect = displayWidth / displayHeight
  
  if (videoAspect > containerAspect) {
    // 视频更宽，上下有黑边
    videoDisplayWidth = displayWidth
    videoDisplayHeight = displayWidth / videoAspect
    offsetY = (displayHeight - videoDisplayHeight) / 2
  } else {
    // 视频更高，左右有黑边
    videoDisplayHeight = displayHeight
    videoDisplayWidth = displayHeight * videoAspect
    offsetX = (displayWidth - videoDisplayWidth) / 2
  }

  // 绘制检测框
  detectionsToDraw.forEach((det) => {
    const [x1, y1, x2, y2] = det.bbox
    
    // ★ 防御性检查：跳过无效的 bbox
    if (!Number.isFinite(x1) || !Number.isFinite(y1) || 
        !Number.isFinite(x2) || !Number.isFinite(y2)) return
    
    const w = (x2 - x1) * videoDisplayWidth
    const h = (y2 - y1) * videoDisplayHeight
    
    // ★ 跳过过小或过大的框
    if (w < 5 || h < 5 || w > videoDisplayWidth * 0.98 || h > videoDisplayHeight * 0.98) return
    
    const x = offsetX + x1 * videoDisplayWidth
    const y = offsetY + y1 * videoDisplayHeight

    const color = getClassColor(det.class)

    // 绘制边框（带圆角效果）
    ctx.strokeStyle = color
    ctx.lineWidth = 2
    ctx.setLineDash([])
    
    // 绘制四角
    const cornerLength = Math.min(w, h) * 0.15
    ctx.beginPath()
    // 左上角
    ctx.moveTo(x, y + cornerLength)
    ctx.lineTo(x, y)
    ctx.lineTo(x + cornerLength, y)
    // 右上角
    ctx.moveTo(x + w - cornerLength, y)
    ctx.lineTo(x + w, y)
    ctx.lineTo(x + w, y + cornerLength)
    // 右下角
    ctx.moveTo(x + w, y + h - cornerLength)
    ctx.lineTo(x + w, y + h)
    ctx.lineTo(x + w - cornerLength, y + h)
    // 左下角
    ctx.moveTo(x + cornerLength, y + h)
    ctx.lineTo(x, y + h)
    ctx.lineTo(x, y + h - cornerLength)
    ctx.stroke()

    // 绘制虚线边框
    ctx.setLineDash([4, 4])
    ctx.strokeRect(x, y, w, h)
    ctx.setLineDash([])

    // 绘制标签背景
    const trackLabel = det.track_id ? `#${det.track_id} ` : ''
    const label = `${trackLabel}${det.class} ${Math.round(det.confidence * 100)}%`
    ctx.font = 'bold 11px sans-serif'
    const textWidth = ctx.measureText(label).width
    const labelHeight = 18
    const labelY = y > labelHeight ? y - labelHeight : y + h

    // 标签背景
    ctx.fillStyle = color
    ctx.beginPath()
    ctx.roundRect(x, labelY, textWidth + 10, labelHeight, 3)
    ctx.fill()

    // 标签文字
    ctx.fillStyle = '#fff'
    ctx.fillText(label, x + 5, labelY + 13)
  })

  // 绘制推理时间
  if (yoloInferenceTime.value > 0) {
    const timeLabel = `${yoloInferenceTime.value}ms`
    ctx.font = '10px sans-serif'
    ctx.fillStyle = 'rgba(0, 0, 0, 0.6)'
    ctx.fillRect(displayWidth - 50, 5, 45, 16)
    ctx.fillStyle = '#0f0'
    ctx.fillText(timeLabel, displayWidth - 45, 16)
  }
}

function getClassColor(className: string): string {
  const colors: Record<string, string> = {
    person: '#00ff00',
    car: '#ff0000',
    truck: '#ff6600',
    bicycle: '#00ffff',
    motorcycle: '#ff00ff',
    default: '#ffff00',
  }
  return colors[className] || colors.default
}

// ============ 事件处理 ============
function handleClick() {
  if (!props.cameraId) {
    emit('select-slot', props.slotIndex)
  }
}

function handleRemove() {
  emit('remove', props.slotIndex)
}

// ============ 全屏控制 ============
async function toggleFullscreen() {
  const container = videoCellRef.value
  if (!container) return
  
  try {
    if (!document.fullscreenElement) {
      await container.requestFullscreen()
      isFullscreen.value = true
    } else {
      await document.exitFullscreen()
      isFullscreen.value = false
    }
  } catch (err) {
    console.error('[VideoCell] Fullscreen error:', err)
  }
}

function handleFullscreenChange() {
  isFullscreen.value = !!document.fullscreenElement
  // 全屏状态变化时更新 Canvas 尺寸
  nextTick(() => {
    cachedDisplayRect = null
    updateCanvasSize()
  })
}

// ============ 暴露方法给父组件 ============
defineExpose({
  getVideoElement: () => videoRef.value,
  getYoloCanvas: () => yoloCanvasRef.value,
  setYoloDetections: (detections: YoloDetection[]) => {
    yoloDetections.value = detections
    drawYoloDetections()
  },
  clearYoloDetections: () => {
    yoloDetections.value = []
    drawYoloDetections()
  },
})

// ============ 生命周期 ============

// 视频流管理：仅处理视频流的启停
watch(() => props.cameraId, (newId, oldId) => {
  if (newId !== oldId) {
    if (newId) {
      nextTick(() => {
        setupVideoEvents()
        startVideoStream()
      })
    } else {
      stopVideoStream()
    }
  }
}, { immediate: true })

// ★ YOLO WebSocket 管理：使用多路复用单连接，subscribe/unsubscribe 替代 connect/disconnect
// 解决浏览器每域名 TCP 连接数限制（6）导致第3+摄像头无法连接的问题
let prevYoloCameraId: string | null = null
watch(
  () => ({ cameraId: props.cameraId, enabled: yoloEnabled.value }),
  ({ cameraId, enabled }) => {
    // 断开旧摄像头的 YOLO（切换摄像头时）
    if (prevYoloCameraId && prevYoloCameraId !== cameraId) {
      yoloWebSocket.disconnect(prevYoloCameraId)
      yoloDirectDetector.stop(prevYoloCameraId)
      yoloFrameSrc.value = ''
      yoloDetections.value = []
      trackedBoxes.value.clear()
      stopYoloAnimation()
      yoloConnected.value = false
    }

    if (cameraId && enabled) {
      // ★ 修复：只使用前端直接检测，禁用 WebSocket 避免双重回调导致满屏框
      // 前端直接检测: <video> Canvas 抓帧 → Gateway YOLO API
      // 延迟更低，无双重回调问题
      if (videoRef.value && videoRef.value.readyState >= 2) {
        yoloDirectDetector.start(cameraId, videoRef.value, yoloCallback)
      }
      // 如果视频未就绪，oncanplay 回调会自动启动 direct detector
      
      // ★ 禁用 WebSocket：避免双重回调导致 trackedBoxes 累积
      // yoloWebSocket.connect(cameraId, yoloCallback)
      startYoloAnimation()
      prevYoloCameraId = cameraId
    } else {
      // 取消订阅
      if (cameraId) {
        disconnectYoloWebSocket()
      }
      prevYoloCameraId = null
    }
  },
  { immediate: true, flush: 'post' }
)

// ★ 移除 watch(yoloDetections) — 动画循环已通过 yoloDirty 标记处理重绘
// 之前 watch + animationFrame 双重触发导致每次YOLO更新重绘两次，浪费CPU

// 同步模式下，帧更新时重绘
watch(yoloFrameSrc, () => {
  if (yoloSyncMode.value && yoloFrameRef.value) {
    yoloFrameRef.value.onload = () => {
      drawYoloDetections()
    }
  }
})

onMounted(() => {
  // 多路复用模式下不需要健康检查 — 单一WS连接有内置重连
  // 监听全屏变化事件
  document.addEventListener('fullscreenchange', handleFullscreenChange)
})

onUnmounted(() => {
  stopVideoStream()
  disconnectYoloWebSocket()
  if (props.cameraId) yoloDirectDetector.stop(props.cameraId)
  stopYoloAnimation()
  clearYoloSyncTimeout()
  clearFrameRateMonitor()
  // 移除全屏事件监听
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
})
</script>

<style scoped>
.video-cell {
  position: relative;
  width: 100%;
  height: 100%;
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 12px;
  overflow: hidden;
  border: 2px solid transparent;
  transition: all 0.3s ease;
}

.video-cell:hover {
  border-color: var(--border-hover, #444);
}

.video-cell.is-vlm-focus {
  border-color: var(--accent-primary, #10a37f);
  box-shadow: 0 0 0 2px rgba(16, 163, 127, 0.2);
}

/* 全屏样式 */
.video-cell.is-fullscreen {
  border-radius: 0;
  border: none;
}

.video-cell.is-fullscreen .video-player {
  object-fit: contain;
}

.video-cell.is-fullscreen .overlay-top {
  padding: 16px 20px;
}

.video-cell.is-fullscreen .camera-name {
  font-size: 16px;
}

.video-cell.is-empty {
  cursor: pointer;
  border: 2px dashed var(--border-color, #333);
}

.video-cell.is-empty:hover {
  border-color: var(--accent-primary, #10a37f);
  background: rgba(16, 163, 127, 0.05);
}

/* 空位样式 */
.empty-slot {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
}

.empty-content {
  text-align: center;
  color: var(--text-muted, #666);
}

.empty-icon {
  width: 48px;
  height: 48px;
  margin: 0 auto 12px;
  background: var(--bg-tertiary, #2a2a3e);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.3s ease;
}

.empty-icon svg {
  width: 24px;
  height: 24px;
  stroke-width: 2;
}

.video-cell.is-empty:hover .empty-icon {
  background: var(--accent-primary, #10a37f);
  color: white;
}

.empty-text {
  font-size: 14px;
  font-weight: 500;
}

.slot-indicator {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 24px;
  height: 24px;
  background: var(--bg-tertiary, #2a2a3e);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary, #888);
}

/* 视频播放器 */
.video-player {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #000;
}

/* 同步模式下隐藏video但保持原始尺寸播放（用于VLM帧捕获） */
.video-player.hidden-video {
  position: absolute;
  top: 0;
  left: 0;
  visibility: hidden;
  pointer-events: none;
}

/* 同步模式加载状态 */
.sync-loading {
  display: flex;
  align-items: center;
  justify-content: center;
}

.sync-loading .loading-text {
  color: #888;
  font-size: 14px;
  text-align: center;
}

.sync-loading .loading-hint {
  color: #666;
  font-size: 12px;
  margin-top: 4px;
}

/* YOLO Canvas 叠加层 */
.yolo-overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  /* ★ GPU合成提示：将canvas提升到独立图层，避免重绘影响视频解码渲染 */
  will-change: contents;
  contain: strict;
}

/* 视频信息叠加层 */
.video-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.3s ease;
}

.video-cell:hover .video-overlay {
  opacity: 1;
}

.video-cell.is-vlm-focus .video-overlay {
  opacity: 1;
}

.overlay-top,
.overlay-bottom {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: linear-gradient(to bottom, rgba(0,0,0,0.7), transparent);
}

.overlay-bottom {
  background: linear-gradient(to top, rgba(0,0,0,0.7), transparent);
}

.camera-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.live-badge {
  background: rgba(239, 68, 68, 0.9);
  color: white;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
}

.camera-name {
  color: white;
  font-size: 13px;
  font-weight: 500;
}

.overlay-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  pointer-events: auto;
}

.vlm-badge {
  display: flex;
  align-items: center;
  gap: 4px;
  background: var(--accent-primary, #10a37f);
  color: white;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 600;
  border: none;
  cursor: default;
}

.vlm-badge svg {
  width: 12px;
  height: 12px;
  fill: currentColor;
  stroke: none;
}

.action-btn {
  width: 28px;
  height: 28px;
  background: rgba(0, 0, 0, 0.6);
  border: none;
  border-radius: 6px;
  color: white;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
}

.action-btn svg {
  width: 14px;
  height: 14px;
  stroke-width: 2;
}

.action-btn.fullscreen-btn:hover {
  background: rgba(59, 130, 246, 0.9);
}

.action-btn.remove-btn:hover {
  background: rgba(239, 68, 68, 0.9);
}

/* YOLO 统计 */
.yolo-stats {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(0, 0, 0, 0.6);
  padding: 4px 10px;
  border-radius: 6px;
}

.yolo-badge {
  background: #ff6600;
  color: white;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
}

.detection-count {
  color: white;
  font-size: 12px;
}

.mode-toggle-btn {
  background: rgba(255, 255, 255, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.4);
  color: white;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
  margin-left: 6px;
}

.mode-toggle-btn:hover {
  background: rgba(255, 255, 255, 0.3);
  border-color: rgba(255, 255, 255, 0.6);
}

/* 分析中指示器 */
.analyzing-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(16, 163, 127, 0.9);
  padding: 4px 10px;
  border-radius: 6px;
  color: white;
  font-size: 12px;
  font-weight: 500;
}

.analyzing-dot {
  width: 8px;
  height: 8px;
  background: white;
  border-radius: 50%;
  animation: pulse 1s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* 加载状态 */
.loading-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.8);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: white;
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 12px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.loading-text {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.8);
}

/* 错误状态 */
.error-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.9);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: white;
  gap: 12px;
}

.error-icon {
  font-size: 32px;
}

.error-text {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.8);
}

.error-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.retry-btn {
  background: var(--accent-primary, #10a37f);
  color: white;
  border: none;
  padding: 8px 20px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.retry-btn:hover {
  background: #0d8a6a;
}

.remove-btn {
  background: #dc3545;
  color: white;
  border: none;
  padding: 8px 20px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.remove-btn:hover {
  background: #c82333;
}
</style>
