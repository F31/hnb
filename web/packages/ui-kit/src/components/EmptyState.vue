<script setup lang="ts">
import HNBButton from './HNBButton.vue'

/**
 * EmptyState — 空数据状态（V2.5 §18.2：说明原因并提供合理下一步）。
 */
defineProps<{
  title: string
  description?: string
  actionText?: string
}>()

const emit = defineEmits<{ action: [] }>()
</script>

<template>
  <div class="empty-state">
    <div class="empty-icon" aria-hidden="true">∅</div>
    <div class="empty-title">{{ title }}</div>
    <div v-if="description" class="empty-description">{{ description }}</div>
    <HNBButton v-if="actionText" class="empty-action" variant="primary" @click="emit('action')">{{ actionText }}</HNBButton>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--hnb-space-sm);
  padding: var(--hnb-space-xl) var(--hnb-space-md);
  color: var(--hnb-color-text-secondary);
}
.empty-icon {
  font-size: 32px;
  color: var(--hnb-color-text-tertiary);
}
.empty-title {
  font-size: var(--hnb-font-size-body);
  font-weight: var(--hnb-font-weight-semibold);
  color: var(--hnb-color-text-primary);
}
.empty-description {
  font-size: var(--hnb-font-size-caption);
  text-align: center;
  max-width: 360px;
}
.empty-action { margin-top: var(--hnb-space-sm); }
</style>
import HNBButton from './HNBButton.vue'
