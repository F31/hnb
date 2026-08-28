<template>
  <HNBAlert v-if="hasError" semantic="error" :title="title" :description="description">
    <template #action>
      <button class="hnb-error-retry" type="button" @click="retry">{{ retryText }}</button>
    </template>
  </HNBAlert>
  <slot v-else />
</template>

<script setup lang="ts">
import { ref, onErrorCaptured } from 'vue'
import HNBAlert from './HNBAlert.vue'

withDefaults(defineProps<{
  title?: string
  description?: string
  retryText?: string
}>(), {
  title: '区块异常',
  description: '渲染时发生错误，请重试。',
  retryText: '重试',
})

const hasError = ref(false)
const errorKey = ref(0)

onErrorCaptured((err) => {
  hasError.value = true
  console.warn('[HNBErrorBoundary] caught:', err)
  return false
})

function retry() {
  hasError.value = false
  errorKey.value++
}
</script>

<style scoped>
.hnb-error-retry {
  padding: 6px 14px;
  border: 1px solid var(--hnb-color-border, #29344a);
  border-radius: var(--hnb-radius-md, 6px);
  background: var(--hnb-color-bg-surface, #101425);
  color: var(--hnb-color-text-primary, #edeff5);
  font-size: 12px;
  cursor: pointer;
}
.hnb-error-retry:hover {
  background: var(--hnb-color-bg-elevated, #171d31);
}
</style>