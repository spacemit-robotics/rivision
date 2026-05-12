// WebSocket message types (§8.2)

export interface WsMessage {
  type: WsMessageType
  data: any
  timestamp: string
}

export type WsMessageType =
  | 'alert'
  | 'detection'
  | 'node_status'
  | 'camera_status'
  | 'analytics_update'
  | 'knowledge_sync'

export interface AlertEvent {
  type: 'alert'
  data: {
    id: string
    severity: string
    message: string
    camera_id: string
    rule_id: string
    thumbnail?: string
  }
}

export interface DetectionEvent {
  type: 'detection'
  data: {
    camera_id: string
    class_name: string
    confidence: number
    track_id?: number
  }
}

export interface NodeStatusEvent {
  type: 'node_status'
  data: {
    node_id: string
    status: 'online' | 'offline'
    cpu_percent?: number
    mem_percent?: number
  }
}

export interface CameraStatusEvent {
  type: 'camera_status'
  data: {
    camera_id: string
    node_id: string
    status: 'online' | 'offline' | 'error'
  }
}
