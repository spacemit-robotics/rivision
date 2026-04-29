/**
 * Gateway API 类型定义
 */

// 节点信息
export interface NodeInfo {
  id: string
  host: string
  port: number
  healthy: boolean
  enabled?: boolean
  connections: number
  weight?: number
  tags?: string[]
  fail_count?: number
  last_check?: string
  last_heartbeat?: string
  load?: number
  cpu_percent?: number
  memory_percent?: number
  llama_healthy?: boolean
  yolo_healthy?: boolean
  slots_idle?: number
  slots_processing?: number
  // 兼容旧字段
  activeTasks?: number
  maxTasks?: number
}

// YOLO Pipeline 统计
export interface YOLOPipelineStats {
  active_workers: number
  desired_workers: number
  healthy_nodes: number
  total_frames: number
  total_errors: number
  sync_frame_rate: number
  enabled_cameras: number
}

// 系统统计
export interface SystemStats {
  totalRequests: number
  successRate: number
  averageResponseTime: number
  healthyNodes: number
  totalNodes: number
  queueLength?: number
  completedTasks?: number
  failedTasks?: number
}

// 任务信息
export interface TaskInfo {
  id: string
  type: 'image' | 'video' | 'search' | 'summary' | 'detection'
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled'
  progress: number
  createdAt: string
  startedAt?: string
  completedAt?: string
  nodeId?: string
  error?: string
  result?: any
  metadata?: {
    inputFile?: string
    query?: string
    frameCount?: number
    duration?: number
  }
}

// 分析配置
export interface AnalysisConfig {
  type: 'search' | 'summary' | 'detection'
  query?: string
  objects?: string[]
  confidenceThreshold?: number
  frameInterval?: number
  maxResults?: number
}

// 视频分析结果
export interface AnalysisResult {
  id: string
  type?: string
  timestamp: number
  frameIndex: number
  confidence: number
  description: string
  content: string
  matched: boolean
  thumbnailUrl?: string
  metadata?: Record<string, any>
}

// 节点类型 (用于 NodeManagement)
export interface Node extends NodeInfo {
  status?: string
}

// 任务类型 (用于 TaskMonitor)
export interface Task extends TaskInfo {
  task_id?: string
}

// 活动类型 (用于 SystemDashboard)
export interface Activity {
  id: string
  type: 'info' | 'success' | 'warning' | 'error'
  title: string
  message: string
  timestamp: string
  icon: string
}

// 状态类型
export type Status = 'idle' | 'uploading' | 'processing' | 'analyzing' | 'completed' | 'error'

// API响应基础类型
export interface ApiResponse<T = any> {
  success: boolean
  data?: T
  error?: string
  message?: string
}

// WebSocket消息类型
export interface WebSocketMessage {
  type: 'node_status_update' | 'task_status_update' | 'stats_update' | 'analysis_result'
  data: any
  timestamp: string
}

// 视频上传状态
export interface UploadStatus {
  status: 'idle' | 'uploading' | 'processing' | 'completed' | 'error'
  progress: number
  message?: string
  error?: string
}

// 性能指标
export interface PerformanceMetrics {
  timestamp: string
  cpuUsage: number
  memoryUsage: number
  requestsPerSecond: number
  responseTime: number
  activeConnections: number
  queueLength: number
}

// 导出选项
export interface ExportOptions {
  format: 'json' | 'csv' | 'txt'
  includeMetadata: boolean
  includeFullContent: boolean
  timeRange?: {
    start: number
    end: number
  }
}
