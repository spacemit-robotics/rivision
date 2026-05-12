<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

const events = ref<any[]>([])
const loading = ref(false)
const filter = ref('all')

async function fetchEvents() {
  loading.value = true
  try {
    const params = filter.value !== 'all' ? { type: filter.value } : {}
    const res = await axios.get('/api/v1/events', { params })
    events.value = res.data.events || []
  } catch (e) {
    console.error('获取事件失败', e)
  } finally {
    loading.value = false
  }
}

onMounted(fetchEvents)

function formatTime(time: string) {
  return time ? new Date(time).toLocaleString('zh-CN') : '-'
}
</script>

<template>
  <div class="events-page">
    <div class="page-header">
      <h2>事件告警</h2>
      <div class="header-actions">
        <select v-model="filter" @change="fetchEvents" class="filter-select">
          <option value="all">全部事件</option>
          <option value="alert">告警</option>
          <option value="detection">检测</option>
          <option value="system">系统</option>
        </select>
        <button class="btn btn-primary" @click="fetchEvents">🔄 刷新</button>
      </div>
    </div>

    <div class="card">
      <div v-if="loading" class="loading"><div class="spinner"></div></div>
      <table v-else class="table">
        <thead>
          <tr>
            <th>类型</th>
            <th>标题</th>
            <th>来源节点</th>
            <th>级别</th>
            <th>时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="event in events" :key="event.id">
            <td>
              <span class="event-type">
                {{ event.type === 'alert' ? '🔔' : event.type === 'detection' ? '📊' : '⚙️' }}
                {{ event.type }}
              </span>
            </td>
            <td>{{ event.title || event.type }}</td>
            <td><code>{{ event.node_id || '-' }}</code></td>
            <td>
              <span :class="['status-badge', `status-${event.level || 'info'}`]">
                {{ event.level || 'info' }}
              </span>
            </td>
            <td>{{ formatTime(event.created_at) }}</td>
            <td>
              <button class="btn btn-sm">详情</button>
            </td>
          </tr>
          <tr v-if="events.length === 0">
            <td colspan="6" class="empty-cell">暂无事件数据</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.events-page { display: flex; flex-direction: column; gap: 24px; }
.page-header { display: flex; justify-content: space-between; align-items: center; }
.page-header h2 { font-size: 20px; color: #1a1a2e; }
.header-actions { display: flex; gap: 12px; }
.filter-select { padding: 8px 12px; border: 1px solid #d1d5db; border-radius: 6px; }
.event-type { display: flex; align-items: center; gap: 6px; }
code { background: #f3f4f6; padding: 2px 6px; border-radius: 4px; font-size: 12px; }
.btn-sm { padding: 4px 12px; font-size: 12px; }
.empty-cell { text-align: center; padding: 40px !important; color: #9ca3af; }
.status-info { background: #dbeafe; color: #1d4ed8; }
.status-warning { background: #fef3c7; color: #d97706; }
.status-critical { background: #fee2e2; color: #dc2626; }
</style>
