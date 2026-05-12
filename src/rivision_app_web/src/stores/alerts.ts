import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { alertsApi } from '../api/alerts'

export const useAlertsStore = defineStore('alerts', () => {
  const alerts = ref<any[]>([])
  const loading = ref(false)
  const unreadCount = computed(() => alerts.value.filter((a) => a.status === 'pending').length)

  async function fetchAlerts(params: any = {}) {
    loading.value = true
    try {
      const res = await alertsApi.list(params) as any
      alerts.value = res.alerts || []
    } finally {
      loading.value = false
    }
  }

  function pushAlert(alert: any) {
    alerts.value.unshift(alert)
  }

  async function acknowledge(id: string) {
    await alertsApi.acknowledge(id)
    const a = alerts.value.find((x) => x.id === id)
    if (a) a.status = 'acknowledged'
  }

  return { alerts, loading, unreadCount, fetchAlerts, pushAlert, acknowledge }
})
