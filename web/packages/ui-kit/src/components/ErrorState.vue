<script setup lang="ts">
import HNBButton from './HNBButton.vue'

/**
 * ErrorState — 失败状态（V2.5 §18.2：错误区块独立重试）。
 */
withDefaults(defineProps<{
  title?: string
  description?: string
  retryText?: string
  retryLoading?: boolean
  code?: string
}>(), { title: '加载失败', retryText: '重试', retryLoading: false })

const emit = defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="error-state" role="alert">
    <div class="error-icon" aria-hidden="true">!</div>
    <div class="error-title">{{ title }}</div>
    <div v-if="description" class="error-description">{{ description }}</div>
    <div v-if="code" class="error-code">{{ code }}</div>
    <HNBButton class="error-retry" :loading="retryLoading" @click="emit('retry')">{{ retryText }}</HNBButton>
  </div>
</template>

<style scoped>
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--hnb-space-sm);
  padding: var(--hnb-space-xl) var(--hnb-space-md);
}
.error-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--hnb-color-status-danger);
  color: var(--hnb-color-text-on-accent);
  font-size: 18px;
  font-weight: 700;
}
.error-title {
  font-size: var(--hnb-font-size-body);
  font-weight: var(--hnb-font-weight-semibold);
  color: var(--hnb-color-status-danger);
}
.error-description {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-secondary);
  text-align: center;
  max-width: 360px;
}
.error-code { font-family: monospace; font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-tertiary); }
.error-retry { margin-top: var(--hnb-space-sm); }
</style>
import HNBButton from './HNBButton.vue'
