// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

/**
 * YOLO 检测服务 - 预留接口
 * 
 * 用于集成 YOLO 目标检测功能，支持：
 * - 行人检测 (person)
 * - 车辆检测 (vehicle)
 * - 物体检测 (object)
 * - 安全帽检测 (helmet)
 * - 自定义模型 (custom)
 */

// ============ 类型定义 ============

export interface YoloDetection {
  /** 类别名称 */
  class: string
  /** 置信度 0-1 */
  confidence: number
  /** 边界框 [x1, y1, x2, y2] 归一化坐标 (0-1) */
  bbox: [number, number, number, number]
  /** 跟踪 ID (可选，用于目标跟踪) */
  trackId?: number
}

export interface YoloConfig {
  /** 模型类型 */
  model: 'person' | 'vehicle' | 'object' | 'helmet' | 'custom'
  /** 置信度阈值 */
  confidence: number
  /** 是否启用 */
  enabled: boolean
  /** 自定义模型路径 (仅 custom 模式) */
  customModelPath?: string
  /** 检测间隔 (毫秒) */
  interval?: number
}

export interface YoloResponse {
  /** 检测结果列表 */
  detections: YoloDetection[]
  /** 处理时间 (毫秒) */
  processingTime: number
  /** 模型名称 */
  modelName: string
}

// ============ 默认配置 ============

export const DEFAULT_YOLO_CONFIG: YoloConfig = {
  model: 'person',
  confidence: 0.5,
  enabled: false,
  interval: 100, // 100ms = 10fps
}

// ============ 颜色映射 ============

const CLASS_COLORS: Record<string, string> = {
  person: '#00ff00',
  car: '#ff0000',
  truck: '#ff6600',
  bus: '#ff9900',
  bicycle: '#00ffff',
  motorcycle: '#ff00ff',
  dog: '#ffff00',
  cat: '#ff66ff',
  helmet: '#00ff66',
  no_helmet: '#ff3333',
  default: '#ffffff',
}

export function getClassColor(className: string): string {
  return CLASS_COLORS[className.toLowerCase()] || CLASS_COLORS.default
}

// ============ API 接口 (预留) ============

/**
 * 发送图像进行 YOLO 检测
 * 
 * @param imageBase64 - Base64 编码的图像数据
 * @param config - YOLO 配置
 * @returns 检测结果
 * 
 * @example
 * ```typescript
 * const result = await detectYolo(imageData, {
 *   model: 'person',
 *   confidence: 0.5,
 *   enabled: true,
 * })
 * console.log(`检测到 ${result.detections.length} 个目标`)
 * ```
 */
export async function detectYolo(
  imageBase64: string,
  config: YoloConfig
): Promise<YoloResponse> {
  // TODO: 实现 YOLO 检测 API 调用
  // POST /api/yolo/detect
  // {
  //   image_base64: string,
  //   model: string,
  //   confidence: number,
  // }
  
  console.warn('[YOLO] Service not implemented yet, image size:', imageBase64.length)
  
  // 返回空结果
  return {
    detections: [],
    processingTime: 0,
    modelName: config.model,
  }
}

/**
 * 获取可用的 YOLO 模型列表
 */
export async function getAvailableModels(): Promise<string[]> {
  // TODO: 实现获取模型列表 API
  // GET /api/yolo/models
  
  return ['person', 'vehicle', 'object', 'helmet']
}

// ============ Canvas 绘制 ============

/**
 * 在 Canvas 上绘制检测框
 * 
 * @param canvas - Canvas 元素
 * @param detections - 检测结果列表
 * @param options - 绘制选项
 */
export function drawDetections(
  canvas: HTMLCanvasElement,
  detections: YoloDetection[],
  options: {
    lineWidth?: number
    fontSize?: number
    showLabel?: boolean
    showConfidence?: boolean
  } = {}
): void {
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const {
    lineWidth = 2,
    fontSize = 12,
    showLabel = true,
    showConfidence = true,
  } = options

  const width = canvas.width
  const height = canvas.height

  // 清空画布
  ctx.clearRect(0, 0, width, height)

  // 绘制每个检测框
  detections.forEach((det) => {
    const [x1, y1, x2, y2] = det.bbox
    const x = x1 * width
    const y = y1 * height
    const w = (x2 - x1) * width
    const h = (y2 - y1) * height

    const color = getClassColor(det.class)

    // 绘制边框
    ctx.strokeStyle = color
    ctx.lineWidth = lineWidth
    ctx.strokeRect(x, y, w, h)

    // 绘制标签背景和文字
    if (showLabel) {
      const label = showConfidence
        ? `${det.class} ${Math.round(det.confidence * 100)}%`
        : det.class

      ctx.font = `${fontSize}px sans-serif`
      const textWidth = ctx.measureText(label).width
      const labelHeight = fontSize + 6

      // 标签背景
      ctx.fillStyle = color
      ctx.fillRect(x, y - labelHeight, textWidth + 8, labelHeight)

      // 标签文字
      ctx.fillStyle = '#000'
      ctx.fillText(label, x + 4, y - 4)
    }

    // 绘制跟踪 ID
    if (det.trackId !== undefined) {
      const trackLabel = `#${det.trackId}`
      ctx.font = `bold ${fontSize}px sans-serif`
      ctx.fillStyle = color
      ctx.fillText(trackLabel, x + 4, y + fontSize + 4)
    }
  })
}

/**
 * 清空 Canvas
 */
export function clearCanvas(canvas: HTMLCanvasElement): void {
  const ctx = canvas.getContext('2d')
  if (ctx) {
    ctx.clearRect(0, 0, canvas.width, canvas.height)
  }
}

// ============ 实时检测控制器 (预留) ============

export interface YoloController {
  start: () => void
  stop: () => void
  isRunning: () => boolean
  setConfig: (config: Partial<YoloConfig>) => void
  onDetection: (callback: (detections: YoloDetection[]) => void) => void
}

/**
 * 创建 YOLO 实时检测控制器
 * 
 * @param video - 视频元素
 * @param canvas - Canvas 元素 (用于绘制检测框)
 * @param config - 初始配置
 */
export function createYoloController(
  video: HTMLVideoElement,
  canvas: HTMLCanvasElement,
  config: YoloConfig = DEFAULT_YOLO_CONFIG
): YoloController {
  let running = false
  let currentConfig = { ...config }
  let detectionCallback: ((detections: YoloDetection[]) => void) | null = null
  let intervalId: number | null = null

  // ★ B6: 复用Canvas（避免每次新建~1.8MB像素缓冲）
  const _yoloCaptureCanvas = document.createElement('canvas')
  const _yoloCaptureCtx = _yoloCaptureCanvas.getContext('2d')!

  const captureFrame = (): string => {
    const w = video.videoWidth || 640
    const h = video.videoHeight || 360
    if (_yoloCaptureCanvas.width !== w || _yoloCaptureCanvas.height !== h) {
      _yoloCaptureCanvas.width = w
      _yoloCaptureCanvas.height = h
    }
    _yoloCaptureCtx.drawImage(video, 0, 0)
    return _yoloCaptureCanvas.toDataURL('image/jpeg', 0.8).replace(/^data:image\/\w+;base64,/, '')
  }

  const runDetection = async () => {
    if (!running || !currentConfig.enabled) return

    try {
      const imageData = captureFrame()
      const result = await detectYolo(imageData, currentConfig)
      
      // 绘制检测框
      drawDetections(canvas, result.detections)
      
      // 触发回调
      if (detectionCallback) {
        detectionCallback(result.detections)
      }
    } catch (e) {
      console.error('[YOLO] Detection error:', e)
    }
  }

  return {
    start: () => {
      if (running) return
      running = true
      const interval = currentConfig.interval || 100
      intervalId = window.setInterval(runDetection, interval)
      console.log('[YOLO] Controller started')
    },

    stop: () => {
      running = false
      if (intervalId) {
        clearInterval(intervalId)
        intervalId = null
      }
      clearCanvas(canvas)
      console.log('[YOLO] Controller stopped')
    },

    isRunning: () => running,

    setConfig: (newConfig: Partial<YoloConfig>) => {
      currentConfig = { ...currentConfig, ...newConfig }
      
      // 如果间隔改变，重启定时器
      if (newConfig.interval && running) {
        if (intervalId) clearInterval(intervalId)
        intervalId = window.setInterval(runDetection, currentConfig.interval || 100)
      }
    },

    onDetection: (callback: (detections: YoloDetection[]) => void) => {
      detectionCallback = callback
    },
  }
}
