// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="video-player-container">
    <video
      ref="videoRef"
      :src="src"
      :poster="poster"
      class="video-element"
      @loadedmetadata="onLoadedMetadata"
      @timeupdate="onTimeUpdate"
      @ended="onEnded"
      @error="onError"
    >
      您的浏览器不支持视频播放
    </video>
    
    <div class="video-controls">
      <div class="progress-bar" @click="seek">
        <div class="progress-filled" :style="{ width: progressPercent + '%' }"></div>
        <div class="progress-marker" :style="{ left: progressPercent + '%' }"></div>
      </div>
      
      <div class="controls-row">
        <div class="left-controls">
          <el-button :icon="isPlaying ? 'Pause' : 'VideoPlay'" circle @click="togglePlay" />
          <span class="time-display">{{ formatTime(currentTime) }} / {{ formatTime(duration) }}</span>
        </div>
        
        <div class="right-controls">
          <el-slider
            v-model="volume"
            :max="100"
            :show-tooltip="false"
            class="volume-slider"
            @input="updateVolume"
          />
          <el-button icon="FullScreen" circle @click="toggleFullscreen" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

const props = defineProps<{
  src: string
  poster?: string
}>()

const emit = defineEmits<{
  (e: 'timeUpdate', time: number): void
  (e: 'ended'): void
  (e: 'error', error: Event): void
}>()

const videoRef = ref<HTMLVideoElement | null>(null)
const isPlaying = ref(false)
const currentTime = ref(0)
const duration = ref(0)
const volume = ref(100)

const progressPercent = computed(() => {
  if (duration.value === 0) return 0
  return (currentTime.value / duration.value) * 100
})

const togglePlay = () => {
  if (!videoRef.value) return
  if (isPlaying.value) {
    videoRef.value.pause()
  } else {
    videoRef.value.play()
  }
  isPlaying.value = !isPlaying.value
}

const seek = (e: MouseEvent) => {
  if (!videoRef.value) return
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const percent = (e.clientX - rect.left) / rect.width
  videoRef.value.currentTime = percent * duration.value
}

const updateVolume = (val: number) => {
  if (!videoRef.value) return
  videoRef.value.volume = val / 100
}

const toggleFullscreen = () => {
  if (!videoRef.value) return
  if (document.fullscreenElement) {
    document.exitFullscreen()
  } else {
    videoRef.value.requestFullscreen()
  }
}

const onLoadedMetadata = () => {
  if (!videoRef.value) return
  duration.value = videoRef.value.duration
}

const onTimeUpdate = () => {
  if (!videoRef.value) return
  currentTime.value = videoRef.value.currentTime
  emit('timeUpdate', currentTime.value)
}

const onEnded = () => {
  isPlaying.value = false
  emit('ended')
}

const onError = (e: Event) => {
  emit('error', e)
}

const formatTime = (seconds: number) => {
  const mins = Math.floor(seconds / 60)
  const secs = Math.floor(seconds % 60)
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
}

const seekTo = (time: number) => {
  if (!videoRef.value) return
  videoRef.value.currentTime = time
}

defineExpose({ seekTo, videoRef })
</script>

<style scoped>
.video-player-container {
  position: relative;
  background: #000;
  border-radius: 8px;
  overflow: hidden;
}

.video-element {
  width: 100%;
  display: block;
}

.video-controls {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.7));
  padding: 20px 15px 10px;
}

.progress-bar {
  height: 4px;
  background: rgba(255, 255, 255, 0.3);
  border-radius: 2px;
  cursor: pointer;
  position: relative;
  margin-bottom: 10px;
}

.progress-filled {
  height: 100%;
  background: #409eff;
  border-radius: 2px;
}

.progress-marker {
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%);
  width: 12px;
  height: 12px;
  background: #409eff;
  border-radius: 50%;
  opacity: 0;
  transition: opacity 0.2s;
}

.progress-bar:hover .progress-marker {
  opacity: 1;
}

.controls-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.left-controls,
.right-controls {
  display: flex;
  align-items: center;
  gap: 10px;
}

.time-display {
  color: #fff;
  font-size: 12px;
}

.volume-slider {
  width: 80px;
}

:deep(.el-button) {
  color: #fff;
  background: transparent;
  border-color: transparent;
}
</style>
