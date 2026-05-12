import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import axios from 'axios'

interface Node {
  id: string
  name: string
  type: string
  status: string
  host: string
  version?: string
  yolo_enabled: boolean
  llama_enabled: boolean
  vlm_model?: string
  embed_model?: string
  capabilities: Record<string, any> | string[]
  address: string
  last_heartbeat: string
  cpu_percent: number
  memory_percent: number
  npu_percent?: number
  load: number
  camera_count?: number
  connections?: number
  uptime_seconds?: number
}

export const useNodesStore = defineStore('nodes', () => {
  const nodes = ref<Node[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  const onlineNodes = computed(() => 
    nodes.value.filter(n => n.status === 'online')
  )

  const offlineNodes = computed(() => 
    nodes.value.filter(n => n.status === 'offline')
  )

  async function fetchNodes() {
    loading.value = true
    error.value = null
    
    try {
      const response = await axios.get('/api/v1/nodes')
      nodes.value = response.data.nodes || []
    } catch (e: any) {
      error.value = e.response?.data?.error || '获取节点列表失败'
    } finally {
      loading.value = false
    }
  }

  async function getNodeById(id: string) {
    try {
      const response = await axios.get(`/api/v1/nodes/${id}`)
      return response.data
    } catch (e: any) {
      throw new Error(e.response?.data?.error || '获取节点详情失败')
    }
  }

  return {
    nodes,
    loading,
    error,
    onlineNodes,
    offlineNodes,
    fetchNodes,
    getNodeById
  }
})
