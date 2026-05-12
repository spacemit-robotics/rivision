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
    context?: string
  }
}>()
defineEmits<{ (e: 'close'): void }>()
</script>

<template>
  <div class="alert-detail-overlay" @click.self="$emit('close')">
    <div class="alert-detail">
      <div class="header">
        <h3>告警详情</h3>
        <button @click="$emit('close')">&times;</button>
      </div>
      <div class="content">
        <img v-if="alert.thumbnail" :src="alert.thumbnail" class="detail-thumb" />
        <div class="info">
          <p><strong>级别:</strong> {{ alert.severity }}</p>
          <p><strong>消息:</strong> {{ alert.message }}</p>
          <p><strong>摄像头:</strong> {{ alert.camera_id }}</p>
          <p><strong>规则:</strong> {{ alert.rule_id }}</p>
          <p><strong>时间:</strong> {{ alert.created_at }}</p>
          <p v-if="alert.context"><strong>上下文:</strong> {{ alert.context }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.alert-detail-overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000; }
.alert-detail { background: #fff; border-radius: 12px; width: 600px; max-height: 80vh; overflow-y: auto; }
.header { display: flex; justify-content: space-between; align-items: center; padding: 16px; border-bottom: 1px solid #eee; }
.header button { background: none; border: none; font-size: 24px; cursor: pointer; }
.content { padding: 16px; }
.detail-thumb { width: 100%; border-radius: 8px; margin-bottom: 16px; }
.info p { margin: 8px 0; }
</style>
