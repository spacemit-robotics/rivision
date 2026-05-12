/**
 * OWL API — GB28181/ONVIF 设备发现与管理
 * 所有请求经由 Hub /api/v1/owl/* 代理
 */
import { apiClient } from './client'

// ─── 类型定义 ────────────────────────────────────────────────────────

export interface OwlDevice {
  id: string
  name: string
  manufacturer?: string
  model?: string
  firmware?: string
  transport?: string
  host?: string
  port?: number
  status?: string
  channels_count?: number
  online?: boolean
}

export interface OwlChannel {
  id: string
  channel_id?: string
  device_id?: string
  name: string
  manufacturer?: string
  model?: string
  status?: string
  ptz_type?: number
  stream_url?: string    // ONVIF 已有 RTSP URL (→ direct mode)
  protocol?: string      // gb28181 | onvif
  online?: boolean
}

export interface OwlServerInfo {
  version?: string
  sip_host?: string
  sip_port?: number
  sip_domain?: string
  sip_id?: string
  media_port_range?: string
}

export interface OwlDiscoverResult {
  devices: Array<{
    ip: string
    port?: number
    name?: string
    manufacturer?: string
    model?: string
    profiles?: Array<{
      token: string
      name: string
      stream_url?: string
    }>
  }>
}

export interface ImportChannelRequest {
  node_id?: string
  group_id?: string
  name?: string
}

export interface ImportBatchRequest {
  device_id?: string
  group_id?: string
  node_id?: string
}

export interface ImportResult {
  camera_id?: string
  channel_id?: string
  name?: string
  error?: string
}

// ─── API 方法 ────────────────────────────────────────────────────────

export const owlApi = {
  // ── 服务信息 ──
  serverInfo() {
    return apiClient.get('/owl/server-info')
  },

  capabilities() {
    return apiClient.get('/owl/capabilities')
  },

  // ── 设备管理 ──
  listDevices() {
    return apiClient.get('/owl/devices')
  },

  addDevice(data: { id: string; name?: string; transport?: string; host?: string; port?: number }) {
    return apiClient.post('/owl/devices', data)
  },

  deleteDevice(id: string) {
    return apiClient.delete(`/owl/devices/${encodeURIComponent(id)}`)
  },

  refreshCatalog(deviceId: string) {
    return apiClient.post(`/owl/devices/${encodeURIComponent(deviceId)}/catalog`)
  },

  // ── 通道管理 ──
  listChannels() {
    return apiClient.get('/owl/channels')
  },

  // ── 导入 ──
  importChannel(channelId: string, data: ImportChannelRequest = {}) {
    return apiClient.post(`/owl/channels/${encodeURIComponent(channelId)}/import`, data)
  },

  importBatch(data: ImportBatchRequest = {}) {
    return apiClient.post('/owl/import-batch', data)
  },

  // ── 流控制 ──
  playChannel(channelId: string) {
    return apiClient.post(`/owl/channels/${encodeURIComponent(channelId)}/play`)
  },

  stopChannel(channelId: string) {
    return apiClient.post(`/owl/channels/${encodeURIComponent(channelId)}/stop`)
  },

  snapshot(channelId: string) {
    return apiClient.post(`/owl/channels/${encodeURIComponent(channelId)}/snapshot`)
  },

  ptz(channelId: string, action: string, speed?: number) {
    return apiClient.post(`/owl/channels/${encodeURIComponent(channelId)}/ptz`, { action, speed })
  },

  // ── ONVIF 发现 ──
  discover() {
    return apiClient.get('/owl/discover')
  },
}
