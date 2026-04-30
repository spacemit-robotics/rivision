<template>
  <div class="analysis-results">
    <div class="results-header">
      <h4>分析结果 ({{ results.length }} 条)</h4>
      <div class="header-actions">
        <el-button size="small" @click="$emit('filter')">
          <el-icon><Filter /></el-icon> 筛选
        </el-button>
        <el-button size="small" @click="$emit('export')">
          <el-icon><Download /></el-icon> 导出
        </el-button>
      </div>
    </div>
    
    <div v-if="results.length === 0" class="empty-state">
      <el-empty description="暂无分析结果" />
    </div>
    
    <div v-else class="results-list">
      <div
        v-for="(result, index) in results"
        :key="index"
        class="result-item"
        :class="{ selected: selectedIndex === index }"
        @click="selectResult(index)"
      >
        <div class="result-thumbnail" v-if="result.thumbnail">
          <img :src="result.thumbnail" :alt="'Frame at ' + result.timestamp" />
        </div>
        <div class="result-content">
          <div class="result-header">
            <span class="timestamp" @click.stop="$emit('seek', result.timestamp)">
              {{ formatTime(result.timestamp) }}
            </span>
            <el-tag size="small" :type="getTypeColor(result.type)">
              {{ result.type }}
            </el-tag>
            <span class="confidence">
              置信度: {{ (result.confidence * 100).toFixed(1) }}%
            </span>
          </div>
          <p class="result-description">{{ result.description }}</p>
          <div v-if="result.tags && result.tags.length" class="result-tags">
            <el-tag
              v-for="tag in result.tags"
              :key="tag"
              size="small"
              type="info"
            >
              {{ tag }}
            </el-tag>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Filter, Download } from '@element-plus/icons-vue'

interface AnalysisResult {
  timestamp: number
  type: string
  description: string
  confidence: number
  thumbnail?: string
  tags?: string[]
}

defineProps<{
  results: AnalysisResult[]
}>()

const emit = defineEmits<{
  (e: 'select', index: number): void
  (e: 'seek', timestamp: number): void
  (e: 'filter'): void
  (e: 'export'): void
}>()

const selectedIndex = ref(-1)

const selectResult = (index: number) => {
  selectedIndex.value = index
  emit('select', index)
}

const getTypeColor = (type: string): 'success' | 'warning' | 'info' | 'danger' | 'primary' => {
  const colors: Record<string, 'success' | 'warning' | 'info' | 'danger' | 'primary'> = {
    scene: 'primary',
    object: 'success',
    text: 'warning',
    action: 'danger'
  }
  return colors[type] || 'info'
}

const formatTime = (seconds: number) => {
  const mins = Math.floor(seconds / 60)
  const secs = Math.floor(seconds % 60)
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
}
</script>

<style scoped>
.analysis-results {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.results-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 15px;
  border-bottom: 1px solid #ebeef5;
}

.results-header h4 {
  margin: 0;
  color: #303133;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.empty-state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.results-list {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
}

.result-item {
  display: flex;
  gap: 12px;
  padding: 12px;
  margin-bottom: 8px;
  background: #fafafa;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
}

.result-item:hover {
  background: #f0f2f5;
}

.result-item.selected {
  background: #ecf5ff;
  border: 1px solid #409eff;
}

.result-thumbnail {
  width: 80px;
  height: 60px;
  border-radius: 4px;
  overflow: hidden;
  flex-shrink: 0;
}

.result-thumbnail img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.result-content {
  flex: 1;
  min-width: 0;
}

.result-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.timestamp {
  color: #409eff;
  font-weight: 500;
  cursor: pointer;
}

.timestamp:hover {
  text-decoration: underline;
}

.confidence {
  font-size: 12px;
  color: #909399;
  margin-left: auto;
}

.result-description {
  margin: 0;
  font-size: 13px;
  color: #606266;
  line-height: 1.5;
}

.result-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 8px;
}
</style>
