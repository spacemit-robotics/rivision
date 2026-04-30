// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <Teleport to="body">
    <div class="modal-overlay" v-if="visible" @click.self="close">
      <div class="modal-container">
        <div class="modal-header">
          <div class="header-title">
            <span class="icon">🔄</span>
            <span>视频回放 - {{ cameraName }}</span>
          </div>
          <button class="close-btn" @click="close">✕</button>
        </div>

        <div class="modal-body">
          <div class="video-section">
            <video
              ref="videoRef"
              class="playback-video"
              controls
              @timeupdate="onTimeUpdate"
              @loadedmetadata="onVideoLoaded"
            >
              <source :src="videoSrc" type="video/mp4" />
            </video>
          </div>

          <div class="controls-section">
            <div class="playback-controls">
              <button class="ctrl-btn" @click="togglePlay">
                {{ isPlaying ? '⏸' : '▶' }}
              </button>
              <button class="ctrl-btn" @click="skipBack">◀◀</button>
              <button class="ctrl-btn" @click="skipForward">▶▶</button>
              
              <div class="progress-container">
                <input
                  type="range"
                  class="progress-bar"
                  :value="currentTime"
                  :max="duration"
                  @input="onSeek"
                />
              </div>
              
              <span class="time-display">
                {{ formatTime(currentTime) }} / {{ formatTime(duration) }}
              </span>
              
              <select v-model="playbackSpeed" class="speed-select" @change="setSpeed">
                <option value="0.5">0.5x</option>
                <option value="1">1x</option>
                <option value="1.5">1.5x</option>
                <option value="2">2x</option>
              </select>
            </div>
          </div>

          <div class="markers-section">
            <div class="section-title">
              <span>📊</span>
              <span>该时段分析标记 (点击跳转)</span>
            </div>
            <div class="markers-list">
              <div
                v-for="marker in markers"
                :key="marker.id"
                class="marker-item"
                :class="{ active: isMarkerActive(marker) }"
                @click="jumpToMarker(marker)"
              >
                <div class="marker-thumb">
                  <img v-if="marker.thumbnail" :src="marker.thumbnail" />
                  <span v-else>📷</span>
                </div>
                <div class="marker-time">{{ formatTime(marker.videoTime) }}</div>
                <div class="marker-summary">{{ truncate(marker.summary, 6) }}</div>
              </div>
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <button class="footer-btn" @click="captureFrame">
            <span>📷</span>
            <span>截取当前帧</span>
          </button>
          <button class="footer-btn" @click="downloadSegment">
            <span>📥</span>
            <span>下载片段</span>
          </button>
          <button class="footer-btn primary" @click="analyzeFrame">
            <span>🔍</span>
            <span>分析当前帧</span>
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'

export interface PlaybackMarker {
  id: string
  timestamp: number
  videoTime: number
  thumbnail?: string
  summary: string
}

const props = defineProps<{
  modelValue?: boolean
  cameraId?: string
  cameraName?: string
  startTime?: number
  videoSrc?: string
  markers?: PlaybackMarker[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'close'): void
  (e: 'capture', time: number): void
  (e: 'analyze', time: number): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const videoRef = ref<HTMLVideoElement | null>(null)
const isPlaying = ref(false)
const currentTime = ref(0)
const duration = ref(0)
const playbackSpeed = ref('1')

const markers = computed(() => props.markers || [])

function close() {
  visible.value = false
  emit('close')
}

function togglePlay() {
  if (!videoRef.value) return
  
  if (isPlaying.value) {
    videoRef.value.pause()
  } else {
    videoRef.value.play()
  }
  isPlaying.value = !isPlaying.value
}

function skipBack() {
  if (videoRef.value) {
    videoRef.value.currentTime = Math.max(0, videoRef.value.currentTime - 10)
  }
}

function skipForward() {
  if (videoRef.value) {
    videoRef.value.currentTime = Math.min(duration.value, videoRef.value.currentTime + 10)
  }
}

function onSeek(e: Event) {
  const target = e.target as HTMLInputElement
  if (videoRef.value) {
    videoRef.value.currentTime = parseFloat(target.value)
  }
}

function setSpeed() {
  if (videoRef.value) {
    videoRef.value.playbackRate = parseFloat(playbackSpeed.value)
  }
}

function onTimeUpdate() {
  if (videoRef.value) {
    currentTime.value = videoRef.value.currentTime
  }
}

function onVideoLoaded() {
  if (videoRef.value) {
    duration.value = videoRef.value.duration
    
    // 如果有起始时间，跳转到该位置
    if (props.startTime && props.markers && props.markers.length > 0) {
      const startMarker = props.markers.find(m => m.timestamp === props.startTime)
      if (startMarker) {
        videoRef.value.currentTime = startMarker.videoTime
      }
    }
  }
}

function formatTime(seconds: number): string {
  const mins = Math.floor(seconds / 60)
  const secs = Math.floor(seconds % 60)
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
}

function truncate(text: string, maxLen: number): string {
  return text.length > maxLen ? text.slice(0, maxLen) + '...' : text
}

function isMarkerActive(marker: PlaybackMarker): boolean {
  return Math.abs(currentTime.value - marker.videoTime) < 5
}

function jumpToMarker(marker: PlaybackMarker) {
  if (videoRef.value) {
    videoRef.value.currentTime = marker.videoTime
    if (!isPlaying.value) {
      videoRef.value.play()
      isPlaying.value = true
    }
  }
}

function captureFrame() {
  if (!videoRef.value) return
  
  const canvas = document.createElement('canvas')
  canvas.width = videoRef.value.videoWidth
  canvas.height = videoRef.value.videoHeight
  
  const ctx = canvas.getContext('2d')
  if (ctx) {
    ctx.drawImage(videoRef.value, 0, 0)
    
    canvas.toBlob((blob) => {
      if (blob) {
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `capture_${props.cameraId}_${Date.now()}.jpg`
        a.click()
        URL.revokeObjectURL(url)
      }
    }, 'image/jpeg', 0.9)
  }
  
  emit('capture', currentTime.value)
}

function downloadSegment() {
  if (props.videoSrc) {
    const a = document.createElement('a')
    a.href = props.videoSrc
    a.download = `video_${props.cameraId}_${Date.now()}.mp4`
    a.click()
  }
}

function analyzeFrame() {
  emit('analyze', currentTime.value)
}

// 清理
watch(visible, (v) => {
  if (!v && videoRef.value) {
    videoRef.value.pause()
    isPlaying.value = false
  }
})

onUnmounted(() => {
  if (videoRef.value) {
    videoRef.value.pause()
  }
})
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.8);
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
  max-width: 1000px;
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
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.video-section {
  background: #000;
  border-radius: 8px;
  overflow: hidden;
}

.playback-video {
  width: 100%;
  max-height: 400px;
  display: block;
}

.controls-section {
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 10px;
  padding: 12px 16px;
}

.playback-controls {
  display: flex;
  align-items: center;
  gap: 12px;
}

.ctrl-btn {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: var(--bg-tertiary, #252535);
  border: 1px solid var(--border-color, #333);
  color: var(--text-primary, #e0e0e0);
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.ctrl-btn:hover {
  background: var(--bg-hover, #2a2a3a);
  border-color: var(--primary-color, #4a9eff);
}

.progress-container {
  flex: 1;
}

.progress-bar {
  width: 100%;
  height: 6px;
  -webkit-appearance: none;
  appearance: none;
  background: var(--bg-tertiary, #252535);
  border-radius: 3px;
  cursor: pointer;
}

.progress-bar::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 14px;
  height: 14px;
  background: var(--primary-color, #4a9eff);
  border-radius: 50%;
  cursor: pointer;
}

.time-display {
  font-size: 12px;
  color: var(--text-secondary, #888);
  font-family: monospace;
  min-width: 80px;
}

.speed-select {
  background: var(--bg-tertiary, #252535);
  border: 1px solid var(--border-color, #333);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--text-primary, #e0e0e0);
  cursor: pointer;
}

.markers-section {
  background: var(--bg-secondary, #1e1e2e);
  border-radius: 10px;
  padding: 16px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border-color, #333);
}

.markers-list {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding: 4px 0;
}

.marker-item {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 8px;
  background: var(--bg-tertiary, #252535);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
  border: 2px solid transparent;
}

.marker-item:hover {
  background: var(--bg-hover, #2a2a3a);
}

.marker-item.active {
  border-color: var(--primary-color, #4a9eff);
  background: var(--bg-selected, #1a2a4a);
}

.marker-thumb {
  width: 60px;
  height: 40px;
  border-radius: 4px;
  background: #000;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.marker-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.marker-thumb span {
  font-size: 16px;
  opacity: 0.5;
}

.marker-time {
  font-size: 11px;
  font-family: monospace;
  color: var(--text-secondary, #888);
}

.marker-summary {
  font-size: 10px;
  color: var(--text-primary, #e0e0e0);
  max-width: 60px;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
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
</style>
