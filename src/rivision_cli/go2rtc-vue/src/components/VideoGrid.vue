<template>
  <div class="video-grid" :class="`grid-${layout}`">
    <VideoCell
      v-for="(cameraId, index) in displayCameras"
      :key="`cell-${index}`"
      :ref="(el: any) => setCellRef(index, el)"
      :camera-id="cameraId"
      :camera-name="getCameraName(cameraId)"
      :slot-index="index"
      :is-vlm-focus="cameraId === vlmFocusId"
      :is-vlm-enabled="cameraId ? vlmEnabledCameras?.has(cameraId) : false"
      :is-analyzing="cameraId === vlmFocusId && isAnalyzing"
      :yolo-config="getYoloConfig(cameraId)"
      @select-slot="handleSelectSlot"
      @remove="handleRemove"
      @video-ready="(video: HTMLVideoElement) => handleVideoReady(index, video)"
      @video-error="(error: string) => handleVideoError(index, error)"
    />

    <!-- 摄像头选择对话框 -->
    <Teleport to="body">
      <div v-if="showCameraSelector" class="dialog-overlay" @click="cancelSelection">
        <div class="dialog" @click.stop>
          <div class="dialog-header">
            <div class="header-info">
              <h4>选择摄像头</h4>
              <p class="header-subtitle">为位置 {{ selectedSlotIndex + 1 }} 选择要显示的摄像头</p>
            </div>
            <button class="close-btn" @click="cancelSelection">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                <line x1="18" y1="6" x2="6" y2="18"/>
                <line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>
          <div class="camera-options">
            <div v-if="availableCameras.length === 0" class="no-cameras">
              <div class="no-cameras-icon">📹</div>
              <div class="no-cameras-text">
                <h5>暂无可用摄像头</h5>
                <p>所有摄像头都已添加到网格中</p>
              </div>
            </div>
            <button
              v-for="camera in availableCameras"
              :key="camera.id"
              class="camera-option"
              @click="selectCamera(camera.id)"
            >
              <div class="option-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                  <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
                  <line x1="8" y1="21" x2="16" y2="21"/>
                  <line x1="12" y1="17" x2="12" y2="21"/>
                </svg>
              </div>
              <div class="camera-info">
                <div class="camera-name">{{ camera.name }}</div>
                <div class="camera-status" :class="{ online: camera.isActive }">
                  <span class="status-dot"></span>
                  {{ camera.isActive ? '在线' : '离线' }}
                </div>
              </div>
              <div class="option-arrow">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                  <path d="m9 18 6-6-6-6"/>
                </svg>
              </div>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import VideoCell from './VideoCell.vue'
import type { YoloConfig } from './VideoCell.vue'

// ============ 类型定义 ============
export type GridLayout = '1x1' | '2x2' | '3x3' | '4x4'

export interface Camera {
  id: string
  name: string
  isActive?: boolean
}

// ============ Props ============
interface Props {
  layout?: GridLayout
  cameras: Camera[]
  selectedCameras: (string | null)[]
  vlmFocusId: string | null
  vlmEnabledCameras?: Set<string>
  isAnalyzing?: boolean
  yoloConfigs?: Record<string, YoloConfig>
}

const props = withDefaults(defineProps<Props>(), {
  layout: '2x2',
  isAnalyzing: false,
  yoloConfigs: () => ({}),
})

// ============ Emits ============
const emit = defineEmits<{
  (e: 'update:selectedCameras', cameras: (string | null)[]): void
  (e: 'video-ready', index: number, video: HTMLVideoElement): void
  (e: 'video-error', index: number, error: string): void
}>()

// ============ 网格尺寸配置 ============
const gridSizes: Record<GridLayout, number> = {
  '1x1': 1,
  '2x2': 4,
  '3x3': 9,
  '4x4': 16,
}

// ============ 状态 ============
const showCameraSelector = ref(false)
const selectedSlotIndex = ref(-1)
const cellRefs = ref<Record<number, InstanceType<typeof VideoCell> | null>>({})

// ============ 计算属性 ============
const displayCameras = computed((): (string | null)[] => {
  const size = gridSizes[props.layout]
  const result: (string | null)[] = new Array(size).fill(null)
  
  props.selectedCameras.forEach((cameraId: string | null, index: number) => {
    if (index < size) {
      result[index] = cameraId
    }
  })
  
  return result
})

const availableCameras = computed((): Camera[] => {
  return props.cameras.filter((camera: Camera) => 
    camera.isActive !== false && !displayCameras.value.includes(camera.id)
  )
})

// ============ 方法 ============
function getCameraName(cameraId: string | null): string {
  if (!cameraId) return ''
  const camera = props.cameras.find((c: Camera) => c.id === cameraId)
  return camera?.name ?? cameraId
}

function getYoloConfig(cameraId: string | null): YoloConfig | null {
  if (!cameraId) return null
  return props.yoloConfigs[cameraId] ?? null
}

function setCellRef(index: number, el: any) {
  cellRefs.value[index] = el
}

function handleSelectSlot(index: number) {
  selectedSlotIndex.value = index
  showCameraSelector.value = true
}

function selectCamera(cameraId: string) {
  const newSelection = [...displayCameras.value]
  newSelection[selectedSlotIndex.value] = cameraId
  emit('update:selectedCameras', newSelection)
  cancelSelection()
}

function handleRemove(index: number) {
  const newSelection = [...displayCameras.value]
  newSelection[index] = null
  emit('update:selectedCameras', newSelection)
}

function cancelSelection() {
  showCameraSelector.value = false
  selectedSlotIndex.value = -1
}

function handleVideoReady(index: number, video: HTMLVideoElement) {
  emit('video-ready', index, video)
}

function handleVideoError(index: number, error: string) {
  emit('video-error', index, error)
}

// ============ 布局变化处理 ============
watch(() => props.layout, (newLayout: GridLayout) => {
  const maxSize = gridSizes[newLayout]
  const currentSelection = props.selectedCameras
  
  if (currentSelection.length > maxSize) {
    // 裁剪
    emit('update:selectedCameras', currentSelection.slice(0, maxSize))
  } else if (currentSelection.length < maxSize) {
    // 扩展
    const expanded = [...currentSelection]
    while (expanded.length < maxSize) {
      expanded.push(null)
    }
    emit('update:selectedCameras', expanded)
  }
})

// ============ 暴露方法 ============
defineExpose({
  getCellRef: (index: number) => cellRefs.value[index],
  getVlmFocusVideoElement: () => {
    if (!props.vlmFocusId) return null
    const index = displayCameras.value.indexOf(props.vlmFocusId)
    if (index === -1) return null
    return cellRefs.value[index]?.getVideoElement() ?? null
  },
  getVideoElementByCameraId: (cameraId: string) => {
    const index = displayCameras.value.indexOf(cameraId)
    if (index === -1) return null
    return cellRefs.value[index]?.getVideoElement() ?? null
  },
  getDisplayCameras: () => displayCameras.value,
})
</script>

<style scoped>
.video-grid {
  display: grid;
  gap: 12px;
  width: 100%;
  height: 100%;
  padding: 12px;
  box-sizing: border-box;
}

.grid-1x1 {
  grid-template-columns: 1fr;
  grid-template-rows: 1fr;
}

.grid-2x2 {
  grid-template-columns: 1fr 1fr;
  grid-template-rows: 1fr 1fr;
}

.grid-3x3 {
  grid-template-columns: repeat(3, 1fr);
  grid-template-rows: repeat(3, 1fr);
}

.grid-4x4 {
  grid-template-columns: repeat(4, 1fr);
  grid-template-rows: repeat(4, 1fr);
}

/* 对话框样式 */
.dialog-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
  animation: fadeIn 0.2s ease;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.dialog {
  background: var(--bg-secondary, #1e1e2e);
  border: 1px solid var(--border-color, #333);
  border-radius: 16px;
  width: 90%;
  max-width: 480px;
  max-height: 80vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.3);
  animation: slideIn 0.3s ease;
}

@keyframes slideIn {
  from { 
    opacity: 0;
    transform: translateY(-20px) scale(0.95);
  }
  to { 
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.dialog-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--border-color, #333);
}

.header-info h4 {
  margin: 0 0 4px;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
}

.header-subtitle {
  margin: 0;
  font-size: 14px;
  color: var(--text-muted, #666);
}

.close-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--text-muted, #666);
  padding: 4px;
  border-radius: 6px;
  transition: all 0.2s ease;
}

.close-btn svg {
  width: 20px;
  height: 20px;
  stroke-width: 2;
}

.close-btn:hover {
  background: var(--bg-hover, #2a2a3e);
  color: var(--text-primary, #e0e0e0);
}

.camera-options {
  flex: 1;
  overflow-y: auto;
  padding: 16px 24px 24px;
}

.no-cameras {
  text-align: center;
  padding: 40px 20px;
  color: var(--text-muted, #666);
}

.no-cameras-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.no-cameras-text h5 {
  margin: 0 0 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary, #e0e0e0);
}

.no-cameras-text p {
  margin: 0;
  font-size: 14px;
}

.camera-option {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--border-color, #333);
  border-radius: 12px;
  background: var(--bg-primary, #0f0f1a);
  cursor: pointer;
  margin-bottom: 8px;
  text-align: left;
  transition: all 0.2s ease;
}

.camera-option:hover {
  border-color: var(--accent-primary, #10a37f);
  background: rgba(16, 163, 127, 0.1);
  transform: translateY(-1px);
}

.camera-option:last-child {
  margin-bottom: 0;
}

.option-icon {
  width: 40px;
  height: 40px;
  background: var(--bg-tertiary, #2a2a3e);
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.option-icon svg {
  width: 20px;
  height: 20px;
  stroke-width: 2;
  color: var(--text-secondary, #888);
}

.camera-option:hover .option-icon {
  background: var(--accent-primary, #10a37f);
}

.camera-option:hover .option-icon svg {
  color: white;
}

.camera-info {
  flex: 1;
  overflow: hidden;
}

.camera-name {
  font-weight: 600;
  font-size: 15px;
  color: var(--text-primary, #e0e0e0);
  margin-bottom: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.camera-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--text-muted, #666);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-danger, #ef4444);
}

.camera-status.online .status-dot {
  background: var(--accent-primary, #10a37f);
  box-shadow: 0 0 0 2px rgba(16, 163, 127, 0.2);
}

.option-arrow {
  color: var(--text-muted, #666);
  transition: all 0.2s ease;
}

.option-arrow svg {
  width: 16px;
  height: 16px;
  stroke-width: 2;
}

.camera-option:hover .option-arrow {
  color: var(--accent-primary, #10a37f);
  transform: translateX(2px);
}
</style>
