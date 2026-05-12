<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="visible" class="modal-overlay" @click="handleClose">
        <div class="modal" :class="size" @click.stop>
          <!-- 头部 -->
          <div class="modal-header">
            <div class="header-content">
              <div v-if="icon" class="header-icon">{{ icon }}</div>
              <div class="header-text">
                <h3>{{ title }}</h3>
                <p v-if="subtitle" class="subtitle">{{ subtitle }}</p>
              </div>
            </div>
            <button class="close-btn" @click="handleClose">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18"/>
                <line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>

          <!-- 内容区域 -->
          <div class="modal-body">
            <!-- 默认插槽 -->
            <slot>
              <!-- 键值对详情展示 -->
              <div v-if="details && details.length > 0" class="details-grid">
                <div 
                  v-for="item in details" 
                  :key="item.label"
                  class="detail-item"
                  :class="{ 'full-width': item.fullWidth }"
                >
                  <div class="detail-label">{{ item.label }}</div>
                  <div class="detail-value" :class="item.type">
                    <!-- 状态类型 -->
                    <template v-if="item.type === 'status'">
                      <span class="status-badge" :class="item.status">
                        {{ item.value }}
                      </span>
                    </template>
                    
                    <!-- 进度条类型 -->
                    <template v-else-if="item.type === 'progress'">
                      <div class="progress-bar">
                        <div 
                          class="progress-fill" 
                          :style="{ width: `${item.value}%` }"
                          :class="getProgressClass(item.value)"
                        ></div>
                        <span class="progress-text">{{ item.value }}%</span>
                      </div>
                    </template>
                    
                    <!-- 标签列表类型 -->
                    <template v-else-if="item.type === 'tags'">
                      <div class="tags-list">
                        <span 
                          v-for="(tag, idx) in item.value" 
                          :key="idx"
                          class="tag"
                        >
                          {{ tag }}
                        </span>
                      </div>
                    </template>
                    
                    <!-- 链接类型 -->
                    <template v-else-if="item.type === 'link'">
                      <a :href="item.value" target="_blank" class="detail-link">
                        {{ item.value }}
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>
                          <polyline points="15,3 21,3 21,9"/>
                          <line x1="10" y1="14" x2="21" y2="3"/>
                        </svg>
                      </a>
                    </template>
                    
                    <!-- 代码类型 -->
                    <template v-else-if="item.type === 'code'">
                      <code class="detail-code">{{ item.value }}</code>
                    </template>
                    
                    <!-- 图片类型 -->
                    <template v-else-if="item.type === 'image'">
                      <img :src="item.value" alt="" class="detail-image" @click="$emit('image-click', item.value)" />
                    </template>
                    
                    <!-- 时间类型 -->
                    <template v-else-if="item.type === 'time'">
                      <span class="time-value">
                        {{ formatTime(item.value) }}
                        <span class="time-relative">{{ getRelativeTime(item.value) }}</span>
                      </span>
                    </template>
                    
                    <!-- 默认文本类型 -->
                    <template v-else>
                      {{ item.value ?? '-' }}
                    </template>
                  </div>
                </div>
              </div>
            </slot>
          </div>

          <!-- 底部操作 -->
          <div v-if="$slots.footer || actions.length > 0" class="modal-footer">
            <slot name="footer">
              <button 
                v-for="action in actions" 
                :key="action.label"
                class="btn"
                :class="action.type || 'secondary'"
                :disabled="action.disabled"
                @click="handleAction(action)"
              >
                <span v-if="action.icon" class="btn-icon">{{ action.icon }}</span>
                {{ action.label }}
              </button>
            </slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
// ============ 类型定义 ============
export interface DetailItem {
  label: string
  value: any
  type?: 'text' | 'status' | 'progress' | 'tags' | 'link' | 'code' | 'image' | 'time'
  status?: 'online' | 'offline' | 'warning' | 'error' | 'success'
  fullWidth?: boolean
}

export interface ActionButton {
  label: string
  type?: 'primary' | 'secondary' | 'danger' | 'outline'
  icon?: string
  disabled?: boolean
  handler?: () => void
}

// ============ Props ============
interface Props {
  visible: boolean
  title: string
  subtitle?: string
  icon?: string
  size?: 'small' | 'medium' | 'large'
  details?: DetailItem[]
  actions?: ActionButton[]
}

const props = withDefaults(defineProps<Props>(), {
  size: 'medium',
  details: () => [],
  actions: () => [],
})

// ============ Emits ============
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'close'): void
  (e: 'action', action: ActionButton): void
  (e: 'image-click', src: string): void
}>()

// ============ 方法 ============
function handleClose() {
  emit('update:visible', false)
  emit('close')
}

function handleAction(action: ActionButton) {
  if (action.handler) {
    action.handler()
  }
  emit('action', action)
}

function getProgressClass(value: number): string {
  if (value > 80) return 'critical'
  if (value > 60) return 'warning'
  return 'normal'
}

function formatTime(dateStr: string): string {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function getRelativeTime(dateStr: string): string {
  if (!dateStr) return ''
  
  const date = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  
  if (diff < 60000) return '(刚刚)'
  if (diff < 3600000) return `(${Math.floor(diff / 60000)} 分钟前)`
  if (diff < 86400000) return `(${Math.floor(diff / 3600000)} 小时前)`
  if (diff < 604800000) return `(${Math.floor(diff / 86400000)} 天前)`
  
  return ''
}
</script>

<style scoped>
/* 过渡动画 */
.modal-enter-active,
.modal-leave-active {
  transition: all 0.3s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .modal,
.modal-leave-to .modal {
  transform: translateY(-20px) scale(0.95);
}

/* 遮罩层 */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
  padding: 20px;
}

/* 弹窗主体 */
.modal {
  background: #fff;
  border-radius: 16px;
  width: 100%;
  max-height: 90vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
}

.modal.small { max-width: 400px; }
.modal.medium { max-width: 560px; }
.modal.large { max-width: 800px; }

/* 头部 */
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 20px 24px;
  border-bottom: 1px solid #e5e7eb;
}

.header-content {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}

.header-icon {
  font-size: 28px;
  line-height: 1;
}

.header-text h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #1a1a2e;
}

.subtitle {
  margin: 4px 0 0;
  font-size: 14px;
  color: #6b7280;
}

.close-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  color: #9ca3af;
  padding: 4px;
  border-radius: 6px;
  transition: all 0.2s ease;
  flex-shrink: 0;
}

.close-btn svg {
  width: 20px;
  height: 20px;
}

.close-btn:hover {
  background: #f3f4f6;
  color: #374151;
}

/* 内容区域 */
.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}

/* 详情网格 */
.details-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.detail-item.full-width {
  grid-column: 1 / -1;
}

.detail-label {
  font-size: 12px;
  font-weight: 500;
  color: #6b7280;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.detail-value {
  font-size: 14px;
  color: #1a1a2e;
  word-break: break-word;
}

/* 状态徽章 */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
}

.status-badge::before {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-badge.online {
  background: #d1fae5;
  color: #059669;
}
.status-badge.online::before { background: #10b981; }

.status-badge.offline {
  background: #fee2e2;
  color: #dc2626;
}
.status-badge.offline::before { background: #ef4444; }

.status-badge.warning {
  background: #fef3c7;
  color: #b45309;
}
.status-badge.warning::before { background: #f59e0b; }

.status-badge.success {
  background: #d1fae5;
  color: #059669;
}
.status-badge.success::before { background: #10b981; }

.status-badge.error {
  background: #fee2e2;
  color: #dc2626;
}
.status-badge.error::before { background: #ef4444; }

/* 进度条 */
.progress-bar {
  position: relative;
  height: 8px;
  background: #e5e7eb;
  border-radius: 4px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: #3b82f6;
  border-radius: 4px;
  transition: width 0.3s ease;
}

.progress-fill.warning { background: #f59e0b; }
.progress-fill.critical { background: #ef4444; }

.progress-text {
  position: absolute;
  right: 0;
  top: -18px;
  font-size: 12px;
  font-weight: 600;
  color: #374151;
}

/* 标签列表 */
.tags-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.tag {
  padding: 4px 10px;
  background: #f3f4f6;
  border-radius: 6px;
  font-size: 13px;
  color: #374151;
}

/* 链接 */
.detail-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #3b82f6;
  text-decoration: none;
  font-size: 14px;
}

.detail-link:hover {
  text-decoration: underline;
}

.detail-link svg {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}

/* 代码 */
.detail-code {
  display: block;
  padding: 10px 14px;
  background: #f3f4f6;
  border-radius: 8px;
  font-family: 'SF Mono', Monaco, Consolas, monospace;
  font-size: 13px;
  color: #374151;
  overflow-x: auto;
}

/* 图片 */
.detail-image {
  max-width: 100%;
  max-height: 200px;
  border-radius: 8px;
  cursor: pointer;
  transition: transform 0.2s ease;
}

.detail-image:hover {
  transform: scale(1.02);
}

/* 时间 */
.time-value {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.time-relative {
  font-size: 12px;
  color: #6b7280;
}

/* 底部操作 */
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 16px 24px;
  border-top: 1px solid #e5e7eb;
  background: #f9fafb;
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 10px 18px;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn.primary {
  background: #3b82f6;
  color: #fff;
}

.btn.primary:hover:not(:disabled) {
  background: #2563eb;
}

.btn.secondary {
  background: #e5e7eb;
  color: #374151;
}

.btn.secondary:hover:not(:disabled) {
  background: #d1d5db;
}

.btn.danger {
  background: #ef4444;
  color: #fff;
}

.btn.danger:hover:not(:disabled) {
  background: #dc2626;
}

.btn.outline {
  background: transparent;
  border: 1px solid #e5e7eb;
  color: #374151;
}

.btn.outline:hover:not(:disabled) {
  border-color: #3b82f6;
  color: #3b82f6;
}

.btn-icon {
  font-size: 16px;
}

/* 响应式 */
@media (max-width: 640px) {
  .details-grid {
    grid-template-columns: 1fr;
  }
  
  .modal-footer {
    flex-direction: column;
  }
  
  .btn {
    width: 100%;
    justify-content: center;
  }
}
</style>
