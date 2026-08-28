<script setup lang="ts">
import type { HNBDetailItem } from '../types'

withDefaults(defineProps<{
  items: HNBDetailItem[]
  column?: number
}>(), { column: 2 })
</script>

<template>
  <dl class="description-list" :style="{ gridTemplateColumns: `repeat(${column}, 1fr)` }">
    <div v-for="item in items" :key="item.label" class="description-item">
      <dt class="description-label">{{ item.label }}</dt>
      <dd class="description-value">{{ item.value ?? '-' }}</dd>
    </div>
  </dl>
</template>

<style scoped>
.description-list {
  display: grid;
  gap: var(--hnb-space-md);
  margin: 0;
}
.description-item {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs);
}
.description-label {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-tertiary);
}
.description-value {
  margin: 0;
  font-size: var(--hnb-font-size-body);
  color: var(--hnb-color-text-primary);
  word-break: break-all;
}
@media (max-width: 768px) {
  .description-list {
    grid-template-columns: 1fr !important;
  }
}
</style>
