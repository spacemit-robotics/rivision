// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="video-uploader">
    <el-upload
      ref="uploadRef"
      class="upload-area"
      drag
      :action="uploadUrl"
      :headers="headers"
      :accept="acceptTypes"
      :before-upload="beforeUpload"
      :on-progress="onProgress"
      :on-success="onSuccess"
      :on-error="onError"
      :show-file-list="false"
      :disabled="uploading"
    >
      <div v-if="!uploading && !uploadedFile" class="upload-content">
        <el-icon class="upload-icon"><Upload /></el-icon>
        <div class="upload-text">
          <p>将视频文件拖拽到此处，或<em>点击上传</em></p>
          <p class="upload-tip">支持 MP4, AVI, MOV, MKV 格式，最大 {{ maxSizeMB }}MB</p>
        </div>
      </div>
      
      <div v-else-if="uploading" class="upload-progress">
        <el-progress type="circle" :percentage="uploadProgress" />
        <p>正在上传: {{ uploadingFileName }}</p>
      </div>
      
      <div v-else class="uploaded-info">
        <el-icon class="success-icon"><CircleCheck /></el-icon>
        <p>{{ uploadedFile?.name }}</p>
        <el-button type="primary" link @click.stop="clearFile">重新上传</el-button>
      </div>
    </el-upload>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Upload, CircleCheck } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import type { UploadFile, UploadRawFile } from 'element-plus'

interface UploadedFile {
  name: string
  url: string
  size: number
}

const props = withDefaults(defineProps<{
  uploadUrl?: string
  maxSizeMB?: number
  acceptTypes?: string
}>(), {
  uploadUrl: '/api/v1/video/upload',
  maxSizeMB: 500,
  acceptTypes: '.mp4,.avi,.mov,.mkv'
})

const emit = defineEmits<{
  (e: 'success', file: UploadedFile): void
  (e: 'error', error: Error): void
  (e: 'clear'): void
}>()

const uploadRef = ref()
const uploading = ref(false)
const uploadProgress = ref(0)
const uploadingFileName = ref('')
const uploadedFile = ref<UploadedFile | null>(null)

const headers = computed(() => ({
  Authorization: `Bearer ${localStorage.getItem('token') || ''}`
}))

const beforeUpload = (file: UploadRawFile) => {
  const maxSize = props.maxSizeMB * 1024 * 1024
  if (file.size > maxSize) {
    ElMessage.error(`文件大小不能超过 ${props.maxSizeMB}MB`)
    return false
  }
  
  uploading.value = true
  uploadProgress.value = 0
  uploadingFileName.value = file.name
  return true
}

const onProgress = (event: { percent: number }) => {
  uploadProgress.value = Math.round(event.percent)
}

const onSuccess = (response: any, file: UploadFile) => {
  uploading.value = false
  uploadedFile.value = {
    name: file.name,
    url: response.url || response.data?.url,
    size: file.size || 0
  }
  emit('success', uploadedFile.value)
  ElMessage.success('上传成功')
}

const onError = (error: Error) => {
  uploading.value = false
  emit('error', error)
  ElMessage.error('上传失败: ' + error.message)
}

const clearFile = () => {
  uploadedFile.value = null
  uploadProgress.value = 0
  emit('clear')
}

defineExpose({ clearFile })
</script>

<style scoped>
.video-uploader {
  width: 100%;
}

.upload-area {
  width: 100%;
}

:deep(.el-upload) {
  width: 100%;
}

:deep(.el-upload-dragger) {
  width: 100%;
  height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.upload-content,
.upload-progress,
.uploaded-info {
  text-align: center;
}

.upload-icon {
  font-size: 48px;
  color: #c0c4cc;
  margin-bottom: 10px;
}

.upload-text p {
  margin: 5px 0;
  color: #606266;
}

.upload-text em {
  color: #409eff;
  font-style: normal;
}

.upload-tip {
  font-size: 12px;
  color: #909399;
}

.upload-progress p {
  margin-top: 15px;
  color: #606266;
}

.success-icon {
  font-size: 48px;
  color: #67c23a;
  margin-bottom: 10px;
}

.uploaded-info p {
  margin: 10px 0;
  color: #606266;
}
</style>
