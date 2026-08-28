<script setup lang="ts">
import type { HNBStatusItem } from '../types'
import StatusBadge from './StatusBadge.vue'

defineProps<{
  items: HNBStatusItem[]
  ariaLabel: string
  lastKnownLabel?: string
}>()
</script>

<template>
  <div class="hnb-status-group" role="group" :aria-label="ariaLabel">
    <div v-for="item in items" :key="item.key" class="hnb-status-group__item">
      <span class="hnb-status-group__label">{{ item.label }}</span>
      <StatusBadge :label="item.valueLabel" :semantic="item.semantic" />
    </div>
    <div v-if="lastKnownLabel" class="hnb-status-group__last-known">{{ lastKnownLabel }}</div>
  </div>
</template>

<style scoped>
.hnb-status-group { display: flex; align-items: center; flex-wrap: wrap; gap: var(--hnb-space-sm) var(--hnb-space-md); }
.hnb-status-group__item { display: inline-flex; align-items: center; gap: var(--hnb-space-xs); }
.hnb-status-group__label, .hnb-status-group__last-known { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-secondary); }
.hnb-status-group__last-known { flex-basis: 100%; }
</style>
