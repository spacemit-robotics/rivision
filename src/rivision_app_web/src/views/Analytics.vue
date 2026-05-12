<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

const stats = ref({
  traffic: { today: 0, week: 0, month: 0 },
  events: { total: 0, alerts: 0, resolved: 0 },
  nodes: { total: 0, online: 0, offline: 0 }
})

async function fetchAnalytics() {
  try {
    const res = await axios.get('/api/v1/analytics/overview').catch(() => ({ data: {} }))
    if (res.data) {
      stats.value = { ...stats.value, ...res.data }
    }
  } catch (e) {
    console.error('获取统计数据失败', e)
  }
}

onMounted(fetchAnalytics)
</script>

<template>
  <div class="analytics-page">
    <div class="page-header">
      <h2>统计分析</h2>
    </div>

    <div class="stats-section">
      <div class="card stat-card">
        <div class="card-title">📊 客流统计</div>
        <div class="stat-items">
          <div class="stat-item">
            <div class="stat-value">{{ stats.traffic.today }}</div>
            <div class="stat-label">今日</div>
          </div>
          <div class="stat-item">
            <div class="stat-value">{{ stats.traffic.week }}</div>
            <div class="stat-label">本周</div>
          </div>
          <div class="stat-item">
            <div class="stat-value">{{ stats.traffic.month }}</div>
            <div class="stat-label">本月</div>
          </div>
        </div>
      </div>

      <div class="card stat-card">
        <div class="card-title">🔔 事件统计</div>
        <div class="stat-items">
          <div class="stat-item">
            <div class="stat-value">{{ stats.events.total }}</div>
            <div class="stat-label">总事件</div>
          </div>
          <div class="stat-item">
            <div class="stat-value text-warning">{{ stats.events.alerts }}</div>
            <div class="stat-label">告警</div>
          </div>
          <div class="stat-item">
            <div class="stat-value text-success">{{ stats.events.resolved }}</div>
            <div class="stat-label">已处理</div>
          </div>
        </div>
      </div>

      <div class="card stat-card">
        <div class="card-title">🖥️ 节点状态</div>
        <div class="stat-items">
          <div class="stat-item">
            <div class="stat-value">{{ stats.nodes.total }}</div>
            <div class="stat-label">总节点</div>
          </div>
          <div class="stat-item">
            <div class="stat-value text-success">{{ stats.nodes.online }}</div>
            <div class="stat-label">在线</div>
          </div>
          <div class="stat-item">
            <div class="stat-value text-danger">{{ stats.nodes.offline }}</div>
            <div class="stat-label">离线</div>
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-title">📈 趋势图表</div>
      <div class="chart-placeholder">
        <div class="placeholder-icon">📊</div>
        <p>图表组件开发中...</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.analytics-page { display: flex; flex-direction: column; gap: 24px; }
.page-header h2 { font-size: 20px; color: #1a1a2e; }
.stats-section { display: grid; grid-template-columns: repeat(3, 1fr); gap: 24px; }
@media (max-width: 768px) { .stats-section { grid-template-columns: 1fr; } }
.stat-card { text-align: center; }
.stat-items { display: flex; justify-content: space-around; margin-top: 16px; }
.stat-value { font-size: 32px; font-weight: 700; color: #1a1a2e; }
.stat-label { font-size: 13px; color: #6b7280; margin-top: 4px; }
.text-success { color: #10b981; }
.text-warning { color: #f59e0b; }
.text-danger { color: #ef4444; }
.chart-placeholder { text-align: center; padding: 80px; color: #9ca3af; }
.placeholder-icon { font-size: 48px; margin-bottom: 16px; }
</style>
