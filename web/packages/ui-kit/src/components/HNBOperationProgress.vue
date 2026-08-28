<script setup lang="ts">
import { computed } from 'vue'
import type { OperationProgressStep } from '../types'
import HNBLiveRegion from './HNBLiveRegion.vue'

const props = defineProps<{
  label: string
  steps: OperationProgressStep[]
  value?: number
  statusMessage?: string
}>()

const normalizedValue = computed(() => props.value === undefined ? undefined : Math.min(100, Math.max(0, props.value)))
</script>

<template>
  <section class="hnb-operation-progress" :aria-label="label">
    <div
      class="hnb-operation-progress__bar"
      role="progressbar"
      :aria-label="label"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="normalizedValue"
    >
      <div class="hnb-operation-progress__fill" :class="{ 'is-indeterminate': normalizedValue === undefined }" :style="normalizedValue === undefined ? undefined : { width: `${normalizedValue}%` }" />
    </div>
    <ol class="hnb-operation-progress__steps">
      <li
        v-for="step in steps"
        :key="step.id"
        class="hnb-operation-progress__step"
        :data-status="step.status"
        :aria-current="step.status === 'running' ? 'step' : undefined"
      >
        <span class="hnb-operation-progress__marker" aria-hidden="true">{{ step.status === 'success' ? '✓' : step.status === 'error' ? '!' : '•' }}</span>
        <div>
          <div class="hnb-operation-progress__label">{{ step.label }}</div>
          <div v-if="step.description" class="hnb-operation-progress__description">{{ step.description }}</div>
          <time v-if="step.timestamp">{{ step.timestamp }}</time>
        </div>
      </li>
    </ol>
    <HNBLiveRegion v-if="statusMessage" :message="statusMessage" />
  </section>
</template>

<style scoped>
.hnb-operation-progress__bar { height: 8px; overflow: hidden; border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); }
.hnb-operation-progress__fill { height: 100%; background: var(--hnb-color-primary); transition: width var(--hnb-duration-normal) ease; }
.hnb-operation-progress__fill.is-indeterminate { width: 35%; animation: hnb-progress 1.2s ease-in-out infinite alternate; }
.hnb-operation-progress__steps { display: grid; gap: var(--hnb-space-md); margin: var(--hnb-space-lg) 0 0; padding: 0; list-style: none; }
.hnb-operation-progress__step { display: grid; grid-template-columns: 20px 1fr; gap: var(--hnb-space-sm); color: var(--hnb-color-text-secondary); }
.hnb-operation-progress__step[data-status='running'] { color: var(--hnb-color-status-info); }
.hnb-operation-progress__step[data-status='success'] { color: var(--hnb-color-status-success); }
.hnb-operation-progress__step[data-status='error'] { color: var(--hnb-color-status-danger); }
.hnb-operation-progress__label { font-weight: var(--hnb-font-weight-semibold); color: var(--hnb-color-text-primary); }
.hnb-operation-progress__description, .hnb-operation-progress time { font-size: var(--hnb-font-size-caption); overflow-wrap: anywhere; }
@keyframes hnb-progress { from { transform: translateX(-100%); } to { transform: translateX(280%); } }
@media (prefers-reduced-motion: reduce) {
  .hnb-operation-progress__fill { transition: none; }
  .hnb-operation-progress__fill.is-indeterminate { animation: none; }
}
</style>
