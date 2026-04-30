// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

import { defineStore } from 'pinia'
import { ref, computed, readonly } from 'vue'
import { gatewayApi } from '@/services/gatewayApi'
import type { NodeInfo, SystemStats, TaskInfo, YOLOPipelineStats } from '@/types/gateway'

export const useGatewayStore = defineStore('gateway', () => {
  // 状态
  const connectionStatus = ref<'connected' | 'connecting' | 'disconnected' | 'error'>('disconnected')
  // 动态获取Gateway URL：优先使用当前页面host，否则使用默认值
  const defaultGatewayUrl = typeof window !== 'undefined' 
    ? `${window.location.protocol}//${window.location.hostname}:8081`
    : 'http://localhost:8081'
  const gatewayUrl = ref(defaultGatewayUrl)
  const nodes = ref<NodeInfo[]>([])
  const systemStats = ref<SystemStats>({
    totalRequests: 0,
    successRate: 0,
    averageResponseTime: 0,
    healthyNodes: 0,
    totalNodes: 0
  })
  const currentTasks = ref<TaskInfo[]>([])
  const yoloPipelineStats = ref<YOLOPipelineStats | null>(null)
  const previousNodeCount = ref<number>(-1)
  const nodeChangeNotification = ref<string>('')
  const websocket = ref<WebSocket | null>(null)
  const wsReconnectTimer = ref<ReturnType<typeof setTimeout> | null>(null)
  const lastError = ref<string>('')

  // 计算属性
  const isConnected = computed(() => connectionStatus.value === 'connected')
  const healthyNodeCount = computed(() => nodes.value.filter(n => n.healthy).length)
  // ★ VLM 任务使用：有 llama 服务的健康节点数
  const llamaHealthyNodeCount = computed(() => nodes.value.filter(n => n.healthy && n.llama_healthy).length)
  // ★ YOLO 任务使用：有 yolo 服务的健康节点数
  const yoloHealthyNodeCount = computed(() => nodes.value.filter(n => n.healthy && n.yolo_healthy).length)
  const nodeStatusSummary = computed(() => {
    const total = nodes.value.length
    const healthy = healthyNodeCount.value
    const offline = total - healthy
    
    return {
      total,
      healthy,
      offline,
      healthyPercentage: total > 0 ? Math.round((healthy / total) * 100) : 0
    }
  })

  // 初始化连接
  async function initialize(url?: string) {
    if (url) {
      gatewayUrl.value = url
    }
    
    connectionStatus.value = 'connecting'
    lastError.value = ''

    try {
      // 测试HTTP连接
      const healthCheck = await gatewayApi.healthCheck()
      if (!healthCheck.success) {
        throw new Error('Gateway健康检查失败')
      }

      // 尝试建立WebSocket连接（可选，不阻塞初始化）
      try {
        await connectWebSocket()
      } catch (wsError) {
        console.warn('⚠️ WebSocket连接失败，使用HTTP轮询模式:', wsError)
      }
      
      // 获取初始数据
      await Promise.all([
        fetchNodeStatus(),
        fetchSystemStats(),
        fetchCurrentTasks()
      ])

      connectionStatus.value = 'connected'
      console.log('✅ Gateway连接成功')
      
    } catch (error) {
      connectionStatus.value = 'error'
      lastError.value = error instanceof Error ? error.message : '连接失败'
      console.error('❌ Gateway连接失败:', error)
      throw error
    }
  }

  // WebSocket连接（使用原生WebSocket）
  async function connectWebSocket() {
    return new Promise<void>((resolve, reject) => {
      // 构建WebSocket URL：使用gatewayUrl转换为ws协议
      const wsUrl = gatewayUrl.value.replace(/^http/, 'ws') + '/api/v1/ws/live'
      
      console.log(`🔌 正在连接WebSocket: ${wsUrl}`)
      console.log(`🔌 gatewayUrl: ${gatewayUrl.value}`)
      
      try {
        websocket.value = new WebSocket(wsUrl)
      } catch (error) {
        console.error('WebSocket创建失败:', error)
        reject(new Error('WebSocket创建失败'))
        return
      }

      // 连接超时处理
      const timeoutId = setTimeout(() => {
        if (websocket.value?.readyState !== WebSocket.OPEN) {
          websocket.value?.close()
          reject(new Error('WebSocket连接超时'))
        }
      }, 10000)

      websocket.value.onopen = () => {
        clearTimeout(timeoutId)
        console.log('🔌 WebSocket连接已建立')
        resolve()
      }

      websocket.value.onerror = (event) => {
        clearTimeout(timeoutId)
        console.error('WebSocket连接错误:', event)
        reject(new Error('WebSocket连接失败'))
      }

      websocket.value.onmessage = (event) => {
        handleWebSocketMessage(event.data)
      }

      websocket.value.onclose = (event) => {
        console.warn(`WebSocket连接断开: code=${event.code}, reason=${event.reason}`)
        
        // 自动重连（延迟5秒）
        if (connectionStatus.value === 'connected') {
          connectionStatus.value = 'disconnected'
          scheduleReconnect()
        }
      }
    })
  }

  // 处理WebSocket消息
  function handleWebSocketMessage(data: string) {
    try {
      const message = JSON.parse(data)
      
      if (message.type === 'welcome') {
        console.log('📡 WebSocket欢迎消息:', message.message)
        return
      }
      
      if (message.type === 'pong') {
        return // 心跳响应
      }
      
      const channel = message.channel
      const payload = message.data
      
      if (channel === 'nodes') {
        // 更新节点状态
        if (payload.nodes) {
          nodes.value = payload.nodes
        }
        if (payload.stats) {
          systemStats.value.totalNodes = payload.stats.total || 0
          systemStats.value.healthyNodes = payload.stats.healthy || 0
        }
      } else if (channel === 'stats') {
        // 更新系统统计
        systemStats.value = {
          ...systemStats.value,
          totalRequests: payload.total_requests || systemStats.value.totalRequests,
          successRate: payload.success_rate || systemStats.value.successRate,
          averageResponseTime: payload.avg_response_time || systemStats.value.averageResponseTime
        }
      } else if (channel === 'tasks') {
        // 更新任务列表
        if (payload.tasks) {
          currentTasks.value = payload.tasks
        }
      }
    } catch (error) {
      console.error('WebSocket消息解析错误:', error)
    }
  }

  // 计划重连
  function scheduleReconnect() {
    if (wsReconnectTimer.value) {
      clearTimeout(wsReconnectTimer.value)
    }
    
    wsReconnectTimer.value = setTimeout(async () => {
      if (connectionStatus.value !== 'connected') {
        console.log('🔄 尝试重连WebSocket...')
        try {
          await connectWebSocket()
          console.log('✅ WebSocket重连成功')
        } catch (error) {
          console.warn('⚠️ WebSocket重连失败，将继续使用HTTP轮询')
        }
      }
    }, 5000)
  }

  // 发送WebSocket消息
  function sendWebSocketMessage(action: string, data?: Record<string, unknown>) {
    if (websocket.value?.readyState === WebSocket.OPEN) {
      websocket.value.send(JSON.stringify({ action, ...data }))
    }
  }

  // 获取节点状态
  async function fetchNodeStatus() {
    try {
      const response = await gatewayApi.getNodeStatus()
      if (response.success && response.data) {
        nodes.value = response.data.nodes || []
        console.debug(`📊 更新节点状态: ${nodes.value.length} 个节点`)
      }
    } catch (error) {
      console.error('获取节点状态失败:', error)
    }
  }

  // 获取系统统计
  async function fetchSystemStats() {
    try {
      const response = await gatewayApi.getSystemStats()
      if (response.success && response.data) {
        systemStats.value = {
          totalRequests: response.data.totalRequests || 0,
          successRate: response.data.successRate || 0,
          averageResponseTime: response.data.averageResponseTime || 0,
          healthyNodes: response.data.healthyNodes || healthyNodeCount.value,
          totalNodes: response.data.totalNodes || nodes.value.length
        }
        console.debug('📈 更新系统统计')
      }
    } catch (error) {
      console.error('获取系统统计失败:', error)
    }
  }

  // 任务统计（从后端获取）
  const taskStats = ref({
    cancelledCount: 0,
    completedCount: 0
  })

  // ★ 获取 YOLO Pipeline 统计
  async function fetchYoloPipelineStats() {
    try {
      const response = await fetch('/api/yolo/pipeline-stats')
      if (response.ok) {
        const data = await response.json()
        if (data.success && data.data) {
          const newStats = data.data as YOLOPipelineStats
          yoloPipelineStats.value = newStats
          // ★ 节点变化通知
          const newCount = newStats.healthy_nodes
          if (previousNodeCount.value >= 0 && newCount !== previousNodeCount.value) {
            nodeChangeNotification.value = `节点变化: ${previousNodeCount.value} → ${newCount}`
            // 5秒后自动清除通知
            setTimeout(() => { nodeChangeNotification.value = '' }, 5000)
          }
          previousNodeCount.value = newCount
        }
      }
    } catch (e) {
      // 静默失败（Go端可能未启动分布式模式）
    }
  }

  // 获取当前任务
  async function fetchCurrentTasks() {
    try {
      const response = await gatewayApi.getCurrentTasks()
      if (response.success && response.data) {
        currentTasks.value = response.data.tasks || []
        // 更新统计
        taskStats.value.cancelledCount = (response.data as any).cancelled_count || 0
        taskStats.value.completedCount = (response.data as any).completed_count || 0
        console.debug(`📋 更新当前任务: ${currentTasks.value.length} 个任务, 已取消: ${taskStats.value.cancelledCount}`)
      }
    } catch (error) {
      console.error('获取当前任务失败:', error)
    }
  }

  // 提交分析任务
  async function submitAnalysisTask(taskData: any): Promise<string> {
    try {
      const response = await gatewayApi.submitAnalysisTask(taskData)
      if (response.success && response.data?.taskId) {
        console.log(`✅ 任务提交成功: ${response.data.taskId}`)
        return response.data.taskId
      } else {
        throw new Error(response.error || '任务提交失败')
      }
    } catch (error) {
      console.error('提交任务失败:', error)
      throw error
    }
  }

  // 获取任务状态
  async function getTaskStatus(taskId: string): Promise<TaskInfo | null> {
    try {
      const response = await gatewayApi.getTaskStatus(taskId)
      if (response.success && response.data) {
        return response.data
      }
      return null
    } catch (error) {
      console.error('获取任务状态失败:', error)
      return null
    }
  }

  // 取消任务
  async function cancelTask(taskId: string, reason: string) {
    try {
      const response = await gatewayApi.cancelTask(taskId, reason)
      if (response.success) {
        console.log(`❌ 任务已取消: ${taskId}`)
        // 刷新任务列表以获取最新状态
        await fetchCurrentTasks()
        return true
      }
      return false
    } catch (error) {
      console.error('取消任务失败:', error)
      return false
    }
  }

  // 取消主任务
  async function cancelMainTask(mainTaskId: string, reason: string = '用户取消主任务') {
    try {
      const response = await gatewayApi.cancelMainTask(mainTaskId, reason)
      if (response.success) {
        const cancelledCount = (response.data as any)?.cancelled_count || 0
        console.log(`❌ 主任务已取消: ${mainTaskId}, 已取消 ${cancelledCount} 个子任务`)
        // 刷新任务列表以获取最新状态
        await fetchCurrentTasks()
        return cancelledCount
      }
      return 0
    } catch (error) {
      console.error('取消主任务失败:', error)
      return 0
    }
  }

  // 强制取消任务
  async function forceCancelTask(taskId: string, reason: string = '强制取消') {
    try {
      const response = await gatewayApi.forceCancelTask(taskId, reason)
      if (response.success) {
        console.log(`⚠️ 强制取消任务: ${taskId}`)
        // 刷新任务列表以获取最新状态
        await fetchCurrentTasks()
        return true
      }
      return false
    } catch (error) {
      console.error('强制取消任务失败:', error)
      return false
    }
  }

  // 刷新数据
  async function refresh() {
    if (!isConnected.value) return

    try {
      await Promise.all([
        fetchNodeStatus(),
        fetchSystemStats(),
        fetchCurrentTasks()
      ])
      console.log('🔄 数据刷新完成')
    } catch (error) {
      console.error('数据刷新失败:', error)
    }
  }

  // 断开连接
  function disconnect() {
    // 清除重连定时器
    if (wsReconnectTimer.value) {
      clearTimeout(wsReconnectTimer.value)
      wsReconnectTimer.value = null
    }
    
    // 关闭WebSocket连接
    if (websocket.value) {
      websocket.value.close()
      websocket.value = null
    }
    connectionStatus.value = 'disconnected'
    console.log('🔌 Gateway连接已断开')
  }

  // 重置状态
  function reset() {
    nodes.value = []
    currentTasks.value = []
    systemStats.value = {
      totalRequests: 0,
      successRate: 0,
      averageResponseTime: 0,
      healthyNodes: 0,
      totalNodes: 0
    }
    lastError.value = ''
  }

  return {
    // 状态
    connectionStatus: readonly(connectionStatus),
    gatewayUrl: readonly(gatewayUrl),
    nodes: readonly(nodes),
    systemStats: readonly(systemStats),
    currentTasks: readonly(currentTasks),
    taskStats: readonly(taskStats),
    yoloPipelineStats: readonly(yoloPipelineStats),
    nodeChangeNotification: readonly(nodeChangeNotification),
    lastError: readonly(lastError),
    
    // 计算属性
    isConnected,
    healthyNodeCount,
    llamaHealthyNodeCount,
    yoloHealthyNodeCount,
    nodeStatusSummary,
    
    // 方法
    initialize,
    fetchNodeStatus,
    fetchSystemStats,
    fetchCurrentTasks,
    fetchYoloPipelineStats,
    submitAnalysisTask,
    getTaskStatus,
    cancelTask,
    cancelMainTask,
    forceCancelTask,
    refresh,
    disconnect,
    reset
  }
})
