<script setup lang="ts">
import type { StatusSemantic } from '../types'
import HNBButton from './HNBButton.vue'

withDefaults(defineProps<{
  title?: string
  semantic?: Exclude<StatusSemantic, 'processing' | 'default'>
  live?: 'off' | 'polite' | 'assertive'
  dismissLabel?: string
  dismissible?: boolean
}>(), { semantic: 'info', live: 'polite', dismissLabel: 'Dismiss', dismissible: false })

const emit = defineEmits<{ dismiss: [] }>()
</script>

<template>
  <div
    class="hnb-alert"
    :class="`hnb-alert--${semantic}`"
    :role="live === 'assertive' ? 'alert' : 'status'"
    :aria-live="live"
    aria-atomic="true"
  >
    <span class="hnb-alert__marker" aria-hidden="true">!</span>
    <div class="hnb-alert__content">
      <div v-if="title" class="hnb-alert__title">{{ title }}</div>
      <div class="hnb-alert__body"><slot /></div>
      <div v-if="$slots.actions" class="hnb-alert__actions"><slot name="actions" /></div>
    </div>
    <HNBButton v-if="dismissible" variant="ghost" size="small" :aria-label="dismissLabel" @click="emit('dismiss')">×</HNBButton>
  </div>
</template>

<style scoped>
.hnb-alert { display: flex; align-items: flex-start; gap: var(--hnb-space-sm); padding: var(--hnb-space-md); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); color: var(--hnb-color-text-primary); }
.hnb-alert--success { background: var(--hnb-color-status-success-surface); border-color: var(--hnb-color-status-success); }
.hnb-alert--warning { background: var(--hnb-color-status-warning-surface); border-color: var(--hnb-color-status-warning); }
.hnb-alert--error { background: var(--hnb-color-status-danger-surface); border-color: var(--hnb-color-status-danger); }
.hnb-alert--info { background: var(--hnb-color-status-info-surface); border-color: var(--hnb-color-status-info); }
.hnb-alert__marker { font-weight: var(--hnb-font-weight-semibold); }
.hnb-alert__content { flex: 1; min-width: 0; }
.hnb-alert__title { margin-bottom: var(--hnb-space-xs); font-weight: var(--hnb-font-weight-semibold); }
.hnb-alert__body { color: var(--hnb-color-text-secondary); overflow-wrap: anywhere; }
.hnb-alert__actions { display: flex; flex-wrap: wrap; gap: var(--hnb-space-sm); margin-top: var(--hnb-space-sm); }
</style>
