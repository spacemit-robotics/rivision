<template>
  <AppLayout>
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">节点管理</h1>
        <p class="page-description">管理和监控推理网关的计算节点状态</p>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="handleCheckAllNodes" :loading="checking">
          <el-icon><RefreshRight /></el-icon>
          检查所有节点
        </el-button>
      </div>
    </div>

    <div class="page-content">
      <!-- 节点统计卡片 -->
      <div class="stats-grid">
        <div class="stat-card success">
          <div class="stat-content">
            <div class="stat-icon success">
              <el-icon><CircleCheckFilled /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ healthyNodes }}</div>
              <div class="stat-label">健康节点</div>
            </div>
          </div>
        </div>
        
        <div class="stat-card danger">
          <div class="stat-content">
            <div class="stat-icon danger">
              <el-icon><CircleCloseFilled /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ offlineNodes }}</div>
              <div class="stat-label">离线节点</div>
            </div>
          </div>
        </div>
        
        <div class="stat-card info">
          <div class="stat-content">
            <div class="stat-icon info">
              <el-icon><DataLine /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ totalTasks }}</div>
              <div class="stat-label">总活跃任务</div>
            </div>
          </div>
        </div>
      </div>

      <!-- 节点列表 -->
      <div class="content-card">
        <div class="card-header">
          <div class="card-title">
            <el-icon><Monitor /></el-icon>
            <span>节点列表</span>
          </div>
        </div>
        
        <div class="card-content">
          <el-table
            :data="gatewayStore.nodes"
            :loading="gatewayStore.connectionStatus === 'connecting'"
            empty-text="暂无节点数据"
            class="nodes-table"
          >
            <el-table-column prop="id" label="节点ID" min-width="120">
              <template #default="{ row }">
                <div class="node-id">
                  <el-icon class="node-icon" :class="{ healthy: row.healthy }">
                    <Monitor />
                  </el-icon>
                  <span>{{ row.id }}</span>
                </div>
              </template>
            </el-table-column>
            
            <el-table-column prop="host" label="主机地址" min-width="140">
              <template #default="{ row }">
                <span>{{ row.host }}:{{ row.port }}</span>
              </template>
            </el-table-column>
            
            <el-table-column prop="healthy" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.healthy ? 'success' : 'danger'">
                  {{ row.healthy ? '健康' : '离线' }}
                </el-tag>
              </template>
            </el-table-column>
            
            <el-table-column prop="connections" label="活跃任务" width="100">
              <template #default="{ row }">
                <span>{{ row.connections || 0 }}/{{ row.weight || 1 }}</span>
              </template>
            </el-table-column>
            
            <el-table-column prop="cpu_percent" label="CPU使用率" width="120">
              <template #default="{ row }">
                <el-progress 
                  :percentage="Math.round(row.cpu_percent || 0)" 
                  :show-text="false"
                  :stroke-width="8"
                  :color="getProgressColor(row.cpu_percent || 0)"
                />
                <span class="usage-text">{{ Math.round(row.cpu_percent || 0) }}%</span>
              </template>
            </el-table-column>
            
            <el-table-column prop="memory_percent" label="内存使用率" width="120">
              <template #default="{ row }">
                <el-progress 
                  :percentage="Math.round(row.memory_percent || 0)" 
                  :show-text="false"
                  :stroke-width="8"
                  :color="getProgressColor(row.memory_percent || 0)"
                />
                <span class="usage-text">{{ Math.round(row.memory_percent || 0) }}%</span>
              </template>
            </el-table-column>
            
            <el-table-column prop="last_check" label="响应时间" width="120">
              <template #default="{ row }">
                <span v-if="row.last_check">{{ formatTime(row.last_check) }}</span>
                <span v-else>--</span>
              </template>
            </el-table-column>
            
            <el-table-column label="操作" width="200" fixed="right">
              <template #default="{ row }">
                <el-button size="small" @click="handleCheckNode(row.id)">
                  检查
                </el-button>
                <el-button 
                  size="small" 
                  type="primary" 
                  @click="handleViewDetails(row)"
                >
                  详情
                </el-button>
                <el-button 
                  size="small" 
                  type="danger" 
                  v-if="row.healthy"
                  @click="handleDisableNode(row.id)"
                >
                  禁用
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>
    </div>

    <!-- 节点详情对话框 -->
    <NodeDetailsDialog
      v-model="showDetailsDialog"
      :node="selectedNode"
      @refresh="handleRefreshNode"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import AppLayout from '@/layouts/AppLayout.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  RefreshRight,
  CircleCheckFilled,
  CircleCloseFilled,
  DataLine,
  Monitor
} from '@element-plus/icons-vue'
import { useGatewayStore } from '@/stores/gateway'
import NodeDetailsDialog from '@/components/nodes/NodeDetailsDialog.vue'
import type { NodeInfo } from '@/types/gateway'

const gatewayStore = useGatewayStore()

const checking = ref(false)
const showDetailsDialog = ref(false)
const selectedNode = ref<NodeInfo | null>(null)

const healthyNodes = computed(() => gatewayStore.healthyNodeCount)
const offlineNodes = computed(() => gatewayStore.nodes.length - gatewayStore.healthyNodeCount)
const totalTasks = computed(() => 
  gatewayStore.nodes.reduce((sum, node) => sum + (node.activeTasks || 0), 0)
)

const getProgressColor = (value: number) => {
  if (value < 50) return '#67c23a'
  if (value < 80) return '#e6a23c'
  return '#f56c6c'
}

const formatTime = (isoString: string) => {
  if (!isoString) return '--'
  const date = new Date(isoString)
  const now = new Date()
  const diff = Math.floor((now.getTime() - date.getTime()) / 1000)
  
  if (diff < 60) return `${diff}秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  return date.toLocaleDateString()
}

const handleCheckAllNodes = async () => {
  checking.value = true
  try {
    await gatewayStore.fetchNodeStatus()
    ElMessage.success('节点状态检查完成')
  } catch (error) {
    ElMessage.error('检查节点失败')
  } finally {
    checking.value = false
  }
}

const handleCheckNode = async (nodeId: string) => {
  try {
    ElMessage.success(`正在检查节点 ${nodeId}`)
  } catch (error) {
    ElMessage.error('检查节点失败')
  }
}

const handleViewDetails = (node: NodeInfo) => {
  selectedNode.value = node
  showDetailsDialog.value = true
}

const handleRefreshNode = async (nodeId: string) => {
  try {
    await gatewayStore.fetchNodeStatus()
    ElMessage.success('节点状态已刷新')
  } catch (error) {
    ElMessage.error('刷新节点状态失败')
  }
}

const handleDisableNode = async (nodeId: string) => {
  try {
    await ElMessageBox.confirm(`确定要禁用节点 ${nodeId} 吗？`, '确认禁用', {
      type: 'warning'
    })
    // TODO: 实现禁用节点逻辑
    ElMessage.success(`节点 ${nodeId} 已禁用`)
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('禁用节点失败')
    }
  }
}
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
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

/* 统计卡片 - 参考设计样式 */
.stat-card {
  background: #fff;
  border-radius: 8px;
  padding: 20px;
  border: 1px solid #ebeef5;
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-card.success {
  background: linear-gradient(135deg, #e6fffb 0%, #f0fff9 100%);
  border-color: #b7eb8f;
}

.stat-card.danger {
  background: linear-gradient(135deg, #fff1f0 0%, #fff7f6 100%);
  border-color: #ffd6d6;
}

.stat-card.info {
  background: linear-gradient(135deg, #e6f4ff 0%, #f0f9ff 100%);
  border-color: #d4e8ff;
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
}

.stat-icon.success { background: #67c23a; }
.stat-icon.danger { background: #f56c6c; }
.stat-icon.info { background: #409eff; }

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
  color: #606266;
  margin-top: 4px;
}

/* 节点列表卡片 */
.node-list-card {
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

.node-id {
  display: flex;
  align-items: center;
  gap: 8px;
}

.node-icon {
  color: #c0c4cc;
}

.node-icon.healthy {
  color: #67c23a;
}

.usage-text {
  font-size: 12px;
  color: #909399;
  margin-left: 8px;
}

.nodes-table :deep(.el-progress-bar__outer) {
  background-color: #ebeef5;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }
}
</style>
