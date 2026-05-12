// P0: 视频核心组件
export { default as VideoCell } from './VideoCell.vue'
export { default as VideoGrid } from './VideoGrid.vue'

// P1: 管理组件
export { default as CameraManagementModal } from './CameraManagementModal.vue'
export { default as OwlDiscoveryPanel } from './OwlDiscoveryPanel.vue'
export { default as VlmHistory } from './VlmHistory.vue'

// P2: 辅助组件
export { default as GatewayPanel } from './GatewayPanel.vue'
export { default as DetailModal } from './DetailModal.vue'

// 类型导出
export type { Detection, YoloConfig } from './VideoCell.vue'
export type { GridLayout, Camera as GridCamera } from './VideoGrid.vue'
export type { Camera, Node } from './CameraManagementModal.vue'
export type { VlmRecord } from './VlmHistory.vue'
export type { DetailItem, ActionButton } from './DetailModal.vue'
