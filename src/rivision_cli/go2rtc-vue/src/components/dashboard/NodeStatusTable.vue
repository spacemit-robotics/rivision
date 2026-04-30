// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="node-status-table">
    <el-table
      :data="nodes"
      :loading="loading"
      empty-text="暂无节点数据"
      class="nodes-table"
      @row-click="handleRowClick"
    >
      <el-table-column prop="id" label="节点ID" min-width="120">
        <template #default="{ row }">
          <div class="node-id">
            <el-icon 
              class="node-icon" 
              :class="{ healthy: row.healthy, offline: !row.healthy }"
            >
              <Monitor />
            </el-icon>
            <span>{{ row.id }}</span>
          </div>
        </template>
      </el-table-column>
      
      <el-table-column prop="host" label="地址" min-width="140">
        <template #default="{ row }">
          <span>{{ row.host }}:{{ row.port }}</span>
        </template>
      </el-table-column>
      
      <el-table-column prop="healthy" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.healthy ? 'success' : 'danger'" size="small">
            {{ row.healthy ? '健康' : '离线' }}
          </el-tag>
        </template>
      </el-table-column>
      
      <el-table-column prop="connections" label="任务" width="80">
        <template #default="{ row }">
          <span class="task-count">{{ row.connections || 0 }}/{{ row.weight || 1 }}</span>
        </template>
      </el-table-column>
      
      <el-table-column prop="cpu_percent" label="CPU/内存" width="100">
        <template #default="{ row }">
          <span v-if="row.cpu_percent !== undefined">
            {{ Math.round(row.cpu_percent) }}%/{{ Math.round(row.memory_percent || 0) }}%
          </span>
          <span v-else class="text-placeholder">--</span>
        </template>
      </el-table-column>
      
      <el-table-column prop="last_check" label="最后检查" width="120">
        <template #default="{ row }">
          <span v-if="row.last_check" class="last-check">
            {{ formatTime(row.last_check) }}
          </span>
          <span v-else class="text-placeholder">从未检查</span>
        </template>
      </el-table-column>
      
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button 
            size="small" 
            @click.stop="handleCheckNode(row.id)"
            :disabled="!row.healthy"
          >
            检查
          </el-button>
          <el-button 
            size="small" 
            type="primary" 
            @click.stop="handleViewDetails(row)"
          >
            详情
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { Monitor } from '@element-plus/icons-vue'
import type { NodeInfo } from '@/types/gateway'

// Props
interface Props {
  nodes: NodeInfo[]
  loading?: boolean
}
defineProps<Props>()

// Emits
const emit = defineEmits(['refresh', 'check-node', 'view-details'])

// 工具函数
const getResponseTimeClass = (responseTime: number) => {
  if (responseTime < 100) return 'response-fast'
  if (responseTime < 500) return 'response-normal'
  return 'response-slow'
}

const formatTime = (timeStr: string) => {
  const time = new Date(timeStr)
  const now = new Date()
  const diff = now.getTime() - time.getTime()
  
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}小时前`
  return time.toLocaleDateString()
}

// 事件处理
const handleRowClick = (row: NodeInfo) => {
  emit('view-details', row)
}

const handleCheckNode = (nodeId: string) => {
  emit('check-node', nodeId)
  ElMessage.success(`正在检查节点 ${nodeId}`)
}

const handleViewDetails = (row: NodeInfo) => {
  emit('view-details', row)
}
</script>

<style scoped>
.node-status-table {
  width: 100%;
}

.node-id {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.node-icon {
  font-size: 16px;
  color: var(--text-placeholder);
  transition: color 0.3s;
}

.node-icon.healthy {
  color: var(--success-color);
}

.node-icon.offline {
  color: var(--danger-color);
}

.task-count {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: var(--font-size-small);
}

.response-fast {
  color: var(--success-color);
  font-weight: 500;
}

.response-normal {
  color: var(--warning-color);
}

.response-slow {
  color: var(--danger-color);
  font-weight: 500;
}

.last-check {
  font-size: var(--font-size-small);
  color: var(--text-secondary);
}

.text-placeholder {
  color: var(--text-placeholder);
  font-style: italic;
}

.nodes-table :deep(.el-table__row) {
  cursor: pointer;
}

.nodes-table :deep(.el-table__row:hover) {
  background-color: var(--bg-page);
}

.nodes-table :deep(.el-table__header) {
  background-color: var(--bg-page);
}

.nodes-table :deep(.el-table th) {
  background-color: var(--bg-page);
  font-weight: 600;
  color: var(--text-primary);
}

@media (max-width: 768px) {
  .nodes-table :deep(.el-table__column--fixed-right) {
    display: none;
  }
}
</style>
