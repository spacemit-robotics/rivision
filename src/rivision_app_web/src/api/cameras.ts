import { apiClient, type CameraCreateParams, type CameraUpdateParams } from './client'

export const camerasApi = {
  list() {
    return apiClient.get('/cameras')
  },
  get(id: string) {
    return apiClient.get(`/cameras/${id}`)
  },
  create(data: CameraCreateParams) {
    return apiClient.post('/cameras', data)
  },
  update(id: string, data: CameraUpdateParams) {
    return apiClient.put(`/cameras/${id}`, data)
  },
  delete(id: string) {
    return apiClient.delete(`/cameras/${id}`)
  },
  stream(id: string) {
    return apiClient.get(`/cameras/${id}/stream`)
  },
}
