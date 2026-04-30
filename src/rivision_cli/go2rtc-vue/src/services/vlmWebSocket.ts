// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// VLM WebSocket 服务 - 连接后端 VLM 推理结果推送
import { ref, reactive } from 'vue'

export interface VlmResult {
  id: string
  camera_id: string
  camera_name: string
  trigger_mode: string
  text: string
  tokens_used: number
  node_id: string
  inference_time_ms: number
  image_base64?: string
  timestamp: string
}

export interface VlmMessage {
  type: string
  timestamp: string
  camera_id?: string
  data: VlmResult
}

export interface VlmConnectionState {
  connected: boolean
  lastResult: VlmResult | null
  resultCount: number
}

class VlmWebSocketService {
  private ws: WebSocket | null = null
  private cameraWs: Map<string, WebSocket> = new Map()
  private callbacks: ((result: VlmResult) => void)[] = []
  private cameraCallbacks: Map<string, (result: VlmResult) => void> = new Map()
  private reconnectTimer: number | null = null

  public state = reactive<VlmConnectionState>({
    connected: false,
    lastResult: null,
    resultCount: 0
  })

  // 连接到全局 VLM WebSocket（接收所有摄像头的结果）
  connect(onResult?: (result: VlmResult) => void): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      console.log('[VLM WS] Already connected')
      if (onResult) this.callbacks.push(onResult)
      return
    }

    const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws/vlm`
    console.log(`[VLM WS] Connecting to ${wsUrl}`)

    this.ws = new WebSocket(wsUrl)
    if (onResult) this.callbacks.push(onResult)

    this.ws.onopen = () => {
      console.log('[VLM WS] Connected')
      this.state.connected = true
      this.clearReconnectTimer()
    }

    this.ws.onmessage = (event) => {
      try {
        const message: VlmMessage = JSON.parse(event.data)
        if (message.type === 'vlm.result' && message.data) {
          const result = message.data
          result.timestamp = message.timestamp

          // 更新状态
          this.state.lastResult = result
          this.state.resultCount++

          // 调用所有回调
          this.callbacks.forEach(cb => cb(result))
        }
      } catch (e) {
        console.error('[VLM WS] Parse error:', e)
      }
    }

    this.ws.onerror = (error) => {
      console.error('[VLM WS] Error:', error)
    }

    this.ws.onclose = () => {
      console.log('[VLM WS] Disconnected')
      this.state.connected = false
      this.ws = null
      this.scheduleReconnect()
    }
  }

  // 连接到特定摄像头的 VLM WebSocket
  connectCamera(cameraId: string, onResult: (result: VlmResult) => void): void {
    if (this.cameraWs.has(cameraId)) {
      console.log(`[VLM WS] Already connected to camera ${cameraId}`)
      return
    }

    const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws/vlm/${cameraId}`
    console.log(`[VLM WS] Connecting to camera ${cameraId}`)

    const ws = new WebSocket(wsUrl)
    this.cameraWs.set(cameraId, ws)
    this.cameraCallbacks.set(cameraId, onResult)

    ws.onopen = () => {
      console.log(`[VLM WS] Connected to camera ${cameraId}`)
    }

    ws.onmessage = (event) => {
      try {
        const message: VlmMessage = JSON.parse(event.data)
        if (message.type === 'vlm.result' && message.data) {
          const result = message.data
          result.timestamp = message.timestamp
          const callback = this.cameraCallbacks.get(cameraId)
          if (callback) callback(result)
        }
      } catch (e) {
        console.error(`[VLM WS] Parse error for camera ${cameraId}:`, e)
      }
    }

    ws.onclose = () => {
      console.log(`[VLM WS] Disconnected from camera ${cameraId}`)
      this.cameraWs.delete(cameraId)
    }
  }

  // 断开全局连接
  disconnect(): void {
    this.clearReconnectTimer()
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.callbacks = []
    this.state.connected = false
  }

  // 断开特定摄像头连接
  disconnectCamera(cameraId: string): void {
    const ws = this.cameraWs.get(cameraId)
    if (ws) {
      ws.close()
      this.cameraWs.delete(cameraId)
    }
    this.cameraCallbacks.delete(cameraId)
  }

  // 断开所有连接
  disconnectAll(): void {
    this.disconnect()
    for (const cameraId of this.cameraWs.keys()) {
      this.disconnectCamera(cameraId)
    }
  }

  // 添加结果回调
  onResult(callback: (result: VlmResult) => void): () => void {
    this.callbacks.push(callback)
    return () => {
      const index = this.callbacks.indexOf(callback)
      if (index > -1) this.callbacks.splice(index, 1)
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return
    if (this.callbacks.length === 0) return

    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null
      console.log('[VLM WS] Reconnecting...')
      this.connect()
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
export const vlmWebSocket = new VlmWebSocketService()

// 组合式函数
export function useVlmWebSocket() {
  const results = ref<VlmResult[]>([])
  const connected = ref(false)
  const lastResult = ref<VlmResult | null>(null)

  const connect = () => {
    vlmWebSocket.connect((result) => {
      lastResult.value = result
      results.value.unshift(result)
      // 保留最近 50 条结果
      if (results.value.length > 50) {
        results.value = results.value.slice(0, 50)
      }
    })
    connected.value = vlmWebSocket.state.connected
  }

  const disconnect = () => {
    vlmWebSocket.disconnect()
    connected.value = false
  }

  return {
    results,
    connected,
    lastResult,
    connect,
    disconnect
  }
}
