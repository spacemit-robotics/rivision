// YOLO WebSocket 服务 - 连接后端 YOLO 检测结果推送
import { ref, reactive } from 'vue'

export interface YoloDetection {
  class_id: number
  class_name: string
  confidence: number
  bbox: [number, number, number, number]  // [x1, y1, x2, y2] 像素坐标
  track_id?: number
  velocity_x?: number  // X方向速度（像素/秒）
  velocity_y?: number  // Y方向速度（像素/秒）
}

export interface YoloMessage {
  type: string
  timestamp: string
  camera_id: string
  data: {
    detections: YoloDetection[]
    inference_time_ms: number
    frame_index: number
    frame_time: number     // 帧时间戳（秒）
    image_width?: number
    image_height?: number
    frame_base64?: string  // 检测帧的 Base64 编码
    use_binary?: boolean   // 是否使用二进制传输
  }
}

export interface YoloConnectionState {
  connected: boolean
  cameraId: string | null
  lastUpdate: Date | null
  detectionCount: number
  inferenceTimeMs: number
}

// 回调函数类型
export type YoloCallback = (
  detections: YoloDetection[],
  imageWidth?: number,
  imageHeight?: number,
  frameBase64?: string,
  frameTime?: number
) => void

// ★ 全局监听器：用于 YOLO→VLM 触发等跨组件功能
export type YoloGlobalListener = (
  cameraId: string,
  detections: YoloDetection[],
  inferenceTimeMs: number
) => void

// ★ 多路复用 YOLO WebSocket 服务
// 使用单一 WebSocket 连接处理所有摄像头的 YOLO 数据
// 解决浏览器每域名 TCP 连接数限制（6）导致第3+摄像头无法连接的问题
class YoloWebSocketService {
  private ws: WebSocket | null = null
  private wsUrl: string = ''
  private subscriptions: Set<string> = new Set()        // 当前已订阅的摄像头
  private callbacks: Map<string, YoloCallback> = new Map()
  private reconnectTimer: number | null = null
  private connected: boolean = false
  
  // 临时存储等待二进制帧的JSON消息（camera_id → message）
  private pendingBinary: { cameraId: string, message: YoloMessage, timer: number | null } | null = null
  private globalListeners: Set<YoloGlobalListener> = new Set()

  public state = reactive<Record<string, YoloConnectionState>>({})

  // 确保 WebSocket 连接存在
  private ensureConnection(): void {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return
    }

    this.wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws/yolo`
    console.log(`[YOLO WS MUX] Connecting to ${this.wsUrl}`)

    const ws = new WebSocket(this.wsUrl)
    this.ws = ws

    ws.onopen = () => {
      if (this.ws !== ws) return
      console.log(`[YOLO WS MUX] Connected, re-subscribing ${this.subscriptions.size} cameras`)
      this.connected = true
      // 重新订阅所有摄像头
      for (const cameraId of this.subscriptions) {
        this.sendCommand('subscribe', cameraId)
        if (this.state[cameraId]) {
          this.state[cameraId].connected = true
        }
      }
      this.clearReconnectTimer()
    }

    ws.onmessage = (event) => {
      try {
        // 二进制消息（sync模式帧数据）
        if (event.data instanceof Blob) {
          if (this.pendingBinary && this.pendingBinary.message.data?.use_binary) {
            if (this.pendingBinary.timer) { clearTimeout(this.pendingBinary.timer); this.pendingBinary.timer = null }
            const { cameraId, message } = this.pendingBinary
            this.pendingBinary = null

            // ★ 使用 readAsDataURL 直接获取 base64（比逐字节拼接快 10-50×）
            const reader = new FileReader()
            reader.onload = () => {
              // dataURL 格式: "data:application/octet-stream;base64,XXXXX"
              const dataUrl = reader.result as string
              const commaIdx = dataUrl.indexOf(',')
              const frameBase64 = commaIdx >= 0 ? dataUrl.substring(commaIdx + 1) : dataUrl
              const callback = this.callbacks.get(cameraId)
              if (callback) {
                const data = message.data
                callback(data.detections || [], data.image_width, data.image_height, frameBase64, data.frame_time)
              }
            }
            reader.readAsDataURL(event.data)
          }
          return
        }

        // JSON消息 — 根据 camera_id 分发到对应回调
        const message: YoloMessage = JSON.parse(event.data)
        if (message.type === 'yolo.detection' && message.data) {
          const cameraId = message.camera_id
          if (!cameraId || !this.subscriptions.has(cameraId)) return

          const detections = message.data.detections || []
          const imageWidth = message.data.image_width
          const imageHeight = message.data.image_height

          // 更新状态
          if (this.state[cameraId]) {
            this.state[cameraId].lastUpdate = new Date()
            this.state[cameraId].detectionCount = detections.length
            this.state[cameraId].inferenceTimeMs = message.data.inference_time_ms
          }

          // ★ 通知全局监听器（YOLO→VLM 触发等）
          this.notifyGlobalListeners(cameraId, detections, message.data.inference_time_ms)

          const callback = this.callbacks.get(cameraId)

          if (message.data.use_binary) {
            // 先发送检测数据（smooth模式），然后等待二进制帧（sync模式）
            if (callback) {
              callback(detections, imageWidth, imageHeight, undefined, message.data.frame_time)
            }
            this.pendingBinary = { cameraId, message, timer: null }
            this.pendingBinary.timer = window.setTimeout(() => {
              this.pendingBinary = null
            }, 2000)
          } else {
            if (callback) {
              callback(detections, imageWidth, imageHeight, message.data.frame_base64, message.data.frame_time)
            }
          }
        }
      } catch (e) {
        console.error(`[YOLO WS MUX] parse error:`, e)
      }
    }

    ws.onerror = (error) => {
      console.error(`[YOLO WS MUX] Error:`, error)
    }

    ws.onclose = () => {
      if (this.ws !== ws) return
      console.log(`[YOLO WS MUX] Disconnected`)
      this.ws = null
      this.connected = false
      // 更新所有订阅状态为断开
      for (const cameraId of this.subscriptions) {
        if (this.state[cameraId]) {
          this.state[cameraId].connected = false
        }
      }
      // 如果还有订阅，自动重连
      if (this.subscriptions.size > 0) {
        this.scheduleReconnect()
      }
    }
  }

  // 发送订阅/取消订阅命令
  private sendCommand(action: string, cameraId: string): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ action, camera_id: cameraId }))
    }
  }

  // 订阅摄像头（对外API保持 connect/disconnect 兼容）
  connect(cameraId: string, onDetections: YoloCallback): void {
    if (this.subscriptions.has(cameraId)) {
      // 更新回调（可能组件重新挂载）
      this.callbacks.set(cameraId, onDetections)
      return
    }

    console.log(`[YOLO WS MUX] Subscribe ${cameraId}`)
    this.subscriptions.add(cameraId)
    this.callbacks.set(cameraId, onDetections)

    this.state[cameraId] = {
      connected: this.connected,
      cameraId,
      lastUpdate: null,
      detectionCount: 0,
      inferenceTimeMs: 0
    }

    // 确保连接存在
    this.ensureConnection()

    // 如果已连接，立即发送订阅
    if (this.connected) {
      this.sendCommand('subscribe', cameraId)
    }
  }

  // 取消订阅摄像头
  disconnect(cameraId: string): void {
    if (!this.subscriptions.has(cameraId)) return

    console.log(`[YOLO WS MUX] Unsubscribe ${cameraId}`)
    this.subscriptions.delete(cameraId)
    this.callbacks.delete(cameraId)
    delete this.state[cameraId]

    // 发送取消订阅
    this.sendCommand('unsubscribe', cameraId)

    // 如果没有订阅了，关闭连接
    if (this.subscriptions.size === 0 && this.ws) {
      this.ws.close()
      this.ws = null
      this.connected = false
      this.clearReconnectTimer()
    }
  }

  // 断开所有
  disconnectAll(): void {
    for (const cameraId of [...this.subscriptions]) {
      this.disconnect(cameraId)
    }
  }

  // 检查是否已订阅
  isConnected(cameraId: string): boolean {
    return this.subscriptions.has(cameraId)
  }

  // 获取连接状态
  getState(cameraId: string): YoloConnectionState | null {
    return this.state[cameraId] || null
  }

  // ★ 全局监听器管理（用于 YOLO→VLM 触发）
  addGlobalListener(listener: YoloGlobalListener): () => void {
    this.globalListeners.add(listener)
    return () => this.globalListeners.delete(listener)
  }

  // ★ 公开：前端 DirectDetector 也需要通知全局监听器（YOLO→VLM 触发）
  // 禁用后端抓帧后，WebSocket 不再产生检测结果，全局监听器依赖此方法
  notifyGlobalListeners(cameraId: string, detections: YoloDetection[], inferenceTimeMs: number): void {
    for (const listener of this.globalListeners) {
      try {
        listener(cameraId, detections, inferenceTimeMs)
      } catch (e) {
        console.error('[YOLO WS MUX] Global listener error:', e)
      }
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null
      if (this.subscriptions.size > 0) {
        console.log(`[YOLO WS MUX] Reconnecting...`)
        this.ensureConnection()
      }
    }, 3000)
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }
}

// 导出单例
export const yoloWebSocket = new YoloWebSocketService()

// 组合式函数
export function useYoloWebSocket(cameraId: string) {
  const detections = ref<YoloDetection[]>([])
  const connected = ref(false)
  const inferenceTimeMs = ref(0)

  const connect = () => {
    yoloWebSocket.connect(cameraId, (newDetections) => {
      detections.value = newDetections
      const state = yoloWebSocket.getState(cameraId)
      if (state) {
        connected.value = state.connected
        inferenceTimeMs.value = state.inferenceTimeMs
      }
    })
  }

  const disconnect = () => {
    yoloWebSocket.disconnect(cameraId)
    detections.value = []
    connected.value = false
  }

  return {
    detections,
    connected,
    inferenceTimeMs,
    connect,
    disconnect
  }
}
