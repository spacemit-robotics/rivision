<script setup lang="ts">
import { ref } from 'vue'

const emit = defineEmits<{
  (e: 'search-text', query: string): void
  (e: 'search-image', imageData: string): void
}>()

const query = ref('')
const mode = ref<'text' | 'image'>('text')

function doSearch() {
  if (mode.value === 'text' && query.value.trim()) {
    emit('search-text', query.value.trim())
  }
}

function onFileChange(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    emit('search-image', reader.result as string)
  }
  reader.readAsDataURL(file)
}
</script>

<template>
  <div class="search-bar">
    <div class="mode-tabs">
      <button :class="{ active: mode === 'text' }" @click="mode = 'text'">文本搜索</button>
      <button :class="{ active: mode === 'image' }" @click="mode = 'image'">以图搜图</button>
    </div>
    <div v-if="mode === 'text'" class="text-input">
      <input v-model="query" placeholder="输入搜索内容..." @keyup.enter="doSearch" />
      <button @click="doSearch">搜索</button>
    </div>
    <div v-else class="image-input">
      <input type="file" accept="image/*" @change="onFileChange" />
    </div>
  </div>
</template>

<style scoped>
.search-bar { margin-bottom: 16px; }
.mode-tabs { display: flex; gap: 8px; margin-bottom: 8px; }
.mode-tabs button { padding: 6px 16px; border: 1px solid #ddd; border-radius: 4px; cursor: pointer; background: #fff; }
.mode-tabs button.active { background: #409eff; color: #fff; border-color: #409eff; }
.text-input { display: flex; gap: 8px; }
.text-input input { flex: 1; padding: 8px 12px; border: 1px solid #ddd; border-radius: 4px; }
.text-input button { padding: 8px 20px; background: #409eff; color: #fff; border: none; border-radius: 4px; cursor: pointer; }
</style>
