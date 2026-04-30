// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="analysis-config">
    <el-form :model="config" label-width="120px" label-position="top">
      <el-form-item label="分析模式">
        <el-radio-group v-model="config.mode">
          <el-radio-button label="full">完整分析</el-radio-button>
          <el-radio-button label="quick">快速分析</el-radio-button>
          <el-radio-button label="custom">自定义</el-radio-button>
        </el-radio-group>
      </el-form-item>
      
      <el-form-item label="分析提示词">
        <el-input
          v-model="config.prompt"
          type="textarea"
          :rows="3"
          placeholder="请输入分析提示词，如：描述视频中的主要内容和场景"
        />
      </el-form-item>
      
      <el-collapse v-model="activeCollapse">
        <el-collapse-item title="高级设置" name="advanced">
          <el-form-item label="采样间隔 (秒)">
            <el-slider
              v-model="config.sampleInterval"
              :min="0.5"
              :max="10"
              :step="0.5"
              show-input
            />
          </el-form-item>
          
          <el-form-item label="最大帧数">
            <el-input-number
              v-model="config.maxFrames"
              :min="1"
              :max="1000"
              :step="10"
            />
          </el-form-item>
          
          <el-form-item label="置信度阈值">
            <el-slider
              v-model="config.confidenceThreshold"
              :min="0"
              :max="1"
              :step="0.05"
              :format-tooltip="(val: number) => val.toFixed(2)"
              show-input
            />
          </el-form-item>
          
          <el-form-item label="启用场景检测">
            <el-switch v-model="config.enableSceneDetection" />
          </el-form-item>
          
          <el-form-item label="启用OCR">
            <el-switch v-model="config.enableOCR" />
          </el-form-item>
        </el-collapse-item>
      </el-collapse>
    </el-form>
    
    <div class="config-actions">
      <el-button @click="resetConfig">重置</el-button>
      <el-button type="primary" @click="applyConfig">应用配置</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue'

interface AnalysisConfigData {
  mode: 'full' | 'quick' | 'custom'
  prompt: string
  sampleInterval: number
  maxFrames: number
  confidenceThreshold: number
  enableSceneDetection: boolean
  enableOCR: boolean
}

const props = withDefaults(defineProps<{
  modelValue?: Partial<AnalysisConfigData>
}>(), {
  modelValue: () => ({})
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: AnalysisConfigData): void
  (e: 'apply', value: AnalysisConfigData): void
}>()

const defaultConfig: AnalysisConfigData = {
  mode: 'full',
  prompt: '',
  sampleInterval: 2,
  maxFrames: 100,
  confidenceThreshold: 0.5,
  enableSceneDetection: true,
  enableOCR: false
}

const config = reactive<AnalysisConfigData>({ ...defaultConfig, ...props.modelValue })
const activeCollapse = ref<string[]>([])

watch(() => props.modelValue, (newVal) => {
  Object.assign(config, { ...defaultConfig, ...newVal })
}, { deep: true })

const resetConfig = () => {
  Object.assign(config, defaultConfig)
  emit('update:modelValue', { ...config })
}

const applyConfig = () => {
  emit('update:modelValue', { ...config })
  emit('apply', { ...config })
}
</script>

<style scoped>
.analysis-config {
  padding: 15px;
}

.config-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
  padding-top: 15px;
  border-top: 1px solid #ebeef5;
}

:deep(.el-collapse-item__header) {
  font-weight: 500;
}

:deep(.el-form-item) {
  margin-bottom: 18px;
}
</style>
