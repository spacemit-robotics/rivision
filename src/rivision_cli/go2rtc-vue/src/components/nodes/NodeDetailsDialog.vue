<template>
  <el-dialog
    v-model="visible"
    :title="node ? `节点详情: ${node.id || node.node_id}` : '节点详情'"
    width="600px"
    @close="handleClose"
  >
    <div v-if="node" class="node-details">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="节点ID">{{ node.id || node.node_id }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(node.status)">{{ node.status }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="地址">{{ node.host || node.url || '-' }}</el-descriptions-item>
        <el-descriptions-item label="端口">{{ node.port || '-' }}</el-descriptions-item>
        <el-descriptions-item label="CPU使用率">{{ node.cpu_percent ? node.cpu_percent + '%' : '-' }}</el-descriptions-item>
        <el-descriptions-item label="内存使用率">{{ node.memory_percent ? node.memory_percent + '%' : '-' }}</el-descriptions-item>
        <el-descriptions-item label="最后心跳">{{ formatTime(node.last_heartbeat) }}</el-descriptions-item>
        <el-descriptions-item label="当前连接">{{ node.connections ?? '-' }}</el-descriptions-item>
      </el-descriptions>
      
      <div v-if="node.capabilities" class="capabilities-section">
        <h4>能力配置</h4>
        <pre>{{ JSON.stringify(node.capabilities, null, 2) }}</pre>
      </div>
    </div>
    <div v-else class="empty-state">
      <el-empty description="无节点数据" />
    </div>
    
    <template #footer>
      <el-button @click="handleClose">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Node {
  id?: string
  node_id?: string
  status: string
  ip_address?: string
  host?: string
  url?: string
  port?: number
  model?: string
  gpu_memory?: string
  last_heartbeat?: string
  registered_at?: string
  capabilities?: Record<string, any>
  cpu_percent?: number
  memory_percent?: number
  connections?: number
}

const props = defineProps<{
  modelValue: boolean
  node: Node | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const handleClose = () => {
  emit('update:modelValue', false)
}

const getStatusType = (status: string): 'success' | 'warning' | 'info' | 'danger' | 'primary' => {
  const types: Record<string, 'success' | 'warning' | 'info' | 'danger' | 'primary'> = {
    online: 'success',
    offline: 'danger',
    busy: 'warning',
    idle: 'info'
  }
  return types[status] || 'info'
}

const formatTime = (time?: string) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}
</script>

<style scoped>
.node-details {
  padding: 10px 0;
}

.capabilities-section {
  margin-top: 20px;
}

.capabilities-section h4 {
  margin-bottom: 10px;
  color: #606266;
}

.capabilities-section pre {
  background: #f5f7fa;
  padding: 15px;
  border-radius: 4px;
  overflow-x: auto;
  font-size: 12px;
}

.empty-state {
  padding: 40px 0;
}
</style>
