// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <el-dialog
    v-model="visible"
    :title="task ? `任务详情: ${task.task_id}` : '任务详情'"
    width="700px"
    @close="handleClose"
  >
    <div v-if="task" class="task-details">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="任务ID">{{ task.task_id }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(task.status)">{{ getStatusLabel(task.status) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="类型">{{ getTypeLabel(task.task_type) }}</el-descriptions-item>
        <el-descriptions-item label="帧进度">{{ task.frame_index ?? 0 }} / {{ task.total_frames ?? '-' }}</el-descriptions-item>
        <el-descriptions-item label="开始时间">{{ formatTimestamp(task.start_time) }}</el-descriptions-item>
        <el-descriptions-item label="结束时间">{{ formatTimestamp(task.end_time) }}</el-descriptions-item>
        <el-descriptions-item label="执行节点">{{ task.node_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="进度">
          <el-progress :percentage="getProgress(task)" :status="getProgressStatus(task.status)" />
        </el-descriptions-item>
      </el-descriptions>
      
      <div v-if="task.error" class="error-section">
        <h4>错误信息</h4>
        <el-alert :title="task.error" type="error" :closable="false" />
      </div>
      
      <div v-if="task.result" class="result-section">
        <h4>执行结果</h4>
        <pre>{{ JSON.stringify(task.result, null, 2) }}</pre>
      </div>
    </div>
    <div v-else class="empty-state">
      <el-empty description="无任务数据" />
    </div>
    
    <template #footer>
      <el-button @click="handleClose">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Task {
  task_id: string
  status: string
  type?: string
  task_type?: string
  priority?: number
  created_at?: string
  completed_at?: string
  start_time?: number
  end_time?: number
  node_id?: string
  progress?: number
  frame_index?: number
  total_frames?: number
  error?: string
  result?: Record<string, any>
}

const props = defineProps<{
  modelValue: boolean
  task: Task | null
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
    pending: 'info',
    running: 'primary',
    completed: 'success',
    failed: 'danger',
    cancelled: 'warning'
  }
  return types[status] || 'info'
}

const getProgressStatus = (status: string) => {
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'exception'
  return undefined
}

const formatTimestamp = (timestamp?: number) => {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString('zh-CN')
}

const getStatusLabel = (status: string) => {
  const labels: Record<string, string> = {
    pending: '等待中',
    running: '运行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消'
  }
  return labels[status] || status
}

const getTypeLabel = (type?: string) => {
  if (!type || type === 'unknown') return '推理'
  const labels: Record<string, string> = {
    image: '图片',
    video: '视频',
    search: '搜索',
    summary: '摘要',
    detection: '检测',
    report: '报告'
  }
  return labels[type] || type
}

const getProgress = (task: any) => {
  if (task.total_frames > 0) {
    return Math.round((task.frame_index / task.total_frames) * 100)
  }
  return task.status === 'completed' ? 100 : 0
}
</script>

<style scoped>
.task-details {
  padding: 10px 0;
}

.error-section,
.result-section {
  margin-top: 20px;
}

.error-section h4,
.result-section h4 {
  margin-bottom: 10px;
  color: #606266;
}

.result-section pre {
  background: #f5f7fa;
  padding: 15px;
  border-radius: 4px;
  overflow-x: auto;
  font-size: 12px;
  max-height: 300px;
}

.empty-state {
  padding: 40px 0;
}
</style>
