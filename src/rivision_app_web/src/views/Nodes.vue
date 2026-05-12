<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useNodesStore } from '../stores/nodes'

const nodesStore = useNodesStore()

// Hub 信息 (从配置或环境获取)
const hubInfo = ref({
  id: 'hub-main',
  ip: '192.168.1.102'
})

onMounted(() => {
  nodesStore.fetchNodes()
})

// 获取 YOLO 模型名
function getYoloModel(caps: Record<string, any> | null) {
  if (!caps) return '-'
  if (caps.yolo_model) {
    const path = caps.yolo_model as string
    return path.split('/').pop() || path
  }
  if (caps.models && Array.isArray(caps.models) && caps.models.length > 0) {
    return caps.models[0]
  }
  return '-'
}

// 获取 VLM 模型名 - 从 capabilities 或 vlm_model 字段读取实际模型名
function getVlmModel(node: any) {
  // 优先从 capabilities.vlm_model 获取
  if (node?.capabilities?.vlm_model) {
    return node.capabilities.vlm_model
  }
  // 兼容直接字段
  if (node?.vlm_model) {
    return node.vlm_model
  }
  return '-'
}

// 获取 EMBED 模型名 - 从 capabilities 或 embed_model 字段读取实际模型名
function getEmbedModel(node: any) {
  // 优先从 capabilities.embed_model 获取
  if (node?.capabilities?.embed_model) {
    return node.capabilities.embed_model
  }
  // 兼容直接字段
  if (node?.embed_model) {
    return node.embed_model
  }
  return '-'
}

// 获取 AI 算力使用率
function getAiUsage(node: any) {
  const platform = node?.capabilities?.platform || ''
  // NPU 使用率 (K3-RISC-V)
  if (node?.npu_percent !== undefined && node?.npu_percent !== null) {
    return `${node.npu_percent.toFixed(0)}%`
  }
  // GPU 使用率
  if (node?.gpu_percent !== undefined && node?.gpu_percent !== null) {
    return `${node.gpu_percent.toFixed(0)}%`
  }
  // x86 无 GPU 时显示 CPU
  if (platform === 'x86_64') {
    return `${node?.cpu_percent?.toFixed(0) || 0}%`
  }
  return '-'
}

function formatTime(time: string) {
  if (!time) return '-'
  const d = new Date(time)
  return `${d.getMonth()+1}/${d.getDate()} ${d.getHours()}:${String(d.getMinutes()).padStart(2,'0')}`
}

// 格式化运行时长
function formatUptime(seconds: number | undefined) {
  if (!seconds) return '-'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 24) {
    const d = Math.floor(h / 24)
    return `${d}天${h % 24}时`
  }
  return `${h}时${m}分`
}

/**
 * 节点类型定义
 * | 类型值   | 显示名   | 说明                           | 获取方式                        |
 * |----------|----------|--------------------------------|--------------------------------|
 * | worker   | 计算节点 | 运行 YOLO/VLM 推理             | Worker 注册时设置 type: "worker" |
 * | gateway  | 网关节点 | 流媒体转发 (RTSP/WebRTC)       | 网关服务注册时设置               |
 * | storage  | 存储节点 | 视频录像存储                    | 存储服务注册时设置               |
 * | hub      | 中心节点 | 管理调度                        | Hub 自身                        |
 */
function formatType(type: string) {
  const typeMap: Record<string, string> = {
    'worker': '计算节点',
    'gateway': '网关节点',
    'storage': '存储节点',
    'hub': '中心节点',
    '': '计算节点'
  }
  return typeMap[type] || type || '计算节点'
}
</script>

<template>
  <div class="nodes-page">
    <div class="page-header">
      <h2>节点管理</h2>
      <button class="btn btn-primary" @click="nodesStore.fetchNodes">
        🔄 刷新
      </button>
    </div>

    <div class="card">
      <div v-if="nodesStore.loading" class="loading">
        <div class="spinner"></div>
      </div>
      <div v-else class="table-wrapper">
        <table class="table">
          <thead>
            <tr>
              <th class="col-mgr-id">管理ID</th>
              <th class="col-mgr-ip">管理IP</th>
              <th class="col-node-id">节点ID</th>
              <th class="col-node-ip">节点IP</th>
              <th class="col-type">类型</th>
              <th class="col-ver">版本</th>
              <th class="col-status">状态</th>
              <th class="col-yolo-s">YOLO</th>
              <th class="col-yolo-m">YOLO模型</th>
              <th class="col-vlm-s">VLM</th>
              <th class="col-vlm-m">VLM模型</th>
              <th class="col-embed">EMBED</th>
              <th class="col-cam">关联</th>
              <th class="col-online">在线</th>
              <th class="col-cpu">CPU</th>
              <th class="col-mem">内存</th>
              <th class="col-ai">AI算力</th>
              <th class="col-uptime">运行时间</th>
              <th class="col-hb">心跳</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="node in nodesStore.nodes" :key="node.id">
              <td><code>{{ hubInfo.id }}</code></td>
              <td><code>{{ hubInfo.ip }}</code></td>
              <td><code>{{ node.id }}</code></td>
              <td><code>{{ node.host || '-' }}</code></td>
              <td>{{ formatType(node.type) }}</td>
              <td><code>{{ node.version || '-' }}</code></td>
              <td>
                <span :class="['badge', node.status === 'online' ? 'badge-on' : 'badge-off']">
                  {{ node.status === 'online' ? '在线' : '离线' }}
                </span>
              </td>
              <td>
                <span :class="['badge', node.yolo_enabled ? 'badge-on' : 'badge-off']">
                  {{ node.yolo_enabled ? '启用' : '禁用' }}
                </span>
              </td>
              <td class="model-cell">{{ getYoloModel(node.capabilities) }}</td>
              <td>
                <span :class="['badge', node.llama_enabled ? 'badge-on' : 'badge-off']">
                  {{ node.llama_enabled ? '启用' : '禁用' }}
                </span>
              </td>
              <td class="model-cell">{{ getVlmModel(node) }}</td>
              <td class="model-cell">{{ getEmbedModel(node) }}</td>
              <td class="center">{{ node.camera_count || 0 }}</td>
              <td class="center">{{ node.connections || 0 }}</td>
              <td class="center">{{ node.cpu_percent?.toFixed(0) || 0 }}%</td>
              <td class="center">{{ node.memory_percent?.toFixed(0) || 0 }}%</td>
              <td class="center">{{ getAiUsage(node) }}</td>
              <td>{{ formatUptime(node.uptime_seconds) || '-' }}</td>
              <td>{{ formatTime(node.last_heartbeat) }}</td>
            </tr>
            <tr v-if="nodesStore.nodes.length === 0">
              <td colspan="18" class="empty-cell">暂无节点数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.nodes-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.page-header h2 {
  font-size: 18px;
  color: #1a1a2e;
}

.table-wrapper {
  overflow-x: auto;
}

.table {
  font-size: 13px;
  width: 100%;
  min-width: 1500px;
  border-collapse: collapse;
  table-layout: fixed;
}

.table th {
  background: #f8fafc;
  font-weight: 600;
  color: #334155;
  padding: 10px 8px;
  text-align: left;
  white-space: nowrap;
  border-bottom: 2px solid #e2e8f0;
}

.table td {
  padding: 10px 8px;
  border-bottom: 1px solid #f1f5f9;
  vertical-align: middle;
}

/* 列宽定义 - 均匀分布 */
.col-mgr-id { width: 5%; }
.col-mgr-ip { width: 7%; }
.col-node-id { width: 6%; }
.col-node-ip { width: 7%; }
.col-type { width: 5%; }
.col-ver { width: 4%; }
.col-status { width: 4%; }
.col-yolo-s { width: 4%; }
.col-yolo-m { width: 7%; }
.col-vlm-s { width: 4%; }
.col-vlm-m { width: 8%; }
.col-embed { width: 8%; }
.col-cam { width: 3%; }
.col-online { width: 3%; }
.col-cpu { width: 4%; }
.col-mem { width: 4%; }
.col-ai { width: 4%; }
.col-uptime { width: 6%; }
.col-hb { width: 6%; }

.table tbody tr:hover {
  background: #f8fafc;
}

code {
  background: #f1f5f9;
  padding: 3px 6px;
  border-radius: 4px;
  font-size: 12px;
  font-family: 'Monaco', 'Consolas', monospace;
  color: #1e293b;
}

.model-cell {
  font-size: 12px;
  color: #334155;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.center {
  text-align: center;
}

.badge {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
}

.badge-on {
  background: #dcfce7;
  color: #166534;
}

.badge-off {
  background: #fee2e2;
  color: #dc2626;
}

.badge-neutral {
  background: #f3f4f6;
  color: #6b7280;
}

.empty-cell {
  text-align: center;
  padding: 40px !important;
  color: #9ca3af;
}
</style>
