// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// YOLO API 服务
// 用于控制后端YOLO检测的启用/禁用

export interface YOLOStatus {
  enabled_cameras: string[]
  sync_frame_rate: number
  jpeg_quality: number
}

export interface YOLOSettings {
  sync_frame_rate?: number
  jpeg_quality?: number
}

class YOLOApiService {
  private baseUrl = '/api/yolo'

  // 获取YOLO状态
  async getStatus(): Promise<YOLOStatus> {
    const response = await fetch(`${this.baseUrl}/status`)
    if (!response.ok) {
      throw new Error(`Failed to get YOLO status: ${response.statusText}`)
    }
    return response.json()
  }

  // 启用摄像头YOLO
  async enable(cameraId: string): Promise<{ success: boolean; camera_id: string; enabled: boolean }> {
    const response = await fetch(`${this.baseUrl}/enable/${cameraId}`, {
      method: 'POST'
    })
    if (!response.ok) {
      throw new Error(`Failed to enable YOLO for ${cameraId}: ${response.statusText}`)
    }
    return response.json()
  }

  // 禁用摄像头YOLO
  async disable(cameraId: string): Promise<{ success: boolean; camera_id: string; enabled: boolean }> {
    const response = await fetch(`${this.baseUrl}/disable/${cameraId}`, {
      method: 'POST'
    })
    if (!response.ok) {
      throw new Error(`Failed to disable YOLO for ${cameraId}: ${response.statusText}`)
    }
    return response.json()
  }

  // 检查摄像头是否启用
  async check(cameraId: string): Promise<{ camera_id: string; enabled: boolean }> {
    const response = await fetch(`${this.baseUrl}/check/${cameraId}`)
    if (!response.ok) {
      throw new Error(`Failed to check YOLO for ${cameraId}: ${response.statusText}`)
    }
    return response.json()
  }

  // 更新设置
  async updateSettings(settings: YOLOSettings): Promise<YOLOStatus> {
    const response = await fetch(`${this.baseUrl}/settings`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(settings)
    })
    if (!response.ok) {
      throw new Error(`Failed to update YOLO settings: ${response.statusText}`)
    }
    return response.json()
  }

  // ★ 禁用所有摄像头的YOLO检测（清除状态）
  async disableAll(): Promise<{ success: boolean; disabled_count: number; enabled_cameras: string[] }> {
    const response = await fetch(`${this.baseUrl}/disable-all`, {
      method: 'POST'
    })
    if (!response.ok) {
      throw new Error(`Failed to disable all YOLO: ${response.statusText}`)
    }
    return response.json()
  }
}

export const yoloApi = new YOLOApiService()
export default yoloApi
