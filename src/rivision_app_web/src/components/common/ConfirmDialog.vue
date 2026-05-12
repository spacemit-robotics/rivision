<script setup lang="ts">
defineProps<{ title?: string; message: string; visible: boolean }>()
defineEmits<{ (e: 'confirm'): void; (e: 'cancel'): void }>()
</script>

<template>
  <div v-if="visible" class="overlay" @click.self="$emit('cancel')">
    <div class="dialog">
      <h3>{{ title || '确认' }}</h3>
      <p>{{ message }}</p>
      <div class="actions">
        <button class="cancel" @click="$emit('cancel')">取消</button>
        <button class="confirm" @click="$emit('confirm')">确认</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.4); display: flex; align-items: center; justify-content: center; z-index: 1000; }
.dialog { background: #fff; border-radius: 12px; padding: 24px; width: 400px; }
.dialog h3 { margin: 0 0 12px; }
.dialog p { margin: 0 0 20px; color: #666; }
.actions { display: flex; justify-content: flex-end; gap: 8px; }
.cancel { padding: 8px 16px; border: 1px solid #ddd; border-radius: 4px; cursor: pointer; background: #fff; }
.confirm { padding: 8px 16px; background: #409eff; color: #fff; border: none; border-radius: 4px; cursor: pointer; }
</style>
