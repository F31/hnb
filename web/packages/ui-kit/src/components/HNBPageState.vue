<script setup lang="ts">
import type { PageStateKind } from '../types'
import EmptyState from './EmptyState.vue'
import ErrorState from './ErrorState.vue'
import HNBAlert from './HNBAlert.vue'
import HNBButton from './HNBButton.vue'
import HNBSkeleton from './HNBSkeleton.vue'

const props = withDefaults(defineProps<{
  state: PageStateKind
  title: string
  description?: string
  actionText?: string
  actionLoading?: boolean
  skeletonRows?: number
}>(), { actionLoading: false, skeletonRows: 3 })

const emit = defineEmits<{ action: [] }>()
</script>

<template>
  <section class="hnb-page-state" :data-state="state" :aria-label="title">
    <HNBSkeleton v-if="state === 'loading'" :rows="skeletonRows" :label="title" title />
    <EmptyState v-else-if="state === 'empty'" :title="title" :description="description" :action-text="actionText" @action="emit('action')" />
    <ErrorState v-else-if="state === 'error'" :title="title" :description="description" :retry-text="actionText || 'Retry'" :retry-loading="actionLoading" @retry="emit('action')" />
    <HNBAlert v-else :semantic="state === 'offline' ? 'warning' : 'info'" :live="state === 'offline' ? 'assertive' : 'polite'" :title="title">
      {{ description }}
      <template v-if="actionText" #actions>
        <HNBButton :loading="actionLoading" @click="emit('action')">{{ actionText }}</HNBButton>
      </template>
    </HNBAlert>
  </section>
</template>

<style scoped>
.hnb-page-state { width: 100%; }
</style>
