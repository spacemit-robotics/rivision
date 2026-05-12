<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { configApi } from '../api/config'

const config = ref<any>(null)
const loading = ref(false)

onMounted(async () => {
  loading.value = true
  try {
    const res = await configApi.getAlgorithm() as any
    config.value = res.config
  } finally {
    loading.value = false
  }
})

async function saveConfig() {
  if (!config.value) return
  await configApi.setAlgorithm(config.value)
}
</script>

<template>
  <div class="settings-page">
    <h2>系统设置</h2>
    <div v-if="!loading" class="settings-form">
      <section>
        <h3>算法配置</h3>
        <pre v-if="config">{{ JSON.stringify(config, null, 2) }}</pre>
        <p v-else class="empty">暂无算法配置</p>
        <button @click="saveConfig">保存配置</button>
      </section>
    </div>
    <div v-else class="loading">加载中...</div>
  </div>
</template>

<style scoped>
.settings-page { padding: 16px; }
.settings-form section { margin-bottom: 24px; }
.settings-form h3 { margin-bottom: 12px; }
pre { background: #f5f5f5; padding: 12px; border-radius: 8px; overflow-x: auto; font-size: 13px; }
button { padding: 8px 20px; background: #409eff; color: #fff; border: none; border-radius: 4px; cursor: pointer; }
.empty, .loading { color: #999; text-align: center; padding: 40px; }
</style>
