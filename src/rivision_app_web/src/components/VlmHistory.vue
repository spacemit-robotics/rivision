<template>
  <div class="vlm-history">
    <!-- 头部筛选栏 -->
    <div class="history-header">
      <div class="header-title">
        <h3>VLM 分析历史</h3>
        <span class="record-count">共 {{ total }} 条记录</span>
      </div>
      
      <div class="header-filters">
        <select v-model="filters.cameraId" class="filter-select">
          <option value="">全部摄像头</option>
          <option v-for="cam in cameras" :key="cam.id" :value="cam.id">
            {{ cam.name }}
          </option>
        </select>
        
        <select v-model="filters.timeRange" class="filter-select">
          <option value="today">今天</option>
          <option value="yesterday">昨天</option>
          <option value="week">最近7天</option>
          <option value="month">最近30天</option>
          <option value="all">全部</option>
        </select>
        
        <select v-model="filters.eventType" class="filter-select">
          <option value="">全部类型</option>
          <option value="scene">场景描述</option>
          <option value="behavior">行为分析</option>
          <option value="anomaly">异常检测</option>
          <option value="compliance">合规检查</option>
        </select>
        
        <div class="search-box">
          <svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8"/>
            <path d="m21 21-4.35-4.35"/>
          </svg>
          <input 
            v-model="filters.keyword"
            type="text" 
            class="search-input"
            placeholder="搜索分析内容..."
            @keyup.enter="() => fetchHistory()"
          />
        </div>
        
        <button class="btn-icon" @click="() => fetchHistory()" title="刷新">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/>
            <path d="M21 3v5h-5"/>
          </svg>
        </button>
        
        <button class="btn-icon" @click="exportHistory" title="导出">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="7,10 12,15 17,10"/>
            <line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- 历史列表 -->
    <div class="history-list" ref="listRef" @scroll="handleScroll">
      <div v-if="loading && records.length === 0" class="loading-state">
        <div class="loading-spinner"></div>
        <span>加载中...</span>
      </div>
      
      <div v-else-if="records.length === 0" class="empty-state">
        <div class="empty-icon">🔍</div>
        <div class="empty-text">暂无分析记录</div>
        <div class="empty-hint">VLM 分析结果将显示在这里</div>
      </div>
      
      <template v-else>
        <div 
          v-for="record in records" 
          :key="record.id"
          class="history-item"
          :class="{ expanded: expandedId === record.id }"
          @click="toggleExpand(record.id)"
        >
          <!-- 缩略图 -->
          <div class="item-thumbnail">
            <img 
              v-if="record.thumbnail" 
              :src="record.thumbnail" 
              alt="场景截图"
              @error="handleImageError"
            />
            <div v-else class="thumbnail-placeholder">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/>
                <circle cx="9" cy="9" r="2"/>
                <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"/>
              </svg>
            </div>
            
            <!-- 风险等级指示器 -->
            <div 
              v-if="record.riskLevel" 
              class="risk-badge"
              :class="record.riskLevel"
            >
              {{ getRiskLabel(record.riskLevel) }}
            </div>
          </div>
          
          <!-- 内容区域 -->
          <div class="item-content">
            <div class="item-header">
              <div class="item-meta">
                <span class="meta-time">{{ formatTime(record.createdAt) }}</span>
                <span class="meta-divider">|</span>
                <span class="meta-camera">{{ record.cameraName }}</span>
              </div>
              <div class="item-type">
                <span :class="['type-badge', record.eventType]">
                  {{ getTypeLabel(record.eventType) }}
                </span>
              </div>
            </div>
            
            <div class="item-description">
              {{ record.description }}
            </div>
            
            <!-- 展开详情 -->
            <div v-if="expandedId === record.id" class="item-details">
              <!-- 行为分析 -->
              <div v-if="record.behavior" class="detail-section">
                <h5>行为分析</h5>
                <p>{{ record.behavior }}</p>
              </div>
              
              <!-- 建议措施 -->
              <div v-if="record.suggestion" class="detail-section">
                <h5>建议措施</h5>
                <p>{{ record.suggestion }}</p>
              </div>
              
              <!-- 评分 -->
              <div v-if="record.score !== undefined" class="detail-section">
                <h5>合规评分</h5>
                <div class="score-bar">
                  <div class="score-fill" :style="{ width: `${record.score}%` }"></div>
                  <span class="score-value">{{ record.score }}分</span>
                </div>
              </div>
              
              <!-- 检测目标 -->
              <div v-if="record.detections && record.detections.length > 0" class="detail-section">
                <h5>检测目标</h5>
                <div class="detection-tags">
                  <span 
                    v-for="(det, idx) in record.detections" 
                    :key="idx"
                    class="detection-tag"
                  >
                    {{ det.class }} ({{ Math.round(det.confidence * 100) }}%)
                  </span>
                </div>
              </div>
              
              <!-- 操作按钮 -->
              <div class="detail-actions">
                <button class="action-btn" @click.stop="playVideo(record)">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polygon points="5,3 19,12 5,21"/>
                  </svg>
                  回放视频
                </button>
                <button class="action-btn" @click.stop="viewFullImage(record)">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/>
                  </svg>
                  查看原图
                </button>
                <button class="action-btn" @click.stop="createRule(record)">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M12 5v14m-7-7h14"/>
                  </svg>
                  创建规则
                </button>
              </div>
            </div>
          </div>
          
          <!-- 展开指示器 -->
          <div class="expand-indicator">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline :points="expandedId === record.id ? '18,15 12,9 6,15' : '6,9 12,15 18,9'"/>
            </svg>
          </div>
        </div>
        
        <!-- 加载更多 -->
        <div v-if="hasMore" class="load-more">
          <button v-if="!loading" class="load-more-btn" @click="loadMore">
            加载更多
          </button>
          <div v-else class="loading-more">
            <div class="loading-spinner small"></div>
            <span>加载中...</span>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, watch } from 'vue'
import axios from 'axios'

// ============ 类型定义 ============
export interface VlmRecord {
  id: string
  cameraId: string
  cameraName: string
  createdAt: string
  eventType: 'scene' | 'behavior' | 'anomaly' | 'compliance'
  description: string
  behavior?: string
  suggestion?: string
  riskLevel?: 'low' | 'medium' | 'high' | 'critical'
  score?: number
  thumbnail?: string
  fullImage?: string
  videoUrl?: string
  detections?: Array<{ class: string; confidence: number }>
}

export interface Camera {
  id: string
  name: string
}

// ============ Props ============
interface Props {
  cameras?: Camera[]
}

const props = withDefaults(defineProps<Props>(), {
  cameras: () => [],
})

// ============ Emits ============
const emit = defineEmits<{
  (e: 'play-video', record: VlmRecord): void
  (e: 'view-image', record: VlmRecord): void
  (e: 'create-rule', record: VlmRecord): void
}>()

// ============ 状态 ============
const listRef = ref<HTMLElement | null>(null)
const records = ref<VlmRecord[]>([])
const total = ref(0)
const loading = ref(false)
const hasMore = ref(true)
const expandedId = ref<string | null>(null)
const page = ref(1)
const pageSize = 20

const filters = reactive({
  cameraId: '',
  timeRange: 'today',
  eventType: '',
  keyword: '',
})

// ============ 方法 ============
async function fetchHistory(reset = true) {
  if (reset) {
    page.value = 1
    records.value = []
    hasMore.value = true
  }
  
  loading.value = true
  
  try {
    const res = await axios.get('/api/v1/vlm/history', {
      params: {
        page: page.value,
        pageSize,
        cameraId: filters.cameraId || undefined,
        timeRange: filters.timeRange,
        eventType: filters.eventType || undefined,
        keyword: filters.keyword || undefined,
      },
    })
    
    const data = res.data
    if (reset) {
      records.value = data.records || []
    } else {
      records.value.push(...(data.records || []))
    }
    total.value = data.total || 0
    hasMore.value = records.value.length < total.value
    
  } catch (e) {
    console.error('获取 VLM 历史失败', e)
  } finally {
    loading.value = false
  }
}

function loadMore() {
  page.value++
  fetchHistory(false)
}

function handleScroll() {
  if (!listRef.value || loading.value || !hasMore.value) return
  
  const { scrollTop, scrollHeight, clientHeight } = listRef.value
  if (scrollHeight - scrollTop - clientHeight < 100) {
    loadMore()
  }
}

function toggleExpand(id: string) {
  expandedId.value = expandedId.value === id ? null : id
}

function formatTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function getRiskLabel(level: string): string {
  const labels: Record<string, string> = {
    low: '低风险',
    medium: '中风险',
    high: '高风险',
    critical: '紧急',
  }
  return labels[level] || level
}

function getTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    scene: '场景描述',
    behavior: '行为分析',
    anomaly: '异常检测',
    compliance: '合规检查',
  }
  return labels[type] || type
}

function handleImageError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.display = 'none'
}

function playVideo(record: VlmRecord) {
  emit('play-video', record)
}

function viewFullImage(record: VlmRecord) {
  emit('view-image', record)
}

function createRule(record: VlmRecord) {
  emit('create-rule', record)
}

async function exportHistory() {
  try {
    const res = await axios.get('/api/v1/vlm/history/export', {
      params: filters,
      responseType: 'blob',
    })
    
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', `vlm-history-${new Date().toISOString().slice(0, 10)}.csv`)
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  } catch (e) {
    console.error('导出失败', e)
  }
}

// ============ 监听 ============
watch(filters, () => {
  fetchHistory()
}, { deep: true })

// ============ 生命周期 ============
onMounted(() => {
  fetchHistory()
})
</script>

<style scoped>
.vlm-history {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #fff;
  border-radius: 12px;
  overflow: hidden;
}

/* 头部筛选栏 */
.history-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
  flex-wrap: wrap;
  gap: 12px;
}

.header-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.header-title h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #1a1a2e;
}

.record-count {
  font-size: 13px;
  color: #6b7280;
}

.header-filters {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.filter-select {
  padding: 8px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  font-size: 13px;
  color: #374151;
  cursor: pointer;
}

.filter-select:focus {
  outline: none;
  border-color: #3b82f6;
}

.search-box {
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon {
  position: absolute;
  left: 10px;
  width: 16px;
  height: 16px;
  color: #9ca3af;
}

.search-input {
  padding: 8px 12px 8px 34px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  font-size: 13px;
  width: 180px;
}

.search-input:focus {
  outline: none;
  border-color: #3b82f6;
}

.btn-icon {
  width: 36px;
  height: 36px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #6b7280;
  transition: all 0.2s ease;
}

.btn-icon:hover {
  background: #f3f4f6;
  color: #374151;
}

.btn-icon svg {
  width: 18px;
  height: 18px;
}

/* 历史列表 */
.history-list {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.loading-state, .empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: #6b7280;
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid #e5e7eb;
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 12px;
}

.loading-spinner.small {
  width: 20px;
  height: 20px;
  border-width: 2px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.empty-text {
  font-size: 16px;
  font-weight: 500;
  color: #374151;
  margin-bottom: 4px;
}

.empty-hint {
  font-size: 13px;
}

/* 历史项 */
.history-item {
  display: flex;
  gap: 16px;
  padding: 16px;
  background: #f9fafb;
  border-radius: 12px;
  margin-bottom: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.history-item:hover {
  background: #f3f4f6;
}

.history-item.expanded {
  background: #f0f9ff;
  border: 1px solid #bfdbfe;
}

.item-thumbnail {
  position: relative;
  width: 120px;
  height: 80px;
  border-radius: 8px;
  overflow: hidden;
  flex-shrink: 0;
  background: #e5e7eb;
}

.item-thumbnail img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.thumbnail-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9ca3af;
}

.thumbnail-placeholder svg {
  width: 32px;
  height: 32px;
}

.risk-badge {
  position: absolute;
  top: 4px;
  left: 4px;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  color: #fff;
}

.risk-badge.low { background: #10b981; }
.risk-badge.medium { background: #f59e0b; }
.risk-badge.high { background: #ef4444; }
.risk-badge.critical { background: #dc2626; animation: pulse 1s infinite; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.7; }
}

.item-content {
  flex: 1;
  min-width: 0;
}

.item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.item-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: #6b7280;
}

.meta-divider {
  color: #d1d5db;
}

.type-badge {
  padding: 3px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 500;
}

.type-badge.scene { background: #dbeafe; color: #1d4ed8; }
.type-badge.behavior { background: #fef3c7; color: #b45309; }
.type-badge.anomaly { background: #fee2e2; color: #dc2626; }
.type-badge.compliance { background: #d1fae5; color: #059669; }

.item-description {
  font-size: 14px;
  color: #374151;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.history-item.expanded .item-description {
  -webkit-line-clamp: unset;
}

.item-details {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #e5e7eb;
}

.detail-section {
  margin-bottom: 16px;
}

.detail-section h5 {
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 600;
  color: #6b7280;
  text-transform: uppercase;
}

.detail-section p {
  margin: 0;
  font-size: 14px;
  color: #374151;
  line-height: 1.5;
}

.score-bar {
  height: 8px;
  background: #e5e7eb;
  border-radius: 4px;
  position: relative;
  overflow: hidden;
}

.score-fill {
  height: 100%;
  background: linear-gradient(90deg, #10b981, #3b82f6);
  border-radius: 4px;
  transition: width 0.3s ease;
}

.score-value {
  position: absolute;
  right: 0;
  top: -20px;
  font-size: 14px;
  font-weight: 600;
  color: #3b82f6;
}

.detection-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.detection-tag {
  padding: 4px 8px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  font-size: 12px;
  color: #374151;
}

.detail-actions {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fff;
  font-size: 13px;
  color: #374151;
  cursor: pointer;
  transition: all 0.2s ease;
}

.action-btn:hover {
  border-color: #3b82f6;
  color: #3b82f6;
}

.action-btn svg {
  width: 14px;
  height: 14px;
}

.expand-indicator {
  width: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9ca3af;
  flex-shrink: 0;
}

.expand-indicator svg {
  width: 16px;
  height: 16px;
  transition: transform 0.2s ease;
}

/* 加载更多 */
.load-more {
  display: flex;
  justify-content: center;
  padding: 16px;
}

.load-more-btn {
  padding: 10px 24px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  font-size: 14px;
  color: #374151;
  cursor: pointer;
  transition: all 0.2s ease;
}

.load-more-btn:hover {
  background: #f3f4f6;
}

.loading-more {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #6b7280;
  font-size: 13px;
}
</style>
