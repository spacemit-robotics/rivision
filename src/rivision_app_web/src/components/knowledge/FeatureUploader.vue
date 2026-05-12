<script setup lang="ts">
import { ref } from 'vue'

const emit = defineEmits<{ (e: 'upload', data: { name: string; file: File }): void }>()
const name = ref('')
const file = ref<File | null>(null)

function onFileChange(e: Event) {
  file.value = (e.target as HTMLInputElement).files?.[0] || null
}

function submit() {
  if (!name.value || !file.value) return
  emit('upload', { name: name.value, file: file.value })
  name.value = ''
  file.value = null
}
</script>

<template>
  <div class="feature-uploader">
    <h4>特征库照片上传</h4>
    <input v-model="name" placeholder="人员姓名" />
    <input type="file" accept="image/*" @change="onFileChange" />
    <button :disabled="!name || !file" @click="submit">上传</button>
  </div>
</template>

<style scoped>
.feature-uploader { display: flex; flex-direction: column; gap: 8px; max-width: 400px; }
.feature-uploader input[type="text"], .feature-uploader input { padding: 8px; border: 1px solid #ddd; border-radius: 4px; }
.feature-uploader button { padding: 8px 20px; background: #67c23a; color: #fff; border: none; border-radius: 4px; cursor: pointer; }
.feature-uploader button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
