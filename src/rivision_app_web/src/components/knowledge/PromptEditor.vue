<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ initialPrompt?: string; label?: string }>()
const emit = defineEmits<{ (e: 'save', prompt: string): void }>()
const prompt = ref(props.initialPrompt || '')
</script>

<template>
  <div class="prompt-editor">
    <h4>{{ label || 'VLM 提示词编辑' }}</h4>
    <textarea v-model="prompt" rows="6" placeholder="输入 VLM 提示词..."></textarea>
    <button @click="emit('save', prompt)">保存</button>
  </div>
</template>

<style scoped>
.prompt-editor textarea { width: 100%; padding: 12px; border: 1px solid #ddd; border-radius: 8px; resize: vertical; font-size: 14px; }
.prompt-editor button { margin-top: 8px; padding: 8px 20px; background: #409eff; color: #fff; border: none; border-radius: 4px; cursor: pointer; }
</style>
