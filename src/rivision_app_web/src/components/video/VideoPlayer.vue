<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'

const props = defineProps<{
  streamUrl?: string
  cameraId?: string
  protocol?: 'webrtc' | 'hls' | 'flv'
}>()

const videoRef = ref<HTMLVideoElement | null>(null)
const status = ref<'connecting' | 'playing' | 'error' | 'idle'>('idle')

onMounted(() => {
  if (props.streamUrl) {
    connect()
  }
})

onUnmounted(() => {
  disconnect()
})

watch(() => props.streamUrl, (newUrl) => {
  if (newUrl) connect()
  else disconnect()
})

function connect() {
  status.value = 'connecting'
  // WebRTC / HLS / FLV integration placeholder
  // In production: use go2rtc WebRTC API or HLS.js / flv.js
  setTimeout(() => {
    status.value = 'playing'
  }, 1000)
}

function disconnect() {
  status.value = 'idle'
}
</script>

<template>
  <div class="video-player">
    <video ref="videoRef" autoplay muted playsinline></video>
    <div class="overlay" v-if="status !== 'playing'">
      <span v-if="status === 'connecting'">连接中...</span>
      <span v-else-if="status === 'error'">连接失败</span>
      <span v-else>{{ cameraId || '未选择摄像头' }}</span>
    </div>
  </div>
</template>

<style scoped>
.video-player { position: relative; width: 100%; aspect-ratio: 16/9; background: #000; border-radius: 8px; overflow: hidden; }
video { width: 100%; height: 100%; object-fit: contain; }
.overlay { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; color: #999; background: #1a1a2e; }
</style>
