// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

/**
 * MSE (Media Source Extensions) 直播流播放器 — WebSocket 传输
 *
 * 通过 go2rtc 原生 WebSocket MSE 协议播放 fMP4 直播流。
 * ★ 核心 MSE 逻辑严格对齐 go2rtc-vue.git/src/composables/useMSE.ts 参考实现。
 *
 * 架构（优先直连，与参考实现一致）：
 *   前端 ←WebSocket→ go2rtc /api/ws（直连，消除代理延迟）
 *   回退：前端 ←WebSocket→ Go代理(透传) ←WebSocket→ go2rtc /api/ws
 *
 * 关键设计：
 *   1. WebSocket 传输 — 每个 WS 消息 = 完整 fMP4 segment（帧边界天然对齐）
 *   2. 服务端编解码器协商 — go2rtc 回复 JSON { value: "video/mp4; codecs=..." }
 *   3. SourceBuffer.mode = 'segments'（与参考实现一致）
 *   4. 首包直接追加，后续走队列（与参考实现一致）
 *   5. 不修改 playbackRate — 变速触发 re-decode 导致帧重复
 *   6. ★ 不做定时 live edge seek — seek 导致浏览器从前置关键帧解码，
 *      产生 "1-2-3, 2-3-4" 帧重播循环（GOP=2s 时重播最多2s已显示帧）
 *   7. 保守缓冲裁剪（仅清理旧数据，不 seek）— 生产级 24/7 稳定
 *   8. 隐藏标签页处理 — 仅标签页隐藏时 seek（与参考实现一致）
 *   9. pause 事件恢复 — currentTime 超出缓冲范围时自动恢复（与参考实现一致）
 */

// 支持的编解码器列表（发送给 go2rtc 进行协商）
const SUPPORTED_CODECS = 'avc1.640029,avc1.64002A,avc1.640033,hvc1.1.6.L153.B0,mp4a.40.2,mp4a.40.5,flac,opus'

// 回调类型
export interface MsePlayerCallbacks {
  onError?: (error: string) => void
  onPlaying?: () => void
}

export class MseStreamPlayer {
  private mediaSource: MediaSource | null = null
  private sourceBuffer: SourceBuffer | null = null
  private ws: WebSocket | null = null
  private queue: ArrayBuffer[] = []
  private videoElement: HTMLVideoElement | null = null
  private objectUrl: string | null = null
  private callbacks: MsePlayerCallbacks = {}
  private trimTimer: number | null = null
  private alive = false
  private visibilityHandler: (() => void) | null = null
  // ★ 参考实现的关键标志：首包直接追加，非首包走队列
  private streamingStarted = false
  // fMP4 诊断计数器（仅分析前几个包）
  private diagCount = 0

  // ============ B-帧 CTO 注入状态 ============
  // go2rtc 的 fMP4 muxer 不设置 compositionTimeOffset (CTO)，
  // 导致 B 帧 PTS=DTS，显示顺序错误 → "1-2-3, 2-3-4" 模式。
  // 修复：检测帧类型，缓冲参考帧，计算并注入 CTO 到 trun。
  private videoTrackId = -1                        // 视频 track_id（从 init segment 解析）
  private pendingRef: ArrayBuffer | null = null   // 缓冲的参考帧（等待下一个参考帧到达后输出）
  private pendingBFrames: ArrayBuffer[] = []      // 缓冲的 B 帧
  private frameDuration = 0                       // 帧时长（从 tfdt 差值检测）
  private lastBmdt = -1                           // 上一帧的 baseMediaDecodeTime
  private bmdtSamples = 0                         // 用于检测 frameDuration 的采样数
  private ctoDiagLogged = false                    // CTO 诊断日志已输出

  // 缓冲裁剪参数（保守策略 — 仅清理旧数据，不做 seek）
  // ★ 参考实现完全没有 trim/seek 定时器，但生产 24/7 需防止内存无限增长
  private readonly BUFFER_TRIM_INTERVAL = 30000  // 每30秒检查一次（极保守）
  private readonly BUFFER_KEEP_SECONDS = 30      // 保留当前时间前30秒的数据

  /**
   * 检查浏览器是否支持 MSE
   */
  static isSupported(): boolean {
    return typeof MediaSource !== 'undefined' && MediaSource.isTypeSupported('video/mp4')
  }

  /**
   * 启动 MSE 直播流（WebSocket 传输）
   * @param video - HTMLVideoElement
   * @param cameraId - 摄像头 ID（go2rtc stream name）
   * @param callbacks - 错误/播放回调
   */
  async start(video: HTMLVideoElement, cameraId: string, callbacks?: MsePlayerCallbacks): Promise<boolean> {
    this.stop()
    this.videoElement = video
    this.callbacks = callbacks || {}
    this.alive = true
    this.streamingStarted = false

    if (!MseStreamPlayer.isSupported()) {
      console.warn('[MsePlayer] MSE not supported')
      return false
    }

    try {
      // 1. 创建 MediaSource 并绑定到 video
      this.mediaSource = new MediaSource()
      this.objectUrl = URL.createObjectURL(this.mediaSource)
      video.src = this.objectUrl

      // 等待 MediaSource 打开
      await new Promise<void>((resolve, reject) => {
        const onOpen = () => {
          this.mediaSource?.removeEventListener('sourceopen', onOpen)
          resolve()
        }
        this.mediaSource!.addEventListener('sourceopen', onOpen)
        this.mediaSource!.addEventListener('error', () => {
          this.mediaSource?.removeEventListener('sourceopen', onOpen)
          reject(new Error('MediaSource failed to open'))
        }, { once: true })
        setTimeout(() => reject(new Error('MediaSource open timeout')), 5000)
      })

      if (!this.alive) return false

      // 2. 建立 WebSocket 连接到 go2rtc
      // ★ 优先直连 go2rtc（与参考实现一致），失败时回退到 Go 代理
      const directUrl = this.buildDirectWsUrl(cameraId)
      const proxyUrl = this.buildProxyWsUrl(cameraId)

      try {
        console.log(`[MsePlayer] Trying direct go2rtc connection:`, directUrl)
        await this.connectWebSocket(directUrl, 2000)
        console.log(`[MsePlayer] Connected directly to go2rtc`)
      } catch (directErr) {
        console.log(`[MsePlayer] Direct connection failed, falling back to proxy:`, proxyUrl)
        await this.connectWebSocket(proxyUrl, 5000)
        console.log(`[MsePlayer] Connected via proxy`)
      }

      // 3. 注册视频事件处理器（pause 恢复 — 与参考实现一致）
      this.setupVideoEvents()

      // 4. 启动保守缓冲裁剪定时器（仅清理旧数据，不 seek）
      this.startTrimTimer()

      // 5. 注册 visibilitychange 处理器（隐藏标签页恢复时 seek to live edge）
      this.setupVisibilityHandler()

      return true
    } catch (e: any) {
      console.error('[MsePlayer] Start error:', e)
      this.callbacks.onError?.(e.message)
      return false
    }
  }

  // go2rtc API 端口（与 go2rtc.yaml api.listen 保持一致）
  private readonly GO2RTC_PORT = 1984

  /**
   * 构建直连 go2rtc 的 WebSocket URL
   * ★ 与参考实现一致：浏览器直接连接 go2rtc，不经过任何代理
   * 消除 Go 代理层可能引入的时序/缓冲/重组装问题
   */
  private buildDirectWsUrl(cameraId: string): string {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.hostname}:${this.GO2RTC_PORT}/api/ws?src=${encodeURIComponent(cameraId)}`
  }

  /**
   * 构建通过 Go 代理的 WebSocket URL（回退方案）
   */
  private buildProxyWsUrl(cameraId: string): string {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}/api/go2rtc/ws?src=${encodeURIComponent(cameraId)}`
  }

  /**
   * 建立 WebSocket 连接并完成编解码器协商
   */
  private connectWebSocket(url: string, timeoutMs = 5000): Promise<void> {
    return new Promise((resolve, reject) => {
      if (!this.alive) { reject(new Error('stopped')); return }

      const ws = new WebSocket(url)
      ws.binaryType = 'arraybuffer'
      this.ws = ws

      const timeout = setTimeout(() => {
        reject(new Error('WebSocket connection timeout'))
        ws.close()
      }, timeoutMs)

      ws.onopen = () => {
        clearTimeout(timeout)
        // 发送编解码器协商请求（go2rtc MSE 协议）
        const request = JSON.stringify({ type: 'mse', value: SUPPORTED_CODECS })
        ws.send(request)
        console.log('[MsePlayer] Sent codec negotiation request')
      }

      ws.onmessage = (event) => {
        if (!this.alive) return

        if (typeof event.data === 'string') {
          // 字符串消息 = go2rtc 返回的编解码器信息（仅处理首次）
          if (!this.sourceBuffer) {
            this.handleCodecResponse(event.data, resolve, reject)
          }
        } else {
          // 二进制消息 = fMP4 segment
          this.handleBinaryData(event.data as ArrayBuffer)
        }
      }

      ws.onclose = () => {
        clearTimeout(timeout)
        if (this.alive) {
          this.signalStreamEnd()
          this.callbacks.onError?.('WebSocket closed')
        }
      }

      ws.onerror = () => {
        clearTimeout(timeout)
        reject(new Error('WebSocket connection failed'))
      }
    })
  }

  /**
   * 处理 go2rtc 返回的编解码器信息
   * ★ 与参考实现对齐：JSON.parse(data).value 直接传给 addSourceBuffer
   */
  private handleCodecResponse(data: string, resolve: () => void, reject: (e: Error) => void) {
    try {
      if (!this.mediaSource || this.mediaSource.readyState !== 'open') {
        reject(new Error('MediaSource not open'))
        return
      }

      // go2rtc 返回 JSON: {"value":"video/mp4; codecs=\"avc1.640029\""}
      // .value 是完整 MIME 类型，直接传给 addSourceBuffer（与参考实现一致）
      let mimeType: string
      try {
        const msg = JSON.parse(data)
        mimeType = msg.value
      } catch {
        // 非 JSON 回退：可能是直接的 MIME 类型字符串
        mimeType = data.trim()
      }

      console.log(`[MsePlayer] Server codec: ${mimeType}`)

      // 如果不是完整 MIME 类型，包装
      if (mimeType && !mimeType.startsWith('video/') && !mimeType.startsWith('audio/')) {
        mimeType = `video/mp4; codecs="${mimeType}"`
      }

      if (!mimeType || !MediaSource.isTypeSupported(mimeType)) {
        console.warn(`[MsePlayer] Codec not supported: ${mimeType}, trying generic`)
        this.sourceBuffer = this.mediaSource.addSourceBuffer('video/mp4')
      } else {
        this.sourceBuffer = this.mediaSource.addSourceBuffer(mimeType)
      }

      // ★ 使用 'segments' 模式（与参考实现一致）
      this.sourceBuffer.mode = 'segments'
      this.sourceBuffer.addEventListener('updateend', () => this.pushPacket())
      console.log(`[MsePlayer] SourceBuffer created: ${mimeType}`)

      resolve()
    } catch (e: any) {
      reject(new Error(`Codec setup failed: ${e.message}`))
    }
  }

  /**
   * 处理二进制 fMP4 segment 数据
   * ★ 首包（init segment）直接追加
   * ★ 后续 media segment 经过 B-帧 CTO 注入管线
   */
  private handleBinaryData(data: ArrayBuffer) {
    if (!data.byteLength) return

    // 首包（init segment）直接追加，同时解析 video track_id
    if (!this.streamingStarted && this.sourceBuffer) {
      try {
        // ★ 从 init segment 的 moov/trak/mdia/hdlr 获取视频 track_id
        this.videoTrackId = MseStreamPlayer.findVideoTrackId(data)
        console.log(`[MsePlayer] Video track_id=${this.videoTrackId}`)
        this.sourceBuffer.appendBuffer(data)
        this.streamingStarted = true
      } catch (e: any) {
        console.warn('[MsePlayer] First append error:', e.message)
      }
      return
    }

    // ★ 用 tfhd track_id 可靠地区分音频/视频段
    // （之前用 NAL 检测不可靠 — 音频数据碰巧匹配 NAL 模式 → 错误 CTO → 解码崩溃）
    const trackId = MseStreamPlayer.readTfhdTrackId(data)
    if (this.videoTrackId > 0 && trackId > 0 && trackId !== this.videoTrackId) {
      // 音频或其他非视频段 → 直接入队，不影响 B 帧重排缓冲
      this.enqueueSegment(data)
      return
    }

    // ★ 以下仅处理视频段

    // ★ 检测帧时长（仅从视频 tfdt baseMediaDecodeTime 差值）
    const bmdt = MseStreamPlayer.readBmdt(data)
    if (bmdt >= 0 && this.lastBmdt >= 0 && bmdt > this.lastBmdt) {
      const diff = bmdt - this.lastBmdt
      // ★ 跳过启动突发的异常小 diff（go2rtc 启动时前几帧 DTS 仅增 91 ticks，不是真实帧时长）
      // 在 90kHz timescale 下，120fps 的帧时长=750，任何 diff<500 都是启动伪影
      if (diff >= 500 && this.bmdtSamples < 8) {
        if (this.frameDuration === 0 || diff < this.frameDuration) {
          this.frameDuration = diff
        }
        this.bmdtSamples++
        if (this.bmdtSamples === 8) {
          console.log(`[MsePlayer] Detected video frameDuration=${this.frameDuration}`)
        }
      }
    }
    if (bmdt >= 0) this.lastBmdt = bmdt

    // ★ B-帧 CTO 注入管线
    const segType = MseStreamPlayer.classifySegment(data)
    const isRef = segType !== 'bframe'

    // ★ 诊断：前 15 个视频帧的详细信息
    if (this.diagCount < 15) {
      if (this.diagCount === 0) MseStreamPlayer.diagnoseFmp4(data, 0)
      console.log(`[MsePlayer] vframe#${this.diagCount} bmdt=${bmdt} segType=${segType} isRef=${isRef}`)
      this.diagCount++
    }

    if (isRef) {
      // 参考帧到达 → 输出之前缓冲的参考帧和 B 帧
      this.flushReorderBuffer()
      // 缓冲当前参考帧（等下一个参考帧到达才能确定 CTO）
      this.pendingRef = data
    } else {
      // B 帧：加入缓冲
      this.pendingBFrames.push(data)
    }
  }

  /**
   * 输出重排缓冲区中的帧（注入 CTO 后追加到 SourceBuffer 队列）
   * 调用时机：新的参考帧到达时
   *
   * CTO 计算规则（每 segment 1 sample）：
   *   参考帧: CTO = +N * frameDuration (N = 后续 B 帧数)
   *   B 帧:   CTO = -1 * frameDuration (始终为 -1 帧)
   */
  private flushReorderBuffer() {
    if (!this.pendingRef) return

    const n = this.pendingBFrames.length
    const dur = this.frameDuration

    if (n > 0 && dur > 0 && this.bmdtSamples >= 4) {
      // 有 B 帧 → 注入 CTO
      const refCTO = n * dur
      if (!this.ctoDiagLogged) {
        console.log(`[MsePlayer] ★ CTO injection active: ref CTO=+${refCTO}, B-frame CTO=${-dur}, bFrameCount=${n}, frameDuration=${dur}`)
        this.ctoDiagLogged = true
      }
      this.enqueueSegment(MseStreamPlayer.injectCTO(this.pendingRef, refCTO))

      for (const bFrame of this.pendingBFrames) {
        this.enqueueSegment(MseStreamPlayer.injectCTO(bFrame, -dur))
      }
    } else {
      // 无 B 帧或帧时长未知 → 直接输出（CTO=0，不注入）
      this.enqueueSegment(this.pendingRef)
      for (const bFrame of this.pendingBFrames) {
        this.enqueueSegment(bFrame)
      }
    }

    this.pendingRef = null
    this.pendingBFrames = []
  }

  /**
   * 将 segment 加入 SourceBuffer 队列
   */
  private enqueueSegment(data: ArrayBuffer) {
    if (!this.sourceBuffer) return
    this.queue.push(data)
    if (!this.sourceBuffer.updating) {
      this.pushPacket()
    }
  }

  /**
   * 从队列取一个包追加到 SourceBuffer
   * ★ 与参考实现对齐：一次一个包，不合并（避免多 moof+mdat 合并导致时间戳异常）
   * 同时处理隐藏标签页的 live edge 追赶
   */
  private pushPacket() {
    if (this.sourceBuffer && !this.sourceBuffer.updating) {
      if (this.queue.length > 0) {
        const packet = this.queue.shift()!
        try {
          this.sourceBuffer.appendBuffer(packet)
        } catch (e: any) {
          console.warn('[MsePlayer] Append error:', e.message)
          if (e.name === 'QuotaExceededError') {
            this.queue.unshift(packet)
            this.forceTrimBuffer()
          }
        }
      }
      // ★ 不重置 streamingStarted — CTO 注入管线要求所有 media segment
      //   都经过 B-帧检测 → 重排缓冲 → CTO 注入路径
    }

    // ★ 隐藏标签页 seek to live edge（与参考实现一致，在 pushPacket/updateend 中处理）
    if (this.videoElement?.buffered && this.videoElement.buffered.length > 0) {
      if (typeof document.hidden !== 'undefined' && document.hidden) {
        const bufferedEnd = this.videoElement.buffered.end(this.videoElement.buffered.length - 1)
        this.videoElement.currentTime = bufferedEnd - 0.5
      }
    }
  }

  /**
   * 定时缓冲裁剪（仅清理旧数据，不做 seek）
   * ★ 参考实现完全没有此机制，但生产 24/7 运行需防止 SourceBuffer 无限增长
   * ★ 不做 checkLiveEdge / seek — 定时 seek 会导致浏览器从前置关键帧解码，
   *   产生 "1-2-3, 2-3-4" 帧重播（seek 目标在 GOP 中间 → 回退到 GOP 头解码）
   */
  private trimOldBuffer() {
    if (!this.sourceBuffer || this.sourceBuffer.updating) return
    if (!this.videoElement) return
    const buffered = this.sourceBuffer.buffered
    if (!buffered.length) return

    const currentTime = this.videoElement.currentTime
    const start = buffered.start(0)

    if (currentTime - start > this.BUFFER_KEEP_SECONDS + 2) {
      try {
        this.sourceBuffer.remove(start, currentTime - this.BUFFER_KEEP_SECONDS)
      } catch (e) {
        // 忽略
      }
    }
  }

  /**
   * 强制裁剪（QuotaExceededError 时）
   */
  private forceTrimBuffer() {
    if (!this.sourceBuffer || this.sourceBuffer.updating) return
    if (!this.videoElement) return
    const buffered = this.sourceBuffer.buffered
    if (!buffered.length) return

    const currentTime = this.videoElement.currentTime
    const start = buffered.start(0)
    if (currentTime - start > 2) {
      try {
        this.sourceBuffer.remove(start, currentTime - 1)
      } catch (e) {
        // 忽略
      }
    }
  }

  /**
   * 通知 MediaSource 流结束，触发 video 'ended' 事件
   */
  private signalStreamEnd() {
    if (!this.mediaSource || this.mediaSource.readyState !== 'open') return
    if (this.sourceBuffer?.updating) {
      this.sourceBuffer.addEventListener('updateend', () => {
        try { this.mediaSource?.endOfStream() } catch (e) { /* ignore */ }
      }, { once: true })
    } else {
      try { this.mediaSource.endOfStream() } catch (e) { /* ignore */ }
    }
  }

  /**
   * 注册视频事件处理器（与参考实现一致）
   * ★ pause 事件处理：当 currentTime 超出缓冲范围时（可能由 seek 或流中断导致），
   *   浏览器会自动暂停。此时回退到缓冲末尾附近并恢复播放。
   */
  private setupVideoEvents() {
    if (!this.videoElement) return
    this.videoElement.addEventListener('pause', () => {
      if (!this.alive) return
      if (this.videoElement?.buffered && this.videoElement.buffered.length > 0) {
        const currentTime = this.videoElement.currentTime
        const bufferedEnd = this.videoElement.buffered.end(this.videoElement.buffered.length - 1)
        if (currentTime > bufferedEnd) {
          this.videoElement.currentTime = bufferedEnd - 0.1
          this.videoElement.play().catch(() => {})
        }
      }
    })
  }

  /**
   * 隐藏标签页可见性处理
   * 标签页从隐藏变为可见时，seek 到 live edge（与参考实现一致）
   */
  private setupVisibilityHandler() {
    this.visibilityHandler = () => {
      if (!document.hidden && this.videoElement && this.sourceBuffer) {
        const buffered = this.sourceBuffer.buffered
        if (buffered.length > 0) {
          const end = buffered.end(buffered.length - 1)
          const lag = end - this.videoElement.currentTime
          if (lag > 1.5) {
            console.log(`[MsePlayer] Tab visible, lag ${lag.toFixed(1)}s, seeking to live edge`)
            this.videoElement.currentTime = end - 0.5
          }
        }
        this.videoElement.play().catch(() => {})
      }
    }
    document.addEventListener('visibilitychange', this.visibilityHandler)
  }

  private startTrimTimer() {
    this.stopTrimTimer()
    this.trimTimer = window.setInterval(() => this.trimOldBuffer(), this.BUFFER_TRIM_INTERVAL)
  }

  private stopTrimTimer() {
    if (this.trimTimer !== null) {
      clearInterval(this.trimTimer)
      this.trimTimer = null
    }
  }

  /**
   * 停止播放并清理资源
   */
  stop() {
    this.alive = false
    this.streamingStarted = false
    this.stopTrimTimer()

    // 重置 B-帧重排缓冲
    this.videoTrackId = -1
    this.pendingRef = null
    this.pendingBFrames = []
    this.frameDuration = 0
    this.lastBmdt = -1
    this.bmdtSamples = 0
    this.diagCount = 0
    this.ctoDiagLogged = false

    // 移除 visibilitychange 监听器
    if (this.visibilityHandler) {
      document.removeEventListener('visibilitychange', this.visibilityHandler)
      this.visibilityHandler = null
    }

    // 关闭 WebSocket
    if (this.ws) {
      this.ws.onopen = null
      this.ws.onmessage = null
      this.ws.onclose = null
      this.ws.onerror = null
      if (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING) {
        this.ws.close()
      }
      this.ws = null
    }
    this.queue = []

    // 清理 SourceBuffer
    if (this.sourceBuffer && this.mediaSource?.readyState === 'open') {
      try {
        this.sourceBuffer.abort()
        this.mediaSource.removeSourceBuffer(this.sourceBuffer)
      } catch (e) {
        // 忽略
      }
    }
    this.sourceBuffer = null

    // 清理 MediaSource
    if (this.mediaSource?.readyState === 'open') {
      try {
        this.mediaSource.endOfStream()
      } catch (e) {
        // 忽略
      }
    }
    this.mediaSource = null

    // 释放 ObjectURL
    if (this.objectUrl) {
      URL.revokeObjectURL(this.objectUrl)
      this.objectUrl = null
    }

    this.videoElement = null
    this.callbacks = {}
  }

  /**
   * 是否正在运行
   */
  get isRunning(): boolean {
    return this.alive
  }

  // ============ fMP4 二进制工具 ============

  /**
   * 从 fMP4 segment 读取 tfdt 中的 baseMediaDecodeTime
   * 返回 -1 表示未找到
   */
  static readBmdt(data: ArrayBuffer): number {
    try {
      const view = new DataView(data)
      // 找到 moof → traf → tfdt
      const tfdtOff = MseStreamPlayer.findBox(view, 0, data.byteLength, 'moof', 'traf', 'tfdt')
      if (tfdtOff < 0) return -1
      // tfdt: size(4) + type(4) + version(1) + flags(3) + baseMediaDecodeTime(4 or 8)
      const version = view.getUint8(tfdtOff + 8)
      if (version === 1) {
        // 64-bit BMDT — 读低32位（高位通常为0）
        return view.getUint32(tfdtOff + 16)
      }
      return view.getUint32(tfdtOff + 12)
    } catch { return -1 }
  }

  /**
   * 从 init segment 的 moov/trak/mdia/hdlr 解析视频 track_id
   * 查找 handler_type='vide' 的 trak，返回其 tkhd track_ID
   */
  static findVideoTrackId(data: ArrayBuffer): number {
    try {
      const view = new DataView(data)
      const len = data.byteLength

      // 找 moov box
      let moovOff = -1, moovEnd = 0
      let off = 0
      while (off + 8 < len) {
        const size = view.getUint32(off)
        if (size < 8 || off + size > len) break
        if (view.getUint8(off+4)===0x6D && view.getUint8(off+5)===0x6F &&
            view.getUint8(off+6)===0x6F && view.getUint8(off+7)===0x76) { // 'moov'
          moovOff = off + 8
          moovEnd = off + size
          break
        }
        off += size
      }
      if (moovOff < 0) return 1

      // 遍历 moov 中的 trak boxes
      off = moovOff
      while (off + 8 < moovEnd) {
        const size = view.getUint32(off)
        if (size < 8 || off + size > moovEnd) break
        if (view.getUint8(off+4)===0x74 && view.getUint8(off+5)===0x72 &&
            view.getUint8(off+6)===0x61 && view.getUint8(off+7)===0x6B) { // 'trak'
          const trakEnd = off + size
          let trackId = -1
          let isVideo = false

          // 遍历 trak 内部的 boxes
          let t = off + 8
          while (t + 8 < trakEnd) {
            const tSize = view.getUint32(t)
            if (tSize < 8 || t + tSize > trakEnd) break

            // tkhd: 获取 track_ID
            if (view.getUint8(t+4)===0x74 && view.getUint8(t+5)===0x6B &&
                view.getUint8(t+6)===0x68 && view.getUint8(t+7)===0x64) { // 'tkhd'
              const ver = view.getUint8(t + 8)
              // tkhd v0: version(1)+flags(3)+creation(4)+modification(4)+track_ID(4)
              // tkhd v1: version(1)+flags(3)+creation(8)+modification(8)+track_ID(4)
              trackId = view.getUint32(t + (ver === 0 ? 20 : 28))
            }

            // mdia: 搜索 hdlr 获取 handler_type
            if (view.getUint8(t+4)===0x6D && view.getUint8(t+5)===0x64 &&
                view.getUint8(t+6)===0x69 && view.getUint8(t+7)===0x61) { // 'mdia'
              const mdiaEnd = t + tSize
              let m = t + 8
              while (m + 8 < mdiaEnd) {
                const mSize = view.getUint32(m)
                if (mSize < 8 || m + mSize > mdiaEnd) break
                if (view.getUint8(m+4)===0x68 && view.getUint8(m+5)===0x64 &&
                    view.getUint8(m+6)===0x6C && view.getUint8(m+7)===0x72) { // 'hdlr'
                  // hdlr: size(4)+type(4)+version+flags(4)+pre_defined(4)+handler_type(4)
                  if (m + 20 <= mdiaEnd &&
                      view.getUint8(m+16)===0x76 && view.getUint8(m+17)===0x69 &&
                      view.getUint8(m+18)===0x64 && view.getUint8(m+19)===0x65) { // 'vide'
                    isVideo = true
                  }
                }
                m += mSize
              }
            }
            t += tSize
          }

          if (isVideo && trackId > 0) return trackId
        }
        off += size
      }
      return 1 // 默认 track_id=1
    } catch { return 1 }
  }

  /**
   * 从 media segment 的 moof/traf/tfhd 读取 track_id
   */
  static readTfhdTrackId(data: ArrayBuffer): number {
    try {
      const view = new DataView(data)
      const tfhdOff = MseStreamPlayer.findBox(view, 0, data.byteLength, 'moof', 'traf', 'tfhd')
      if (tfhdOff < 0) return -1
      // tfhd: size(4) + type(4) + version+flags(4) + track_ID(4)
      return view.getUint32(tfhdOff + 12)
    } catch { return -1 }
  }

  /**
   * 读取 unsigned exp-golomb 编码值（H.264 slice header 解析用）
   * @returns [value, newBitPos]
   */
  static readExpGolomb(bytes: Uint8Array, bitPos: number): [number, number] {
    let zeros = 0
    const totalBits = bytes.length * 8
    while (bitPos < totalBits) {
      if ((bytes[bitPos >> 3] >> (7 - (bitPos & 7))) & 1) break
      zeros++
      bitPos++
    }
    bitPos++ // 跳过 '1' bit
    let value = 0
    for (let i = 0; i < zeros && bitPos < totalBits; i++) {
      value = (value << 1) | ((bytes[bitPos >> 3] >> (7 - (bitPos & 7))) & 1)
      bitPos++
    }
    return [(1 << zeros) - 1 + value, bitPos]
  }

  /**
   * 分类 fMP4 segment：视频参考帧 / 视频B帧 / 非视频（音频等）
   *
   * ★ 使用 H.264 slice_type（而非 nal_ref_idc）来区分 B 帧：
   *   - nal_ref_idc 不可靠：hierarchical B prediction 中，reference B-frame
   *     的 nal_ref_idc > 0，会被误判为 P 帧
   *   - slice_type % 5 === 1 → B-slice（无论 nal_ref_idc 值）
   *   - slice_type % 5 === 0 → P-slice, === 2 → I-slice
   *   - IDR NAL (type 5) → 始终为 ref (I 帧)
   */
  static classifySegment(data: ArrayBuffer): 'ref' | 'bframe' | 'non-video' {
    try {
      const view = new DataView(data)
      let offset = 0
      // 查找 mdat box
      while (offset + 8 < data.byteLength) {
        const size = view.getUint32(offset)
        if (size < 8 || offset + size > data.byteLength) break
        if (view.getUint8(offset + 4) === 0x6D && view.getUint8(offset + 5) === 0x64 &&
            view.getUint8(offset + 6) === 0x61 && view.getUint8(offset + 7) === 0x74) {
          // mdat 找到 — 遍历 AVCC 格式 NAL units (4-byte length prefix)
          const mdatEnd = offset + size
          let nalOff = offset + 8
          while (nalOff + 5 <= mdatEnd) {
            const nalLen = view.getUint32(nalOff)
            if (nalLen < 1 || nalOff + 4 + nalLen > mdatEnd) break
            const nalHeader = view.getUint8(nalOff + 4)
            const nalType = nalHeader & 0x1F
            if (nalType === 5) {
              // IDR slice → 始终 I 帧
              return 'ref'
            }
            if (nalType === 1 && nalLen >= 2) {
              // Non-IDR slice → 解析 slice_type (exp-golomb)
              // slice header: first_mb_in_slice (ue), slice_type (ue), ...
              const hdrBytes = Math.min(nalLen - 1, 8)
              const bytes = new Uint8Array(data, nalOff + 5, hdrBytes)
              const [, pos1] = MseStreamPlayer.readExpGolomb(bytes, 0) // skip first_mb_in_slice
              const [sliceType] = MseStreamPlayer.readExpGolomb(bytes, pos1)
              // slice_type % 5: 0=P, 1=B, 2=I, 3=SP, 4=SI
              return (sliceType % 5) === 1 ? 'bframe' : 'ref'
            }
            nalOff += 4 + nalLen
          }
          return 'non-video' // mdat 无 VCL NAL → 音频段
        }
        offset += size
      }
      return 'non-video' // 无 mdat → 非 media segment
    } catch { return 'non-video' }
  }

  /**
   * 注入 compositionTimeOffset (CTO) 到 fMP4 segment 的 trun atom
   *
   * 原始 trun (flags=0x1, 仅 data_offset):
   *   size | 'trun' | ver+flags | sample_count | data_offset
   *
   * 修改后 (flags=0x801, 加 CTO):
   *   size+4*N | 'trun' | ver=1+flags | sample_count | data_offset+4*N | [CTO]*N
   *
   * 同时更新 traf size, moof size, data_offset
   */
  static injectCTO(data: ArrayBuffer, cto: number): ArrayBuffer {
    try {
      const view = new DataView(data)

      // 定位 moof, traf, trun 的偏移
      const moofOff = MseStreamPlayer.findBoxOffset(view, 0, data.byteLength, 'moof')
      if (moofOff < 0) return data
      const moofSize = view.getUint32(moofOff)
      const moofEnd = moofOff + moofSize

      const trafOff = MseStreamPlayer.findBoxOffset(view, moofOff + 8, moofEnd, 'traf')
      if (trafOff < 0) return data
      const trafSize = view.getUint32(trafOff)

      const trunOff = MseStreamPlayer.findBoxOffset(view, trafOff + 8, trafOff + trafSize, 'trun')
      if (trunOff < 0) return data
      const trunSize = view.getUint32(trunOff)

      // 读取 trun 内容
      const flags = (view.getUint8(trunOff + 9) << 16) |
                    (view.getUint8(trunOff + 10) << 8) |
                     view.getUint8(trunOff + 11)
      if (flags & 0x800) return data // CTO 已存在，无需修改

      const sampleCount = view.getUint32(trunOff + 12)
      const extraBytes = sampleCount * 4 // 每个 sample 增加 4 bytes CTO

      // 计算 trun 中各 per-sample 字段在 header 后的偏移
      let trunHeaderLen = 12 + 4 // version+flags(4) + sample_count(4)
      if (flags & 0x1) trunHeaderLen += 4  // data_offset
      if (flags & 0x4) trunHeaderLen += 4  // first_sample_flags

      // 计算每个 sample 的字段长度（不含 CTO）
      let perSampleLen = 0
      if (flags & 0x100) perSampleLen += 4 // sample_duration
      if (flags & 0x200) perSampleLen += 4 // sample_size
      if (flags & 0x400) perSampleLen += 4 // sample_flags

      // 构建新 ArrayBuffer
      const newBuf = new ArrayBuffer(data.byteLength + extraBytes)
      const src = new Uint8Array(data)
      const dst = new Uint8Array(newBuf)
      const dstView = new DataView(newBuf)

      // 拷贝 trun header 及之前的所有数据（包括 moof/traf/trun 头部）
      dst.set(src.subarray(0, trunOff + trunHeaderLen), 0)

      // 补丁 trun: size, version, flags, data_offset
      dstView.setUint32(trunOff, trunSize + extraBytes) // 新 trun size
      dstView.setUint8(trunOff + 8, 1) // version = 1 (支持有符号 CTO)
      const newFlags = flags | 0x800
      dstView.setUint8(trunOff + 9, (newFlags >> 16) & 0xFF)
      dstView.setUint8(trunOff + 10, (newFlags >> 8) & 0xFF)
      dstView.setUint8(trunOff + 11, newFlags & 0xFF)
      if (flags & 0x1) {
        const oldDataOffset = view.getInt32(trunOff + 16)
        dstView.setInt32(trunOff + 16, oldDataOffset + extraBytes)
      }

      // 拷贝 per-sample 数据并插入 CTO
      let srcPos = trunOff + trunHeaderLen
      let dstPos = trunOff + trunHeaderLen

      for (let i = 0; i < sampleCount; i++) {
        // 拷贝原有 per-sample 字段
        if (perSampleLen > 0) {
          dst.set(src.subarray(srcPos, srcPos + perSampleLen), dstPos)
          srcPos += perSampleLen
          dstPos += perSampleLen
        }
        // 插入 CTO (signed int32)
        dstView.setInt32(dstPos, cto)
        dstPos += 4
      }

      // 拷贝 trun 之后的所有数据（其余 traf boxes + mdat）
      const afterTrun = trunOff + trunSize
      dst.set(src.subarray(afterTrun), dstPos)

      // 更新 moof size
      dstView.setUint32(moofOff, moofSize + extraBytes)
      // 更新 traf size
      dstView.setUint32(trafOff, trafSize + extraBytes)

      return newBuf
    } catch (e) {
      // 解析失败时返回原始数据（不阻塞播放）
      return data
    }
  }

  /**
   * 在 fMP4 数据中查找嵌套 box 的偏移（支持多级路径）
   */
  private static findBox(view: DataView, start: number, end: number, ...path: string[]): number {
    let offset = start
    let searchEnd = end
    for (let i = 0; i < path.length; i++) {
      const found = MseStreamPlayer.findBoxOffset(view, offset, searchEnd, path[i])
      if (found < 0) return -1
      if (i < path.length - 1) {
        // 容器 box：搜索内部
        searchEnd = found + view.getUint32(found)
        offset = found + 8
      } else {
        return found
      }
    }
    return -1
  }

  /**
   * 在范围内查找指定类型的 box，返回 box 起始偏移
   */
  private static findBoxOffset(view: DataView, start: number, end: number, type: string): number {
    let offset = start
    const c0 = type.charCodeAt(0), c1 = type.charCodeAt(1), c2 = type.charCodeAt(2), c3 = type.charCodeAt(3)
    while (offset + 8 <= end) {
      const size = view.getUint32(offset)
      if (size < 8) break
      if (view.getUint8(offset + 4) === c0 && view.getUint8(offset + 5) === c1 &&
          view.getUint8(offset + 6) === c2 && view.getUint8(offset + 7) === c3) {
        return offset
      }
      offset += size
    }
    return -1
  }

  // ============ fMP4 诊断工具 ============

  /**
   * 诊断 fMP4 segment：检查 trun 中是否有 compositionTimeOffset (CTO)
   * ★ 如果 CTO 缺失 (flag 0x800 未设置)，则 PTS=DTS，B 帧显示顺序错误
   *   → 表现为 "1-2-3, 2-3-4" 锯齿模式
   */
  static diagnoseFmp4(data: ArrayBuffer, index: number): void {
    try {
      const view = new DataView(data)
      let offset = 0
      while (offset + 8 < data.byteLength) {
        const size = view.getUint32(offset)
        if (size < 8 || offset + size > data.byteLength) break
        const type = String.fromCharCode(
          view.getUint8(offset + 4), view.getUint8(offset + 5),
          view.getUint8(offset + 6), view.getUint8(offset + 7)
        )
        if (type === 'moof') {
          MseStreamPlayer.scanBox(view, offset + 8, offset + size, index)
          return
        }
        offset += size
      }
    } catch { /* ignore parse errors */ }
  }

  private static scanBox(view: DataView, start: number, end: number, index: number): void {
    let offset = start
    while (offset + 8 < end) {
      const size = view.getUint32(offset)
      if (size < 8) break
      const type = String.fromCharCode(
        view.getUint8(offset + 4), view.getUint8(offset + 5),
        view.getUint8(offset + 6), view.getUint8(offset + 7)
      )
      if (type === 'traf') {
        MseStreamPlayer.scanBox(view, offset + 8, offset + size, index)
        return
      }
      if (type === 'trun') {
        // trun is FullBox: version(1) + flags(3)
        const flags = (view.getUint8(offset + 9) << 16) |
                      (view.getUint8(offset + 10) << 8) |
                       view.getUint8(offset + 11)
        const hasCTO = (flags & 0x800) !== 0
        if (index === 0) {
          console.log(`[MsePlayer] ★ fMP4 trun flags=0x${flags.toString(16)}, compositionTimeOffset=${hasCTO}`)
          if (!hasCTO) {
            console.warn(
              '[MsePlayer] ★ CTO NOT set → B-frame PTS=DTS → 显示顺序错误!\n' +
              '  这是 "1-2-3, 2-3-4" 模式的根因。\n' +
              '  解决：使用原生 video.src (stream.mp4) 代替 MSE。'
            )
          }
        }
        return
      }
      offset += size
    }
  }
}
