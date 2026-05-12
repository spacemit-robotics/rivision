import { apiClient } from './client'

export const nodesApi = {
  list() {
    return apiClient.get('/nodes')
  },
  status(id: string) {
    return apiClient.get(`/nodes/${id}/status`)
  },
}
