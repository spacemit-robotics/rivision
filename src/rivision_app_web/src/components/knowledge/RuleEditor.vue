<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ initialYaml?: string }>()
const emit = defineEmits<{ (e: 'save', yaml: string): void }>()
const yaml = ref(props.initialYaml || '')

function save() {
  emit('save', yaml.value)
}
</script>

<template>
  <div class="rule-editor">
    <h4>规则 YAML DSL 编辑器</h4>
    <textarea v-model="yaml" rows="16" placeholder="输入 YAML 规则定义..."></textarea>
    <button @click="save">保存规则</button>
  </div>
</template>

<style scoped>
.rule-editor textarea { width: 100%; font-family: monospace; font-size: 13px; padding: 12px; border: 1px solid #ddd; border-radius: 8px; resize: vertical; }
.rule-editor button { margin-top: 8px; padding: 8px 20px; background: #409eff; color: #fff; border: none; border-radius: 4px; cursor: pointer; }
</style>
