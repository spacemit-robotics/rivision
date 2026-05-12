<script setup lang="ts">
defineProps<{ data: any }>()
</script>

<template>
  <div class="dwell-chart">
    <h4>停留时间分布</h4>
    <div v-if="data" class="chart-placeholder">
      <p>平均停留: {{ data.avg_dwell_seconds || 0 }}s</p>
    </div>
    <p v-else class="empty">暂无数据</p>
  </div>
</template>

<style scoped>
.dwell-chart { padding: 12px; border: 1px solid #eee; border-radius: 8px; }
.empty { color: #999; text-align: center; }
</style>
