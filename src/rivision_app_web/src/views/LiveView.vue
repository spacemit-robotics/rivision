<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useCamerasStore } from '../stores/cameras'

const camerasStore = useCamerasStore()
const gridLayout = ref<1 | 4 | 9 | 16>(4)

onMounted(() => {
  camerasStore.fetchCameras()
})
</script>

<template>
  <div class="live-view">
    <div class="toolbar">
      <h2>实时监控</h2>
      <div class="grid-selector">
        <button v-for="n in [1, 4, 9, 16]" :key="n"
          :class="{ active: gridLayout === n }"
          @click="gridLayout = n as 1|4|9|16">
          {{ n }}宫格
        </button>
      </div>
    </div>
    <div class="video-grid" :class="`grid-${gridLayout}`">
      <div v-for="(cam, i) in camerasStore.cameras.slice(0, gridLayout)" :key="cam.id || i"
        class="video-cell">
        <div class="video-placeholder">
          <span>{{ cam.name || `摄像头 ${i + 1}` }}</span>
          <small>{{ cam.url }}</small>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.live-view { padding: 16px; }
.toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.grid-selector button { margin-left: 8px; padding: 4px 12px; border: 1px solid #ddd; border-radius: 4px; cursor: pointer; }
.grid-selector button.active { background: #409eff; color: #fff; border-color: #409eff; }
.video-grid { display: grid; gap: 8px; }
.grid-1 { grid-template-columns: 1fr; }
.grid-4 { grid-template-columns: repeat(2, 1fr); }
.grid-9 { grid-template-columns: repeat(3, 1fr); }
.grid-16 { grid-template-columns: repeat(4, 1fr); }
.video-cell { aspect-ratio: 16/9; background: #1a1a2e; border-radius: 8px; display: flex; align-items: center; justify-content: center; }
.video-placeholder { text-align: center; color: #999; }
.video-placeholder small { display: block; font-size: 12px; margin-top: 4px; color: #666; }
</style>
