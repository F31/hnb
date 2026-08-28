<script setup lang="ts">
/**
 * StatusBadge — 统一状态徽标（V2.5 §11.3）。
 * 状态颜色由语义决定，不只依赖颜色：同时展示圆点与文字。
 */
import { computed } from 'vue'
import type { StatusSemantic } from '../types'

const props = withDefaults(defineProps<{
  label: string
  semantic?: StatusSemantic
}>(), { semantic: 'default' })

const colorVar = computed(() => {
  switch (props.semantic) {
    case 'success': return 'var(--hnb-color-status-success)'
    case 'warning': return 'var(--hnb-color-status-warning)'
    case 'error': return 'var(--hnb-color-status-danger)'
    case 'info':
    case 'processing': return 'var(--hnb-color-status-info)'
    default: return 'var(--hnb-color-text-tertiary)'
  }
})
</script>

<template>
  <span class="status-badge" :data-semantic="semantic">
    <span class="status-dot" :class="{ pulsing: semantic === 'processing' }" :style="{ background: colorVar }" />
    <span class="status-label">{{ label }}</span>
  </span>
</template>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--hnb-space-sm);
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-primary);
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.pulsing {
  animation: hnb-pulse 1.2s ease-in-out infinite;
}
@keyframes hnb-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}
@media (prefers-reduced-motion: reduce) {
  .pulsing { animation: none; }
}
</style>
