// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <el-dialog
    v-model="visible"
    title="导出分析结果"
    width="450px"
    @close="handleClose"
  >
    <el-form :model="exportConfig" label-width="100px">
      <el-form-item label="导出格式">
        <el-radio-group v-model="exportConfig.format">
          <el-radio-button label="json">JSON</el-radio-button>
          <el-radio-button label="csv">CSV</el-radio-button>
          <el-radio-button label="xlsx">Excel</el-radio-button>
          <el-radio-button label="pdf">PDF</el-radio-button>
        </el-radio-group>
      </el-form-item>
      
      <el-form-item label="导出内容">
        <el-checkbox-group v-model="exportConfig.fields">
          <el-checkbox label="timestamp">时间戳</el-checkbox>
          <el-checkbox label="description">描述</el-checkbox>
          <el-checkbox label="confidence">置信度</el-checkbox>
          <el-checkbox label="type">类型</el-checkbox>
          <el-checkbox label="tags">标签</el-checkbox>
          <el-checkbox label="thumbnail">缩略图</el-checkbox>
        </el-checkbox-group>
      </el-form-item>
      
      <el-form-item label="时间范围">
        <el-radio-group v-model="exportConfig.range">
          <el-radio label="all">全部结果</el-radio>
          <el-radio label="filtered">筛选后结果</el-radio>
          <el-radio label="selected">选中结果</el-radio>
        </el-radio-group>
      </el-form-item>
      
      <el-form-item label="文件名">
        <el-input
          v-model="exportConfig.filename"
          placeholder="输入文件名（不含扩展名）"
        >
          <template #append>.{{ exportConfig.format }}</template>
        </el-input>
      </el-form-item>
    </el-form>
    
    <template #footer>
      <el-button @click="handleClose">取消</el-button>
      <el-button type="primary" :loading="exporting" @click="handleExport">
        {{ exporting ? '导出中...' : '导出' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, computed } from 'vue'
import { ElMessage } from 'element-plus'

interface ExportConfig {
  format: 'json' | 'csv' | 'xlsx' | 'pdf'
  fields: string[]
  range: 'all' | 'filtered' | 'selected'
  filename: string
}

const props = defineProps<{
  modelValue: boolean
  resultCount?: number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'export', config: ExportConfig): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const exporting = ref(false)

const exportConfig = reactive<ExportConfig>({
  format: 'json',
  fields: ['timestamp', 'description', 'confidence', 'type'],
  range: 'all',
  filename: `analysis_results_${Date.now()}`
})

const handleClose = () => {
  emit('update:modelValue', false)
}

const handleExport = async () => {
  if (exportConfig.fields.length === 0) {
    ElMessage.warning('请至少选择一个导出字段')
    return
  }
  
  if (!exportConfig.filename.trim()) {
    ElMessage.warning('请输入文件名')
    return
  }
  
  exporting.value = true
  try {
    emit('export', { ...exportConfig })
    ElMessage.success('导出成功')
    handleClose()
  } catch (error) {
    ElMessage.error('导出失败')
  } finally {
    exporting.value = false
  }
}
</script>

<style scoped>
:deep(.el-checkbox-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

:deep(.el-form-item) {
  margin-bottom: 18px;
}
</style>
