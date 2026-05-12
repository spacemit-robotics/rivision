<template>
  <div 
    ref="cellRef"
    class="video-cell" 
    :class="{ 
      'is-vlm-focus': isVlmFocus,
      'is-empty': !cameraId,
      'is-analyzing': isVlmFocus && isAnalyzing,
      'is-fullscreen': isFullscreen,
      'is-error': !!errorMessage
    }"
    @click="handleClick"
  >
    <!-- 空位状态 -->
    <div v-if="!cameraId" class="empty-slot">
      <div class="empty-content">
        <div class="empty-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M12 5v14m-7-7h14"/>
          </svg>
        </div>
        <div class="empty-text">点击添加摄像头</div>
      </div>
      <div class="slot-indicator">{{ slotIndex + 1 }}</div>
    </div>

    <!-- 视频播放状态 -->
    <template v-else>
      <!-- fMP4 / MSE / WebRTC 视频播放 -->
      <video
        v-if="useFMP4 || useMSE || useWebRTC"
        ref="videoRef"
        class="video-player"
        autoplay
        muted
        playsinline
      ></video>
      <!-- go2rtc 内置播放器 (iframe 嵌入，支持 HEVC，但无法叠加 YOLO) -->
      <iframe
        v-else-if="useIframe"
        ref="iframeRef"
        class="video-player video-iframe"
        frameborder="0"
        allow="autoplay"
        allowfullscreen
      ></iframe>
      <!-- YOLO 服务端渲染流 (MJPEG，检测框已绘制) -->
      <img
        v-else-if="useYoloStream && yoloEnabled"
        ref="imgRef"
        class="video-player yolo-stream"
        alt="YOLO Stream"
      />
      <!-- MJPEG 图片流播放器 -->
      <img
        v-else-if="useMjpeg"
        ref="imgRef"
        class="video-player"
        alt="Camera Stream"
      />
      <!-- 视频播放器 (MP4 备选) -->
      <video
        v-else
        ref="videoRef"
        class="video-player"
        autoplay
        muted
        playsinline
      ></video>

      <!-- YOLO Canvas 叠加层 (仅客户端渲染模式使用) -->
      <canvas 
        v-if="!useYoloStream || !yoloEnabled"
        ref="canvasRef" 
        class="detection-overlay"
      ></canvas>

      <!-- 视频信息叠加层 -->
      <div class="video-overlay">
        <!-- 顶部信息栏 -->
        <div class="overlay-top">
          <div class="camera-info">
            <span class="live-badge" :class="{ offline: !isConnected }">
              {{ isConnected ? '● LIVE' : '○ 离线' }}
            </span>
            <span class="camera-name">{{ cameraName }}</span>
          </div>
          <div class="overlay-actions">
            <!-- YOLO 快捷开关 -->
            <button 
              class="ai-toggle-btn yolo-toggle"
              :class="{ active: isYoloEnabled }"
              @click.stop="toggleYolo"
              title="YOLO 检测"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="3" width="18" height="18" rx="2"/>
                <circle cx="8.5" cy="8.5" r="1.5"/>
                <path d="M21 15l-5-5L5 21"/>
              </svg>
              YOLO
            </button>
            <!-- VLM 快捷开关 -->
            <button 
              class="ai-toggle-btn vlm-toggle"
              :class="{ active: isVlmEnabled, 'vlm-focus': isVlmFocus }"
              @click.stop="toggleVlm"
              title="VLM 分析"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
              </svg>
              VLM
            </button>
            <button 
              class="action-btn" 
              @click.stop="toggleFullscreen"
              :title="isFullscreen ? '退出全屏' : '全屏'"
            >
              <svg v-if="!isFullscreen" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M8 3v3a2 2 0 0 1-2 2H3m18 0h-3a2 2 0 0 1-2-2V3m0 18v-3a2 2 0 0 1 2-2h3M3 16h3a2 2 0 0 1 2 2v3"/>
              </svg>
            </button>
            <button 
              class="action-btn remove-btn" 
              @click.stop="handleRemove"
              title="移除摄像头"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18"/>
                <line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>
        </div>

        <!-- 底部状态栏 -->
        <div class="overlay-bottom">
          <!-- 检测结果统计 -->
          <div v-if="detections.length > 0" class="detection-stats">
            <span class="detection-badge">YOLO</span>
            <span class="detection-count">{{ detections.length }} 目标</span>
          </div>
          
          <!-- VLM 分析状态 -->
          <div v-if="isVlmFocus && isAnalyzing" class="analyzing-indicator">
            <span class="analyzing-dot"></span>
            <span>AI 分析中...</span>
          </div>

          <!-- 帧率/延迟 -->
          <div v-if="fps > 0" class="stream-stats">
            {{ fps }} FPS
          </div>
        </div>
      </div>

      <!-- 加载状态 -->
      <div v-if="isLoading" class="loading-overlay">
        <div class="loading-spinner"></div>
        <div class="loading-text">连接中...</div>
      </div>

      <!-- 错误状态 -->
      <div v-if="errorMessage" class="error-overlay">
        <div class="error-icon">⚠️</div>
        <div class="error-text">{{ errorMessage }}</div>
        <div class="error-actions">
          <button class="retry-btn" @click.stop="reconnect">重试</button>
          <button class="remove-btn-small" @click.stop="handleRemove">移除</button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'

// ============ 类型定义 ============
export interface Detection {
  class: string
  confidence: number
  bbox: [number, number, number, number]  // [x1, y1, x2, y2] 归一化坐标
  track_id?: number
}

export interface YoloConfig {
  model: string
  confidence: number
  enabled: boolean
  serverRendering?: boolean  // 使用服务端渲染模式
}

// ============ Props ============
interface Props {
  cameraId: string | null
  cameraName?: string
  slotIndex: number
  isVlmFocus?: boolean
  isVlmEnabled?: boolean
  isYoloEnabled?: boolean
  useServerYolo?: boolean    // 使用服务端 YOLO 渲染 (15-20fps，检测框已绘制)
  isAnalyzing?: boolean
  yoloConfig?: YoloConfig | null
  streamBaseUrl?: string
}

const props = withDefaults(defineProps<Props>(), {
  cameraName: '',
  isVlmFocus: false,
  isVlmEnabled: false,
  isYoloEnabled: false,
  useServerYolo: false,  // 默认使用客户端渲染，设为 true 启用服务端渲染
  isAnalyzing: false,
  yoloConfig: null,
  streamBaseUrl: '/api/v1/streams',
})

// ============ Emits ============
const emit = defineEmits<{
  (e: 'select-slot', index: number): void
  (e: 'remove', index: number): void
  (e: 'video-ready', video: HTMLVideoElement): void
  (e: 'video-error', error: string): void
  (e: 'detection', detections: Detection[]): void
  (e: 'toggle-yolo', cameraId: string, enabled: boolean): void
  (e: 'toggle-vlm', cameraId: string, enabled: boolean): void
}>()

// ============ Refs ============
const cellRef = ref<HTMLDivElement | null>(null)
const videoRef = ref<HTMLVideoElement | null>(null)
const imgRef = ref<HTMLImageElement | null>(null)
const iframeRef = ref<HTMLIFrameElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)

// ============ 状态 ============
const isLoading = ref(false)
const isConnected = ref(false)
const errorMessage = ref<string | null>(null)
const isFullscreen = ref(false)
const fps = ref(0)
const detections = ref<Detection[]>([])
// 播放模式: fMP4 (默认) > MSE > iframe (备选)
const useFMP4 = ref(true)       // HTTP fMP4 (go2rtc stream.mp4, 仅 H.264 可播)
const useWebRTC = ref(false)    // WebRTC 直连 (低延迟, 仅 H264)
const useMSE = ref(false)       // MSE/WebSocket (支持 HEVC + YOLO, 但不稳定)
const useIframe = ref(false)    // iframe 嵌入 go2rtc 播放器 (支持 HEVC, 但无法叠加 YOLO)
const useMjpeg = ref(false)     // MJPEG 模式 (备选)

// YOLO 流处理器端口
const YOLO_STREAM_PORT = 9082

// YOLO 流失败标记 (回退到普通流)
const yoloStreamFailed = ref(false)

// YOLO 服务端渲染模式 (基于 prop 或 config，且未失败)
const useYoloStream = computed(() => 
  !yoloStreamFailed.value && (props.useServerYolo || props.yoloConfig?.serverRendering || false)
)

// (Jessibuca 预留，当前未使用)
let jessibucaPlayer: any = null
let jessibucaLoaded = false

// WebRTC/MSE 连接
let peerConnection: RTCPeerConnection | null = null
let mseWebSocket: WebSocket | null = null
let mediaSource: MediaSource | null = null
let sourceBuffer: SourceBuffer | null = null

// 加载 Jessibuca 库
async function loadJessibuca(): Promise<void> {
  if (jessibucaLoaded || (window as any).Jessibuca) {
    jessibucaLoaded = true
    return
  }
  return new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = '/assets/js/jessibuca.js'
    script.onload = () => {
      jessibucaLoaded = true
      resolve()
    }
    script.onerror = reject
    document.head.appendChild(script)
  })
}

// 重连相关
let reconnectTimer: number | null = null
let reconnectAttempts = 0
const MAX_RECONNECT_ATTEMPTS = 5

// 帧率计算
let frameCount = 0
let lastFpsTime = 0
let fpsInterval: number | null = null

// ============ 计算属性 ============
const yoloEnabled = computed(() => props.isYoloEnabled || props.yoloConfig?.enabled || false)

// ============ 视频流管理 ============
async function startStream() {
  if (!props.cameraId) return
  
  console.log(`[VideoCell ${props.slotIndex}] startStream called, cameraId=${props.cameraId}`)

  isLoading.value = true
  errorMessage.value = null

  try {
    stopStream()
    await new Promise(resolve => setTimeout(resolve, 100))

    const cameraId = props.cameraId
    
    // 获取 Token
    const token = localStorage.getItem('token') || ''
    
    // 1. 先从 Hub 获取 Worker 流地址
    const urlResp = await fetch(`${props.streamBaseUrl}/${encodeURIComponent(cameraId)}/url`, {
      headers: { 'Authorization': `Bearer ${token}` }
    })
    
    if (!urlResp.ok) {
      const errData = await urlResp.json().catch(() => ({}))
      throw new Error(errData.error || `获取流地址失败: ${urlResp.status}`)
    }
    
    const urlData = await urlResp.json()
    const workerHost = urlData.worker_host
    const go2rtcPort = urlData.go2rtc_port || 1984
    
    // 等待 DOM 更新
    await nextTick()
    
    // ★ 方案A: YOLO 服务端渲染 (MJPEG 流，检测框已绘制，15-20fps)
    if (useYoloStream.value && yoloEnabled.value) {
      if (!imgRef.value) {
        console.error(`[VideoCell ${props.slotIndex}] imgRef is null for YOLO stream`)
        throw new Error('img 元素未就绪')
      }
      
      const yoloStreamUrl = `http://${workerHost}:${YOLO_STREAM_PORT}/stream/${encodeURIComponent(cameraId)}`
      console.log(`[VideoCell ${props.slotIndex}] YOLO 服务端渲染流: ${yoloStreamUrl}`)
      
      imgRef.value.src = yoloStreamUrl
      imgRef.value.onload = () => {
        console.log(`[VideoCell ${props.slotIndex}] YOLO 流加载成功`)
        isLoading.value = false
        isConnected.value = true
        reconnectAttempts = 0
      }
      imgRef.value.onerror = () => {
        console.warn(`[VideoCell ${props.slotIndex}] YOLO 流不可用，回退到普通流`)
        // 强制使用普通视频流模式
        useMjpeg.value = false
        useFMP4.value = true
        yoloStreamFailed.value = true
        startStream()
      }
    } else if (useFMP4.value) {
      // ★ HTTP fMP4: go2rtc stream.mp4 直接给 <video> 标签
      // 注意: go2rtc 不会自动转码，如果源是 HEVC 则浏览器无法播放
      if (!videoRef.value) {
        console.error(`[VideoCell ${props.slotIndex}] videoRef is null`)
        throw new Error('video 元素未就绪')
      }
      
      const timestamp = Date.now()
      const mp4Url = `http://${workerHost}:${go2rtcPort}/api/stream.mp4?src=${encodeURIComponent(cameraId)}&_t=${timestamp}`
      console.log(`[VideoCell ${props.slotIndex}] HTTP fMP4 流: ${mp4Url}`)
      
      videoRef.value.src = mp4Url
      videoRef.value.muted = true
      
      try {
        await videoRef.value.play()
        console.log(`[VideoCell ${props.slotIndex}] fMP4 播放成功`)
        isLoading.value = false
        isConnected.value = true
        reconnectAttempts = 0
      } catch (e: any) {
        if (e.name !== 'AbortError') {
          console.warn(`[VideoCell ${props.slotIndex}] fMP4 播放失败 (可能是 HEVC)，自动回退 MSE:`, e.message)
          // fMP4 失败 → 自动切换到 MSE 模式 (支持 HEVC 协商)
          useFMP4.value = false
          useMSE.value = true
          startStream()
          return
        }
      }
      
    } else if (useMSE.value) {
      // MSE 模式: 通过 WebSocket 接收 fMP4 (支持 HEVC + YOLO 叠加)
      if (!videoRef.value) {
        console.error(`[VideoCell ${props.slotIndex}] videoRef is null`)
        throw new Error('video 元素未就绪')
      }
      
      const wsUrl = `ws://${workerHost}:${go2rtcPort}/api/ws?src=${encodeURIComponent(cameraId)}`
      console.log(`[VideoCell ${props.slotIndex}] MSE/WebSocket 连接: ${wsUrl}`)
      
      const video = videoRef.value
      const ms = new MediaSource()
      mediaSource = ms
      video.src = URL.createObjectURL(ms)
      
      // 支持的 codec 列表 (go2rtc 协议要求)
      const CODECS = [
        'avc1.640029', 'avc1.64002A', 'avc1.640033',  // H.264
        'hvc1.1.6.L153.B0',                           // H.265/HEVC
        'mp4a.40.2', 'mp4a.40.5',                     // AAC
        'flac', 'opus'                                // 音频
      ]
      
      const getSupportedCodecs = () => {
        return CODECS.filter(codec => 
          MediaSource.isTypeSupported(`video/mp4; codecs="${codec}"`)
        ).join(',')
      }
      
      ms.addEventListener('sourceopen', () => {
        console.log(`[VideoCell ${props.slotIndex}] MediaSource opened`)
        URL.revokeObjectURL(video.src)
        
        const ws = new WebSocket(wsUrl)
        mseWebSocket = ws
        ws.binaryType = 'arraybuffer'
        
        // 缓冲区 (2MB)
        const buf = new Uint8Array(2 * 1024 * 1024)
        let bufLen = 0
        
        ws.onopen = () => {
          console.log(`[VideoCell ${props.slotIndex}] WebSocket connected`)
          // 发送支持的 codec 列表
          const supportedCodecs = getSupportedCodecs()
          console.log(`[VideoCell ${props.slotIndex}] Supported codecs: ${supportedCodecs}`)
          ws.send(JSON.stringify({ type: 'mse', value: supportedCodecs }))
        }
        
        ws.onmessage = (event) => {
          if (typeof event.data === 'string') {
            const msg = JSON.parse(event.data)
            if (msg.type === 'mse' && msg.value) {
              const mimeCodec = msg.value
              console.log(`[VideoCell ${props.slotIndex}] Server codec: ${mimeCodec}`)
              
              if (MediaSource.isTypeSupported(mimeCodec)) {
                const sb = ms.addSourceBuffer(mimeCodec)
                sourceBuffer = sb
                sb.mode = 'segments'
                
                // 处理缓冲区更新完成
                let hasStarted = false
                sb.addEventListener('updateend', () => {
                  // 追加缓冲的数据
                  if (!sb.updating && bufLen > 0) {
                    try {
                      sb.appendBuffer(buf.slice(0, bufLen))
                      bufLen = 0
                    } catch (e) { /* ignore */ }
                    return
                  }
                  
                  // 管理缓冲区
                  if (!sb.updating && sb.buffered && sb.buffered.length) {
                    const end = sb.buffered.end(sb.buffered.length - 1)
                    const start0 = sb.buffered.start(0)
                    
                    // 保持 10 秒缓冲，避免频繁清理
                    const keepDuration = 10
                    const removeEnd = end - keepDuration
                    if (removeEnd > start0 + 1) {
                      sb.remove(start0, removeEnd)
                      return
                    }
                    
                    // 首次播放：跳转到最新位置
                    if (!hasStarted && end > 0.5) {
                      hasStarted = true
                      video.currentTime = Math.max(0, end - 0.5)
                      console.log(`[VideoCell ${props.slotIndex}] Starting playback at ${video.currentTime.toFixed(2)}s`)
                      video.play().catch(console.error)
                    }
                    
                    // 如果落后太多，跳转追赶
                    const gap = end - video.currentTime
                    if (gap > 3) {
                      video.currentTime = end - 0.5
                      console.log(`[VideoCell ${props.slotIndex}] Catching up, gap was ${gap.toFixed(2)}s`)
                    }
                    
                    // 平滑调整播放速率 (避免跳帧)
                    if (gap > 1) {
                      video.playbackRate = 1.05
                    } else if (gap < 0.3) {
                      video.playbackRate = 0.95
                    } else {
                      video.playbackRate = 1.0
                    }
                  }
                })
                
                isLoading.value = false
                isConnected.value = true
                reconnectAttempts = 0
              } else {
                console.warn(`[VideoCell ${props.slotIndex}] Codec not supported: ${mimeCodec}, falling back to iframe`)
                ws.close()
                useMSE.value = false
                useIframe.value = true
                // 等待 DOM 更新后重新启动
                nextTick(() => startStream())
                return
              }
            } else if (msg.type === 'error') {
              console.error(`[VideoCell ${props.slotIndex}] Server error: ${msg.value}`)
              errorMessage.value = msg.value
            }
          } else if (sourceBuffer) {
            // 追加视频数据 (使用缓冲区)
            if (sourceBuffer.updating || bufLen > 0) {
              const b = new Uint8Array(event.data)
              buf.set(b, bufLen)
              bufLen += b.byteLength
            } else {
              try {
                sourceBuffer.appendBuffer(event.data)
              } catch (e) {
                console.error(`[VideoCell ${props.slotIndex}] Buffer append error:`, e)
              }
            }
          }
        }
        
        ws.onerror = (e) => {
          console.error(`[VideoCell ${props.slotIndex}] WebSocket error:`, e)
          isConnected.value = false
          errorMessage.value = 'MSE 连接失败'
        }
        
        ws.onclose = () => {
          console.log(`[VideoCell ${props.slotIndex}] WebSocket closed`)
          isConnected.value = false
        }
      })
      
      // 自动播放 (静音)
      video.muted = true
      video.play().catch(() => {
        console.log(`[VideoCell ${props.slotIndex}] Autoplay blocked, trying muted`)
        video.muted = true
        video.play().catch(console.error)
      })
      
    } else if (useWebRTC.value) {
      // WebRTC 模式: 直连 go2rtc (低延迟, 仅 H264)
      if (!videoRef.value) {
        console.error(`[VideoCell ${props.slotIndex}] videoRef is null`)
        throw new Error('video 元素未就绪')
      }
      
      const webrtcUrl = `http://${workerHost}:${go2rtcPort}/api/webrtc?src=${encodeURIComponent(cameraId)}`
      console.log(`[VideoCell ${props.slotIndex}] WebRTC 连接: ${webrtcUrl}`)
      
      // 创建 RTCPeerConnection
      const pc = new RTCPeerConnection({
        iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
      })
      peerConnection = pc
      
      // 监听远程流
      pc.ontrack = (event) => {
        console.log(`[VideoCell ${props.slotIndex}] WebRTC track received`)
        if (videoRef.value && event.streams[0]) {
          videoRef.value.srcObject = event.streams[0]
          videoRef.value.play().catch(console.error)
        }
      }
      
      pc.oniceconnectionstatechange = () => {
        console.log(`[VideoCell ${props.slotIndex}] ICE state: ${pc.iceConnectionState}`)
        if (pc.iceConnectionState === 'connected') {
          isLoading.value = false
          isConnected.value = true
          reconnectAttempts = 0
        } else if (pc.iceConnectionState === 'failed' || pc.iceConnectionState === 'disconnected') {
          isConnected.value = false
          errorMessage.value = 'WebRTC 连接断开'
        }
      }
      
      // 添加收发器
      pc.addTransceiver('video', { direction: 'recvonly' })
      pc.addTransceiver('audio', { direction: 'recvonly' })
      
      // 创建 offer
      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)
      
      // 发送 offer 到 go2rtc
      const response = await fetch(webrtcUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/sdp' },
        body: offer.sdp
      })
      
      if (!response.ok) {
        throw new Error(`WebRTC offer failed: ${response.status}`)
      }
      
      // 设置 answer
      const answerSdp = await response.text()
      await pc.setRemoteDescription({ type: 'answer', sdp: answerSdp })
      
      console.log(`[VideoCell ${props.slotIndex}] WebRTC 连接已建立`)
      
    } else if (useIframe.value) {
      // iframe 模式: 使用 go2rtc 内置播放器 (支持 HEVC/WebRTC/MSE)
      if (!iframeRef.value) {
        console.error(`[VideoCell ${props.slotIndex}] iframeRef is null`)
        throw new Error('iframe 元素未就绪')
      }
      const playerUrl = `http://${workerHost}:${go2rtcPort}/stream.html?src=${encodeURIComponent(cameraId)}`
      console.log(`[VideoCell ${props.slotIndex}] 使用 go2rtc 播放器: ${playerUrl}`)
      iframeRef.value.src = playerUrl
      iframeRef.value.onload = () => {
        console.log(`[VideoCell ${props.slotIndex}] go2rtc 播放器已加载`)
        isLoading.value = false
        isConnected.value = true
        reconnectAttempts = 0
      }
      iframeRef.value.onerror = () => {
        isConnected.value = false
        errorMessage.value = '播放器加载失败'
      }
      // iframe 设置 src 后立即认为已连接
      isLoading.value = false
      isConnected.value = true
      reconnectAttempts = 0
    } else if (useMjpeg.value) {
      // MJPEG 模式: 使用 img 标签
      const streamUrl = urlData.streams?.mjpeg
      if (!streamUrl) throw new Error('未获取到 MJPEG 流地址')
      if (!imgRef.value) {
        console.error(`[VideoCell ${props.slotIndex}] imgRef is null`)
        throw new Error('图片元素未就绪')
      }
      imgRef.value.src = streamUrl
      imgRef.value.onload = () => {
        console.log(`[VideoCell ${props.slotIndex}] MJPEG 加载成功`)
        isLoading.value = false
        isConnected.value = true
        reconnectAttempts = 0
        updateCanvasSize()
      }
      imgRef.value.onerror = (e) => {
        console.error(`[VideoCell ${props.slotIndex}] MJPEG 加载失败`, e)
        isConnected.value = false
        errorMessage.value = 'MJPEG 流加载失败'
      }
      // MJPEG 是持续流，设置 src 后立即认为已连接
      isLoading.value = false
      isConnected.value = true
      reconnectAttempts = 0
    } else if (videoRef.value) {
      // MP4/WebRTC 模式: 使用 video 标签
      const mp4Url = urlData.streams?.mp4
      if (!mp4Url) throw new Error('未获取到 MP4 流地址')
      videoRef.value.src = mp4Url
      await videoRef.value.play().catch((e) => {
        if (e.name !== 'AbortError') throw e
      })
      isLoading.value = false
      isConnected.value = true
      reconnectAttempts = 0
      emit('video-ready', videoRef.value)
    }
    
    // 启动帧率计算
    startFpsCounter()

  } catch (e) {
    console.error(`[VideoCell ${props.slotIndex}] Stream error:`, e)
    isLoading.value = false
    isConnected.value = false
    errorMessage.value = e instanceof Error ? e.message : '视频流连接失败'
    emit('video-error', errorMessage.value)
  }
}

function stopStream() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  // ★ 重置视频状态
  videoReady.value = false
  debugLoggedOnce = false
  stopFpsCounter()

  // 销毁 Jessibuca 播放器
  if (jessibucaPlayer) {
    try { jessibucaPlayer.destroy() } catch (e) { /* ignore */ }
    jessibucaPlayer = null
  }

  // 关闭 WebRTC 连接
  if (peerConnection) {
    peerConnection.close()
    peerConnection = null
  }
  
  // 关闭 MSE/WebSocket 连接
  if (mseWebSocket) {
    mseWebSocket.close()
    mseWebSocket = null
  }
  if (mediaSource && mediaSource.readyState === 'open') {
    try {
      mediaSource.endOfStream()
    } catch (e) { /* ignore */ }
  }
  mediaSource = null
  sourceBuffer = null

  if (useIframe.value && iframeRef.value) {
    iframeRef.value.removeAttribute('src')
  } else if (useMjpeg.value && imgRef.value) {
    imgRef.value.removeAttribute('src')
  } else if (videoRef.value) {
    videoRef.value.pause()
    videoRef.value.srcObject = null
    videoRef.value.removeAttribute('src')
    videoRef.value.load()
  }
  
  isConnected.value = false
}

function setupVideoEvents() {
  if (!videoRef.value) return

  videoRef.value.onended = () => {
    console.log(`[VideoCell ${props.slotIndex}] Stream ended`)
    scheduleReconnect(1000)
  }

  videoRef.value.onerror = () => {
    console.log(`[VideoCell ${props.slotIndex}] Stream error`)
    isConnected.value = false
    scheduleReconnect(2000)
  }

  videoRef.value.oncanplay = () => {
    isLoading.value = false
    isConnected.value = true
    // ★ 关键: 视频加载完成，标记可以绘制 YOLO
    videoReady.value = true
    updateCanvasSize()
    console.log(`[VideoCell ${props.slotIndex}] Video ready: ${videoRef.value?.videoWidth}x${videoRef.value?.videoHeight}`)
  }

  videoRef.value.onresize = () => {
    updateCanvasSize()
  }

  // 使用 requestVideoFrameCallback 获取真实帧率 (比 ontimeupdate 更准确)
  if ('requestVideoFrameCallback' in videoRef.value) {
    const countFrame = () => {
      if (videoRef.value && isConnected.value) {
        frameCount++
        ;(videoRef.value as any).requestVideoFrameCallback(countFrame)
      }
    }
    ;(videoRef.value as any).requestVideoFrameCallback(countFrame)
  } else {
    // 回退: 使用 timeupdate (不准确，约4-5fps)
    videoRef.value.ontimeupdate = () => {
      frameCount++
    }
  }
}

function scheduleReconnect(delay: number) {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
  }
  
  reconnectAttempts++
  if (reconnectAttempts > MAX_RECONNECT_ATTEMPTS) {
    errorMessage.value = '连接失败，请检查摄像头'
    isLoading.value = false
    return
  }
  
  const backoffDelay = Math.min(delay * Math.pow(1.5, reconnectAttempts - 1), 30000)
  
  reconnectTimer = window.setTimeout(() => {
    if (props.cameraId) {
      startStream()
    }
  }, backoffDelay)
}

function reconnect() {
  errorMessage.value = null
  reconnectAttempts = 0
  startStream()
}

// ============ 帧率计算 ============
function startFpsCounter() {
  frameCount = 0
  lastFpsTime = performance.now()
  
  fpsInterval = window.setInterval(() => {
    const now = performance.now()
    const elapsed = (now - lastFpsTime) / 1000
    fps.value = Math.round(frameCount / elapsed)
    frameCount = 0
    lastFpsTime = now
  }, 1000)
}

function stopFpsCounter() {
  if (fpsInterval) {
    clearInterval(fpsInterval)
    fpsInterval = null
  }
  fps.value = 0
}

// ============ Canvas 管理 ============
// ★ 参考 rivision_cli: canvas 填满容器，绘制时计算 offset
function updateCanvasSize() {
  if (!canvasRef.value || !cellRef.value) return
  
  // 使用容器实际显示尺寸作为 canvas 像素尺寸
  const rect = cellRef.value.getBoundingClientRect()
  const displayWidth = Math.round(rect.width)
  const displayHeight = Math.round(rect.height)
  
  // 仅在尺寸变化时更新，避免每帧重设导致闪烁
  if (canvasRef.value.width !== displayWidth || canvasRef.value.height !== displayHeight) {
    canvasRef.value.width = displayWidth
    canvasRef.value.height = displayHeight
  }
}

function drawDetections() {
  // Legacy entry point — now handled by interpolateAndDraw loop
  if (trackedBoxes.size > 0 && yoloEnabled.value) {
    startAnimLoop()
  }
}

function getClassColor(className: string): string {
  const colors: Record<string, string> = {
    person: '#00ff00',
    car: '#ff0000',
    truck: '#ff6600',
    bicycle: '#00ffff',
    motorcycle: '#ff00ff',
  }
  return colors[className] || '#ffff00'
}

// ============ 全屏控制 ============
async function toggleFullscreen() {
  if (!cellRef.value) return
  
  try {
    if (!document.fullscreenElement) {
      await cellRef.value.requestFullscreen()
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
  nextTick(() => updateCanvasSize())
}

// ============ 事件处理 ============
function handleClick() {
  if (!props.cameraId) {
    emit('select-slot', props.slotIndex)
  }
}

function handleRemove() {
  stopStream()
  emit('remove', props.slotIndex)
}

function toggleYolo() {
  if (!props.cameraId) return
  emit('toggle-yolo', props.cameraId, !props.isYoloEnabled)
}

function toggleVlm() {
  if (!props.cameraId) return
  emit('toggle-vlm', props.cameraId, !props.isVlmEnabled)
}

// ============ Smooth bbox interpolation engine ============
interface TrackedBox {
  bbox: [number, number, number, number]  // current (interpolated) position
  target: [number, number, number, number] // target position from latest detection
  cls: string
  confidence: number
  trackId: number
  lastSeen: number  // timestamp of last detection update
  opacity: number   // for fade-out
}

const trackedBoxes = new Map<number, TrackedBox>()
const LERP_SPEED = 0.5           // ★ 平滑插值速度 (0.5 = 中等响应)
const FADE_OUT_MS = 300          // ★ 淡出时间 300ms
let animLoopId: number | null = null

// ★ 参考 rivision_cli: 存储 YOLO 帧尺寸，用于宽高比计算
// 这是 Worker 处理的帧尺寸，与原始视频同宽高比
const yoloFrameWidth = ref(640)
const yoloFrameHeight = ref(360)
// 视频是否已加载元数据
const videoReady = ref(false)
// 调试：只输出一次绘制参数
let debugLoggedOnce = false

function startAnimLoop() {
  if (animLoopId !== null) return
  const loop = () => {
    animLoopId = requestAnimationFrame(loop)
    updateCanvasSize()
    interpolateAndDraw()
  }
  animLoopId = requestAnimationFrame(loop)
}

function stopAnimLoop() {
  if (animLoopId !== null) {
    cancelAnimationFrame(animLoopId)
    animLoopId = null
  }
}

function lerp4(a: [number, number, number, number], b: [number, number, number, number], t: number): [number, number, number, number] {
  return [
    a[0] + (b[0] - a[0]) * t,
    a[1] + (b[1] - a[1]) * t,
    a[2] + (b[2] - a[2]) * t,
    a[3] + (b[3] - a[3]) * t,
  ]
}

function interpolateAndDraw() {
  if (!canvasRef.value) return
  const ctx = canvasRef.value.getContext('2d')
  if (!ctx) return
  const w = canvasRef.value.width
  const h = canvasRef.value.height
  ctx.clearRect(0, 0, w, h)

  if (!yoloEnabled.value || trackedBoxes.size === 0) return
  
  // ★ 关键修复: 视频未加载时不绘制，避免坐标错误
  if (!videoReady.value) return
  
  // ★ 获取实际视频尺寸 (参考 rivision_cli line 732)
  const actualVideoW = videoRef.value?.videoWidth || 0
  const actualVideoH = videoRef.value?.videoHeight || 0
  if (!actualVideoW || !actualVideoH) return

  // ★ 修复: 使用实际视频尺寸计算宽高比 (不是 Worker 帧!)
  // rivision_cli: const videoAspect = yoloImageWidth.value / yoloImageHeight.value
  // yoloImageWidth/Height 是原始视频尺寸，不是 640x360
  const videoAspect = actualVideoW / actualVideoH
  const canvasAspect = w / h
  
  let videoDisplayWidth = w
  let videoDisplayHeight = h
  let offsetX = 0
  let offsetY = 0
  
  if (videoAspect > canvasAspect) {
    // 视频更宽，上下有黑边
    videoDisplayHeight = w / videoAspect
    offsetY = (h - videoDisplayHeight) / 2
  } else if (videoAspect < canvasAspect) {
    // 视频更高，左右有黑边
    videoDisplayWidth = h * videoAspect
    offsetX = (w - videoDisplayWidth) / 2
  }
  
  // ★ DEBUG: 首次绘制时输出详细信息
  if (trackedBoxes.size > 0 && !debugLoggedOnce) {
    debugLoggedOnce = true
    console.log(`[VideoCell ${props.slotIndex}] Draw params: canvas=${w}x${h}, ` +
      `video=${actualVideoW}x${actualVideoH}, ` +
      `videoAspect=${videoAspect.toFixed(4)}, canvasAspect=${canvasAspect.toFixed(4)}, ` +
      `display=${videoDisplayWidth.toFixed(1)}x${videoDisplayHeight.toFixed(1)}, ` +
      `offset=(${offsetX.toFixed(1)},${offsetY.toFixed(1)})`)
  }

  const now = performance.now()
  const toRemove: number[] = []

  trackedBoxes.forEach((box, trackId) => {
    // Interpolate toward target
    box.bbox = lerp4(box.bbox, box.target, LERP_SPEED)

    // Fade out boxes not seen recently
    const age = now - box.lastSeen
    if (age > FADE_OUT_MS) {
      toRemove.push(trackId)
      return
    }
    box.opacity = age > FADE_OUT_MS * 0.5 ? 1.0 - (age - FADE_OUT_MS * 0.5) / (FADE_OUT_MS * 0.5) : 1.0

    const [x1, y1, x2, y2] = box.bbox
    
    // ★ 防御性检查：跳过无效的 bbox (参考 rivision_cli)
    if (!Number.isFinite(x1) || !Number.isFinite(y1) || 
        !Number.isFinite(x2) || !Number.isFinite(y2)) return
    
    // ★ 使用 videoDisplay 尺寸并加上 offset
    const bx = offsetX + x1 * videoDisplayWidth
    const by = offsetY + y1 * videoDisplayHeight
    const bw = (x2 - x1) * videoDisplayWidth
    const bh = (y2 - y1) * videoDisplayHeight
    
    // ★ 跳过过小或过大的框 (更严格的过滤)
    // 1. 太小的框 (< 5px)
    // 2. 任一维度 > 80% (很可能是误检)
    // 3. 总面积 > 60% (几乎覆盖全屏)
    const boxArea = (bw / videoDisplayWidth) * (bh / videoDisplayHeight)
    const isOversized = bw > videoDisplayWidth * 0.80 || bh > videoDisplayHeight * 0.80 || boxArea > 0.60
    if (bw < 5 || bh < 5 || isOversized) {
      return
    }

    const color = getClassColor(box.cls)
    ctx.globalAlpha = box.opacity

    // Corner markers
    ctx.strokeStyle = color
    ctx.lineWidth = 2
    ctx.setLineDash([])
    const cl = Math.min(bw, bh) * 0.15
    ctx.beginPath()
    ctx.moveTo(bx, by + cl); ctx.lineTo(bx, by); ctx.lineTo(bx + cl, by)
    ctx.moveTo(bx + bw - cl, by); ctx.lineTo(bx + bw, by); ctx.lineTo(bx + bw, by + cl)
    ctx.moveTo(bx + bw, by + bh - cl); ctx.lineTo(bx + bw, by + bh); ctx.lineTo(bx + bw - cl, by + bh)
    ctx.moveTo(bx + cl, by + bh); ctx.lineTo(bx, by + bh); ctx.lineTo(bx, by + bh - cl)
    ctx.stroke()

    // Dashed border
    ctx.setLineDash([4, 4])
    ctx.strokeRect(bx, by, bw, bh)
    ctx.setLineDash([])

    // Label
    const label = `${box.cls} ${Math.round(box.confidence * 100)}%`
    ctx.font = 'bold 11px sans-serif'
    const tw = ctx.measureText(label).width
    const lh = 18
    const ly = by > lh ? by - lh : by + bh
    ctx.fillStyle = color
    ctx.beginPath()
    ctx.roundRect(bx, ly, tw + 10, lh, 3)
    ctx.fill()
    ctx.fillStyle = '#fff'
    ctx.fillText(label, bx + 5, ly + 13)
  })

  ctx.globalAlpha = 1.0
  toRemove.forEach(id => trackedBoxes.delete(id))

  // Stop loop when no boxes left and yolo disabled
  if (trackedBoxes.size === 0 && !yoloEnabled.value) {
    stopAnimLoop()
  }
}

// ============ 公开方法 ============

/**
 * ★ 坐标转换：Worker 帧坐标 → 原始视频坐标
 * 
 * 数据流:
 * 1. Worker FFmpeg: 原始视频 → letterbox 到 640×360 (保持宽高比)
 * 2. YOLO: 检测框坐标相对于 640×360 Worker 帧
 * 3. 浏览器: 显示原始视频 (非 Worker 帧!)
 * 
 * 对于 16:9 视频: Worker 帧完全匹配，无需转换
 * 对于 4:3 视频: Worker 帧有 letterbox，需要移除填充并重新归一化
 */
function transformBBox(
  bbox: [number, number, number, number],
  frameW: number, frameH: number,
  videoW: number, videoH: number
): [number, number, number, number] {
  // 视频未加载时，返回原坐标 (16:9 视频可直接使用)
  if (!videoW || !videoH || !frameW || !frameH) return bbox
  
  const frameAspect = frameW / frameH   // Worker 帧: 640/360 = 1.778
  const videoAspect = videoW / videoH   // 原始视频: 可能是 1.778 (16:9) 或 1.333 (4:3)
  
  // 宽高比相同 (如都是 16:9)，无需转换
  if (Math.abs(frameAspect - videoAspect) < 0.01) return bbox
  
  let [x1, y1, x2, y2] = bbox
  
  // 计算 Worker letterbox 参数
  let contentW = frameW, contentH = frameH
  let offsetX = 0, offsetY = 0
  
  if (videoAspect > frameAspect) {
    // 原始视频更宽 (如 21:9)，Worker 帧上下有黑边
    contentH = frameW / videoAspect
    offsetY = (frameH - contentH) / 2 / frameH
  } else {
    // 原始视频更窄 (如 4:3)，Worker 帧左右有黑边
    contentW = frameH * videoAspect
    offsetX = (frameW - contentW) / 2 / frameW
  }
  
  const scaleX = frameW / contentW
  const scaleY = frameH / contentH
  
  // 转换: 移除 Worker letterbox offset，缩放到原始视频 [0,1]
  x1 = (x1 - offsetX) * scaleX
  y1 = (y1 - offsetY) * scaleY
  x2 = (x2 - offsetX) * scaleX
  y2 = (y2 - offsetY) * scaleY
  
  // 裁剪到 [0,1] (处理边界检测)
  return [
    Math.max(0, Math.min(1, x1)),
    Math.max(0, Math.min(1, y1)),
    Math.max(0, Math.min(1, x2)),
    Math.max(0, Math.min(1, y2))
  ]
}

function setDetections(dets: Detection[], frameWidth = 640, frameHeight = 360) {
  // 存储帧尺寸
  if (frameWidth > 0 && frameHeight > 0) {
    yoloFrameWidth.value = frameWidth
    yoloFrameHeight.value = frameHeight
  }

  // 获取实际视频尺寸 (用于坐标转换)
  const videoW = videoRef.value?.videoWidth || 0
  const videoH = videoRef.value?.videoHeight || 0
  
  // ★ 视频未准备好时，跳过处理 (避免存储错误坐标)
  if (!videoW || !videoH) {
    return
  }

  // ★ 严格过滤 (参考 rivision_cli lines 372-401)
  const validDetections = dets.filter(d => {
    // 1. bbox 格式检查
    if (!d.bbox || !Array.isArray(d.bbox) || d.bbox.length !== 4) return false
    
    const [x1, y1, x2, y2] = d.bbox
    
    // 2. 数字有效性检查
    if (!Number.isFinite(x1) || !Number.isFinite(y1) || 
        !Number.isFinite(x2) || !Number.isFinite(y2)) return false
    
    // 3. 范围 [0,1] 检查
    if (x1 < 0 || y1 < 0 || x2 > 1 || y2 > 1) return false
    
    // 4. 大小检查: min 1%, max 80% (比 CLI 的 95% 更严格)
    const width = Math.abs(x2 - x1)
    const height = Math.abs(y2 - y1)
    if (width < 0.01 || height < 0.01) return false  // 太小
    if (width > 0.80 || height > 0.80) return false  // 太大
    if (width * height > 0.50) return false          // 面积太大
    
    // 5. 置信度 >= 0.3
    if (!d.confidence || d.confidence < 0.3) return false
    
    return true
  })

  // ★ 限制最多 15 个 (参考 rivision_cli)
  const limitedDets = validDetections.slice(0, 15)
  
  detections.value = limitedDets
  emit('detection', limitedDets)

  // DEBUG: 日志
  if (limitedDets.length > 0 && !debugLoggedOnce) {
    console.log(`[VideoCell ${props.slotIndex}] filtered ${dets.length}→${limitedDets.length}, ` +
      `frame=${frameWidth}x${frameHeight}, video=${videoW}x${videoH}`)
  }

  const now = performance.now()
  const seenIds = new Set<number>()

  // ★ 检查是否有有效的 track_id (参考 rivision_cli line 365)
  const hasValidTrackIds = limitedDets.some(d => d.track_id && d.track_id > 0)
  
  // ★ 如果没有有效 track_id，清空跟踪状态
  if (!hasValidTrackIds) {
    trackedBoxes.clear()
  }
  
  for (const det of limitedDets) {
    // ★ 使用真实 track_id (> 0) 或按顺序分配临时 ID
    const tid = (det.track_id && det.track_id > 0) ? det.track_id : (100000 + seenIds.size)
    seenIds.add(tid)
    
    // ★ 修复坐标顺序 (x1 > x2 或 y1 > y2)
    let [rx1, ry1, rx2, ry2] = det.bbox
    if (rx1 > rx2) [rx1, rx2] = [rx2, rx1]
    if (ry1 > ry2) [ry1, ry2] = [ry2, ry1]
    
    const fixedBbox: [number, number, number, number] = [rx1, ry1, rx2, ry2]
    
    // ★ 坐标转换: Worker 帧 → 原始视频
    const bbox = transformBBox(fixedBbox, frameWidth, frameHeight, videoW, videoH)
    
    const existing = trackedBoxes.get(tid)
    if (existing) {
      existing.target = [...bbox]
      existing.cls = det.class
      existing.confidence = det.confidence
      existing.lastSeen = now
      existing.opacity = 1.0
    } else {
      trackedBoxes.set(tid, {
        bbox: [...bbox],
        target: [...bbox],
        cls: det.class,
        confidence: det.confidence,
        trackId: tid,
        lastSeen: now,
        opacity: 1.0,
      })
    }
  }

  // When empty detections arrive, don't update lastSeen → boxes fade out via FADE_OUT_MS.
  // This works because the backend now publishes empty detection events.

  // Start animation loop if not running
  if (trackedBoxes.size > 0 && yoloEnabled.value) {
    startAnimLoop()
  }
}

defineExpose({
  getVideoElement: () => videoRef.value,
  getCanvas: () => canvasRef.value,
  setDetections,
  clearDetections: () => {
    detections.value = []
    trackedBoxes.clear()
    stopAnimLoop()
    if (canvasRef.value) {
      const ctx = canvasRef.value.getContext('2d')
      if (ctx) ctx.clearRect(0, 0, canvasRef.value.width, canvasRef.value.height)
    }
  },
  reconnect,
})

// ============ 生命周期 ============
watch(() => props.cameraId, (newId, oldId) => {
  if (newId !== oldId) {
    if (newId) {
      nextTick(() => {
        setupVideoEvents()
        startStream()
      })
    } else {
      stopStream()
    }
  }
}, { immediate: true })

watch(detections, () => {
  drawDetections()
}, { deep: true })

onMounted(() => {
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  window.addEventListener('resize', updateCanvasSize)
})

onUnmounted(() => {
  stopAnimLoop()
  stopStream()
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  window.removeEventListener('resize', updateCanvasSize)
})
</script>

<style scoped>
.video-cell {
  position: relative;
  background: #0a0a0f;
  border-radius: 12px;
  overflow: hidden;
  aspect-ratio: 16 / 9;
  border: 2px solid transparent;
  transition: all 0.3s ease;
}

.video-cell:hover {
  border-color: rgba(59, 130, 246, 0.5);
}

.video-cell.is-vlm-focus {
  border-color: #10b981;
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.3);
}

.video-cell.is-empty {
  cursor: pointer;
}

.video-cell.is-empty:hover {
  background: #12121a;
  border-color: rgba(59, 130, 246, 0.3);
}

.video-cell.is-fullscreen {
  border-radius: 0;
  aspect-ratio: auto;
}

/* 空位状态 */
.empty-slot {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-content {
  text-align: center;
  color: #4b5563;
}

.empty-icon {
  width: 48px;
  height: 48px;
  margin: 0 auto 12px;
  border: 2px dashed #374151;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-icon svg {
  width: 24px;
  height: 24px;
}

.empty-text {
  font-size: 14px;
}

.slot-indicator {
  position: absolute;
  top: 8px;
  left: 8px;
  width: 24px;
  height: 24px;
  background: rgba(0, 0, 0, 0.5);
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  color: #9ca3af;
}

/* 视频播放器 - 绝对定位确保与 canvas 对齐 */
.video-player {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #000;
}

.video-iframe {
  border: none;
}

/* 检测叠加层 - 填满容器，绘制时计算 offset (参考 rivision_cli) */
.detection-overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 5;
  /* GPU 合成提示 */
  will-change: contents;
  contain: strict;
}

/* 视频信息叠加层 */
.video-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 12px;
  pointer-events: none;
  background: linear-gradient(
    to bottom,
    rgba(0, 0, 0, 0.6) 0%,
    transparent 30%,
    transparent 70%,
    rgba(0, 0, 0, 0.6) 100%
  );
  opacity: 0;
  transition: opacity 0.3s ease;
}

.video-cell:hover .video-overlay {
  opacity: 1;
}

.overlay-top, .overlay-bottom {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.camera-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.live-badge {
  font-size: 11px;
  font-weight: 600;
  color: #10b981;
  padding: 2px 6px;
  background: rgba(16, 185, 129, 0.2);
  border-radius: 4px;
}

.live-badge.offline {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.2);
}

.camera-name {
  font-size: 13px;
  font-weight: 500;
  color: #fff;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
}

.overlay-actions {
  display: flex;
  gap: 6px;
  pointer-events: auto;
}

.action-btn {
  width: 28px;
  height: 28px;
  border: none;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  transition: all 0.2s ease;
}

.action-btn svg {
  width: 16px;
  height: 16px;
}

.action-btn:hover {
  background: rgba(255, 255, 255, 0.2);
}

.action-btn.remove-btn:hover {
  background: rgba(239, 68, 68, 0.5);
}

.ai-toggle-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  background: rgba(0, 0, 0, 0.4);
  color: rgba(255, 255, 255, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.2);
  cursor: pointer;
  transition: all 0.2s ease;
}

.ai-toggle-btn svg {
  width: 12px;
  height: 12px;
}

.ai-toggle-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.ai-toggle-btn.active {
  background: #3b82f6;
  color: #fff;
  border-color: #3b82f6;
}

.ai-toggle-btn.yolo-toggle.active {
  background: #f59e0b;
  border-color: #f59e0b;
}

.ai-toggle-btn.vlm-toggle.active {
  background: #10b981;
  border-color: #10b981;
}

.ai-toggle-btn.vlm-focus {
  animation: pulse 2s infinite;
}

.vlm-badge {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border: none;
  background: rgba(16, 185, 129, 0.2);
  border-radius: 6px;
  font-size: 11px;
  font-weight: 600;
  color: #10b981;
  cursor: default;
}

.vlm-badge svg {
  width: 12px;
  height: 12px;
}

.vlm-badge.vlm-focus {
  background: #10b981;
  color: #fff;
  animation: pulse 2s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.7; }
}

/* 底部状态栏 */
.detection-stats {
  display: flex;
  align-items: center;
  gap: 6px;
}

.detection-badge {
  font-size: 10px;
  font-weight: 700;
  color: #000;
  background: #10b981;
  padding: 2px 5px;
  border-radius: 3px;
}

.detection-count {
  font-size: 12px;
  color: #fff;
}

.analyzing-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #10b981;
}

.analyzing-dot {
  width: 6px;
  height: 6px;
  background: #10b981;
  border-radius: 50%;
  animation: blink 1s infinite;
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.stream-stats {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.7);
  background: rgba(0, 0, 0, 0.5);
  padding: 2px 6px;
  border-radius: 4px;
}

/* 加载状态 */
.loading-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.7);
  gap: 12px;
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(255, 255, 255, 0.2);
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.loading-text {
  font-size: 13px;
  color: #9ca3af;
}

/* 错误状态 */
.error-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.85);
  gap: 12px;
}

.error-icon {
  font-size: 36px;
}

.error-text {
  font-size: 14px;
  color: #ef4444;
}

.error-actions {
  display: flex;
  gap: 8px;
}

.retry-btn, .remove-btn-small {
  padding: 6px 12px;
  border: none;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.retry-btn {
  background: #3b82f6;
  color: #fff;
}

.retry-btn:hover {
  background: #2563eb;
}

.remove-btn-small {
  background: rgba(255, 255, 255, 0.1);
  color: #9ca3af;
}

.remove-btn-small:hover {
  background: rgba(239, 68, 68, 0.3);
  color: #ef4444;
}
</style>
