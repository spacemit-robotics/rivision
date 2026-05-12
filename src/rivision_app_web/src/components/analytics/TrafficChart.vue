<script setup lang="ts">
defineProps<{ data: any[] }>()
</script>

<template>
  <div class="traffic-chart">
    <h4>客流趋势</h4>
    <div class="chart-placeholder" v-if="data && data.length > 0">
      <!-- ECharts integration point -->
      <div class="bars">
        <div v-for="(item, i) in data.slice(0, 24)" :key="i" class="bar"
          :style="{ height: Math.min((item.count || 0) * 2, 100) + 'px' }">
        </div>
      </div>
    </div>
    <p v-else class="empty">暂无数据</p>
  </div>
</template>

<style scoped>
.traffic-chart { padding: 12px; border: 1px solid #eee; border-radius: 8px; }
.bars { display: flex; align-items: flex-end; gap: 4px; height: 120px; }
.bar { width: 16px; background: #409eff; border-radius: 2px 2px 0 0; min-height: 2px; }
.empty { color: #999; text-align: center; }
</style>
