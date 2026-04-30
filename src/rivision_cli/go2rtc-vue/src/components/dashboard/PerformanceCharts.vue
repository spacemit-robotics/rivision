<template>
  <div class="performance-charts">
    <div v-if="loading" class="loading-state">
      <el-skeleton :rows="5" animated />
    </div>
    
    <div v-else class="charts-container">
      <div class="charts-grid">
        <!-- 响应时间图表 -->
        <div class="chart-card">
          <div class="chart-header">
            <h4 class="chart-title">响应时间趋势</h4>
            <div class="chart-legend">
              <span class="legend-item">
                <div class="legend-color primary"></div>
                <span>平均响应时间</span>
              </span>
            </div>
          </div>
          <div class="chart-content">
            <div ref="responseTimeChart" class="chart"></div>
          </div>
        </div>

        <!-- 吞吐量图表 -->
        <div class="chart-card">
          <div class="chart-header">
            <h4 class="chart-title">系统吞吐量</h4>
            <div class="chart-legend">
              <span class="legend-item">
                <div class="legend-color success"></div>
                <span>请求/秒</span>
              </span>
            </div>
          </div>
          <div class="chart-content">
            <div ref="throughputChart" class="chart"></div>
          </div>
        </div>

        <!-- 节点使用率图表 -->
        <div class="chart-card">
          <div class="chart-header">
            <h4 class="chart-title">节点资源使用率</h4>
            <div class="chart-legend">
              <span class="legend-item">
                <div class="legend-color warning"></div>
                <span>CPU</span>
              </span>
              <span class="legend-item">
                <div class="legend-color info"></div>
                <span>内存</span>
              </span>
            </div>
          </div>
          <div class="chart-content">
            <div ref="resourceChart" class="chart"></div>
          </div>
        </div>

        <!-- 成功率图表 -->
        <div class="chart-card">
          <div class="chart-header">
            <h4 class="chart-title">请求成功率</h4>
            <div class="chart-legend">
              <span class="legend-item">
                <div class="legend-color success"></div>
                <span>成功率</span>
              </span>
            </div>
          </div>
          <div class="chart-content">
            <div ref="successRateChart" class="chart"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, nextTick } from 'vue'

// Props
interface Props {
  timeRange: string
  loading?: boolean
}
const props = defineProps<Props>()

// 图表引用
const responseTimeChart = ref<HTMLElement>()
const throughputChart = ref<HTMLElement>()
const resourceChart = ref<HTMLElement>()
const successRateChart = ref<HTMLElement>()

// 模拟数据生成
const generateMockData = (timeRange: string) => {
  const now = Date.now()
  let dataPoints = 24
  let interval = 3600000 // 1小时

  switch (timeRange) {
    case '1h':
      dataPoints = 12
      interval = 300000 // 5分钟
      break
    case '6h':
      dataPoints = 24
      interval = 900000 // 15分钟
      break
    case '24h':
      dataPoints = 24
      interval = 3600000 // 1小时
      break
    case '7d':
      dataPoints = 7
      interval = 86400000 // 1天
      break
  }

  const timestamps = Array.from({ length: dataPoints }, (_, i) => 
    new Date(now - (dataPoints - 1 - i) * interval)
  )

  return {
    timestamps,
    responseTime: Array.from({ length: dataPoints }, () => 
      Math.floor(Math.random() * 200 + 50)
    ),
    throughput: Array.from({ length: dataPoints }, () => 
      Math.floor(Math.random() * 100 + 20)
    ),
    cpuUsage: Array.from({ length: dataPoints }, () => 
      Math.floor(Math.random() * 60 + 20)
    ),
    memoryUsage: Array.from({ length: dataPoints }, () => 
      Math.floor(Math.random() * 50 + 30)
    ),
    successRate: Array.from({ length: dataPoints }, () => 
      Math.floor(Math.random() * 10 + 90)
    )
  }
}

// 创建简单的SVG图表
const createSVGChart = (container: HTMLElement, data: number[], color: string, label: string) => {
  container.innerHTML = ''
  
  const width = container.clientWidth || 300
  const height = container.clientHeight || 200
  const padding = 20
  
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  svg.setAttribute('width', width.toString())
  svg.setAttribute('height', height.toString())
  svg.style.width = '100%'
  svg.style.height = '100%'
  
  const chartWidth = width - padding * 2
  const chartHeight = height - padding * 2
  
  if (data.length === 0) return
  
  const maxValue = Math.max(...data)
  const minValue = Math.min(...data)
  const range = maxValue - minValue || 1
  
  // 创建路径
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  const pathData = data.map((value, index) => {
    const x = padding + (index / (data.length - 1)) * chartWidth
    const y = padding + chartHeight - ((value - minValue) / range) * chartHeight
    return `${index === 0 ? 'M' : 'L'} ${x} ${y}`
  }).join(' ')
  
  path.setAttribute('d', pathData)
  path.setAttribute('stroke', color)
  path.setAttribute('stroke-width', '2')
  path.setAttribute('fill', 'none')
  path.style.filter = 'drop-shadow(0 1px 2px rgba(0,0,0,0.1))'
  
  // 创建填充区域
  const area = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  const areaData = pathData + ` L ${padding + chartWidth} ${padding + chartHeight} L ${padding} ${padding + chartHeight} Z`
  area.setAttribute('d', areaData)
  area.setAttribute('fill', `url(#gradient-${color.replace('#', '')})`)
  area.setAttribute('opacity', '0.3')
  
  // 创建渐变
  const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs')
  const gradient = document.createElementNS('http://www.w3.org/2000/svg', 'linearGradient')
  gradient.setAttribute('id', `gradient-${color.replace('#', '')}`)
  gradient.setAttribute('x1', '0%')
  gradient.setAttribute('y1', '0%')
  gradient.setAttribute('x2', '0%')
  gradient.setAttribute('y2', '100%')
  
  const stop1 = document.createElementNS('http://www.w3.org/2000/svg', 'stop')
  stop1.setAttribute('offset', '0%')
  stop1.setAttribute('stop-color', color)
  stop1.setAttribute('stop-opacity', '0.3')
  
  const stop2 = document.createElementNS('http://www.w3.org/2000/svg', 'stop')
  stop2.setAttribute('offset', '100%')
  stop2.setAttribute('stop-color', color)
  stop2.setAttribute('stop-opacity', '0')
  
  gradient.appendChild(stop1)
  gradient.appendChild(stop2)
  defs.appendChild(gradient)
  
  svg.appendChild(defs)
  svg.appendChild(area)
  svg.appendChild(path)
  
  // 添加数据点
  data.forEach((value, index) => {
    const x = padding + (index / (data.length - 1)) * chartWidth
    const y = padding + chartHeight - ((value - minValue) / range) * chartHeight
    
    const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle')
    circle.setAttribute('cx', x.toString())
    circle.setAttribute('cy', y.toString())
    circle.setAttribute('r', '3')
    circle.setAttribute('fill', color)
    circle.style.cursor = 'pointer'
    
    // 添加悬停效果
    circle.addEventListener('mouseenter', () => {
      circle.setAttribute('r', '5')
      
      // 显示tooltip（简单实现）
      const tooltip = document.createElement('div')
      tooltip.textContent = `${label}: ${value}`
      tooltip.style.position = 'absolute'
      tooltip.style.background = 'rgba(0,0,0,0.8)'
      tooltip.style.color = 'white'
      tooltip.style.padding = '4px 8px'
      tooltip.style.borderRadius = '4px'
      tooltip.style.fontSize = '12px'
      tooltip.style.pointerEvents = 'none'
      tooltip.style.zIndex = '1000'
      tooltip.style.left = `${x}px`
      tooltip.style.top = `${y - 30}px`
      container.appendChild(tooltip)
      
      setTimeout(() => {
        if (container.contains(tooltip)) {
          container.removeChild(tooltip)
        }
      }, 2000)
    })
    
    circle.addEventListener('mouseleave', () => {
      circle.setAttribute('r', '3')
    })
    
    svg.appendChild(circle)
  })
  
  container.appendChild(svg)
}

// 初始化所有图表
const initCharts = async () => {
  await nextTick()
  
  if (props.loading) return
  
  const mockData = generateMockData(props.timeRange)
  
  if (responseTimeChart.value) {
    createSVGChart(responseTimeChart.value, mockData.responseTime, '#409eff', '响应时间(ms)')
  }
  
  if (throughputChart.value) {
    createSVGChart(throughputChart.value, mockData.throughput, '#67c23a', '请求/秒')
  }
  
  if (resourceChart.value) {
    // CPU使用率图表
    createSVGChart(resourceChart.value, mockData.cpuUsage, '#e6a23c', 'CPU(%)')
  }
  
  if (successRateChart.value) {
    createSVGChart(successRateChart.value, mockData.successRate, '#67c23a', '成功率(%)')
  }
}

// 监听时间范围变化
watch(() => props.timeRange, () => {
  if (!props.loading) {
    initCharts()
  }
})

// 监听加载状态变化
watch(() => props.loading, (newLoading) => {
  if (!newLoading) {
    initCharts()
  }
})

// 窗口大小变化时重新绘制
const handleResize = () => {
  if (!props.loading) {
    setTimeout(initCharts, 100)
  }
}

// 生命周期
onMounted(() => {
  initCharts()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.performance-charts {
  width: 100%;
  min-height: 400px;
}

.loading-state {
  padding: var(--spacing-lg);
}

.charts-container {
  width: 100%;
}

.charts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: var(--spacing-lg);
}

.chart-card {
  background: var(--bg-color);
  border-radius: var(--border-radius-base);
  border: 1px solid var(--border-lighter);
  overflow: hidden;
}

.chart-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-md) var(--spacing-lg);
  background-color: var(--bg-page);
  border-bottom: 1px solid var(--border-lighter);
}

.chart-title {
  font-size: var(--font-size-base);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.chart-legend {
  display: flex;
  gap: var(--spacing-md);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-small);
  color: var(--text-secondary);
}

.legend-color {
  width: 12px;
  height: 12px;
  border-radius: 2px;
}

.legend-color.primary {
  background-color: var(--primary-color);
}

.legend-color.success {
  background-color: var(--success-color);
}

.legend-color.warning {
  background-color: var(--warning-color);
}

.legend-color.info {
  background-color: var(--info-color);
}

.chart-content {
  padding: var(--spacing-lg);
  height: 200px;
  position: relative;
}

.chart {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 响应式设计 */
@media (max-width: 1200px) {
  .charts-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .charts-grid {
    grid-template-columns: 1fr;
    gap: var(--spacing-md);
  }
  
  .chart-header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-sm);
  }
  
  .chart-legend {
    gap: var(--spacing-sm);
  }
  
  .chart-content {
    height: 150px;
    padding: var(--spacing-md);
  }
}
</style>
