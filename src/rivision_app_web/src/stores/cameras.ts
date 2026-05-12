import { defineStore } from 'pinia'
import { ref } from 'vue'
import { camerasApi } from '../api/cameras'

export const useCamerasStore = defineStore('cameras', () => {
  const cameras = ref<any[]>([])
  const loading = ref(false)

  async function fetchCameras() {
    loading.value = true
    try {
      const res = await camerasApi.list() as any
      cameras.value = res.cameras || []
    } finally {
      loading.value = false
    }
  }

  async function addCamera(data: any) {
    const res = await camerasApi.create(data)
    await fetchCameras()
    return res
  }

  async function removeCamera(id: string) {
    await camerasApi.delete(id)
    cameras.value = cameras.value.filter((c) => c.id !== id)
  }

  return { cameras, loading, fetchCameras, addCamera, removeCamera }
})
