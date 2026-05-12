import { apiClient, type AlertListParams } from './client'

export const alertsApi = {
  list(params: AlertListParams = {}) {
    return apiClient.get('/alerts', { params })
  },
  get(id: string) {
    return apiClient.get(`/alerts/${id}`)
  },
  acknowledge(id: string) {
    return apiClient.put(`/alerts/${id}`, { status: 'acknowledged' })
  },
  resolve(id: string) {
    return apiClient.put(`/alerts/${id}`, { status: 'resolved' })
  },
}
