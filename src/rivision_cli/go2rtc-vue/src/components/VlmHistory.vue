// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="vlm-history">
    <!-- 标题栏 -->
    <div class="history-header">
      <div class="header-left">
        <span class="header-icon">📊</span>
        <h3 class="header-title">VLM 分析历史</h3>
      </div>
      <div class="header-right">
        <button class="refresh-btn" @click="$emit('refresh')" title="刷新">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path d="M23 4v6h-6M1 20v-6h6"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <div class="filter-left">
        <span class="filter-label">筛选:</span>
        <select v-model="filterCamera" class="filter-select">
          <option value="">全部摄像头</option>
          <option v-for="cam in uniqueCameras" :key="cam.id" :value="cam.id">{{ cam.name || cam.id }}</option>
        </select>
        <select v-model="filterTime" class="filter-select">
          <option value="1h">最近1小时</option>
          <option value="6h">最近6小时</option>
          <option value="24h">最近24小时</option>
          <option value="all">全部</option>
        </select>
      </div>
      <div class="filter-right">
        <span class="item-count">共 {{ filteredItems.length }} 条记录</span>
      </div>
    </div>

    <!-- 历史记录列表 -->
    <div class="history-list" ref="listRef">
      <div v-if="filteredItems.length === 0" class="empty-state">
        <div class="empty-icon">🔍</div>
        <div class="empty-text">暂无分析记录</div>
        <div class="empty-hint">开启摄像头的 VLM 分析开始记录</div>
      </div>

      <div
        v-for="item in filteredItems"
        :key="item.id"
        class="history-item"
        :class="{ 'is-new': item.isNew }"
        @click="handleItemClick(item)"
      >
        <!-- 缩略图 -->
        <div class="item-thumbnail">
          <img 
            v-if="item.thumbnail" 
            :src="item.thumbnail" 
            :alt="item.id"
            loading="lazy"
          />
          <div v-else class="thumbnail-placeholder">
            <span>📷</span>
          </div>
        </div>

        <!-- 内容区 -->
        <div class="item-content">
          <!-- 元信息 -->
          <div class="item-meta">
            <span class="item-time">{{ formatTime(item.timestamp) }}</span>
            <span class="meta-divider">|</span>
            <span class="item-camera">{{ getCameraName(item.cameraId) }}</span>
            <span class="meta-divider" v-if="item.nodeId">|</span>
            <span class="item-node" v-if="item.nodeId">{{ item.nodeId }}</span>
            <span class="meta-divider" v-if="item.processingTime">|</span>
            <span class="item-duration" v-if="item.processingTime">{{ (item.processingTime / 1000).toFixed(1) }}s</span>
          </div>

          <!-- 分析结果 -->
          <div class="item-result">
            {{ truncateText(item.result, 120) }}
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="item-actions">
          <button 
            class="action-btn detail-btn" 
            @click.stop="handleViewDetail(item)"
            title="查看详情"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
              <circle cx="12" cy="12" r="3"/>
            </svg>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'

// ============ 类型定义 ============
export interface VlmHistoryItem {
  id: string
  timestamp: number
  cameraId: string
  cameraName?: string
  thumbnail?: string
  result: string
  nodeId?: string
  processingTime?: number
  resolution?: string
  isNew?: boolean
}

// ============ Props ============
export interface CameraOption {
  id: string
  name: string
}

interface Props {
  items: VlmHistoryItem[]
  cameraId?: string | null
  cameraName?: string
  availableCameras?: CameraOption[]
}

const props = withDefaults(defineProps<Props>(), {
  cameraId: null,
  cameraName: '',
  availableCameras: () => [],
})

// ============ Emits ============
const emit = defineEmits<{
  (e: 'view-detail', item: VlmHistoryItem): void
  (e: 'item-click', item: VlmHistoryItem): void
  (e: 'refresh'): void
}>()

// ============ 筛选状态 ============
const filterCamera = ref('')
const filterTime = ref('1h')

// 获取唯一摄像头列表：优先使用传入的完整列表，回退到从记录提取
const uniqueCameras = computed<CameraOption[]>(() => {
  if (props.availableCameras.length > 0) {
    return [...props.availableCameras]
  }
  // 回退：从记录提取，ID作为名称
  const cameras = new Set(props.items.map(item => item.cameraId))
  return Array.from(cameras).map(id => ({ id, name: id }))
})

// 根据筛选条件过滤历史记录
const filteredItems = computed(() => {
  let result = [...props.items]
  
  // 按摄像头筛选
  if (filterCamera.value) {
    result = result.filter(item => item.cameraId === filterCamera.value)
  }
  
  // 按时间筛选
  const now = Date.now()
  const timeFilters: Record<string, number> = {
    '1h': 60 * 60 * 1000,
    '6h': 6 * 60 * 60 * 1000,
    '24h': 24 * 60 * 60 * 1000,
  }
  
  if (filterTime.value !== 'all' && timeFilters[filterTime.value]) {
    const cutoff = now - timeFilters[filterTime.value]
    result = result.filter(item => item.timestamp >= cutoff)
  }
  
  // 按时间倒序排列
  return result.sort((a, b) => b.timestamp - a.timestamp)
})

// ============ Refs ============
const listRef = ref<HTMLDivElement | null>(null)

// ============ 方法 ============
function formatTime(timestamp: number): string {
  const date = new Date(timestamp)
  return date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function truncateText(text: string, maxLength: number): string {
  if (!text) return ''
  if (text.length <= maxLength) return text
  return text.slice(0, maxLength) + '...'
}

function getCameraName(cameraId: string): string {
  const cam = props.availableCameras.find(c => c.id === cameraId)
  return cam?.name || cameraId
}

function handleItemClick(item: VlmHistoryItem) {
  emit('item-click', item)
}

function handleViewDetail(item: VlmHistoryItem) {
  emit('view-detail', item)
}

// 新记录添加时滚动到顶部
watch(() => props.items.length, (newLen: number, oldLen: number) => {
  if (newLen > oldLen && listRef.value) {
    nextTick(() => {
      listRef.value?.scrollTo({ top: 0, behavior: 'smooth' })
    })
  }
})

// ============ 暴露方法 ============
defineExpose({
  scrollToTop: () => {
    listRef.value?.scrollTo({ top: 0, behavior: 'smooth' })
  },
})
</script>

<style scoped>
.vlm-history {
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 12px;
  border: 1px solid var(--border-color, #333);
  overflow: hidden;
  height: 100%;
}

/* 标题栏 */
.history-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 16px;
  background: var(--bg-tertiary, #2a2a3e);
  border-bottom: 1px solid var(--border-color, #333);
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-icon {
  font-size: 16px;
}

.header-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
}

.camera-tag {
  font-size: 13px;
  color: var(--text-muted, #666);
}

.header-right {
  display: flex;
  align-items: center;
}

.refresh-btn {
  width: 28px;
  height: 28px;
  padding: 6px;
  border: none;
  background: transparent;
  color: var(--text-muted, #666);
  cursor: pointer;
  border-radius: 6px;
  transition: all 0.2s ease;
}

.refresh-btn:hover {
  background: var(--bg-hover, #333);
  color: var(--text-primary, #e0e0e0);
}

.refresh-btn svg {
  width: 100%;
  height: 100%;
  stroke-width: 2;
}

/* 筛选栏 */
.filter-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: var(--bg-secondary, #1a1a2e);
  border-bottom: 1px solid var(--border-color, #333);
  flex-shrink: 0;
}

.filter-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.filter-label {
  font-size: 12px;
  color: var(--text-muted, #666);
}

.filter-select {
  padding: 4px 8px;
  border: 1px solid var(--border-color, #444);
  border-radius: 4px;
  background: var(--bg-tertiary, #252535);
  color: var(--text-primary, #e0e0e0);
  font-size: 12px;
  cursor: pointer;
}

.filter-select:focus {
  outline: none;
  border-color: var(--primary-color, #4a9eff);
}

.filter-right {
  display: flex;
  align-items: center;
}

.item-count {
  font-size: 12px;
  color: var(--text-muted, #666);
  background: var(--bg-primary, #0f0f1a);
  padding: 4px 10px;
  border-radius: 12px;
}

/* 历史列表 */
.history-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

/* 空状态 */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  color: var(--text-muted, #666);
  text-align: center;
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.empty-text {
  font-size: 15px;
  font-weight: 500;
  color: var(--text-secondary, #888);
  margin-bottom: 8px;
}

.empty-hint {
  font-size: 13px;
  color: var(--text-muted, #666);
}

/* 历史项 */
.history-item {
  display: flex;
  gap: 12px;
  padding: 14px;
  background: var(--bg-primary, #0f0f1a);
  border: 1px solid var(--border-color, #333);
  border-radius: 10px;
  margin-bottom: 10px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.history-item:last-child {
  margin-bottom: 0;
}

.history-item:hover {
  border-color: var(--accent-primary, #10a37f);
  background: rgba(16, 163, 127, 0.05);
  transform: translateY(-1px);
}

.history-item.is-new {
  animation: highlight 2s ease-out;
}

@keyframes highlight {
  0% {
    background: rgba(16, 163, 127, 0.2);
    border-color: var(--accent-primary, #10a37f);
  }
  100% {
    background: var(--bg-primary, #0f0f1a);
    border-color: var(--border-color, #333);
  }
}

/* 缩略图 */
.item-thumbnail {
  width: 80px;
  height: 60px;
  flex-shrink: 0;
  border-radius: 6px;
  overflow: hidden;
  background: var(--bg-tertiary, #2a2a3e);
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
  font-size: 24px;
  opacity: 0.5;
}

/* 内容区 */
.item-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.item-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--text-muted, #666);
}

.meta-divider {
  color: var(--border-color, #333);
}

.item-time {
  font-weight: 500;
  color: var(--text-secondary, #888);
}

.item-node {
  color: var(--accent-primary, #10a37f);
}

.item-duration {
  color: var(--text-muted, #666);
}

.item-result {
  font-size: 13px;
  line-height: 1.5;
  color: var(--text-primary, #e0e0e0);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 操作按钮 */
.item-actions {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.2s ease;
}

.history-item:hover .item-actions {
  opacity: 1;
}

.action-btn {
  width: 32px;
  height: 32px;
  background: var(--bg-tertiary, #2a2a3e);
  border: 1px solid var(--border-color, #333);
  border-radius: 6px;
  color: var(--text-secondary, #888);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
}

.action-btn svg {
  width: 16px;
  height: 16px;
  stroke-width: 2;
}

.action-btn:hover {
  background: var(--accent-primary, #10a37f);
  border-color: var(--accent-primary, #10a37f);
  color: white;
}

/* 滚动条样式 */
.history-list::-webkit-scrollbar {
  width: 6px;
}

.history-list::-webkit-scrollbar-track {
  background: transparent;
}

.history-list::-webkit-scrollbar-thumb {
  background: var(--border-color, #333);
  border-radius: 3px;
}

.history-list::-webkit-scrollbar-thumb:hover {
  background: var(--text-muted, #666);
}
</style>
