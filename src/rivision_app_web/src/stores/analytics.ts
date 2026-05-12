import { defineStore } from 'pinia'
import { ref } from 'vue'
import { analyticsApi } from '../api/analytics'

export const useAnalyticsStore = defineStore('analytics', () => {
  const trafficData = ref<any[]>([])
  const heatmapData = ref<any>(null)
  const dwellData = ref<any>(null)
  const loading = ref(false)

  async function fetchTraffic(cameraId?: string, period?: string) {
    loading.value = true
    try {
      const res = await analyticsApi.traffic({ camera_id: cameraId, period }) as any
      trafficData.value = res.data || []
    } finally {
      loading.value = false
    }
  }

  async function fetchHeatmap(cameraId: string) {
    const res = await analyticsApi.heatmap(cameraId) as any
    heatmapData.value = res.data
  }

  async function fetchDwell(cameraId: string) {
    const res = await analyticsApi.dwell(cameraId) as any
    dwellData.value = res.data
  }

  return { trafficData, heatmapData, dwellData, loading, fetchTraffic, fetchHeatmap, fetchDwell }
})
