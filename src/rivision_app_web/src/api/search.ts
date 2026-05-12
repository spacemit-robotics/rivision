import { apiClient, type SearchOptions, type HybridSearchParams } from './client'

export const searchApi = {
  text(query: string, options: SearchOptions = {}) {
    return apiClient.post('/search/text', { query, ...options })
  },
  image(image: string, options: SearchOptions = {}) {
    return apiClient.post('/search/image', { image, ...options })
  },
  hybrid(params: HybridSearchParams) {
    return apiClient.post('/search/hybrid', params)
  },
  video(formData: FormData) {
    return apiClient.post('/search/video', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
}
