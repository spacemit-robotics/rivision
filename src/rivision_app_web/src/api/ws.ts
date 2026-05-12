// WebSocket client for real-time event stream (§8.2)
// Endpoint: /api/v1/ws/stream

type EventHandler = (event: any) => void

export class WsClient {
  private ws: WebSocket | null = null
  private handlers: Map<string, EventHandler[]> = new Map()
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private url: string

  constructor(url?: string) {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.url = url || `${proto}//${window.location.host}/api/v1/ws/stream`
  }

  connect() {
    if (this.ws?.readyState === WebSocket.OPEN) return

    this.ws = new WebSocket(this.url)

    this.ws.onopen = () => {
      console.log('[ws] connected')
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer)
        this.reconnectTimer = null
      }
    }

    this.ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data)
        const type = msg.type || 'unknown'
        const cbs = this.handlers.get(type) || []
        cbs.forEach((cb) => cb(msg))
        // Also fire wildcard listeners
        const wildcards = this.handlers.get('*') || []
        wildcards.forEach((cb) => cb(msg))
      } catch (e) {
        console.warn('[ws] parse error', e)
      }
    }

    this.ws.onclose = () => {
      console.log('[ws] closed, reconnecting in 3s')
      this.reconnectTimer = setTimeout(() => this.connect(), 3000)
    }

    this.ws.onerror = (err) => {
      console.error('[ws] error', err)
      this.ws?.close()
    }
  }

  on(type: string, handler: EventHandler) {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, [])
    }
    this.handlers.get(type)!.push(handler)
  }

  off(type: string, handler: EventHandler) {
    const cbs = this.handlers.get(type)
    if (cbs) {
      this.handlers.set(type, cbs.filter((cb) => cb !== handler))
    }
  }

  close() {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer)
    this.ws?.close()
    this.ws = null
  }
}

export const wsClient = new WsClient()
