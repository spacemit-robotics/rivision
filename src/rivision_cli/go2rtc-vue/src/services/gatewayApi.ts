// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

/**
 * Gateway API 客户端服务
 * 
 * 提供与Inference Gateway后端的HTTP API交互功能
 */

import axios, { AxiosInstance, AxiosResponse } from 'axios'
import type { 
  ApiResponse, 
  NodeInfo, 
  SystemStats, 
  TaskInfo, 
  AnalysisConfig,
  PerformanceMetrics 
} from '@/types/gateway'

class GatewayApiClient {
  private client: AxiosInstance
  private baseURL: string

  constructor(baseURL: string = '/api') {
    this.baseURL = baseURL
    this.client = axios.create({
      baseURL,
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json'
      }
    })

    this.setupInterceptors()
  }

  private setupInterceptors() {
    // 请求拦截器
    this.client.interceptors.request.use(
      (config) => {
        console.debug(`🔗 API请求: ${config.method?.toUpperCase()} ${config.url}`)
        return config
      },
      (error) => {
        console.error('❌ 请求拦截器错误:', error)
        return Promise.reject(error)
      }
    )

    // 响应拦截器
    this.client.interceptors.response.use(
      (response: AxiosResponse) => {
        console.debug(`✅ API响应: ${response.config.url} - ${response.status}`)
        return response
      },
      (error) => {
        console.error('❌ 响应拦截器错误:', error.response?.status, error.message)
        
        // 统一错误处理
        if (error.response?.status === 500) {
          console.error('服务器内部错误')
        } else if (error.response?.status === 404) {
          console.error('接口不存在')
        } else if (error.code === 'ECONNABORTED') {
          console.error('请求超时')
        } else if (error.code === 'NETWORK_ERROR') {
          console.error('网络连接错误')
        }
        
        return Promise.reject(error)
      }
    )
  }

  /**
   * 健康检查
   */
  async healthCheck(): Promise<ApiResponse<{ status: string }>> {
    try {
      const response = await this.client.get('/v1/health')
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '健康检查失败'
      }
    }
  }

  /**
   * 获取节点状态
   */
  async getNodeStatus(): Promise<ApiResponse<{ nodes: NodeInfo[] }>> {
    try {
      const response = await this.client.get('/v1/nodes')
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取节点状态失败'
      }
    }
  }

  /**
   * 获取系统统计信息
   */
  async getSystemStats(): Promise<ApiResponse<SystemStats>> {
    try {
      const response = await this.client.get('/v1/stats/overview')
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取系统统计失败'
      }
    }
  }

  /**
   * 获取当前任务（运行中+等待中）
   */
  async getCurrentTasks(): Promise<ApiResponse<{ tasks: TaskInfo[] }>> {
    try {
      const response = await this.client.get('/v1/stats/tasks/pending')
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取当前任务失败'
      }
    }
  }

  /**
   * 获取性能指标
   */
  async getPerformanceMetrics(): Promise<ApiResponse<PerformanceMetrics[]>> {
    try {
      const response = await this.client.get('/v1/metrics/realtime')
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取性能指标失败'
      }
    }
  }

  /**
   * 取消单个任务
   */
  async cancelTask(taskId: string, reason: string = '用户取消'): Promise<ApiResponse<{ success: boolean }>> {
    try {
      const response = await this.client.post(`/v1/stats/tasks/${taskId}/cancel`, {
        reason
      })
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '取消任务失败'
      }
    }
  }

  /**
   * 取消主任务下的所有子任务
   */
  async cancelMainTask(mainTaskId: string, reason: string = '用户取消主任务'): Promise<ApiResponse<{ success: boolean, cancelled_count: number }>> {
    try {
      const response = await this.client.post(`/v1/stats/tasks/main/${mainTaskId}/cancel`, {
        reason
      })
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '取消主任务失败'
      }
    }
  }

  /**
   * 强制取消任务
   */
  async forceCancelTask(taskId: string, reason: string = '强制取消'): Promise<ApiResponse<{ success: boolean }>> {
    try {
      const response = await this.client.post(`/v1/stats/tasks/${taskId}/force-cancel`, {
        reason
      })
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '强制取消任务失败'
      }
    }
  }

  /**
   * 提交图片分析任务
   * 
   * 使用异步任务API，支持任务取消功能
   */
  async analyzeImage(imageFile: File, prompt: string): Promise<ApiResponse<{ taskId: string }>> {
    try {
      // 生成主任务ID
      const mainTaskId = Date.now().toString(36) + Math.random().toString(36).substr(2)
      
      // 读取图片为Base64
      const imageBase64 = await this.fileToBase64(imageFile)
      
      // 使用异步任务API提交（支持取消功能）
      const response = await this.client.post('/v1/async-tasks/submit', {
        image: imageBase64,
        prompt: prompt,
        task_type: 'image',
        main_task_id: mainTaskId,
        frame_index: 0,
        total_frames: 1,
        max_tokens: 500,
        temperature: 0.7
      })

      return {
        success: true,
        data: { taskId: response.data.task_id }
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '图片分析失败'
      }
    }
  }
  
  /**
   * 将File转换为Base64字符串
   */
  private async fileToBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result as string
        // 移除 data:image/xxx;base64, 前缀
        const base64 = result.split(',')[1] || result
        resolve(base64)
      }
      reader.onerror = reject
      reader.readAsDataURL(file)
    })
  }

  /**
   * 提交视频分析任务
   */
  async analyzeVideo(videoFile: File, config: AnalysisConfig): Promise<ApiResponse<{ taskId: string }>> {
    try {
      const formData = new FormData()
      formData.append('video', videoFile)
      formData.append('config', JSON.stringify(config))

      const response = await this.client.post('/v1/video/analyze', formData, {
        headers: {
          'Content-Type': 'multipart/form-data'
        },
        timeout: 60000 // 视频上传需要更长时间
      })

      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '视频分析失败'
      }
    }
  }

  /**
   * 提交异步分析任务
   */
  async submitAnalysisTask(taskData: any): Promise<ApiResponse<{ taskId: string }>> {
    try {
      const response = await this.client.post('/v1/async-tasks/submit', taskData)
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '提交任务失败'
      }
    }
  }

  /**
   * 获取任务状态
   */
  async getTaskStatus(taskId: string): Promise<ApiResponse<TaskInfo>> {
    try {
      const response = await this.client.get(`/v1/async-tasks/status/${taskId}`)
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取任务状态失败'
      }
    }
  }


  /**
   * 获取任务结果
   */
  async getTaskResult(taskId: string): Promise<ApiResponse<any>> {
    try {
      const response = await this.client.get(`/v1/async-tasks/result/${taskId}`)
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取任务结果失败'
      }
    }
  }

  /**
   * 检查节点健康状态
   */
  async checkNodeHealth(nodeId: string): Promise<ApiResponse<NodeInfo>> {
    try {
      const response = await this.client.post(`/v1/nodes/${nodeId}/check`)
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '检查节点失败'
      }
    }
  }

  /**
   * 检查所有节点健康状态
   */
  async checkAllNodesHealth(): Promise<ApiResponse<{ results: NodeInfo[] }>> {
    try {
      const response = await this.client.post('/v1/nodes/check-all')
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '批量检查节点失败'
      }
    }
  }

  /**
   * 获取节点详细统计
   */
  async getNodeStats(): Promise<ApiResponse<{ nodes: NodeInfo[] }>> {
    try {
      const response = await this.client.get('/v1/stats/nodes')
      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '获取节点统计失败'
      }
    }
  }

  /**
   * 上传文件并获取进度
   */
  async uploadFile(
    file: File, 
    onProgress?: (progress: number) => void
  ): Promise<ApiResponse<{ fileId: string, url: string }>> {
    try {
      const formData = new FormData()
      formData.append('file', file)

      const response = await this.client.post('/v1/files/upload', formData, {
        headers: {
          'Content-Type': 'multipart/form-data'
        },
        onUploadProgress: (progressEvent) => {
          if (progressEvent.total && onProgress) {
            const percentCompleted = Math.round((progressEvent.loaded * 100) / progressEvent.total)
            onProgress(percentCompleted)
          }
        }
      })

      return {
        success: true,
        data: response.data
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '文件上传失败'
      }
    }
  }

  /**
   * 设置基础URL
   */
  setBaseURL(url: string) {
    this.baseURL = url
    this.client.defaults.baseURL = url
  }

  /**
   * 获取当前基础URL
   */
  getBaseURL(): string {
    return this.baseURL
  }
}

// 创建默认实例
export const gatewayApi = new GatewayApiClient()

// 导出类以供其他地方使用
export { GatewayApiClient }
export default gatewayApi
