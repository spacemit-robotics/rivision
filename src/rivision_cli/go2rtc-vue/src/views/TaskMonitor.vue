<template>
  <AppLayout>
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">任务监控</h1>
        <p class="page-description">实时监控推理任务的执行状态和性能指标</p>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="handleRefresh" :loading="refreshing">
          <el-icon><RefreshRight /></el-icon>
          刷新
        </el-button>
        <el-button type="danger" @click="handleClearCompleted">
          <el-icon><Delete /></el-icon>
          清空已完成
        </el-button>
      </div>
    </div>

    <div class="page-content">
      <!-- 任务统计 -->
      <div class="stats-grid">
        <div class="stat-card primary">
          <div class="stat-content">
            <div class="stat-icon primary">
              <el-icon><List /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ totalTasks }}</div>
              <div class="stat-label">总提交</div>
            </div>
          </div>
        </div>
        
        <div class="stat-card info">
          <div class="stat-content">
            <div class="stat-icon info">
              <el-icon><Clock /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ runningTasks }}</div>
              <div class="stat-label">运行中</div>
            </div>
          </div>
        </div>
        
        <div class="stat-card warning">
          <div class="stat-content">
            <div class="stat-icon warning">
              <el-icon><Loading /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ queuedTasks }}</div>
              <div class="stat-label">队列中</div>
            </div>
          </div>
        </div>
        
        <div class="stat-card success">
          <div class="stat-content">
            <div class="stat-icon success">
              <el-icon><CircleCheckFilled /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ completedTasks }}</div>
              <div class="stat-label">已完成</div>
            </div>
          </div>
        </div>
        
        <div class="stat-card cancel">
          <div class="stat-content">
            <div class="stat-icon cancel">
              <el-icon><Close /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ cancelledTasks }}</div>
              <div class="stat-label">已取消</div>
            </div>
          </div>
        </div>
      </div>

      <!-- 任务列表 -->
      <div class="content-card">
        <div class="card-header">
          <div class="card-title">
            <el-icon><DataLine /></el-icon>
            <span>任务列表</span>
          </div>
          <div class="card-actions">
            <el-select v-model="statusFilter" size="small" @change="handleFilterChange">
              <el-option label="全部状态" value="" />
              <el-option label="运行中" value="running" />
              <el-option label="队列中" value="queued" />
              <el-option label="已完成" value="completed" />
              <el-option label="失败" value="failed" />
            </el-select>
          </div>
        </div>
        
        <div class="card-content">
          <el-table
            :data="filteredTasks"
            :loading="refreshing"
            empty-text="暂无任务数据"
            class="tasks-table"
            @row-click="handleRowClick"
          >
            <el-table-column prop="task_id" label="任务ID" min-width="120">
              <template #default="{ row }">
                <div class="task-id-cell" :title="row.task_id || row.id || ''">
                  <el-icon class="task-icon" :class="getTaskIconClass(row.status)">
                    <component :is="getTaskIcon(row.status)" />
                  </el-icon>
                  <span>{{ (row.task_id || row.id || '').substring(0, 12) }}...</span>
                </div>
              </template>
            </el-table-column>
            
            <el-table-column prop="task_type" label="类型" width="100">
              <template #default="{ row }">
                <el-tag :type="getTypeTagType(row.task_type)">
                  {{ getTypeLabel(row.task_type) }}
                </el-tag>
              </template>
            </el-table-column>
            
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="getStatusTagType(row.status)">
                  {{ getStatusLabel(row.status) }}
                </el-tag>
              </template>
            </el-table-column>
            
            <el-table-column prop="progress" label="进度" width="150">
              <template #default="{ row }">
                <div class="progress-container">
                  <el-progress 
                    :percentage="row.progress" 
                    :show-text="false"
                    :stroke-width="8"
                    :color="getProgressColor(row.status)"
                  />
                  <span class="progress-text">{{ row.progress }}%</span>
                </div>
              </template>
            </el-table-column>
            
            <el-table-column prop="node_id" label="执行节点" width="120">
              <template #default="{ row }">
                <span v-if="row.node_id">{{ row.node_id }}</span>
                <span v-else class="text-placeholder">未分配</span>
              </template>
            </el-table-column>
            
            <el-table-column prop="start_time" label="开始时间" width="160">
              <template #default="{ row }">
                {{ formatTimestamp(row.start_time) }}
              </template>
            </el-table-column>
            
            <el-table-column prop="duration" label="耗时" width="100">
              <template #default="{ row }">
                {{ formatDuration(row) }}
              </template>
            </el-table-column>
            
            <el-table-column label="操作" width="220" fixed="right">
              <template #default="{ row }">
                <el-button 
                  size="small" 
                  @click.stop="handleViewDetails(row)"
                >
                  详情
                </el-button>
                <el-button 
                  v-if="row.status === 'running' || row.status === 'queued' || row.status === 'pending'"
                  size="small" 
                  type="warning" 
                  @click.stop="handleCancelTask(row)"
                >
                  停止
                </el-button>
                <el-button 
                  v-if="row.status === 'running' && getRunningSeconds(row) > 60"
                  size="small" 
                  type="danger" 
                  @click.stop="handleForceCancelTask(row)"
                  title="将重启该节点的llama-server，影响该节点上所有任务"
                >
                  重启节点
                </el-button>
              </template>
            </el-table-column>
          </el-table>
          
          <div class="table-pagination">
            <el-pagination
              v-model:current-page="currentPage"
              :page-size="pageSize"
              :total="totalTasks"
              :pager-count="5"
              size="small"
              layout="prev, pager, next, total"
              @current-change="handlePageChange"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- 任务详情对话框 -->
    <TaskDetailsDialog
      v-model="showDetailsDialog"
      :task="selectedTask"
      @refresh="handleRefresh"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  RefreshRight,
  Delete,
  Clock,
  Loading,
  CircleCheckFilled,
  Close,
  List,
  DataLine
} from '@element-plus/icons-vue'
import { useGatewayStore } from '@/stores/gateway'
import AppLayout from '@/layouts/AppLayout.vue'
import TaskDetailsDialog from '@/components/tasks/TaskDetailsDialog.vue'
import type { TaskInfo } from '@/types/gateway'

const gatewayStore = useGatewayStore()

// 状态管理
const refreshing = ref(false)
const statusFilter = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const showDetailsDialog = ref(false)
const selectedTask = ref<TaskInfo | null>(null)

// 任务数据（从Gateway API获取）
const allTasks = ref<any[]>([])

// 定时刷新
const refreshInterval = ref<NodeJS.Timeout>()

// 计算属性
const filteredTasks = computed(() => {
  let tasks = allTasks.value
  
  if (statusFilter.value) {
    tasks = tasks.filter(task => task.status === statusFilter.value)
  }
  
  // 分页
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return tasks.slice(start, end)
})

const totalTasks = computed(() => allTasks.value.length)

const runningTasks = computed(() => 
  allTasks.value.filter(task => task.status === 'running').length
)

const queuedTasks = computed(() => 
  allTasks.value.filter(task => 
    task.status === 'queued' || task.status === 'pending'
  ).length
)

// 使用后端返回的统计数据（已完成和已取消任务不在currentTasks中）
const cancelledTasks = computed(() => gatewayStore.taskStats.cancelledCount)

const completedTasks = computed(() => gatewayStore.taskStats.completedCount)

const failedTasks = computed(() => 
  allTasks.value.filter(task => task.status === 'failed').length
)

// 工具函数
const getRunningSeconds = (task: any): number => {
  if (!task.start_time && !task.started_at) return 0
  const startTime = task.start_time || new Date(task.started_at).getTime() / 1000
  const now = Date.now() / 1000
  return Math.floor(now - startTime)
}

const getTaskIcon = (status: string) => {
  switch (status) {
    case 'running': return 'Loading'
    case 'queued': return 'Clock'
    case 'completed': return 'CircleCheckFilled'
    case 'failed': return 'CircleCloseFilled'
    default: return 'Clock'
  }
}

const getTaskIconClass = (status: string) => {
  switch (status) {
    case 'running': return 'running'
    case 'queued': return 'queued'
    case 'completed': return 'completed'
    case 'failed': return 'failed'
    default: return ''
  }
}

const getTypeLabel = (type: string) => {
  if (!type || type === 'unknown') return '推理'
  const labels: Record<string, string> = {
    image: '图片',
    video: '视频',
    search: '搜索',
    summary: '摘要',
    detection: '检测',
    report: '报告'
  }
  return labels[type] || type
}

const getTypeTagType = (type: string) => {
  const types: Record<string, string> = {
    image: 'info',
    video: 'primary',
    search: 'success',
    summary: 'warning',
    detection: 'danger'
  }
  return types[type] || 'info'
}

const getStatusLabel = (status: string) => {
  const labels: Record<string, string> = {
    queued: '队列中',
    running: '运行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消'
  }
  return labels[status] || status
}

const getStatusTagType = (status: string) => {
  const types: Record<string, string> = {
    queued: 'info',
    running: 'warning',
    completed: 'success',
    failed: 'danger',
    cancelled: 'info'
  }
  return types[status] || 'info'
}

const getProgressColor = (status: string) => {
  switch (status) {
    case 'running': return '#409eff'
    case 'completed': return '#67c23a'
    case 'failed': return '#f56c6c'
    default: return '#909399'
  }
}

const formatTime = (timeStr: string) => {
  return new Date(timeStr).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

const formatTimestamp = (timestamp: number | null) => {
  if (!timestamp) return '--'
  return new Date(timestamp * 1000).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

const formatDuration = (task: any) => {
  const start = task.start_time ? task.start_time * 1000 : null
  const end = task.end_time ? task.end_time * 1000 : Date.now()
  
  if (!start) return '--'
  
  const duration = Math.floor((end - start) / 1000)
  
  if (duration < 60) return `${duration}秒`
  if (duration < 3600) return `${Math.floor(duration / 60)}分钟`
  return `${Math.floor(duration / 3600)}小时`
}

// 事件处理
const handleRefresh = async () => {
  refreshing.value = true
  try {
    await gatewayStore.fetchCurrentTasks()
    // 使用Gateway任务数据，添加progress字段
    allTasks.value = (gatewayStore.currentTasks as any[]).map(task => ({
      ...task,
      // 进度计算：运行中=50%，完成=100%，其他=0%
      progress: task.status === 'completed' ? 100 
        : task.status === 'running' ? 50 
        : 0
    }))
    ElMessage.success('任务数据已刷新')
  } catch (error) {
    ElMessage.error('刷新失败')
  } finally {
    refreshing.value = false
  }
}

const handleFilterChange = () => {
  currentPage.value = 1
}

const handlePageChange = (page: number) => {
  currentPage.value = page
}

const handleRowClick = (row: TaskInfo) => {
  handleViewDetails(row)
}

const handleViewDetails = (task: TaskInfo) => {
  selectedTask.value = task
  showDetailsDialog.value = true
}

const handleCancelTask = async (task: any) => {
  try {
    const taskId = task.task_id || task.id || ''
    await ElMessageBox.confirm(`确定要取消任务 ${taskId.substring(0, 12)}... 吗？`, '确认取消', {
      type: 'warning'
    })

    await gatewayStore.cancelTask(taskId, '用户手动取消')
    
    // 更新本地状态  
    const index = allTasks.value.findIndex(t => (t.task_id || t.id) === taskId)
    if (index >= 0) {
      allTasks.value[index].status = 'cancelled'
    }
    
    ElMessage.success('任务已取消')
    
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('取消任务失败')
    }
  }
}

const handleForceCancelTask = async (task: any) => {
  try {
    const nodeId = task.node_id || '未知'
    await ElMessageBox.confirm(
      `⚠️ 此操作将重启节点 ${nodeId} 的 llama-server 进程！\n\n注意事项：\n• 该节点上所有正在执行的任务都会被中断\n• 节点将短暂不可用（约5-10秒）\n• 仅在任务卡死无法响应时使用\n\n确定要重启节点吗？`, 
      '确认重启节点', 
      { 
        type: 'warning',
        confirmButtonText: '重启节点',
        cancelButtonText: '取消',
        dangerouslyUseHTMLString: false
      }
    )

    const taskId = task.task_id || task.id || ''
    const success = await gatewayStore.forceCancelTask(taskId, '用户重启节点')
    
    if (success) {
      ElMessage.success(`节点 ${nodeId} 已重启`)
      // 刷新任务列表
      await handleRefresh()
    } else {
      ElMessage.error('重启节点失败')
    }
    
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('重启节点失败')
    }
  }
}

const handleClearCompleted = async () => {
  const completedCount = completedTasks.value + failedTasks.value + cancelledTasks.value
  
  if (completedCount === 0) {
    ElMessage.info('没有已完成的任务需要清理')
    return
  }

  try {
    await ElMessageBox.confirm(
      `将清空 ${completedCount} 个已完成/失败/取消的任务记录，确定继续吗？`, 
      '确认清空', 
      { type: 'warning' }
    )

    allTasks.value = allTasks.value.filter(
      task => task.status !== 'completed' && task.status !== 'failed' && task.status !== 'cancelled'
    )
    
    ElMessage.success(`已清空 ${completedCount} 个任务记录`)
    
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('清空失败')
    }
  }
}

// 生命周期
onMounted(() => {
  handleRefresh()
  
  // 设置定时刷新（每10秒）
  refreshInterval.value = setInterval(() => {
    if (gatewayStore.isConnected) {
      handleRefresh()
    }
  }, 10000)
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

.page-content {
  /* 页面内容区 */
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

/* 统计卡片 - 参考设计样式 */
.stat-card {
  background: #fff;
  border-radius: 8px;
  padding: 16px 20px;
  border: 1px solid #ebeef5;
  display: flex;
  align-items: center;
  gap: 12px;
}

.stat-card.primary {
  background: linear-gradient(135deg, #e6f4ff 0%, #f0f9ff 100%);
  border-color: #d4e8ff;
}

.stat-card.info {
  background: linear-gradient(135deg, #fff7e6 0%, #fffbf0 100%);
  border-color: #ffe7ba;
}

.stat-card.warning {
  background: linear-gradient(135deg, #fffbe6 0%, #fffef0 100%);
  border-color: #ffe58f;
}

.stat-card.success {
  background: linear-gradient(135deg, #e6fffb 0%, #f0fff9 100%);
  border-color: #b7eb8f;
}

.stat-card.danger {
  background: linear-gradient(135deg, #fff1f0 0%, #fff7f6 100%);
  border-color: #ffd6d6;
}

.stat-content {
  display: flex;
  align-items: center;
  gap: 12px;
}

.stat-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  color: #fff;
}

.stat-icon.primary { background: #409eff; }
.stat-icon.info { background: #e6a23c; }
.stat-icon.warning { background: #f0c020; }
.stat-icon.success { background: #67c23a; }
.stat-icon.danger { background: #f56c6c; }

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
  line-height: 1.2;
}

.stat-label {
  font-size: 13px;
  color: #606266;
  margin-top: 2px;
}

/* 任务列表卡片 */
.task-list-card {
  background: #fff;
  border-radius: 8px;
  border: 1px solid #ebeef5;
  overflow: hidden;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #ebeef5;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.card-title .el-icon {
  color: #409eff;
}

.card-content {
  padding: 0;
}

.task-id {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.task-icon {
  font-size: 16px;
}

.task-icon.running {
  color: var(--primary-color);
  animation: spin 1s linear infinite;
}

.task-icon.queued {
  color: var(--info-color);
}

.task-icon.completed {
  color: var(--success-color);
}

.task-icon.failed {
  color: var(--danger-color);
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.progress-container {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.progress-text {
  font-size: var(--font-size-small);
  color: var(--text-secondary);
  min-width: 35px;
}

.text-placeholder {
  color: var(--text-placeholder);
  font-style: italic;
}

.table-pagination {
  margin-top: var(--spacing-lg);
  display: flex;
  justify-content: center;
}

.tasks-table :deep(.el-table__row) {
  cursor: pointer;
}

.tasks-table :deep(.el-table__row:hover) {
  background-color: var(--bg-page);
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  
  .header-content {
    flex-direction: column;
    gap: var(--spacing-md);
  }
  
  .tasks-table :deep(.el-table__column--fixed-right) {
    display: none;
  }
}
</style>
