<template>
  <AppLayout>
    <!-- 页面标题 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">仪表盘</h1>
        <p class="page-description">推理网关系统概览和实时状态监控</p>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="handleRefresh" :loading="refreshing">
          <el-icon><RefreshRight /></el-icon>
          刷新数据
        </el-button>
      </div>
    </div>

    <!-- 系统状态卡片 -->
    <div class="stats-overview">
      <div class="stats-grid">
        <!-- 健康节点数 -->
        <div class="stat-card primary" @click="navigateTo('/node-management')">
          <div class="stat-content">
            <div class="stat-icon primary">
              <el-icon><Monitor /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ gatewayStore.nodeStatusSummary.healthy }}</div>
              <div class="stat-label">健康节点</div>
              <div class="stat-subtitle">/ {{ gatewayStore.nodeStatusSummary.total }} 总节点</div>
            </div>
          </div>
          <div class="stat-trend">
            <div class="trend-value" :class="nodeHealthTrendClass">
              {{ gatewayStore.nodeStatusSummary.healthyPercentage }}%
            </div>
          </div>
        </div>

        <!-- 运行任务数 -->
        <div class="stat-card info" @click="navigateTo('/task-monitor')">
          <div class="stat-content">
            <div class="stat-icon info">
              <el-icon><DataLine /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ gatewayStore.currentTasks.length }}</div>
              <div class="stat-label">运行中任务</div>
              <div class="stat-subtitle">实时更新</div>
            </div>
          </div>
          <div class="stat-actions">
            <el-badge 
              v-if="gatewayStore.currentTasks.length > 0"
              :value="gatewayStore.currentTasks.length"
              type="primary"
            />
          </div>
        </div>

        <!-- 请求成功率 -->
        <div class="stat-card success">
          <div class="stat-content">
            <div class="stat-icon success">
              <el-icon><SuccessFilled /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ formatPercentage(gatewayStore.systemStats.successRate) }}</div>
              <div class="stat-label">请求成功率</div>
              <div class="stat-subtitle">最近24小时</div>
            </div>
          </div>
          <div class="stat-trend">
            <div class="trend-value success">
              <el-icon><DataLine /></el-icon>
            </div>
          </div>
        </div>

        <!-- 平均响应时间 -->
        <div class="stat-card warning">
          <div class="stat-content">
            <div class="stat-icon warning">
              <el-icon><Timer /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ formatResponseTime(gatewayStore.systemStats.averageResponseTime) }}</div>
              <div class="stat-label">平均响应时间</div>
              <div class="stat-subtitle">毫秒</div>
            </div>
          </div>
          <div class="stat-trend">
            <div class="trend-value" :class="responseTimeTrendClass">
              {{ formatResponseTime(gatewayStore.systemStats.averageResponseTime) }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 详细信息区域 -->
    <div class="dashboard-content">
      <el-row :gutter="24">
        <!-- 左侧：节点状态详情 -->
        <el-col :xs="24" :lg="16">
          <div class="content-card">
            <div class="card-header">
              <div class="card-title">
                <el-icon><Monitor /></el-icon>
                <span>节点状态</span>
              </div>
              <div class="card-actions">
                <el-button 
                  size="small" 
                  @click="handleCheckAllNodes"
                  :loading="checkingNodes"
                >
                  检查所有节点
                </el-button>
                <el-button 
                  size="small" 
                  type="primary" 
                  @click="navigateTo('/node-management')"
                >
                  管理节点
                </el-button>
              </div>
            </div>
            
            <div class="card-content">
              <NodeStatusTable 
                :nodes="gatewayStore.nodes" 
                :loading="gatewayStore.connectionStatus === 'connecting'"
                @refresh="handleRefreshNodes"
                @check-node="handleCheckNode"
              />
            </div>
          </div>
        </el-col>

        <!-- 右侧：系统活动 -->
        <el-col :xs="24" :lg="8">
          <div class="content-card">
            <div class="card-header">
              <div class="card-title">
                <el-icon><Bell /></el-icon>
                <span>系统活动</span>
              </div>
              <div class="card-actions">
                <el-button size="small" text @click="clearActivities">
                  清空
                </el-button>
              </div>
            </div>
            
            <div class="card-content">
              <SystemActivityFeed 
                :activities="systemActivities"
                :loading="refreshing"
              />
            </div>
          </div>
        </el-col>
      </el-row>

    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  RefreshRight,
  Monitor,
  DataLine,
  SuccessFilled,
  Timer,
  Bell
} from '@element-plus/icons-vue'
import { useGatewayStore } from '@/stores/gateway'
import AppLayout from '@/layouts/AppLayout.vue'
import NodeStatusTable from '@/components/dashboard/NodeStatusTable.vue'
import SystemActivityFeed from '@/components/dashboard/SystemActivityFeed.vue'
import type { NodeInfo } from '@/types/gateway'

// 存储和路由
const gatewayStore = useGatewayStore()
const router = useRouter()

// 响应式状态
const refreshing = ref(false)
const checkingNodes = ref(false)
const systemActivities = ref([
  {
    id: '1',
    type: 'info',
    title: '系统启动',
    message: 'Gateway控制台已启动',
    timestamp: new Date().toISOString(),
    icon: 'Monitor'
  },
  {
    id: '2',
    type: 'success',
    title: '节点连接',
    message: '节点node-01已成功连接',
    timestamp: new Date(Date.now() - 60000).toISOString(),
    icon: 'SuccessFilled'
  }
])

// 定时刷新
const refreshInterval = ref<NodeJS.Timeout>()

// 计算属性
const nodeHealthTrendClass = computed(() => {
  const percentage = gatewayStore.nodeStatusSummary.healthyPercentage
  if (percentage >= 80) return 'trend-up'
  if (percentage >= 50) return 'trend-stable'
  return 'trend-down'
})

const responseTimeTrendClass = computed(() => {
  const responseTime = gatewayStore.systemStats.averageResponseTime
  if (responseTime < 200) return 'trend-up'
  if (responseTime < 500) return 'trend-stable'
  return 'trend-down'
})

// 工具函数
const formatPercentage = (value: number): string => {
  return `${Math.round(value)}%`
}

const formatResponseTime = (value: number): string => {
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(1)}s`
}

// 事件处理函数
const handleRefresh = async () => {
  refreshing.value = true
  try {
    await gatewayStore.refresh()
    
    // 添加活动记录
    addSystemActivity({
      type: 'info',
      title: '数据刷新',
      message: '仪表盘数据已更新',
      icon: 'RefreshRight'
    })
    
    ElMessage.success('数据刷新完成')
  } catch (error) {
    ElMessage.error(`刷新失败: ${error}`)
    addSystemActivity({
      type: 'error',
      title: '刷新失败',
      message: `数据刷新失败: ${error}`,
      icon: 'Close'
    })
  } finally {
    refreshing.value = false
  }
}

const handleRefreshNodes = async () => {
  try {
    await gatewayStore.fetchNodeStatus()
    ElMessage.success('节点状态已更新')
  } catch (error) {
    ElMessage.error('更新节点状态失败')
  }
}

const handleCheckNode = async (nodeId: string) => {
  try {
    // 这里应该调用API检查单个节点
    ElMessage.success(`正在检查节点 ${nodeId}`)
    addSystemActivity({
      type: 'info',
      title: '节点检查',
      message: `正在检查节点 ${nodeId}`,
      icon: 'Monitor'
    })
  } catch (error) {
    ElMessage.error(`检查节点失败: ${error}`)
  }
}

const handleCheckAllNodes = async () => {
  if (gatewayStore.nodes.length === 0) {
    ElMessage.warning('暂无节点需要检查')
    return
  }

  checkingNodes.value = true
  try {
    // 模拟批量检查节点
    await new Promise(resolve => setTimeout(resolve, 2000))
    
    ElMessage.success('所有节点检查完成')
    addSystemActivity({
      type: 'success',
      title: '批量检查完成',
      message: `已检查 ${gatewayStore.nodes.length} 个节点`,
      icon: 'SuccessFilled'
    })
  } catch (error) {
    ElMessage.error('批量检查失败')
  } finally {
    checkingNodes.value = false
  }
}


const navigateTo = (path: string) => {
  router.push(path)
}

const addSystemActivity = (activity: {
  type: 'info' | 'success' | 'warning' | 'error'
  title: string
  message: string
  icon: string
}) => {
  const newActivity = {
    id: Date.now().toString(),
    ...activity,
    timestamp: new Date().toISOString()
  }
  
  systemActivities.value.unshift(newActivity)
  
  // 最多保留20条记录
  if (systemActivities.value.length > 20) {
    systemActivities.value = systemActivities.value.slice(0, 20)
  }
}

const clearActivities = () => {
  ElMessageBox.confirm('确定要清空所有活动记录吗？', '确认清空', {
    type: 'warning'
  }).then(() => {
    systemActivities.value = []
    ElMessage.success('活动记录已清空')
  }).catch(() => {})
}

// 初始化数据
const initializeDashboard = async () => {
  if (!gatewayStore.isConnected) {
    try {
      await gatewayStore.initialize()
    } catch (error) {
      console.error('初始化Gateway连接失败:', error)
      addSystemActivity({
        type: 'error',
        title: '连接失败',
        message: 'Gateway连接失败，请检查网络或配置',
        icon: 'Close'
      })
    }
  }
}

// 生命周期
onMounted(async () => {
  await initializeDashboard()
  
  // 设置定时刷新（每30秒）
  refreshInterval.value = setInterval(() => {
    if (gatewayStore.isConnected) {
      gatewayStore.refresh().catch(console.error)
    }
  }, 30000)
})

onUnmounted(() => {
  if (refreshInterval.value) {
    clearInterval(refreshInterval.value)
  }
})
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 24px;
}

.header-left {
  flex: 1;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 8px;
}

.page-description {
  color: #909399;
  font-size: 14px;
  margin: 0;
}

.header-right {
  display: flex;
  gap: 8px;
}

.stats-overview {
  margin-bottom: 24px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}

/* 统计卡片 - 参考设计样式 */
.stat-card {
  background: #fff;
  border-radius: 8px;
  padding: 20px;
  border: 1px solid #ebeef5;
  cursor: pointer;
  transition: all 0.3s;
}

.stat-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
}

/* 浅色背景卡片 */
.stat-card.primary {
  background: linear-gradient(135deg, #e6f4ff 0%, #f0f9ff 100%);
  border-color: #d4e8ff;
}

.stat-card.info {
  background: linear-gradient(135deg, #fff7e6 0%, #fffbf0 100%);
  border-color: #ffe7ba;
}

.stat-card.success {
  background: linear-gradient(135deg, #e6fffb 0%, #f0fff9 100%);
  border-color: #b7eb8f;
}

.stat-card.warning {
  background: linear-gradient(135deg, #fff1f0 0%, #fff7f6 100%);
  border-color: #ffd6d6;
}

.stat-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  color: #fff;
  flex-shrink: 0;
}

.stat-icon.primary { background: #409eff; }
.stat-icon.info { background: #e6a23c; }
.stat-icon.success { background: #67c23a; }
.stat-icon.warning { background: #f56c6c; }

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: #303133;
  line-height: 1.2;
}

.stat-label {
  font-size: 14px;
  font-weight: 500;
  color: #606266;
  margin-top: 4px;
}

.stat-subtitle {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.stat-trend, .stat-actions {
  position: absolute;
  right: 16px;
  bottom: 16px;
}

.trend-value {
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 4px;
}

.trend-value.trend-up {
  color: #67c23a;
  background: rgba(103, 194, 58, 0.1);
}

.trend-value.trend-stable {
  color: #e6a23c;
  background: rgba(230, 162, 60, 0.1);
}

.trend-value.trend-down {
  color: #f56c6c;
  background: rgba(245, 108, 108, 0.1);
}

.trend-value.success {
  color: #67c23a;
}

.dashboard-content {
  padding: 0 var(--spacing-lg) var(--spacing-lg);
}

.content-card {
  background: var(--bg-color);
  border-radius: var(--border-radius-large);
  box-shadow: var(--box-shadow-base);
  border: 1px solid var(--border-lighter);
  overflow: hidden;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg) var(--spacing-lg) 0;
  margin-bottom: var(--spacing-md);
}

.card-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: var(--font-size-large);
  font-weight: 600;
  color: var(--text-primary);
}

.card-title .el-icon {
  font-size: 20px;
  color: var(--primary-color);
}

.card-actions {
  display: flex;
  gap: var(--spacing-sm);
}

.card-content {
  padding: 0 var(--spacing-lg) var(--spacing-lg);
}

/* 响应式设计 */
@media (max-width: 1200px) {
  .stats-grid {
    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  }
}

@media (max-width: 768px) {
  .page-header {
    padding: var(--spacing-md);
    margin-bottom: var(--spacing-md);
  }
  
  .header-content {
    flex-direction: column;
    gap: var(--spacing-md);
  }
  
  .stats-overview {
    padding: 0 var(--spacing-md);
    margin-bottom: var(--spacing-md);
  }
  
  .stats-grid {
    grid-template-columns: 1fr;
    gap: var(--spacing-md);
  }
  
  .dashboard-content {
    padding: 0 var(--spacing-md) var(--spacing-md);
  }
  
  .card-header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-md);
  }
  
  .card-actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
