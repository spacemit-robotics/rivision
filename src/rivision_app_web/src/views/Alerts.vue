<script setup lang="ts">
import { onMounted } from 'vue'
import { useAlertsStore } from '../stores/alerts'

const alertsStore = useAlertsStore()

onMounted(() => {
  alertsStore.fetchAlerts()
})
</script>

<template>
  <div class="alerts-page">
    <div class="header">
      <h2>告警管理</h2>
      <span class="badge" v-if="alertsStore.unreadCount > 0">{{ alertsStore.unreadCount }} 未处理</span>
    </div>
    <div class="alert-list" v-if="!alertsStore.loading">
      <div v-for="alert in alertsStore.alerts" :key="alert.id" class="alert-card" :class="alert.severity">
        <div class="alert-header">
          <span class="severity">{{ alert.severity }}</span>
          <span class="time">{{ alert.created_at }}</span>
        </div>
        <div class="alert-body">
          <p>{{ alert.message }}</p>
          <small>摄像头: {{ alert.camera_id }} | 规则: {{ alert.rule_id }}</small>
        </div>
        <div class="alert-actions">
          <button v-if="alert.status === 'pending'" @click="alertsStore.acknowledge(alert.id)">确认</button>
        </div>
      </div>
      <div v-if="alertsStore.alerts.length === 0" class="empty">暂无告警</div>
    </div>
    <div v-else class="loading">加载中...</div>
  </div>
</template>

<style scoped>
.alerts-page { padding: 16px; }
.header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.badge { background: #f56c6c; color: #fff; padding: 2px 8px; border-radius: 12px; font-size: 12px; }
.alert-card { border: 1px solid #eee; border-radius: 8px; padding: 12px; margin-bottom: 8px; }
.alert-card.critical { border-left: 4px solid #f56c6c; }
.alert-card.warning { border-left: 4px solid #e6a23c; }
.alert-card.info { border-left: 4px solid #409eff; }
.alert-header { display: flex; justify-content: space-between; margin-bottom: 8px; }
.severity { font-weight: bold; text-transform: uppercase; font-size: 12px; }
.time { color: #999; font-size: 12px; }
.alert-actions button { padding: 4px 12px; border: 1px solid #409eff; border-radius: 4px; color: #409eff; cursor: pointer; background: transparent; }
.empty, .loading { text-align: center; color: #999; padding: 40px; }
</style>
