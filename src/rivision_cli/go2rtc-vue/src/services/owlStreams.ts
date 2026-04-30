// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

/**
 * owlStreams.ts — Unified camera stream source for go2rtc-vue
 *
 * Provides a single interface to fetch camera streams that works with:
 *   1. OWL GB28181  (primary: /api/owl/streams)
 *   2. go2rtc native (fallback: /api/go2rtc/streams)
 *
 * The frontend CameraList.vue calls getOWLStreams() — if OWL is not configured
 * (404 or network error), it transparently falls back to go2rtc streams.
 * This means the UI works unchanged in both modes.
 */

export interface StreamSource {
  id: string          // go2rtc stream key / OWL channel internal ID
  name: string        // display name
  type: string        // GB28181 | RTMP | RTSP | ONVIF
  online: boolean     // is the stream active
  source: string      // raw source URL or placeholder
  rtspUrl?: string   // RTSP URL from OWL.Play() (available after play)
  flvUrl?: string    // HTTP-FLV URL from OWL.Play()
  webrtcUrl?: string // WebRTC URL from OWL.Play()
}

interface Go2rtcStreamInfo {
  name?: string
  sources?: string[]
  producers?: Array<{ type?: string; url?: string }>
}

/** Fetch all camera streams. Tries OWL first, falls back to go2rtc. */
export async function getOWLStreams(): Promise<StreamSource[]> {
  const fetchGo2rtc = async (): Promise<StreamSource[]> => {
    const resp = await fetch('/api/go2rtc/streams')
    if (!resp.ok) return []
    const data: Record<string, Go2rtcStreamInfo> = await resp.json()
    return Object.entries(data)
      .filter(([name]) => !['status', 'payload'].includes(name))
      .map(([id, info]) => {
        const producers = info.producers ?? []
        const firstProducer = Array.isArray(producers) ? producers[0] : null
        const fallbackName = (typeof id === 'string' && id.trim()) ? id : '未命名摄像头'
        return {
          id,
          name: (info.name && info.name.trim()) || fallbackName,
          type: firstProducer?.type ?? 'unknown',
          online: Array.isArray(producers) ? producers.length > 0 : false,
          source: info.sources?.[0] ?? '',
        }
      })
  }

  try {
    const resp = await fetch('/api/owl/streams')
    if (!resp.ok) throw new Error(`OWL streams HTTP ${resp.status}`)
    const data: Record<string, Go2rtcStreamInfo> = await resp.json()
    const owlStreams = Object.entries(data)
      .filter(([name]) => !['status', 'payload', 'error'].includes(name))
      .map(([id, info]) => {
        const producers = info.producers ?? []
        const firstProducer = Array.isArray(producers) ? producers[0] : null
        const producerUrl = (firstProducer?.url || '').toLowerCase()
        const hasSource = Array.isArray(info.sources) && info.sources.length > 0
        const online = producerUrl === 'online' || (producerUrl !== 'offline' && (producers.length > 0 || hasSource))
        const fallbackName = (typeof id === 'string' && id.trim()) ? id : '未命名摄像头'
        return {
          id,
          name: (info.name && info.name.trim()) || fallbackName,
          type: firstProducer?.type ?? 'unknown',
          online,
          source: info.sources?.[0] ?? '',
        }
      })

    // OWL 返回空列表时，自动回退 go2rtc，避免前端“无摄像头”
    if (owlStreams.length > 0) {
      return owlStreams
    }
    return await fetchGo2rtc()
  } catch {
    return await fetchGo2rtc()
  }
}

/** Trigger OWL to start streaming a channel and return all available URLs. */
export async function playChannel(channelId: string): Promise<{
  app: string
  stream: string
  urls: Record<string, string>
} | null> {
  try {
    const resp = await fetch(`/api/owl/channels/${channelId}/play`, {
      method: 'POST',
    })
    if (!resp.ok) return null
    return await resp.json()
  } catch {
    return null
  }
}

/** Get the go2rtc-compatible stream URL for a channel.
 *  For OWL channels: triggers OWL.Play() first, then returns the go2rtc stream URL.
 *  For native go2rtc channels: returns the direct stream URL.
 */
export async function getStreamUrl(channelId: string): Promise<string | null> {
  const resp = await fetch(`/api/owl/channels/${channelId}/play`, {
    method: 'POST',
  })
  if (!resp.ok) {
    // Fallback: native go2rtc stream URL
    return `/api/go2rtc/stream.mp4?src=${encodeURIComponent(channelId)}`
  }
  try {
    const data = await resp.json()
    // After play, the channel is registered in go2rtc by the bridge.
    // Use the go2rtc stream.mp4 endpoint.
    return `/api/go2rtc/stream.mp4?src=${encodeURIComponent(channelId)}`
  } catch {
    return null
  }
}

/** 
 * Capture a snapshot from an OWL channel (via ZLM).
 * ⚠️ 调用方必须在使用完后调用 URL.revokeObjectURL(url) 释放内存！
 */
export async function captureSnapshot(channelId: string): Promise<string | null> {
  try {
    const resp = await fetch(`/api/owl/channels/${channelId}/snapshot`, {
      method: 'POST',
    })
    if (!resp.ok) return null
    const blob = await resp.blob()
    // ★ 返回 ObjectURL，调用方负责 revoke
    return URL.createObjectURL(blob)
  } catch {
    return null
  }
}

/** 已创建的 ObjectURL 缓存，用于手动释放 */
const _snapshotUrlCache = new Set<string>()

/** 创建 snapshot 并记录到缓存（便于批量清理） */
export async function captureSnapshotManaged(channelId: string): Promise<string | null> {
  const url = await captureSnapshot(channelId)
  if (url) _snapshotUrlCache.add(url)
  return url
}

/** 释放单个 snapshot URL */
export function revokeSnapshotUrl(url: string): void {
  if (_snapshotUrlCache.has(url)) {
    URL.revokeObjectURL(url)
    _snapshotUrlCache.delete(url)
  }
}

/** 释放所有 snapshot URLs（页面卸载时调用） */
export function revokeAllSnapshots(): void {
  _snapshotUrlCache.forEach(url => URL.revokeObjectURL(url))
  _snapshotUrlCache.clear()
}
