/**
 * 自动触发控制器
 * 针对情况3 (A << B) 的智能触发策略
 * 
 * 策略：队列感知 + 运动检测 + 定时兜底
 */

export interface AutoTriggerConfig {
  // 运动检测配置
  motionThreshold: number      // 运动检测阈值 (0-1)，默认0.05
  motionSampleInterval: number // 运动检测采样间隔(ms)，默认500
  
  // 队列感知配置
  maxQueueSize: number         // 最大队列长度，超过则暂停，默认1
  
  // 定时兜底配置
  maxIdleInterval: number      // 最大空闲间隔(ms)，超过则强制采样，默认30000
  minAnalysisInterval: number  // 最小分析间隔(ms)，防止过于频繁，默认5000
  
  // 模式
  enabled: boolean             // 是否启用自动触发
  motionDetectionEnabled: boolean // 是否启用运动检测
  
  // 流水线并发配置
  pipelineMode: boolean        // 是否启用流水线模式
  nodeCount: number            // 在线节点数，用于并发任务数
}

export interface TriggerEvent {
  type: 'motion' | 'idle' | 'manual'
  timestamp: number
  motionScore?: number
}

type TriggerCallback = (imageData: string, event: TriggerEvent) => void

export class AutoTriggerController {
  private config: AutoTriggerConfig
  private videoElement: HTMLVideoElement | null = null
  private canvas: HTMLCanvasElement
  private ctx: CanvasRenderingContext2D
  private previousFrame: ImageData | null = null
  
  // 状态
  private isRunning: boolean = false
  private lastAnalysisTime: number = 0
  private lastMotionTime: number = 0
  private currentQueueSize: number = 0
  
  // 定时器
  private motionDetectionTimer: number | null = null
  private idleCheckTimer: number | null = null
  
  // 回调
  private onTrigger: TriggerCallback | null = null
  private onMotionDetected: ((score: number) => void) | null = null

  // 流水线状态
  private activeTaskCount: number = 0
  private pipelineInitialized: boolean = false
  
  // ★ 503退避：防止503定时器链式累积导致事件循环饿死
  private _503BackoffUntil: number = 0
  private _503BackoffTimer: number | null = null
  
  // 日志节流：减少重复输出
  private lastPipelineFullLogTime: number = 0
  private pipelineFullSkipCount: number = 0

  // ★ B6: 复用帧捕获Canvas（避免每次新建3.7MB像素缓冲导致OOM）
  private _captureCanvas: HTMLCanvasElement
  private _captureCtx: CanvasRenderingContext2D

  constructor(config: Partial<AutoTriggerConfig> = {}) {
    this.config = {
      motionThreshold: 0.05,
      motionSampleInterval: 500,
      maxQueueSize: 1,
      maxIdleInterval: 30000,
      minAnalysisInterval: 5000,
      enabled: true,
      motionDetectionEnabled: true,
      pipelineMode: false,
      nodeCount: 1,
      ...config,
    }

    // 创建离屏Canvas用于运动检测
    this.canvas = document.createElement('canvas')
    this.canvas.width = 160  // 低分辨率用于检测
    this.canvas.height = 90
    this.ctx = this.canvas.getContext('2d', { willReadFrequently: true })!

    // ★ B6: 复用帧捕获Canvas（避免每次captureFrame新建~3.7MB像素缓冲）
    this._captureCanvas = document.createElement('canvas')
    this._captureCtx = this._captureCanvas.getContext('2d')!
  }

  /**
   * 绑定视频元素
   */
  bindVideo(video: HTMLVideoElement): void {
    this.videoElement = video
  }

  /**
   * 设置触发回调
   */
  setTriggerCallback(callback: TriggerCallback): void {
    this.onTrigger = callback
  }

  /**
   * 设置运动检测回调
   */
  setMotionCallback(callback: (score: number) => void): void {
    this.onMotionDetected = callback
  }

  /**
   * 更新队列大小
   */
  updateQueueSize(size: number): void {
    this.currentQueueSize = size
  }

  /**
   * 标记分析完成
   * 流水线模式下会立即触发新任务
   * @param cameraId 任务对应的摄像头ID（可选）
   * @param currentCameraId 当前选中的摄像头ID（可选）
   */
  markAnalysisComplete(cameraId?: string, currentCameraId?: string): void {
    this.lastAnalysisTime = Date.now()
    
    // 流水线模式：完成一个任务后立即补充新任务
    if (this.config.pipelineMode && this.isRunning) {
      // 只有当任务的摄像头与当前选中摄像头匹配时才减少计数
      // 如果未提供参数则按旧逻辑处理
      if (!cameraId || !currentCameraId || cameraId === currentCameraId) {
        this.activeTaskCount = Math.max(0, this.activeTaskCount - 1)
        console.debug(`[AutoTrigger] Task completed, active: ${this.activeTaskCount}/${this.config.nodeCount}`)
        this.fillPipeline()
      } else {
        console.debug(`[AutoTrigger] Task completed for other camera (${cameraId}), current: ${currentCameraId}, not decrementing`)
      }
    }
  }

  /**
   * ★ 设置503退避：在指定时间内禁止fillPipeline提交新任务
   * 解决问题：503定时器链式累积导致每秒1-2次无效提交，饿死YOLO事件循环
   * @param durationMs 退避时长(ms)
   */
  set503Backoff(durationMs: number): void {
    this._503BackoffUntil = Date.now() + durationMs
    
    // 清除旧的退避定时器，避免多个定时器累积
    if (this._503BackoffTimer) {
      clearTimeout(this._503BackoffTimer)
    }
    
    // 退避结束后调度一次fillPipeline（唯一的定时器，不会累积）
    this._503BackoffTimer = window.setTimeout(() => {
      this._503BackoffTimer = null
      if (this.isRunning && this.config.pipelineMode) {
        console.debug(`[AutoTrigger] 503退避结束，尝试填充pipeline`)
        this.fillPipeline()
      }
    }, durationMs)
  }

  /**
   * 重置Pipeline状态（切换摄像头时调用）
   */
  resetPipeline(): void {
    console.log(`[AutoTrigger] Resetting pipeline, was: ${this.activeTaskCount}/${this.config.nodeCount}`)
    this.activeTaskCount = 0
    this.pipelineInitialized = false
    this.lastAnalysisTime = 0
    // ★ 清除503退避状态，新pipeline不应继承旧的退避
    this._503BackoffUntil = 0
    if (this._503BackoffTimer) {
      clearTimeout(this._503BackoffTimer)
      this._503BackoffTimer = null
    }
    // 如果正在运行，立即开始填充新摄像头的Pipeline
    if (this.isRunning && this.config.pipelineMode) {
      setTimeout(() => this.initPipeline(), 500) // 延迟500ms等待视频流准备好
    }
  }

  /**
   * 启动自动触发
   */
  start(): void {
    if (this.isRunning) return
    this.isRunning = true
    
    console.log('[AutoTrigger] Started with config:', this.config)
    
    // 流水线模式：启动时同时提交N个任务
    if (this.config.pipelineMode && !this.pipelineInitialized) {
      this.initPipeline()
    }
    
    // 启动运动检测
    if (this.config.motionDetectionEnabled) {
      this.startMotionDetection()
    }
    
    // 启动空闲检查
    this.startIdleCheck()
  }

  /**
   * 停止自动触发
   */
  stop(): void {
    this.isRunning = false
    this.stopMotionDetection()
    this.stopIdleCheck()
    console.log('[AutoTrigger] Stopped')
  }

  /**
   * 更新配置
   */
  updateConfig(config: Partial<AutoTriggerConfig>): void {
    const wasRunning = this.isRunning
    const oldNodeCount = this.config.nodeCount
    
    this.config = { ...this.config, ...config }
    
    // 如果节点数增加且正在运行，立即填充流水线
    if (this.isRunning && this.config.pipelineMode && this.config.nodeCount > oldNodeCount) {
      console.log(`[AutoTrigger] NodeCount increased: ${oldNodeCount} -> ${this.config.nodeCount}, filling pipeline`)
      this.fillPipeline()
    }
    
    // 如果之前停止了，重新启动
    if (!this.isRunning && wasRunning && this.config.enabled) {
      this.start()
    }
  }

  /**
   * 手动触发
   */
  manualTrigger(): void {
    this.tryTrigger('manual')
  }

  // ========== 私有方法 ==========

  private startMotionDetection(): void {
    this.motionDetectionTimer = window.setInterval(() => {
      this.detectMotion()
    }, this.config.motionSampleInterval)
  }

  private stopMotionDetection(): void {
    if (this.motionDetectionTimer) {
      clearInterval(this.motionDetectionTimer)
      this.motionDetectionTimer = null
    }
  }

  private startIdleCheck(): void {
    this.idleCheckTimer = window.setInterval(() => {
      this.checkIdle()
    }, 5000) // 每5秒检查一次
  }

  private stopIdleCheck(): void {
    if (this.idleCheckTimer) {
      clearInterval(this.idleCheckTimer)
      this.idleCheckTimer = null
    }
  }

  /**
   * 运动检测
   */
  private detectMotion(): void {
    if (!this.videoElement || this.videoElement.paused) return

    try {
      // 绘制当前帧到Canvas
      this.ctx.drawImage(
        this.videoElement,
        0, 0,
        this.canvas.width, this.canvas.height
      )
      
      const currentFrame = this.ctx.getImageData(
        0, 0,
        this.canvas.width, this.canvas.height
      )

      if (this.previousFrame) {
        const motionScore = this.calculateMotionScore(
          this.previousFrame,
          currentFrame
        )
        
        this.onMotionDetected?.(motionScore)

        // 检测到运动
        if (motionScore > this.config.motionThreshold) {
          this.lastMotionTime = Date.now()
          this.tryTrigger('motion', motionScore)
        }
      }

      this.previousFrame = currentFrame
    } catch (e) {
      console.warn('[AutoTrigger] Motion detection error:', e)
    }
  }

  /**
   * 计算运动分数
   * 使用帧差法，比较像素变化
   */
  private calculateMotionScore(prev: ImageData, curr: ImageData): number {
    const data1 = prev.data
    const data2 = curr.data
    let diffCount = 0
    const threshold = 30 // 像素差异阈值

    // 采样检测，每4个像素检测一次
    for (let i = 0; i < data1.length; i += 16) {
      const rDiff = Math.abs(data1[i] - data2[i])
      const gDiff = Math.abs(data1[i + 1] - data2[i + 1])
      const bDiff = Math.abs(data1[i + 2] - data2[i + 2])
      
      if (rDiff > threshold || gDiff > threshold || bDiff > threshold) {
        diffCount++
      }
    }

    // 返回变化像素比例
    return diffCount / (data1.length / 16)
  }

  /**
   * 检查空闲超时
   */
  private checkIdle(): void {
    // 流水线模式下不使用idle检查，改用fillPipeline
    if (this.config.pipelineMode) {
      this.fillPipeline()
      return
    }
    
    const now = Date.now()
    const timeSinceLastAnalysis = now - this.lastAnalysisTime
    
    // 如果超过最大空闲间隔，触发兜底分析
    if (timeSinceLastAnalysis > this.config.maxIdleInterval) {
      console.log('[AutoTrigger] Idle timeout, triggering fallback analysis')
      this.tryTrigger('idle')
    }
  }

  /**
   * 初始化流水线：同时提交N个任务
   */
  private initPipeline(): void {
    if (!this.videoElement) {
      console.log('[AutoTrigger] Pipeline init delayed: no video element')
      return
    }
    
    console.log(`[AutoTrigger] Initializing pipeline with ${this.config.nodeCount} nodes`)
    this.pipelineInitialized = true
    this.fillPipeline()
  }

  /**
   * 填充流水线：保持活跃任务数 = 节点数
   */
  private fillPipeline(): void {
    if (!this.config.pipelineMode || !this.isRunning) return
    
    // ★ 503退避检查：后端队列满时不提交，避免无效的captureFrame+fetch浪费事件循环
    if (Date.now() < this._503BackoffUntil) {
      return  // 静默跳过，退避定时器会在到期后自动调用fillPipeline
    }
    
    const targetCount = this.config.nodeCount
    const toSubmit = targetCount - this.activeTaskCount
    
    if (toSubmit <= 0) {
      // 节流日志：每10秒或跳过20次才输出一次
      this.pipelineFullSkipCount++
      const now = Date.now()
      if (now - this.lastPipelineFullLogTime > 10000 || this.pipelineFullSkipCount >= 20) {
        console.debug(`[AutoTrigger] Pipeline full: ${this.activeTaskCount}/${targetCount}, skipped ${this.pipelineFullSkipCount}x`)
        this.lastPipelineFullLogTime = now
        this.pipelineFullSkipCount = 0
      }
      return
    }
    
    // 有新任务要提交时重置计数器
    this.pipelineFullSkipCount = 0
    
    console.debug(`[AutoTrigger] Filling pipeline: ${this.activeTaskCount}/${targetCount}, submitting ${toSubmit} tasks NOW`)
    
    // 批量提交所有任务（不等待）
    const submitTime = Date.now()
    for (let i = 0; i < toSubmit; i++) {
      const imageData = this.captureFrame()
      if (!imageData) {
        console.warn('[AutoTrigger] Failed to capture frame for pipeline')
        break
      }
      
      const event: TriggerEvent = {
        type: 'idle',
        timestamp: Date.now(),
      }
      
      this.activeTaskCount++
      console.debug(`[AutoTrigger] Submit task ${i+1}/${toSubmit} at T+${Date.now() - submitTime}ms`)
      this.onTrigger?.(imageData, event)
    }
    console.debug(`[AutoTrigger] All ${toSubmit} tasks submitted in ${Date.now() - submitTime}ms`)
  }

  /**
   * 尝试触发分析
   */
  private tryTrigger(type: 'motion' | 'idle' | 'manual', motionScore?: number): void {
    // 检查是否启用
    if (!this.config.enabled && type !== 'manual') return

    // 检查最小间隔
    const now = Date.now()
    if (now - this.lastAnalysisTime < this.config.minAnalysisInterval) {
      console.log('[AutoTrigger] Too soon since last analysis, skipping')
      return
    }

    // 检查队列
    if (this.currentQueueSize >= this.config.maxQueueSize) {
      console.log('[AutoTrigger] Queue full, skipping')
      return
    }

    // 捕获帧并触发
    const imageData = this.captureFrame()
    if (!imageData) {
      console.warn('[AutoTrigger] Failed to capture frame')
      return
    }

    const event: TriggerEvent = {
      type,
      timestamp: now,
      motionScore,
    }

    console.log(`[AutoTrigger] Triggering analysis: ${type}`, motionScore ? `score=${motionScore.toFixed(3)}` : '')
    this.lastAnalysisTime = now
    this.onTrigger?.(imageData, event)
  }

  /**
   * 捕获当前帧
   */
  private captureFrame(): string | null {
    if (!this.videoElement) return null

    try {
      const video = this.videoElement
      const vw = video.videoWidth
      const vh = video.videoHeight
      const readyState = video.readyState
      const currentTime = video.currentTime
      
      // 严格检查视频是否真正准备好:
      // 1. readyState >= 2 (HAVE_CURRENT_DATA)
      // 2. currentTime > 0 (视频已开始播放) - 注意：WebRTC/MSE 直播流 currentTime 可能为 0
      // 3. 尺寸大于200像素 (排除默认小尺寸)
      const isLiveStream = video.srcObject instanceof MediaStream
      if (readyState < 2 || (!isLiveStream && currentTime <= 0) || vw < 200 || vh < 100) {
        console.warn(`[AutoTrigger] Video not ready: ${vw}x${vh} readyState=${readyState} currentTime=${currentTime.toFixed(2)} isLive=${isLiveStream}`)
        return null
      }
      
      // ✅ 优化：与MultiVLM一致，限制最大维度800px + JPEG 0.75
      // VLM场景理解不需要高分辨率，减少vision编码slice数量
      const MAX_DIM = 800
      let cw = vw, ch = vh
      if (vw > MAX_DIM || vh > MAX_DIM) {
        const scale = MAX_DIM / Math.max(vw, vh)
        cw = Math.round(vw * scale)
        ch = Math.round(vh * scale)
      }

      // ★ B6: 复用Canvas（避免每次新建3.7MB像素缓冲）
      if (this._captureCanvas.width !== cw || this._captureCanvas.height !== ch) {
        this._captureCanvas.width = cw
        this._captureCanvas.height = ch
      }
      this._captureCtx.drawImage(this.videoElement, 0, 0, cw, ch)
      
      // 转换为base64 (去掉data:image/jpeg;base64,前缀)
      const dataUrl = this._captureCanvas.toDataURL('image/jpeg', 0.75)
      // ✅ 修复：captureFrame高频调用，移除日志避免console积压导致浏览器卡顿
      return dataUrl.replace(/^data:image\/\w+;base64,/, '')
    } catch (e) {
      console.error('[AutoTrigger] Capture error:', e)
      return null
    }
  }

  /**
   * 获取当前状态
   */
  getStatus(): {
    isRunning: boolean
    lastAnalysisTime: number
    lastMotionTime: number
    queueSize: number
    config: AutoTriggerConfig
  } {
    return {
      isRunning: this.isRunning,
      lastAnalysisTime: this.lastAnalysisTime,
      lastMotionTime: this.lastMotionTime,
      queueSize: this.currentQueueSize,
      config: { ...this.config },
    }
  }
}

// 单例
let instance: AutoTriggerController | null = null

export function getAutoTriggerController(config?: Partial<AutoTriggerConfig>): AutoTriggerController {
  if (!instance) {
    instance = new AutoTriggerController(config)
  } else if (config) {
    instance.updateConfig(config)
  }
  return instance
}

export function destroyAutoTriggerController(): void {
  if (instance) {
    instance.stop()
    instance = null
  }
}
