<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

const targets = ref<any[]>([])
const loading = ref(false)

async function fetchTargets() {
  loading.value = true
  try {
    const res = await axios.get('/api/v1/knowledge/targets')
    targets.value = res.data.targets || []
  } catch (e) {
    console.error('获取知识库失败', e)
  } finally {
    loading.value = false
  }
}

onMounted(fetchTargets)
</script>

<template>
  <div class="knowledge-page">
    <div class="page-header">
      <h2>知识库管理</h2>
      <button class="btn btn-primary">➕ 添加目标</button>
    </div>

    <div class="card">
      <div v-if="loading" class="loading"><div class="spinner"></div></div>
      <div v-else-if="targets.length === 0" class="empty-state">
        <div class="empty-icon">📚</div>
        <p>知识库为空</p>
        <p class="hint">添加目标人物、车辆等信息用于智能识别</p>
      </div>
      <div v-else class="targets-grid">
        <div v-for="target in targets" :key="target.id" class="target-card">
          <div class="target-avatar">{{ target.type === 'person' ? '👤' : '🚗' }}</div>
          <div class="target-info">
            <div class="target-name">{{ target.name }}</div>
            <div class="target-type">{{ target.type }}</div>
            <div class="target-tags">
              <span v-for="tag in (target.tags || [])" :key="tag" class="tag">{{ tag }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.knowledge-page { display: flex; flex-direction: column; gap: 24px; }
.page-header { display: flex; justify-content: space-between; align-items: center; }
.page-header h2 { font-size: 20px; color: #1a1a2e; }
.targets-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 16px; }
.target-card { background: #f9fafb; border-radius: 12px; padding: 20px; text-align: center; }
.target-avatar { font-size: 48px; margin-bottom: 12px; }
.target-name { font-weight: 600; color: #1a1a2e; }
.target-type { font-size: 12px; color: #6b7280; margin: 4px 0 8px; }
.target-tags { display: flex; flex-wrap: wrap; gap: 4px; justify-content: center; }
.tag { background: #dbeafe; color: #1d4ed8; padding: 2px 8px; border-radius: 12px; font-size: 11px; }
.empty-state { text-align: center; padding: 60px; color: #9ca3af; }
.empty-icon { font-size: 48px; margin-bottom: 16px; }
.hint { font-size: 13px; margin-top: 8px; }
</style>
