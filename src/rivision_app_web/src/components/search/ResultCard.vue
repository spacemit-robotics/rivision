<script setup lang="ts">
defineProps<{
  result: {
    detection_id?: string
    thumbnail?: string
    description?: string
    score?: number
    camera_id?: string
    timestamp?: string
  }
}>()
</script>

<template>
  <div class="result-card">
    <div class="thumbnail">
      <img v-if="result.thumbnail" :src="result.thumbnail" alt="缩略图" />
      <div v-else class="no-thumb">无图</div>
    </div>
    <div class="info">
      <p class="desc">{{ result.description || '无描述' }}</p>
      <div class="meta">
        <span v-if="result.score !== undefined" class="score">相似度: {{ (result.score * 100).toFixed(1) }}%</span>
        <span v-if="result.camera_id" class="camera">摄像头: {{ result.camera_id }}</span>
        <span v-if="result.timestamp" class="time">{{ result.timestamp }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.result-card { display: flex; gap: 12px; padding: 12px; border: 1px solid #eee; border-radius: 8px; margin-bottom: 8px; }
.thumbnail { width: 120px; height: 80px; flex-shrink: 0; border-radius: 4px; overflow: hidden; background: #f5f5f5; }
.thumbnail img { width: 100%; height: 100%; object-fit: cover; }
.no-thumb { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; color: #ccc; font-size: 12px; }
.info { flex: 1; }
.desc { margin: 0 0 8px; font-size: 14px; }
.meta { display: flex; gap: 12px; font-size: 12px; color: #999; }
.score { color: #409eff; font-weight: 500; }
</style>
