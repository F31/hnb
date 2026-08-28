<script setup lang="ts">
import { computed } from 'vue'
import HNBButton from './HNBButton.vue'
import HNBLiveRegion from './HNBLiveRegion.vue'

const props = withDefaults(defineProps<{
  page: number
  pageSize: number
  total: number
  pageSizes?: number[]
  ariaLabel?: string
  previousLabel?: string
  nextLabel?: string
  pageSizeLabel?: string
  statusText?: string
  previousDisabledReason?: string
  nextDisabledReason?: string
}>(), {
  pageSizes: () => [10, 20, 50],
  ariaLabel: 'Pagination',
  previousLabel: 'Previous page',
  nextLabel: 'Next page',
  pageSizeLabel: 'Items per page',
  statusText: '',
  previousDisabledReason: 'Already on the first page',
  nextDisabledReason: 'Already on the last page',
})

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()

const pageCount = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const status = computed(() => {
  if (props.statusText) return props.statusText
  return `Page ${props.page} of ${pageCount.value}, ${props.total} items`
})

function updatePageSize(event: Event) {
  emit('update:pageSize', Number((event.target as HTMLSelectElement).value))
}
</script>

<template>
  <nav class="hnb-pagination" :aria-label="ariaLabel">
    <HNBButton
      size="small"
      :disabled="page <= 1"
      :disabled-reason="previousDisabledReason"
      :aria-label="previousLabel"
      @click="emit('update:page', page - 1)"
    >‹</HNBButton>
    <span class="hnb-pagination__status" aria-current="page">{{ status }}</span>
    <HNBButton
      size="small"
      :disabled="page >= pageCount"
      :disabled-reason="nextDisabledReason"
      :aria-label="nextLabel"
      @click="emit('update:page', page + 1)"
    >›</HNBButton>
    <label class="hnb-pagination__size">
      <span>{{ pageSizeLabel }}</span>
      <select :value="pageSize" @change="updatePageSize">
        <option v-for="size in pageSizes" :key="size" :value="size">{{ size }}</option>
      </select>
    </label>
    <HNBLiveRegion :message="status" />
  </nav>
</template>

<style scoped>
.hnb-pagination { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: var(--hnb-space-sm); color: var(--hnb-color-text-secondary); }
.hnb-pagination__status { white-space: nowrap; }
.hnb-pagination__size { display: inline-flex; align-items: center; gap: var(--hnb-space-xs); }
.hnb-pagination__size select { min-height: 28px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); }
.hnb-pagination__size select:focus-visible { outline: 2px solid var(--hnb-color-focus); outline-offset: 2px; }
@media (max-width: 480px) {
  .hnb-pagination { justify-content: center; }
  .hnb-pagination__size { flex-basis: 100%; justify-content: center; }
}
</style>
