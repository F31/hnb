<script setup lang="ts">
/**
 * RegionWrapper — 区块级错误隔离（V2.5 §4.4）：
 * 单个区块渲染失败只显示安全占位符，不影响整页。
 */
import { onErrorCaptured, ref } from 'vue'

const props = defineProps<{ regionId: string }>()
const failed = ref(false)
const errorMessage = ref('')

onErrorCaptured((err) => {
  failed.value = true
  errorMessage.value = err instanceof Error ? err.message : String(err)
  console.error(`[PageRenderer] region "${props.regionId}" failed:`, err)
  return false
})
</script>

<template>
  <div v-if="failed" class="region-error" role="alert">
    <span class="region-error-icon">!</span>
    区块加载失败（{{ regionId }}）
  </div>
  <slot v-else />
</template>

<style scoped>
.region-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px;
  border: 1px dashed var(--hnb-color-status-danger, #f04438);
  border-radius: var(--hnb-radius-md, 6px);
  color: var(--hnb-color-status-danger, #f04438);
  font-size: 13px;
  min-height: 64px;
}
.region-error-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--hnb-color-status-danger, #f04438);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
}
</style>
