import { apiClient } from './client'

export const configApi = {
  getAlgorithm() {
    return apiClient.get('/config/algorithm')
  },
  setAlgorithm(config: any) {
    return apiClient.post('/config/algorithm', config)
  },
}
