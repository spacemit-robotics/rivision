// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <Teleport to="body">
    <div class="modal-overlay" v-if="visible" @click.self="close">
      <div class="modal-container">
        <div class="modal-header">
          <div class="header-title">
            <span class="icon">📷</span>
            <span>分析详情</span>
          </div>
          <button class="close-btn" @click="close">✕</button>
        </div>

        <div class="modal-body">
          <div class="image-section">
            <img 
              v-if="data?.imageSrc" 
              :src="data.imageSrc" 
              class="detail-image"
              @click="toggleZoom"
              :class="{ zoomed: isZoomed }"
            />
            <div v-else class="no-image">
              <span>📷</span>
              <p>暂无图片</p>
            </div>

            <div class="image-nav" v-if="hasPrevNext">
              <button class="nav-btn prev" @click="goPrev" :disabled="!hasPrev">
                ◀ 上一帧
              </button>
              <span class="current-time">{{ formatTime(data?.timestamp) }}</span>
              <button class="nav-btn next" @click="goNext" :disabled="!hasNext">
                下一帧 ▶
              </button>
            </div>
          </div>

          <div class="info-section">
            <div class="info-block">
              <div class="info-title">基本信息</div>
              <div class="info-items">
                <div class="info-item">
                  <span class="label">📍 摄像头</span>
                  <span class="value">{{ data?.cameraName || data?.cameraId }}</span>
                </div>
                <div class="info-item">
                  <span class="label">⏱️ 时间</span>
                  <span class="value">{{ formatDateTime(data?.timestamp) }}</span>
                </div>
                <div class="info-item">
                  <span class="label">📐 分辨率</span>
                  <span class="value">{{ data?.resolution || '--' }}</span>
                </div>
                <div class="info-item">
                  <span class="label">⏳ 分析耗时</span>
                  <span class="value">{{ formatDuration(data?.processingTime) }}</span>
                </div>
                <div class="info-item" v-if="data?.nodeId">
                  <span class="label">🖥️ 节点</span>
                  <span class="value node-value">{{ data.nodeId }}</span>
                </div>
              </div>
            </div>

            <div class="info-block result-block">
              <div class="info-title">🧠 AI分析结果</div>
              <div class="result-text">
                {{ data?.result || '暂无分析结果' }}
              </div>
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <button class="footer-btn primary" @click="playback">
            <span>🔄</span>
            <span>回放该时刻视频</span>
          </button>
          <button class="footer-btn" @click="download">
            <span>📥</span>
            <span>下载截图</span>
          </button>
          <button class="footer-btn" @click="copyResult">
            <span>📋</span>
            <span>复制分析结果</span>
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

export interface DetailData {
  id: string
  cameraId: string
  cameraName?: string
  timestamp: number
  imageSrc?: string
  result?: string
  resolution?: string
  processingTime?: number
  videoSegment?: string
  nodeId?: string
}

const props = defineProps<{
  modelValue?: boolean
  data?: DetailData
  hasPrev?: boolean
  hasNext?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'close'): void
  (e: 'playback', data: DetailData): void
  (e: 'prev'): void
  (e: 'next'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const isZoomed = ref(false)
const hasPrevNext = computed(() => props.hasPrev !== undefined || props.hasNext !== undefined)

function close() {
  visible.value = false
  emit('close')
}

function toggleZoom() {
  isZoomed.value = !isZoomed.value
}

function formatTime(timestamp?: number): string {
  if (!timestamp) return '--:--:--'
  // 后端返回秒级时间戳，转换为毫秒级
  const ms = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const date = new Date(ms)
  return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}:${date.getSeconds().toString().padStart(2, '0')}`
}

function formatDateTime(timestamp?: number): string {
  if (!timestamp) return '--'
  // 后端返回秒级时间戳，转换为毫秒级
  const ms = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const date = new Date(ms)
  return `${date.getFullYear()}-${(date.getMonth() + 1).toString().padStart(2, '0')}-${date.getDate().toString().padStart(2, '0')} ${formatTime(timestamp)}`
}

function formatDuration(milliseconds?: number): string {
  if (!milliseconds) return '--'
  // ★ 输入是毫秒，转换为秒
  const totalSeconds = milliseconds / 1000
  const mins = Math.floor(totalSeconds / 60)
  const secs = (totalSeconds % 60).toFixed(1)
  return mins > 0 ? `${mins}分${secs}秒` : `${secs}秒`
}

function playback() {
  if (props.data) {
    emit('playback', props.data)
  }
}

function goPrev() {
  emit('prev')
}

function goNext() {
  emit('next')
}

async function download() {
  if (!props.data?.imageSrc) return
  
  try {
    const response = await fetch(props.data.imageSrc)
    const blob = await response.blob()
    const url = URL.createObjectURL(blob)
    
    const a = document.createElement('a')
    a.href = url
    a.download = `capture_${props.data.cameraId}_${props.data.timestamp}.jpg`
    a.click()
    
    URL.revokeObjectURL(url)
  } catch (e) {
    console.error('Download failed:', e)
  }
}

async function copyResult() {
  if (!props.data?.result) return
  
  try {
    await navigator.clipboard.writeText(props.data.result)
  } catch (e) {
    console.error('Copy failed:', e)
  }
}
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.75);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal-container {
  background: var(--bg-primary, #12121a);
  border-radius: 16px;
  width: 100%;
  max-width: 900px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color, #333);
}

.header-title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
}

.header-title .icon {
  font-size: 20px;
}

.close-btn {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--bg-tertiary, #252535);
  border: none;
  color: var(--text-secondary, #888);
  font-size: 16px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.close-btn:hover {
  background: #f87171;
  color: white;
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  display: grid;
  grid-template-columns: 1fr 300px;
  gap: 20px;
}

.image-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-image {
  width: 100%;
  height: auto;
  max-height: 400px;
  object-fit: contain;
  border-radius: 8px;
  background: #000;
  cursor: zoom-in;
  transition: transform 0.3s ease;
}

.detail-image.zoomed {
  cursor: zoom-out;
  transform: scale(1.5);
}

.no-image {
  width: 100%;
  height: 300px;
  background: var(--bg-tertiary, #252535);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary, #888);
}

.no-image span {
  font-size: 48px;
  margin-bottom: 8px;
}

.image-nav {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

.nav-btn {
  padding: 8px 16px;
  background: var(--bg-tertiary, #252535);
  border: 1px solid var(--border-color, #333);
  border-radius: 6px;
  color: var(--text-primary, #e0e0e0);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.nav-btn:hover:not(:disabled) {
  background: var(--bg-hover, #2a2a3a);
  border-color: var(--primary-color, #4a9eff);
}

.nav-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.current-time {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary, #e0e0e0);
}

.info-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.info-block {
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 10px;
  padding: 16px;
}

.info-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border-color, #333);
}

.info-items {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
}

.info-item .label {
  color: var(--text-secondary, #888);
}

.info-item .value {
  color: var(--text-primary, #e0e0e0);
  font-weight: 500;
}

.info-item .value.node-value {
  color: var(--primary-color, #4a9eff);
  background: rgba(74, 158, 255, 0.15);
  padding: 2px 8px;
  border-radius: 4px;
}

.result-block {
  flex: 1;
}

.result-text {
  font-size: 13px;
  line-height: 1.7;
  color: var(--text-primary, #e0e0e0);
  white-space: pre-wrap;
}

.modal-footer {
  display: flex;
  gap: 10px;
  padding: 16px 20px;
  border-top: 1px solid var(--border-color, #333);
}

.footer-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  background: var(--bg-tertiary, #252535);
  border: 1px solid var(--border-color, #333);
  border-radius: 8px;
  color: var(--text-primary, #e0e0e0);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.footer-btn:hover {
  background: var(--bg-hover, #2a2a3a);
  border-color: var(--primary-color, #4a9eff);
}

.footer-btn.primary {
  background: linear-gradient(135deg, var(--primary-color, #4a9eff), #6366f1);
  border: none;
  color: white;
}

.footer-btn.primary:hover {
  box-shadow: 0 4px 12px rgba(74, 158, 255, 0.4);
}

@media (max-width: 768px) {
  .modal-body {
    grid-template-columns: 1fr;
  }
}
</style>
