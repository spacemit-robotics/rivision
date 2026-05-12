// Domain model types (§8.2)

export interface Detection {
  id: string
  class_id: number
  class_name: string
  confidence: number
  bbox: { x1: number; y1: number; x2: number; y2: number }
  track_id?: number
  camera_id: string
  timestamp: string
}

export interface Alert {
  id: string
  severity: 'critical' | 'warning' | 'info'
  status: 'pending' | 'acknowledged' | 'resolved'
  message: string
  rule_id: string
  camera_id: string
  thumbnail?: string
  created_at: string
  updated_at?: string
}

export type StreamMode = 'hub_sip_remote' | 'direct' | 'worker_owl' | ''

export interface Camera {
  id: string
  name: string
  url: string
  protocol?: string
  group_id?: string
  node_id?: string
  status: 'online' | 'offline' | 'error'
  enabled: boolean
  config?: Record<string, any>
  stream_mode?: StreamMode
  owl_channel_id?: string
}

export interface Node {
  id: string
  host: string
  port: number
  status: 'online' | 'offline' | 'degraded'
  active_streams: number
  max_streams: number
  cpu_percent?: number
  mem_percent?: number
  disk_percent?: number
  last_heartbeat?: string
}

export interface Rule {
  id: string
  name: string
  description?: string
  rule_yaml: string
  enabled: boolean
  camera_ids?: string[]
  created_at: string
}

export interface KnowledgeBase {
  id: string
  name: string
  type: 'rules' | 'features' | 'prompts'
  version: number
  entries_count: number
}
