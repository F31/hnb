<script setup lang="ts">
import type { HNBDetailItem } from '../types'

defineProps<{
  title?: string
  items: HNBDetailItem[]
}>()
</script>

<template>
  <section class="hnb-detail-panel">
    <h2 v-if="title">{{ title }}</h2>
    <dl>
      <template v-for="item in items" :key="item.label">
        <dt>{{ item.label }}</dt>
        <dd>{{ item.value ?? '-' }}</dd>
      </template>
    </dl>
  </section>
</template>

<style scoped>
.hnb-detail-panel { padding: var(--hnb-space-md); background: var(--hnb-color-bg-surface); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-lg); }
.hnb-detail-panel h2 { margin: 0 0 var(--hnb-space-md); font-size: var(--hnb-font-size-body); font-weight: var(--hnb-font-weight-semibold); }
.hnb-detail-panel dl { display: grid; grid-template-columns: minmax(120px, 180px) 1fr; gap: var(--hnb-space-sm) var(--hnb-space-md); margin: 0; }
.hnb-detail-panel dt { color: var(--hnb-color-text-secondary); }
.hnb-detail-panel dd { margin: 0; color: var(--hnb-color-text-primary); }
@media (max-width: 768px) { .hnb-detail-panel dl { grid-template-columns: 1fr; } }
</style>
