<script setup lang="ts">
/**
 * MetricCard — 指标卡（V2.5 §11.1 仪表盘 Widget 基础件）。
 */
withDefaults(defineProps<{
  title: string
  value: string | number
  unit?: string
  description?: string
}>(), { unit: '', description: '' })
</script>

<template>
  <div class="metric-card">
    <div class="metric-title">{{ title }}</div>
    <div class="metric-value-row">
      <span class="metric-value">{{ value }}</span>
      <span v-if="unit" class="metric-unit">{{ unit }}</span>
    </div>
    <div v-if="description" class="metric-description">{{ description }}</div>
  </div>
</template>

<style scoped>
.metric-card {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs);
  padding: var(--hnb-space-md);
  background: var(--hnb-color-bg-surface);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-lg);
  box-shadow: var(--hnb-shadow-1);
}
.metric-title {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-secondary);
}
.metric-value-row {
  display: flex;
  align-items: baseline;
  gap: var(--hnb-space-xs);
}
.metric-value {
  font-size: 28px;
  font-weight: var(--hnb-font-weight-semibold);
  color: var(--hnb-color-text-primary);
  line-height: 1.2;
}
.metric-unit {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-tertiary);
}
.metric-description {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-tertiary);
}
</style>
