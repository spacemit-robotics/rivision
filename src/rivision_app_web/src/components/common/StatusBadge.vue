<script setup lang="ts">
defineProps<{ status: 'online' | 'offline' | 'warning' | 'error'; label?: string }>()
</script>

<template>
  <span class="status-badge" :class="status">
    <span class="dot"></span>
    {{ label || status }}
  </span>
</template>

<style scoped>
.status-badge { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; padding: 2px 8px; border-radius: 12px; }
.dot { width: 6px; height: 6px; border-radius: 50%; }
.online { background: #f0f9eb; color: #67c23a; }
.online .dot { background: #67c23a; }
.offline { background: #f4f4f5; color: #909399; }
.offline .dot { background: #909399; }
.warning { background: #fdf6ec; color: #e6a23c; }
.warning .dot { background: #e6a23c; }
.error { background: #fef0f0; color: #f56c6c; }
.error .dot { background: #f56c6c; }
</style>
