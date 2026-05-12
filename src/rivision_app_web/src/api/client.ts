/**
 * RiVision API 客户端
 * 独立的API层，支持多种后端配置
 */
import axios, { AxiosInstance, AxiosRequestConfig } from 'axios'

// API 配置
export interface ApiConfig {
  baseURL: string
  timeout?: number
  token?: string
}

// 默认配置
const defaultConfig: ApiConfig = {
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 30000,
}

// 创建 API 客户端
export function createApiClient(config: Partial<ApiConfig> = {}): AxiosInstance {
  const finalConfig = { ...defaultConfig, ...config }

  const client = axios.create({
    baseURL: finalConfig.baseURL,
    timeout: finalConfig.timeout,
    headers: {
      'Content-Type': 'application/json',
    },
  })

  // 请求拦截器
  client.interceptors.request.use(
    (config) => {
      const token = localStorage.getItem('rivision_token')
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
      return config
    },
    (error) => Promise.reject(error)
  )

  // 响应拦截器
  client.interceptors.response.use(
    (response) => response.data,
    (error) => {
      if (error.response?.status === 401) {
        // 未授权，清除token
        localStorage.removeItem('rivision_token')
        window.location.href = '/login'
      }
      return Promise.reject(error)
    }
  )

  return client
}

// 默认客户端实例
export const apiClient = createApiClient()

// ===== 搜索 API =====
export const searchApi = {
  // 文本搜索
  searchByText(query: string, options: SearchOptions = {}) {
    return apiClient.post('/search/text', { query, ...options })
  },

  // 图像搜索
  searchByImage(image: string, options: SearchOptions = {}) {
    return apiClient.post('/search/image', { image, ...options })
  },

  // 混合搜索
  searchHybrid(params: HybridSearchParams) {
    return apiClient.post('/search/hybrid', params)
  },

  // 事件搜索
  searchEvents(params: EventSearchParams) {
    return apiClient.post('/search/event', params)
  },
}

// ===== 摄像头 API =====
export const cameraApi = {
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

  snapshot(id: string) {
    return apiClient.get(`/cameras/${id}/snapshot`)
  },
}

// ===== 节点 API =====
export const nodeApi = {
  list() {
    return apiClient.get('/nodes')
  },

  get(id: string) {
    return apiClient.get(`/nodes/${id}`)
  },

  enable(id: string) {
    return apiClient.post(`/nodes/${id}/enable`)
  },

  disable(id: string) {
    return apiClient.post(`/nodes/${id}/disable`)
  },

  stats() {
    return apiClient.get('/nodes/stats')
  },
}

// ===== 告警 API =====
export const alertApi = {
  list(params: AlertListParams = {}) {
    return apiClient.get('/alerts', { params })
  },

  get(id: string) {
    return apiClient.get(`/alerts/${id}`)
  },

  acknowledge(id: string) {
    return apiClient.post(`/alerts/${id}/acknowledge`)
  },

  resolve(id: string) {
    return apiClient.post(`/alerts/${id}/resolve`)
  },
}

// ===== 系统 API =====
export const systemApi = {
  status() {
    return apiClient.get('/system/status')
  },

  stats() {
    return apiClient.get('/stats/overview')
  },

  health() {
    return apiClient.get('/health')
  },
}

// ===== 类型定义 =====
export interface SearchOptions {
  cameras?: string[]
  time_range?: {
    start: string
    end: string
  }
  limit?: number
  offset?: number
}

export interface HybridSearchParams extends SearchOptions {
  text_query?: string
  image?: string
  text_weight?: number
  image_weight?: number
}

export interface EventSearchParams extends SearchOptions {
  event_type?: string
  severity?: string
}

export interface CameraCreateParams {
  name: string
  url?: string
  source?: string
  protocol?: string
  node_id?: string
  group_id?: string
  config?: Record<string, any>
  enabled?: boolean
}

export interface CameraUpdateParams {
  name?: string
  url?: string
  source?: string
  protocol?: string
  node_id?: string
  group_id?: string
  config?: Record<string, any>
  enabled?: boolean
}

export interface AlertListParams {
  status?: string
  severity?: string
  camera_id?: string
  limit?: number
  offset?: number
}
