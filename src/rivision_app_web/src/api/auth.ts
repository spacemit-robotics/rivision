import { apiClient } from './client'

export const authApi = {
  login(username: string, password: string) {
    return apiClient.post('/auth/login', { username, password })
  },
  logout() {
    return apiClient.post('/auth/logout')
  },
  me() {
    return apiClient.get('/auth/me')
  },
  createToken(name: string) {
    return apiClient.post('/auth/tokens', { name })
  },
}
