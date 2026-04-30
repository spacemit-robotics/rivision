<template>
  <div class="system-activity-feed">
    <div v-if="loading" class="loading-state">
      <el-skeleton :rows="3" animated />
    </div>
    
    <div v-else-if="activities.length === 0" class="empty-state">
      <el-icon class="empty-icon"><InfoFilled /></el-icon>
      <div class="empty-text">暂无系统活动</div>
    </div>
    
    <div v-else class="activity-list">
      <div
        v-for="activity in activities"
        :key="activity.id"
        class="activity-item"
        :class="activity.type"
      >
        <div class="activity-icon">
          <el-icon>
            <component :is="getActivityIcon(activity.icon)" />
          </el-icon>
        </div>
        
        <div class="activity-content">
          <div class="activity-title">{{ activity.title }}</div>
          <div class="activity-message">{{ activity.message }}</div>
          <div class="activity-time">{{ formatTime(activity.timestamp) }}</div>
        </div>
        
        <div class="activity-status">
          <div class="status-dot" :class="activity.type"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { InfoFilled } from '@element-plus/icons-vue'

// Props
interface Activity {
  id: string
  type: 'info' | 'success' | 'warning' | 'error'
  title: string
  message: string
  timestamp: string
  icon: string
}

interface Props {
  activities: Activity[]
  loading?: boolean
}
defineProps<Props>()

// 工具函数
const getActivityIcon = (iconName: string) => {
  // 动态返回图标组件名称
  return iconName || 'InfoFilled'
}

const formatTime = (timestamp: string) => {
  const time = new Date(timestamp)
  const now = new Date()
  const diff = now.getTime() - time.getTime()
  
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}小时前`
  
  return time.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}
</script>

<style scoped>
.system-activity-feed {
  height: 400px;
  overflow-y: auto;
}

.loading-state {
  padding: var(--spacing-lg);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-placeholder);
}

.empty-icon {
  font-size: 48px;
  margin-bottom: var(--spacing-md);
}

.empty-text {
  font-size: var(--font-size-base);
}

.activity-list {
  padding: var(--spacing-sm) 0;
}

.activity-item {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
  border-radius: var(--border-radius-base);
  transition: background-color 0.3s;
  position: relative;
}

.activity-item:hover {
  background-color: var(--bg-page);
}

.activity-item:last-child {
  margin-bottom: 0;
}

.activity-icon {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  font-size: 16px;
  color: white;
}

.activity-item.info .activity-icon {
  background-color: var(--info-color);
}

.activity-item.success .activity-icon {
  background-color: var(--success-color);
}

.activity-item.warning .activity-icon {
  background-color: var(--warning-color);
}

.activity-item.error .activity-icon {
  background-color: var(--danger-color);
}

.activity-content {
  flex: 1;
  min-width: 0;
}

.activity-title {
  font-size: var(--font-size-base);
  font-weight: 500;
  color: var(--text-primary);
  margin-bottom: 2px;
  line-height: 1.4;
}

.activity-message {
  font-size: var(--font-size-small);
  color: var(--text-secondary);
  margin-bottom: var(--spacing-xs);
  line-height: 1.4;
  word-break: break-word;
}

.activity-time {
  font-size: var(--font-size-extra-small);
  color: var(--text-placeholder);
}

.activity-status {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-dot.info {
  background-color: var(--info-color);
}

.status-dot.success {
  background-color: var(--success-color);
}

.status-dot.warning {
  background-color: var(--warning-color);
}

.status-dot.error {
  background-color: var(--danger-color);
}

/* 滚动条样式 */
.system-activity-feed::-webkit-scrollbar {
  width: 4px;
}

.system-activity-feed::-webkit-scrollbar-track {
  background: transparent;
}

.system-activity-feed::-webkit-scrollbar-thumb {
  background: var(--border-base);
  border-radius: 2px;
}

.system-activity-feed::-webkit-scrollbar-thumb:hover {
  background: var(--border-dark);
}

/* 响应式设计 */
@media (max-width: 480px) {
  .activity-item {
    padding: var(--spacing-sm);
    gap: var(--spacing-sm);
  }
  
  .activity-icon {
    width: 24px;
    height: 24px;
    font-size: 14px;
  }
  
  .activity-title {
    font-size: var(--font-size-small);
  }
  
  .activity-message {
    font-size: var(--font-size-extra-small);
  }
}
</style>
