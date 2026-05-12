import { apiClient } from './client'

export const analyticsApi = {
  traffic(params: { camera_id?: string; period?: string } = {}) {
    return apiClient.get('/analytics/traffic', { params })
  },
  heatmap(cameraId: string) {
    return apiClient.get('/analytics/heatmap', { params: { camera_id: cameraId } })
  },
  dwell(cameraId: string) {
    return apiClient.get('/analytics/dwell', { params: { camera_id: cameraId } })
  },
  export(format: string = 'csv') {
    return apiClient.get('/analytics/export', { params: { format } })
  },
}
