<template>
  <AppLayout>
    <!-- 页面标题 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">视频分析</h1>
        <p class="page-description">上传视频进行AI内容搜索、摘要生成和目标检测</p>
      </div>
      <div class="header-right">
        <el-button v-if="analysisResults.length > 0" type="success" @click="handleExportResults">
          <el-icon><Download /></el-icon>
          导出结果
        </el-button>
      </div>
    </div>

    <div class="analysis-container">
      <el-row :gutter="24">
        <!-- 左侧：视频上传和配置 -->
        <el-col :xs="24" :lg="8">
          <div class="upload-section">
            <!-- 视频上传区域 -->
            <div class="content-card">
              <div class="card-header">
                <div class="card-title">
                  <el-icon><Upload /></el-icon>
                  <span>上传视频</span>
                </div>
              </div>
              <div class="card-content">
                <VideoUploader 
                  @upload-success="handleUploadSuccess"
                  @upload-progress="handleUploadProgress"
                  @upload-error="handleUploadError"
                />
              </div>
            </div>

            <!-- 分析配置 -->
            <div class="content-card">
              <div class="card-header">
                <div class="card-title">
                  <el-icon><Setting /></el-icon>
                  <span>分析配置</span>
                </div>
              </div>
              <div class="card-content">
                <AnalysisConfig 
                  v-model="analysisConfig"
                  :disabled="isAnalyzing"
                  @config-change="handleConfigChange"
                />
              </div>
            </div>

            <!-- 开始分析按钮 -->
            <div class="action-section">
              <el-button 
                type="primary" 
                size="large"
                :loading="isAnalyzing"
                :disabled="!uploadedVideo || !gatewayStore.isConnected"
                @click="handleStartAnalysis"
                block
              >
                <el-icon v-if="!isAnalyzing"><VideoPlay /></el-icon>
                {{ isAnalyzing ? '分析中...' : '开始分析' }}
              </el-button>
              
              <div class="analysis-info" v-if="isAnalyzing">
                <div class="info-item">
                  <span class="label">任务ID:</span>
                  <span class="value">{{ currentTaskId }}</span>
                </div>
                <div class="info-item">
                  <span class="label">进度:</span>
                  <span class="value">{{ analysisProgress }}%</span>
                </div>
                <div class="info-item">
                  <span class="label">预计剩余:</span>
                  <span class="value">{{ estimatedTimeRemaining }}</span>
                </div>
              </div>
            </div>
          </div>
        </el-col>

        <!-- 右侧：分析结果 -->
        <el-col :xs="24" :lg="16">
          <div class="results-section">
            <!-- 视频预览 -->
            <div class="content-card" v-if="uploadedVideo">
              <div class="card-header">
                <div class="card-title">
                  <el-icon><VideoCamera /></el-icon>
                  <span>视频预览</span>
                </div>
                <div class="card-actions">
                  <el-button-group size="small">
                    <el-button @click="handleVideoPlay">
                      <el-icon><VideoPlay /></el-icon>
                      播放
                    </el-button>
                    <el-button @click="handleVideoPause">
                      <el-icon><VideoPause /></el-icon>
                      暂停
                    </el-button>
                  </el-button-group>
                </div>
              </div>
              <div class="card-content">
                <VideoPlayer 
                  :src="uploadedVideo.url"
                  :poster="uploadedVideo.poster"
                  @timeupdate="handleVideoTimeUpdate"
                  @loadedmetadata="handleVideoLoadedMetadata"
                  ref="videoPlayerRef"
                />
              </div>
            </div>

            <!-- 分析进度 -->
            <div class="content-card" v-if="isAnalyzing">
              <div class="card-header">
                <div class="card-title">
                  <el-icon><Loading /></el-icon>
                  <span>分析进度</span>
                </div>
                <div class="card-actions">
                  <el-button 
                    type="danger" 
                    size="small"
                    @click="handleCancelAnalysis"
                  >
                    取消分析
                  </el-button>
                </div>
              </div>
              <div class="card-content">
                <AnalysisProgress 
                  :progress="analysisProgress"
                  :status="analysisStatus"
                  :current-frame="currentFrame"
                  :total-frames="totalFrames"
                  :message="progressMessage"
                />
              </div>
            </div>

            <!-- 分析结果 -->
            <div class="content-card" v-if="analysisResults.length > 0">
              <div class="card-header">
                <div class="card-title">
                  <el-icon><DataAnalysis /></el-icon>
                  <span>分析结果</span>
                  <el-badge :value="analysisResults.length" class="result-badge" />
                </div>
                <div class="card-actions">
                  <el-select 
                    v-model="resultDisplayMode" 
                    size="small"
                    @change="handleDisplayModeChange"
                  >
                    <el-option label="时间轴视图" value="timeline" />
                    <el-option label="列表视图" value="list" />
                    <el-option label="缩略图视图" value="thumbnail" />
                  </el-select>
                  <el-button 
                    size="small"
                    @click="handleFilterResults"
                  >
                    <el-icon><Filter /></el-icon>
                    筛选
                  </el-button>
                </div>
              </div>
              <div class="card-content">
                <AnalysisResults 
                  :results="analysisResults"
                  :display-mode="resultDisplayMode"
                  :video-duration="videoDuration"
                  @result-click="handleResultClick"
                  @result-export="handleResultExport"
                />
              </div>
            </div>

            <!-- 空状态 -->
            <div class="empty-state" v-if="!uploadedVideo && !isAnalyzing && analysisResults.length === 0">
              <div class="empty-content">
                <el-icon class="empty-icon"><VideoCamera /></el-icon>
                <h3>开始视频分析</h3>
                <p>请先上传视频文件，然后配置分析参数开始AI分析</p>
                <div class="empty-tips">
                  <div class="tip-item">
                    <el-icon><Check /></el-icon>
                    <span>支持 MP4、AVI、MOV 等常见格式</span>
                  </div>
                  <div class="tip-item">
                    <el-icon><Check /></el-icon>
                    <span>提供内容搜索、摘要生成、目标检测</span>
                  </div>
                  <div class="tip-item">
                    <el-icon><Check /></el-icon>
                    <span>实时显示分析进度和结果</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </el-col>
      </el-row>
    </div>

    <!-- 结果筛选对话框 -->
    <ResultFilterDialog 
      v-model="showFilterDialog"
      :filters="resultFilters"
      @apply-filters="handleApplyFilters"
      @reset-filters="handleResetFilters"
    />

    <!-- 导出结果对话框 -->
    <ExportResultDialog 
      v-model="showExportDialog"
      :results="analysisResults"
      :format-options="exportFormatOptions"
      @export-confirm="handleExportConfirm"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue'
import AppLayout from '@/layouts/AppLayout.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Download,
  Upload,
  Setting,
  VideoPlay,
  VideoPause,
  VideoCamera,
  Loading,
  DataAnalysis,
  Filter,
  Check
} from '@element-plus/icons-vue'
import { useGatewayStore } from '@/stores/gateway'
import VideoUploader from '@/components/video/VideoUploader.vue'
import VideoPlayer from '@/components/video/VideoPlayer.vue'
import AnalysisConfig from '@/components/video/AnalysisConfig.vue'
import AnalysisProgress from '@/components/video/AnalysisProgress.vue'
import AnalysisResults from '@/components/video/AnalysisResults.vue'
import ResultFilterDialog from '@/components/video/ResultFilterDialog.vue'
import ExportResultDialog from '@/components/video/ExportResultDialog.vue'
import type { AnalysisConfig as AnalysisConfigType, AnalysisResult } from '@/types/gateway'

// 存储
const gatewayStore = useGatewayStore()

// 响应式状态
const uploadedVideo = ref<{
  id: string
  name: string
  url: string
  poster: string
  size: number
  duration: number
} | null>(null)

const analysisConfig = reactive<AnalysisConfigType>({
  type: 'search',
  query: '',
  confidenceThreshold: 0.7,
  frameInterval: 1.0,
  maxResults: 50
})

const isAnalyzing = ref(false)
const currentTaskId = ref('')
const analysisProgress = ref(0)
const analysisStatus = ref('准备中')
const currentFrame = ref(0)
const totalFrames = ref(0)
const progressMessage = ref('')
const videoDuration = ref(0)

const analysisResults = ref<AnalysisResult[]>([])
const resultDisplayMode = ref('timeline')
const resultFilters = reactive({
  minConfidence: 0.5,
  timeRange: [0, 100],
  contentTypes: []
})

const showFilterDialog = ref(false)
const showExportDialog = ref(false)
const exportFormatOptions = ref([
  { label: 'JSON格式', value: 'json' },
  { label: 'CSV格式', value: 'csv' },
  { label: '文本格式', value: 'txt' }
])

// 引用
const videoPlayerRef = ref()

// 轮询任务状态
const statusPolling = ref<NodeJS.Timeout>()

// 计算属性
const estimatedTimeRemaining = computed(() => {
  if (!isAnalyzing.value || analysisProgress.value === 0) return '--'
  
  const elapsed = Date.now() - analysisStartTime.value
  const remaining = (elapsed / analysisProgress.value) * (100 - analysisProgress.value)
  
  if (remaining < 60000) return `${Math.round(remaining / 1000)}秒`
  if (remaining < 3600000) return `${Math.round(remaining / 60000)}分钟`
  return `${Math.round(remaining / 3600000)}小时`
})

// 分析开始时间
const analysisStartTime = ref(0)

// 事件处理函数
const handleUploadSuccess = (video: any) => {
  uploadedVideo.value = video
  ElMessage.success(`视频上传成功: ${video.name}`)
  
  // 重置之前的结果
  analysisResults.value = []
  analysisProgress.value = 0
}

const handleUploadProgress = (progress: number) => {
  console.log('上传进度:', progress)
}

const handleUploadError = (error: string) => {
  ElMessage.error(`上传失败: ${error}`)
}

const handleConfigChange = (config: AnalysisConfigType) => {
  console.log('配置改变:', config)
}

const handleStartAnalysis = async () => {
  if (!uploadedVideo.value) {
    ElMessage.warning('请先上传视频')
    return
  }

  if (!gatewayStore.isConnected) {
    ElMessage.error('Gateway未连接，请检查连接状态')
    return
  }

  // 验证配置
  if (analysisConfig.type === 'search' && !analysisConfig.query?.trim()) {
    ElMessage.warning('请输入搜索关键词')
    return
  }

  try {
    isAnalyzing.value = true
    analysisStartTime.value = Date.now()
    analysisProgress.value = 0
    analysisStatus.value = '提交任务中...'
    progressMessage.value = '正在向Gateway提交分析任务'

    // 提交分析任务
    const taskData = {
      videoId: uploadedVideo.value.id,
      config: analysisConfig
    }

    const taskId = await gatewayStore.submitAnalysisTask(taskData)
    currentTaskId.value = taskId
    
    ElMessage.success('分析任务已提交')
    
    // 开始轮询任务状态
    startStatusPolling(taskId)
    
  } catch (error) {
    ElMessage.error(`提交任务失败: ${error}`)
    isAnalyzing.value = false
  }
}

const startStatusPolling = (taskId: string) => {
  statusPolling.value = setInterval(async () => {
    try {
      const taskStatus = await gatewayStore.getTaskStatus(taskId)
      
      if (!taskStatus) {
        console.warn('获取任务状态失败')
        return
      }

      analysisProgress.value = taskStatus.progress
      analysisStatus.value = taskStatus.status
      currentFrame.value = taskStatus.metadata?.frameIndex || 0
      totalFrames.value = taskStatus.metadata?.frameCount || 0
      
      // 根据状态更新进度消息
      switch (taskStatus.status) {
        case 'queued':
          progressMessage.value = '任务排队中，等待处理...'
          break
        case 'running':
          progressMessage.value = `正在分析第 ${currentFrame.value}/${totalFrames.value} 帧`
          break
        case 'completed':
          progressMessage.value = '分析完成！'
          handleAnalysisComplete(taskStatus)
          break
        case 'failed':
          progressMessage.value = `分析失败: ${taskStatus.error || '未知错误'}`
          handleAnalysisError(taskStatus.error || '分析失败')
          break
      }
      
    } catch (error) {
      console.error('轮询任务状态错误:', error)
    }
  }, 2000) // 每2秒轮询一次
}

const handleAnalysisComplete = async (taskStatus: any) => {
  stopStatusPolling()
  isAnalyzing.value = false
  
  try {
    // 获取分析结果
    if (taskStatus.result && taskStatus.result.results) {
      analysisResults.value = taskStatus.result.results
      ElMessage.success(`分析完成！找到 ${analysisResults.value.length} 个匹配结果`)
    } else {
      ElMessage.info('分析完成，但未找到匹配结果')
    }
  } catch (error) {
    console.error('处理分析结果错误:', error)
    ElMessage.error('获取分析结果失败')
  }
}

const handleAnalysisError = (error: string) => {
  stopStatusPolling()
  isAnalyzing.value = false
  ElMessage.error(`分析失败: ${error}`)
}

const handleCancelAnalysis = async () => {
  if (!currentTaskId.value) return

  try {
    await ElMessageBox.confirm('确定要取消当前分析任务吗？', '确认取消', {
      type: 'warning'
    })

    await gatewayStore.cancelTask(currentTaskId.value, '用户手动取消')
    
    stopStatusPolling()
    isAnalyzing.value = false
    analysisProgress.value = 0
    currentTaskId.value = ''
    
    ElMessage.success('分析任务已取消')
    
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('取消任务失败')
    }
  }
}

const stopStatusPolling = () => {
  if (statusPolling.value) {
    clearInterval(statusPolling.value)
    statusPolling.value = undefined
  }
}

const handleVideoPlay = () => {
  videoPlayerRef.value?.play()
}

const handleVideoPause = () => {
  videoPlayerRef.value?.pause()
}

const handleVideoTimeUpdate = (currentTime: number) => {
  console.log('视频时间更新:', currentTime)
}

const handleVideoLoadedMetadata = (metadata: any) => {
  videoDuration.value = metadata.duration
  totalFrames.value = Math.ceil(metadata.duration * 30) // 假设30fps
}

const handleResultClick = (result: AnalysisResult) => {
  console.log('点击结果:', result)
  
  // 跳转到视频对应时间点
  if (videoPlayerRef.value && result.timestamp) {
    videoPlayerRef.value.currentTime = result.timestamp
  }
}

const handleResultExport = (result: AnalysisResult) => {
  console.log('导出单个结果:', result)
}

const handleDisplayModeChange = (mode: string) => {
  console.log('显示模式改变:', mode)
}

const handleFilterResults = () => {
  showFilterDialog.value = true
}

const handleApplyFilters = (filters: any) => {
  console.log('应用筛选器:', filters)
  showFilterDialog.value = false
}

const handleResetFilters = () => {
  console.log('重置筛选器')
}

const handleExportResults = () => {
  showExportDialog.value = true
}

const handleExportConfirm = (exportConfig: any) => {
  console.log('确认导出:', exportConfig)
  showExportDialog.value = false
  ElMessage.success('结果导出成功')
}

// 生命周期
onMounted(() => {
  // 初始化检查Gateway连接状态
  if (!gatewayStore.isConnected) {
    ElMessage.warning('Gateway未连接，请先连接到推理网关')
  }
})

onUnmounted(() => {
  stopStatusPolling()
})
</script>

<style scoped>
.video-analysis {
  min-height: 100vh;
  background-color: var(--bg-page);
}

.page-header {
  background: var(--bg-color);
  border-bottom: 1px solid var(--border-lighter);
  padding: var(--spacing-lg);
  margin-bottom: var(--spacing-lg);
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--spacing-lg);
}

.page-title {
  font-size: 28px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 var(--spacing-sm);
}

.page-description {
  color: var(--text-secondary);
  font-size: var(--font-size-base);
  margin: 0;
}

.analysis-container {
  padding: 0 var(--spacing-lg) var(--spacing-lg);
}

.upload-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.results-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
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

.action-section {
  background: var(--bg-color);
  border-radius: var(--border-radius-large);
  padding: var(--spacing-lg);
  box-shadow: var(--box-shadow-base);
  border: 1px solid var(--border-lighter);
}

.analysis-info {
  margin-top: var(--spacing-md);
  padding: var(--spacing-md);
  background-color: var(--bg-page);
  border-radius: var(--border-radius-base);
  border: 1px solid var(--border-light);
}

.info-item {
  display: flex;
  justify-content: space-between;
  margin-bottom: var(--spacing-xs);
  font-size: var(--font-size-small);
}

.info-item:last-child {
  margin-bottom: 0;
}

.label {
  color: var(--text-secondary);
}

.value {
  color: var(--text-primary);
  font-weight: 500;
}

.result-badge {
  margin-left: var(--spacing-sm);
}

.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  background: var(--bg-color);
  border-radius: var(--border-radius-large);
  box-shadow: var(--box-shadow-base);
  border: 1px solid var(--border-lighter);
}

.empty-content {
  text-align: center;
  max-width: 400px;
  padding: var(--spacing-xl);
}

.empty-icon {
  font-size: 64px;
  color: var(--text-placeholder);
  margin-bottom: var(--spacing-lg);
}

.empty-content h3 {
  font-size: var(--font-size-large);
  color: var(--text-primary);
  margin-bottom: var(--spacing-md);
}

.empty-content p {
  color: var(--text-secondary);
  margin-bottom: var(--spacing-lg);
  line-height: 1.6;
}

.empty-tips {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  text-align: left;
}

.tip-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  color: var(--text-regular);
  font-size: var(--font-size-small);
}

.tip-item .el-icon {
  color: var(--success-color);
  font-size: 16px;
}

/* 响应式设计 */
@media (max-width: 1024px) {
  .analysis-container :deep(.el-col) {
    margin-bottom: var(--spacing-lg);
  }
}

@media (max-width: 768px) {
  .page-header {
    padding: var(--spacing-md);
  }
  
  .header-content {
    flex-direction: column;
    gap: var(--spacing-md);
  }
  
  .analysis-container {
    padding: 0 var(--spacing-md) var(--spacing-md);
  }
  
  .upload-section,
  .results-section {
    gap: var(--spacing-md);
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
