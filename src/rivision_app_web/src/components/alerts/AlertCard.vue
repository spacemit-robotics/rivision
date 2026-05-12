<script setup lang="ts">
defineProps<{
  alert: {
    id: string
    severity: string
    message: string
    camera_id?: string
    rule_id?: string
    thumbnail?: string
    created_at?: string
  }
}>()
defineEmits<{ (e: 'acknowledge', id: string): void }>()
</script>

<template>
  <div class="alert-card" :class="alert.severity">
    <div class="thumb" v-if="alert.thumbnail">
      <img :src="alert.thumbnail" alt="" />
    </div>
    <div class="body">
      <div class="top">
        <span class="severity-badge">{{ alert.severity }}</span>
        <span class="time">{{ alert.created_at }}</span>
      </div>
      <p>{{ alert.message }}</p>
      <small>规则: {{ alert.rule_id }} | 摄像头: {{ alert.camera_id }}</small>
    </div>
  </div>
</template>

<style scoped>
.alert-card { display: flex; gap: 12px; padding: 12px; border: 1px solid #eee; border-radius: 8px; margin-bottom: 8px; }
.alert-card.critical { border-left: 4px solid #f56c6c; }
.alert-card.warning { border-left: 4px solid #e6a23c; }
.alert-card.info { border-left: 4px solid #409eff; }
.thumb { width: 80px; height: 60px; flex-shrink: 0; border-radius: 4px; overflow: hidden; }
.thumb img { width: 100%; height: 100%; object-fit: cover; }
.body { flex: 1; }
.top { display: flex; justify-content: space-between; margin-bottom: 4px; }
.severity-badge { font-size: 11px; font-weight: bold; text-transform: uppercase; }
.time { font-size: 12px; color: #999; }
</style>
