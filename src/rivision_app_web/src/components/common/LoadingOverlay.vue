<script setup lang="ts">
defineProps<{ visible: boolean; text?: string }>()
</script>

<template>
  <div v-if="visible" class="loading-overlay">
    <div class="spinner"></div>
    <span v-if="text">{{ text }}</span>
  </div>
</template>

<style scoped>
.loading-overlay { position: absolute; inset: 0; background: rgba(255,255,255,0.8); display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; z-index: 100; }
.spinner { width: 32px; height: 32px; border: 3px solid #eee; border-top-color: #409eff; border-radius: 50%; animation: spin 0.8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
