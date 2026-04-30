/**
 * Frontend-driven YOLO detection service
 *
 * ★ 解决 go2rtc frame.jpeg 瓶颈（每帧等待 H264 关键帧 ~5s）
 *
 * 原理：前端 MSE 播放器已有解码后的全帧率视频，
 * 从 <video> 元素用 Canvas 抓帧 → base64 JPEG → 直接调 Gateway YOLO API
 *
 * 流程：
 *   <video> → Canvas.drawImage → toBlob(jpeg) → FormData POST /api/v1/yolo/detect → detections
 *
 * 与后端 pipeline 对比：
 *   后端: go2rtc frame.jpeg (5000ms) → Gateway (100ms) → WebSocket → 前端
 *   前端: Canvas capture (2ms) → Gateway (100ms) → 直接回调
 *   提速: ~50× (5000ms → ~100ms per detection)
 */

import { yoloWebSocket, type YoloDetection, type YoloCallback } from './yoloWebSocket'

interface CameraSession {
  videoEl: HTMLVideoElement
  callback: YoloCallback
  timer: number | null
  running: boolean
  inflight: boolean  // 防止请求堆积
}

class YoloDirectDetector {
  private sessions: Map<string, CameraSession> = new Map()
  private captureCanvas: HTMLCanvasElement
  private captureCtx: CanvasRenderingContext2D
  private captureWidth = 640      // ★ YOLO 模型固定 640×640，不能降
  private jpegQuality = 0.55      // ★ JPEG 质量（0.55 ≈ 20KB/帧，平衡传输与质量）
  private _fps = 15               // ★ K3 实际能力: 4并发×15fps减少卡顿感
  private _enabled = false

  // 统计
  private _totalFrames = 0
  private _totalErrors = 0
  private _totalSkipped = 0       // inflight 跳过计数
  private _latencyRing: number[] = []
  private _latencyIdx = 0
  private readonly LATENCY_RING_SIZE = 30

  // ★ 实时 FPS 追踪（每摄像头）
  private _cameraFps: Map<string, { frames: number, startTime: number }> = new Map()
  private _fpsLogTimer: number | null = null

  // ★ 窗口计数器（用于自适应 FPS，每 10s 重置）
  private _windowFrames = 0
  private _windowSkipped = 0
  private _windowErrors = 0

  // ★ 后端感知的自适应FPS上限
  // 通过 /api/v1/yolo/health 获取集群YOLO吞吐量，计算每摄像头最大FPS
  // 例: 1×x86(12fps) + 1×K3(18fps) = 30fps总容量, 4摄像头 → ceiling=7.5fps/cam
  // 例: 2×K3(18fps) = 36fps总容量, 2摄像头 → ceiling=18fps/cam
  private _adaptiveCeiling = 15    // ★ K3 单节点: 4并发 Worker, 总吞吐 15fps
  private _clusterCapacityFps = 0  // 后端报告的集群总吞吐量
  private _capacityFetchTimer: number | null = null
  private _lastCapacityFetch = 0

  // ★ 全局并发限制（解决浏览器连接池饱和导致的延迟）
  // 动态计算: 基于健康节点数 × 每节点 workers (默认 2)
  // 例: 2节点 × 2workers = 4 并发容量
  private _globalInflight = 0
  private _maxGlobalInflight = 6  // ★ 提高初始并发限制 (浏览器限制为 6)
  private _healthyNodes = 0       // 健康节点数，用于动态计算并发限制

  constructor() {
    this.captureCanvas = document.createElement('canvas')
    this.captureCtx = this.captureCanvas.getContext('2d', { willReadFrequently: true })!
  }

  // ── 公开属性 ──

  get enabled() { return this._enabled }
  get fps() { return this._fps }

  setFps(fps: number) {
    this._fps = Math.max(1, Math.min(30, fps))
  }

  getStats() {
    const validLatencies = this._latencyRing.filter(v => v > 0)
    const avgMs = validLatencies.length > 0
      ? Math.round(validLatencies.reduce((a, b) => a + b, 0) / validLatencies.length)
      : 0

    // 计算每摄像头实际 FPS
    const now = performance.now()
    const perCameraFps: Record<string, number> = {}
    for (const [cam, tracker] of this._cameraFps) {
      const elapsed = (now - tracker.startTime) / 1000
      perCameraFps[cam] = elapsed > 1 ? parseFloat((tracker.frames / elapsed).toFixed(1)) : 0
    }

    return {
      totalFrames: this._totalFrames,
      totalErrors: this._totalErrors,
      totalSkipped: this._totalSkipped,
      avgLatencyMs: avgMs,
      activeCameras: this.sessions.size,
      targetFps: this._fps,
      perCameraFps,
    }
  }

  // ── 生命周期 ──

  /**
   * 启动前端直接检测
   * @param cameraId  摄像头名称
   * @param videoEl   已播放的 <video> 元素
   * @param callback  检测结果回调（与 yoloWebSocket 格式一致）
   */
  start(cameraId: string, videoEl: HTMLVideoElement, callback: YoloCallback) {
    this.stop(cameraId)
    this._enabled = true

    const session: CameraSession = {
      videoEl,
      callback,
      timer: null,
      running: true,
      inflight: false,
    }
    this.sessions.set(cameraId, session)

    // 初始化 FPS 追踪
    this._cameraFps.set(cameraId, { frames: 0, startTime: performance.now() })

    // 启动 FPS 日志定时器（全局唯一）
    if (!this._fpsLogTimer) {
      this._fpsLogTimer = window.setInterval(() => this.logFps(), 10000)
    }

    // ★ 启动集群容量探测（30s一次，首次立即）
    if (!this._capacityFetchTimer) {
      this.fetchClusterCapacity()  // 立即首次获取
      this._capacityFetchTimer = window.setInterval(() => this.fetchClusterCapacity(), 30000)
    }

    // ★ 错开启动时间：避免所有摄像头同时发请求导致节点排队
    // 4 cameras at 5fps (200ms interval) → offsets: 0, 50, 100, 150ms
    // 效果: 每个节点同时只处理 1 个请求, 延迟从 ~150ms 降到 ~100ms
    const intervalMs = 1000 / this._fps
    const staggerIdx = this.sessions.size - 1
    const staggerMs = Math.round(staggerIdx * intervalMs / Math.max(this.sessions.size, 1))

    console.log(`[YOLO Direct] ${cameraId}: started (${this._fps}fps, ${this.captureWidth}px, stagger=${staggerMs}ms)`)
    if (staggerMs > 0) {
      window.setTimeout(() => {
        if (session.running) this.scheduleNext(cameraId, session)
      }, staggerMs)
    } else {
      this.scheduleNext(cameraId, session)
    }
  }

  /**
   * 停止某摄像头的前端检测
   */
  stop(cameraId: string) {
    const session = this.sessions.get(cameraId)
    if (session) {
      session.running = false
      if (session.timer !== null) {
        clearTimeout(session.timer)
        session.timer = null
      }
      this.sessions.delete(cameraId)
    }
    this._cameraFps.delete(cameraId)
    if (this.sessions.size === 0) {
      this._enabled = false
      if (this._fpsLogTimer) {
        clearInterval(this._fpsLogTimer)
        this._fpsLogTimer = null
      }
    }
  }

  /**
   * 停止所有
   */
  stopAll() {
    for (const cameraId of [...this.sessions.keys()]) {
      this.stop(cameraId)
    }
    this._enabled = false
    this._cameraFps.clear()
    if (this._fpsLogTimer) {
      clearInterval(this._fpsLogTimer)
      this._fpsLogTimer = null
    }
    if (this._capacityFetchTimer) {
      clearInterval(this._capacityFetchTimer)
      this._capacityFetchTimer = null
    }
  }

  // ── 集群容量探测 ──

  /**
   * 从后端 /api/v1/yolo/health 获取集群YOLO吞吐量，计算每摄像头FPS上限
   * 
   * 后端返回:
   *   cluster_capacity_fps: 集群总YOLO吞吐量 (det/s)
   *   per_node_capacity: { node_id: fps_per_node }
   *   例: { "x86-node": 12.0, "k3-node": 18.2 } → total=30.2
   * 
   * 前端计算:
   *   ceiling = cluster_capacity_fps / active_cameras * 0.85 (85%安全余量)
   *   例: 30.2 / 4cam * 0.85 = 6.4fps/cam
   *   例: 36.4 / 2cam * 0.85 = 15.5fps/cam
   */
  private async fetchClusterCapacity() {
    try {
      const resp = await fetch('/api/v1/yolo/health', { signal: AbortSignal.timeout(3000) })
      if (!resp.ok) return

      const data = await resp.json()
      const capacity = data.cluster_capacity_fps || 0
      if (capacity <= 0) return

      this._clusterCapacityFps = capacity
      this._lastCapacityFetch = Date.now()

      // ★ 动态调整并发限制 = 健康节点数 × 每节点workers(2)
      // 但不超过浏览器连接限制 (6)，避免连接池饱和
      const healthyNodes = data.healthy_nodes || 1
      this._healthyNodes = healthyNodes
      const workersPerNode = 2  // 假设每节点2个YOLO workers
      this._maxGlobalInflight = Math.min(healthyNodes * workersPerNode, 6)

      // 计算每摄像头FPS上限 = 集群总容量 / 活跃摄像头数 × 85%安全余量
      const numCameras = Math.max(this.sessions.size, 1)
      const rawCeiling = capacity / numCameras
      // 安全余量85%: 留出headroom给网络延迟、突发抖动、round-robin不均匀
      // 下限2fps, 上限30fps (硬件理论极限)
      this._adaptiveCeiling = Math.max(2, Math.min(30, Math.round(rawCeiling * 0.85 * 10) / 10))

      console.log(
        `[YOLO Direct] 🎯 集群容量更新: ${capacity.toFixed(1)}fps total, ` +
        `${numCameras}cam → ceiling=${this._adaptiveCeiling}fps/cam, ` +
        `maxInflight=${this._maxGlobalInflight}` +
        (data.per_node_capacity ? ` | nodes: ${JSON.stringify(data.per_node_capacity)}` : '')
      )
    } catch {
      // 静默失败（Gateway可能未就绪）
    }
  }

  // ── 内部循环 ──

  private scheduleNext(cameraId: string, session: CameraSession) {
    if (!session.running) return

    const intervalMs = 1000 / this._fps
    session.timer = window.setTimeout(() => {
      if (!session.running) return
      // ★ 先调度下一次，再执行检测（不等待完成）
      // 定时器按固定间隔触发，inflight 防止请求堆积
      // 旧模式: timer(200ms) → detect(110ms) → timer = 310ms/帧 = 3.2fps
      // 新模式: timer(200ms) 固定间隔, detect 并行 = 5fps（如果 detect < 200ms）
      this.scheduleNext(cameraId, session)
      this.detectOnce(cameraId, session)
    }, intervalMs)
  }

  private async detectOnce(cameraId: string, session: CameraSession) {
    if (!session.running) return
    if (session.inflight) {
      this._totalSkipped++
      this._windowSkipped++
      return
    }

    // ★ 全局并发限制：避免浏览器连接池饱和
    if (this._globalInflight >= this._maxGlobalInflight) {
      this._totalSkipped++
      this._windowSkipped++
      return
    }

    const { videoEl, callback } = session
    if (!videoEl || videoEl.readyState < 2) return  // HAVE_CURRENT_DATA

    session.inflight = true
    this._globalInflight++
    const t0 = performance.now()

    try {
      // 1. Canvas 抓帧 → Blob（异步，不阻塞主线程）
      const blob = await this.captureFrameBlob(videoEl)
      if (!blob) { session.inflight = false; this._globalInflight--; return }

      // 2. FormData binary POST（无 base64 开销，payload 减小 33%）
      const result = await this.callGatewayDetectBlob(blob)
      if (!result) { session.inflight = false; this._globalInflight--; return }

      const latencyMs = Math.round(performance.now() - t0)

      // 3. 统计
      this._totalFrames++
      this._windowFrames++
      this._latencyRing[this._latencyIdx % this.LATENCY_RING_SIZE] = latencyMs
      this._latencyIdx++

      // 4. FPS 追踪
      const tracker = this._cameraFps.get(cameraId)
      if (tracker) tracker.frames++

      // 5. 回调（格式与 yoloWebSocket 完全一致）
      if (session.running) {
        callback(
          result.detections,
          videoEl.videoWidth,
          videoEl.videoHeight,
          undefined,          // frameBase64（不需要，前端已有视频）
          Date.now() / 1000   // frameTime
        )
        // ★ 通知全局监听器（YOLO→VLM 触发等跨组件功能）
        // 禁用后端抓帧后，WebSocket 不再产生检测结果，
        // 全局监听器必须由 DirectDetector 驱动
        yoloWebSocket.notifyGlobalListeners(
          cameraId, result.detections, result.inference_ms
        )
      }
    } catch (err: any) {
      this._totalErrors++
      this._windowErrors++
      if (this._totalErrors <= 5 || this._totalErrors % 100 === 0) {
        console.warn(`[YOLO Direct] ${cameraId}: error #${this._totalErrors}:`, err.message || err)
      }
    } finally {
      session.inflight = false
      this._globalInflight--
    }
  }

  // ── FPS 日志 + 自适应调速 ──

  private logFps() {
    const now = performance.now()
    const entries: string[] = []
    let totalFps = 0
    for (const [cam, tracker] of this._cameraFps) {
      const elapsed = (now - tracker.startTime) / 1000
      const fps = elapsed > 1 ? tracker.frames / elapsed : 0
      entries.push(`${cam}=${fps.toFixed(1)}`)
      totalFps += fps
    }
    const stats = this.getStats()

    // ★ 自适应 FPS：基于窗口延迟 + 窗口 skip 率 + 窗口错误率动态调整
    // 窗口 = 上次 logFps 到现在的 10s 间隔（而非累计值）
    // 这确保系统对突发节点故障 / 推理变慢在 10s 内响应
    const wFrames = this._windowFrames
    const wSkipped = this._windowSkipped
    const wErrors = this._windowErrors
    const wTotal = wFrames + wSkipped  // ★ 修复: 分母=总 tick 数，而非仅成功帧
    const wSkipRate = wTotal > 0 ? wSkipped / wTotal : 0
    const wErrorRate = (wFrames + wErrors) > 0 ? wErrors / (wFrames + wErrors) : 0
    // 重置窗口计数器
    this._windowFrames = 0
    this._windowSkipped = 0
    this._windowErrors = 0

    // ★ 比例步长自适应：过载越严重，降速越快——大幅加速收敛
    // 场景对照：
    //   节点下线(2→1): 固定步长需 50s 收敛 → 比例步长只需 20s
    //   节点增加(2→3): 固定步长需 50s 提速 → 比例步长只需 30s
    //   推理变慢(80→200ms): 延迟直接反映，10s 内响应
    const oldFps = this._fps
    // ★ 预热门槛: 10帧(~2s) 后启动自适应 或 紧急降速（高错误/高skip率时立即响应）
    // 原值50帧导致 Gateway 启动期不可用时自适应永远不触发（成功帧<50）
    if (this._totalFrames > 10 || wErrorRate > 0.3 || wSkipRate > 0.5) {
      const lat = stats.avgLatencyMs

      // ★ 节点崩溃检测: 错误率高表示 Gateway/节点故障
      if (wErrorRate > 0.3) {
        // >30% 请求失败 → 急剧降速，最低1fps（健康探测级别）
        // 2fps×4cam=8req/10s 全部浪费; 1fps×4cam=4req/10s 作为探针足够
        this._fps = Math.max(this._fps - 2.0, 1)
      }
      // ★ 严重过载: 延迟极高 或 skip率极高（放宽阈值，K3 延迟本身较高）
      else if (lat > 500 || wSkipRate > 0.50) {
        this._fps = Math.max(this._fps - 1.0, 3)
      }
      // ★ 中度过载: 延迟高 或 skip率高
      else if (lat > 400 || wSkipRate > 0.35) {
        this._fps = Math.max(this._fps - 0.5, 3)
      }
      // ★ 健康: 延迟低 + skip率低 + 无错误 → 提速
      // ✅ 优化：放宽延迟阈值 (K3 正常延迟 100-200ms)
      else if (lat > 0 && lat < 250 && wSkipRate < 0.10 && wErrorRate === 0) {
        const gap = this._adaptiveCeiling - this._fps
        // 距离ceiling远时大步快追，接近时小步精调
        const step = gap > 5 ? 2.0 : gap > 2 ? 1.0 : 0.5
        this._fps = Math.min(this._fps + step, this._adaptiveCeiling)
      }
      // ★ 死区: 150ms ≤ lat ≤ 200ms, 5% ≤ skip ≤ 15% → 不调整（避免振荡）
    }

    const fpsChanged = Math.abs(this._fps - oldFps) >= 0.1
    console.log(
      `[YOLO Direct] 📊 FPS: ${entries.join(', ')} | ` +
      `total=${totalFps.toFixed(1)}fps #${this._totalFrames} ` +
      `avg=${stats.avgLatencyMs}ms wSkip=${wSkipped}/${wTotal}(${(wSkipRate*100).toFixed(0)}%) ` +
      `wErr=${wErrors}(${(wErrorRate*100).toFixed(0)}%) err=${this._totalErrors} ` +
      `ceiling=${this._adaptiveCeiling.toFixed(1)}` +
      (fpsChanged ? ` ★target: ${oldFps.toFixed(1)}→${this._fps.toFixed(1)}` : ` target=${this._fps.toFixed(1)}`)
    )
  }

  // ── Canvas 抓帧（异步 Blob，不阻塞主线程） ──

  private captureFrameBlob(videoEl: HTMLVideoElement): Promise<Blob | null> {
    const vw = videoEl.videoWidth
    const vh = videoEl.videoHeight
    if (!vw || !vh) return Promise.resolve(null)

    // 等比缩放到 captureWidth
    const scale = this.captureWidth / vw
    const cw = this.captureWidth
    const ch = Math.round(vh * scale)

    if (this.captureCanvas.width !== cw || this.captureCanvas.height !== ch) {
      this.captureCanvas.width = cw
      this.captureCanvas.height = ch
    }

    this.captureCtx.drawImage(videoEl, 0, 0, cw, ch)

    // ★ toBlob: 异步 + 直接产生二进制，无 base64 编码开销
    return new Promise((resolve) => {
      this.captureCanvas.toBlob(
        (blob) => resolve(blob),
        'image/jpeg',
        this.jpegQuality
      )
    })
  }

  // ── Gateway API 调用（FormData binary，无 base64 开销） ──

  private async callGatewayDetectBlob(blob: Blob): Promise<{
    detections: YoloDetection[]
    inference_ms: number
    node_id?: string
  } | null> {
    // ★ FormData 二进制上传: 比 JSON base64 小 33%，且无编码 CPU 开销
    const formData = new FormData()
    formData.append('image', blob, 'frame.jpg')

    // ★ 5s 超时: 防止 Gateway 不可用时 fetch 无限挂起 → inflight=true 永不释放 → 100% skip
    const controller = new AbortController()
    const timeoutId = setTimeout(() => controller.abort(), 5000)

    let resp: Response
    try {
      resp = await fetch('/api/v1/yolo/detect', {
        method: 'POST',
        body: formData,
        signal: controller.signal,
      })
    } finally {
      clearTimeout(timeoutId)
    }

    if (!resp.ok) {
      throw new Error(`HTTP ${resp.status} ${resp.statusText}`)
    }

    const data = await resp.json()
    if (!data.success && data.detections === undefined) {
      throw new Error(data.error || data.detail || 'Detection failed')
    }

    // 映射到 YoloDetection 格式（与 yoloWebSocket 完全一致）
    const detections: YoloDetection[] = (data.detections || []).map((d: any) => ({
      class_id:   d.class_id   ?? d.classId   ?? 0,
      class_name: d.class_name ?? d.className ?? '',
      confidence: d.confidence ?? 0,
      bbox:       d.bbox       ?? [0, 0, 0, 0],
      // 前端直连无 track_id / velocity（无后端 ByteTrack），留空
      track_id:   undefined,
      velocity_x: 0,
      velocity_y: 0,
    }))

    return {
      detections,
      inference_ms: data.inference_ms ?? data.inferenceMs ?? 0,
      node_id: data.node_id,
    }
  }
}

// 单例
export const yoloDirectDetector = new YoloDirectDetector()
