// API request/response types (§8.2)

export interface ApiResponse<T = any> {
  data: T
  error?: string
  status: number
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  limit: number
}

export interface SearchRequest {
  query?: string
  image?: string
  query_vec?: number[]
  cameras?: string[]
  time_start?: string
  time_end?: string
  limit?: number
  text_weight?: number
  image_weight?: number
}

export interface SearchResponse {
  results: SearchResult[]
  total_nodes: number
  success_nodes: number
  search_time_ms: number
}

export interface SearchResult {
  task_id: string
  detection_id?: string
  timestamp: string
  camera_id: string
  thumbnail?: string
  result: string
  node_id: string
  score: number
}

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  token: string
  user: {
    id: string
    username: string
    role: string
  }
}
