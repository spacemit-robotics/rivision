import { apiClient } from './client'

export const knowledgeApi = {
  // Rules
  listRules() { return apiClient.get('/rules') },
  getRule(id: string) { return apiClient.get(`/rules/${id}`) },
  createRule(data: any) { return apiClient.post('/rules', data) },
  updateRule(id: string, data: any) { return apiClient.put(`/rules/${id}`, data) },
  deleteRule(id: string) { return apiClient.delete(`/rules/${id}`) },

  // Feature targets
  listTargets() { return apiClient.get('/targets') },
  getTarget(id: string) { return apiClient.get(`/targets/${id}`) },
  createTarget(data: FormData) {
    return apiClient.post('/targets', data, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
  deleteTarget(id: string) { return apiClient.delete(`/targets/${id}`) },

  // Prompts
  listPrompts() { return apiClient.get('/prompts') },
  updatePrompt(id: string, data: any) { return apiClient.put(`/prompts/${id}`, data) },
  createPrompt(data: any) { return apiClient.post('/prompts', data) },
}
