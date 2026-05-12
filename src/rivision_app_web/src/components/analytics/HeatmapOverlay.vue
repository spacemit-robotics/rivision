<script setup lang="ts">
import { ref, onMounted } from 'vue'

defineProps<{ data: any }>()
const canvas = ref<HTMLCanvasElement | null>(null)

onMounted(() => {
  // Heatmap Canvas 2D rendering placeholder
  // In production, integrate with actual heatmap data
  const ctx = canvas.value?.getContext('2d')
  if (ctx) {
    ctx.fillStyle = '#f0f0f0'
    ctx.fillRect(0, 0, 640, 360)
    ctx.fillStyle = '#999'
    ctx.font = '14px sans-serif'
    ctx.textAlign = 'center'
    ctx.fillText('热区地图 (等待数据)', 320, 180)
  }
})
</script>

<template>
  <div class="heatmap-overlay">
    <h4>热区分析</h4>
    <canvas ref="canvas" width="640" height="360"></canvas>
  </div>
</template>

<style scoped>
.heatmap-overlay { padding: 12px; border: 1px solid #eee; border-radius: 8px; }
canvas { width: 100%; max-width: 640px; border-radius: 4px; }
</style>
