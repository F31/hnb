<script setup lang="ts">
/**
 * AllocationMetricCell — 项目配额单元格：`额度(已用 | 22%)` + 进度条。
 * 安全计算：limit<=0 时显示 0%，不产生 NaN/Infinity，进度不溢出。
 */
import { computed } from 'vue'
import type { AllocationMetric } from '../types/p4'

const props = defineProps<{ metric: AllocationMetric; unit?: string }>()

const safePercent = computed(() => {
  const p = Number(props.metric.percent)
  if (!Number.isFinite(p)) return 0
  return Math.min(100, Math.max(0, p))
})

const display = computed(() => {
  const limit = Number(props.metric.limit)
  const used = Number(props.metric.used)
  const limitText = Number.isFinite(limit) && limit > 0 ? String(limit) : '--'
  const usedText = Number.isFinite(used) ? String(used) : '0'
  const unit = props.unit ?? ''
  return `${limitText}${unit}(${usedText}${unit} | ${safePercent.value}%)`
})
</script>

<template>
  <div class="allocation-cell" :title="display">
    <span class="allocation-cell__text">{{ display }}</span>
    <div class="allocation-cell__bar" role="progressbar" :aria-valuenow="safePercent" aria-valuemin="0" aria-valuemax="100">
      <div class="allocation-cell__fill" :style="{ width: `${safePercent}%` }"></div>
    </div>
  </div>
</template>

<style scoped>
.allocation-cell { display: flex; flex-direction: column; gap: 4px; min-width: 120px; }
.allocation-cell__text { font-size: 12px; color: var(--hnb-color-text-primary, #12172a); white-space: nowrap; }
.allocation-cell__bar {
  height: 6px;
  border-radius: 3px;
  background: var(--hnb-color-border, #e2e7ef);
  overflow: hidden;
}
.allocation-cell__fill {
  height: 100%;
  border-radius: 3px;
  background: var(--hnb-color-primary, #2f6fed);
}
</style>
