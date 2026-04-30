// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <el-dialog
    v-model="visible"
    title="筛选分析结果"
    width="500px"
    @close="handleClose"
  >
    <el-form :model="filters" label-width="100px">
      <el-form-item label="时间范围">
        <el-slider
          v-model="filters.timeRange"
          range
          :min="0"
          :max="maxTime"
          :format-tooltip="formatTime"
        />
      </el-form-item>
      
      <el-form-item label="置信度">
        <el-slider
          v-model="filters.confidenceRange"
          range
          :min="0"
          :max="1"
          :step="0.05"
          :format-tooltip="(val: number) => val.toFixed(2)"
        />
      </el-form-item>
      
      <el-form-item label="关键词">
        <el-input
          v-model="filters.keyword"
          placeholder="输入关键词过滤结果"
          clearable
        />
      </el-form-item>
      
      <el-form-item label="结果类型">
        <el-checkbox-group v-model="filters.types">
          <el-checkbox label="scene">场景</el-checkbox>
          <el-checkbox label="object">物体</el-checkbox>
          <el-checkbox label="text">文字</el-checkbox>
          <el-checkbox label="action">动作</el-checkbox>
        </el-checkbox-group>
      </el-form-item>
      
      <el-form-item label="排序方式">
        <el-select v-model="filters.sortBy" style="width: 100%">
          <el-option label="时间顺序" value="time" />
          <el-option label="置信度降序" value="confidence_desc" />
          <el-option label="置信度升序" value="confidence_asc" />
        </el-select>
      </el-form-item>
    </el-form>
    
    <template #footer>
      <el-button @click="resetFilters">重置</el-button>
      <el-button @click="handleClose">取消</el-button>
      <el-button type="primary" @click="applyFilters">应用筛选</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, computed, watch } from 'vue'

interface FilterData {
  timeRange: [number, number]
  confidenceRange: [number, number]
  keyword: string
  types: string[]
  sortBy: 'time' | 'confidence_desc' | 'confidence_asc'
}

const props = withDefaults(defineProps<{
  modelValue: boolean
  maxTime?: number
  initialFilters?: Partial<FilterData>
}>(), {
  maxTime: 3600,
  initialFilters: () => ({})
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'apply', filters: FilterData): void
}>()

const defaultFilters: FilterData = {
  timeRange: [0, props.maxTime],
  confidenceRange: [0, 1],
  keyword: '',
  types: ['scene', 'object', 'text', 'action'],
  sortBy: 'time'
}

const filters = reactive<FilterData>({
  ...defaultFilters,
  ...props.initialFilters
})

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

watch(() => props.maxTime, (newMax) => {
  if (filters.timeRange[1] > newMax) {
    filters.timeRange = [filters.timeRange[0], newMax]
  }
})

const handleClose = () => {
  emit('update:modelValue', false)
}

const resetFilters = () => {
  Object.assign(filters, {
    ...defaultFilters,
    timeRange: [0, props.maxTime]
  })
}

const applyFilters = () => {
  emit('apply', { ...filters })
  handleClose()
}

const formatTime = (seconds: number) => {
  const mins = Math.floor(seconds / 60)
  const secs = Math.floor(seconds % 60)
  return `${mins}:${secs.toString().padStart(2, '0')}`
}
</script>

<style scoped>
:deep(.el-form-item) {
  margin-bottom: 20px;
}

:deep(.el-checkbox-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
</style>
