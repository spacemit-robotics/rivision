// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="analysis-progress">
    <div class="progress-header">
      <h4>{{ title }}</h4>
      <el-tag :type="statusType" size="small">{{ statusText }}</el-tag>
    </div>
    
    <el-progress
      :percentage="percentage"
      :status="progressStatus"
      :stroke-width="12"
      :text-inside="true"
    />
    
    <div class="progress-info">
      <div class="info-item">
        <span class="label">已处理帧数:</span>
        <span class="value">{{ processedFrames }} / {{ totalFrames }}</span>
      </div>
      <div class="info-item">
        <span class="label">耗时:</span>
        <span class="value">{{ formattedElapsed }}</span>
      </div>
      <div class="info-item">
        <span class="label">预计剩余:</span>
        <span class="value">{{ formattedRemaining }}</span>
      </div>
    </div>
    
    <div v-if="currentTask" class="current-task">
      <el-icon class="loading-icon" v-if="status === 'running'"><Loading /></el-icon>
      <span>{{ currentTask }}</span>
    </div>
    
    <div v-if="error" class="error-message">
      <el-alert :title="error" type="error" :closable="false" />
    </div>
    
    <div class="progress-actions">
      <el-button v-if="status === 'running'" type="warning" @click="$emit('pause')">
        暂停
      </el-button>
      <el-button v-if="status === 'paused'" type="primary" @click="$emit('resume')">
        继续
      </el-button>
      <el-button v-if="status !== 'completed'" type="danger" @click="$emit('cancel')">
        取消
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Loading } from '@element-plus/icons-vue'

type Status = 'pending' | 'running' | 'paused' | 'completed' | 'failed'

const props = withDefaults(defineProps<{
  title?: string
  status?: Status
  percentage?: number
  processedFrames?: number
  totalFrames?: number
  elapsedSeconds?: number
  remainingSeconds?: number
  currentTask?: string
  error?: string
}>(), {
  title: '视频分析进度',
  status: 'pending',
  percentage: 0,
  processedFrames: 0,
  totalFrames: 0,
  elapsedSeconds: 0,
  remainingSeconds: 0
})

defineEmits<{
  (e: 'pause'): void
  (e: 'resume'): void
  (e: 'cancel'): void
}>()

const statusType = computed((): 'success' | 'warning' | 'info' | 'danger' | 'primary' => {
  const types: Record<Status, 'success' | 'warning' | 'info' | 'danger' | 'primary'> = {
    pending: 'info',
    running: 'primary',
    paused: 'warning',
    completed: 'success',
    failed: 'danger'
  }
  return types[props.status]
})

const statusText = computed(() => {
  const texts: Record<Status, string> = {
    pending: '等待中',
    running: '分析中',
    paused: '已暂停',
    completed: '已完成',
    failed: '失败'
  }
  return texts[props.status]
})

const progressStatus = computed(() => {
  if (props.status === 'completed') return 'success'
  if (props.status === 'failed') return 'exception'
  return undefined
})

const formatTime = (seconds: number) => {
  if (seconds < 60) return `${Math.round(seconds)}秒`
  if (seconds < 3600) {
    const mins = Math.floor(seconds / 60)
    const secs = Math.round(seconds % 60)
    return `${mins}分${secs}秒`
  }
  const hours = Math.floor(seconds / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  return `${hours}小时${mins}分`
}

const formattedElapsed = computed(() => formatTime(props.elapsedSeconds))
const formattedRemaining = computed(() => {
  if (props.remainingSeconds <= 0) return '-'
  return formatTime(props.remainingSeconds)
})
</script>

<style scoped>
.analysis-progress {
  padding: 20px;
  background: #fff;
  border-radius: 8px;
  border: 1px solid #ebeef5;
}

.progress-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 15px;
}

.progress-header h4 {
  margin: 0;
  color: #303133;
}

.progress-info {
  display: flex;
  gap: 30px;
  margin-top: 15px;
  padding: 10px 0;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.info-item .label {
  font-size: 12px;
  color: #909399;
}

.info-item .value {
  font-size: 14px;
  color: #303133;
  font-weight: 500;
}

.current-task {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 15px;
  padding: 10px;
  background: #f5f7fa;
  border-radius: 4px;
  font-size: 13px;
  color: #606266;
}

.loading-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.error-message {
  margin-top: 15px;
}

.progress-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
  padding-top: 15px;
  border-top: 1px solid #ebeef5;
}
</style>
