<script setup lang="ts">
/**
 * MetricProgress — 可调度资源概览（CPU/内存）：总量 + 使用率/分配率/超分率。
 * 百分比文案可超过 100（超分率）；进度条填充按 min(percent,100) 视觉呈现。
 */
import type { ResourceUsageSummary } from '../types/clusterMonitoring'

defineProps<{
  title: string
  unit: string
  data: ResourceUsageSummary
}>()

function clampFill(percent: number): string {
  return `${Math.min(100, Math.max(0, percent))}%`
}
</script>

<template>
  <section class="metric-progress">
    <header class="mp-head">
      <h4 class="mp-title">{{ title }}</h4>
      <span class="mp-total">{{ data.total }} {{ unit }}</span>
    </header>

    <div class="mp-row">
      <span class="mp-label">{{ $t('resource.clusterMgmt.monitoring.usageRate') }}</span>
      <div class="mp-bar"><div class="mp-bar__fill usage" :style="{ width: clampFill(data.usagePercent) }"></div></div>
      <span class="mp-value">{{ data.usagePercent }}%</span>
      <span class="mp-abs">{{ data.used }} {{ unit }}</span>
    </div>

    <div class="mp-row">
      <span class="mp-label">{{ $t('resource.clusterMgmt.monitoring.allocationRate') }}</span>
      <div class="mp-bar"><div class="mp-bar__fill allocation" :style="{ width: clampFill(data.allocationPercent) }"></div></div>
      <span class="mp-value">{{ data.allocationPercent }}%</span>
      <span class="mp-abs">{{ data.allocated }} {{ unit }}</span>
    </div>

    <div class="mp-row">
      <span class="mp-label">{{ $t('resource.clusterMgmt.monitoring.overcommitRate') }}</span>
      <div class="mp-bar"><div class="mp-bar__fill overcommit" :style="{ width: clampFill(data.overcommitPercent) }"></div></div>
      <span class="mp-value over">{{ data.overcommitPercent }}%</span>
      <span class="mp-abs">{{ data.overcommitted }} {{ unit }}</span>
    </div>
  </section>
</template>

<style scoped>
.metric-progress {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
}
.mp-head { display: flex; align-items: baseline; justify-content: space-between; }
.mp-title { margin: 0; font-size: 14px; font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.mp-total { font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.mp-row {
  display: grid;
  grid-template-columns: 88px 1fr 56px 84px;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.mp-label { color: var(--hnb-color-text-secondary, #5b6675); white-space: nowrap; }
.mp-bar {
  height: 8px;
  border-radius: 4px;
  background: var(--hnb-color-border, #e2e7ef);
  overflow: hidden;
}
.mp-bar__fill { height: 100%; border-radius: 4px; }
.mp-bar__fill.usage { background: var(--hnb-color-primary, #2f6fed); }
.mp-bar__fill.allocation { background: var(--hnb-color-primary-soft, #9cc0ff); }
.mp-bar__fill.overcommit { background: var(--hnb-color-status-warning, #f79009); }
.mp-value { color: var(--hnb-color-text-primary, #12172a); font-weight: 600; text-align: right; }
.mp-value.over { color: var(--hnb-color-status-warning, #f79009); }
.mp-abs { color: var(--hnb-color-text-secondary, #5b6675); text-align: right; white-space: nowrap; }
</style>
