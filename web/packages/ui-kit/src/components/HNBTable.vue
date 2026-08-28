<script setup lang="ts" generic="T extends Record<string, any>">
import { computed, ref, watch, nextTick, onMounted } from 'vue'
import { render } from 'vue'
import type { HNBTableColumn, HNBTablePagination, HNBTableRowKey } from '../types'
import EmptyState from './EmptyState.vue'
import ErrorState from './ErrorState.vue'
import HNBPagination from './HNBPagination.vue'

const props = withDefaults(defineProps<{
  columns: HNBTableColumn<T>[]
  data: T[]
  loading?: boolean
  rowKey?: HNBTableRowKey
  pagination?: HNBTablePagination
  selectable?: boolean
  checkedRowKeys?: (string | number)[]
  ariaLabel?: string
  minWidth?: string
  emptyTitle?: string
  error?: string
  errorRetryText?: string
  errorRetryLoading?: boolean
}>(), {
  loading: false,
  selectable: false,
  checkedRowKeys: () => [],
  ariaLabel: 'Data table',
  minWidth: '640px',
  emptyTitle: '暂无数据',
  errorRetryText: '重试',
  errorRetryLoading: false,
})

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
  'update:checkedRowKeys': [keys: (string | number)[]]
  'errorRetry': []
}>()

const tableRef = ref<HTMLElement | null>(null)

function renderCell(container: HTMLElement, row: T, col: HNBTableColumn<T>, idx: number) {
  if (!col.render) return
  container.innerHTML = ''
  const vnode = col.render(row, idx)
  if (vnode == null) return
  if (typeof vnode === 'string' || typeof vnode === 'number') {
    container.textContent = String(vnode)
  } else {
    render(vnode, container)
  }
}

function renderCells() {
  const table = tableRef.value
  if (!table) return
  const cells = table.querySelectorAll('[data-cell-key]')
  cells.forEach((el) => {
    const key = el.getAttribute('data-cell-key') || ''
    const parts = key.split('::')
    if (parts.length !== 2) return
    const [rowIdxStr, colKey] = parts
    const rowIdx = parseInt(rowIdxStr, 10)
    const row = props.data[rowIdx]
    const col = props.columns.find((c) => c.key === colKey)
    if (row && col) {
      renderCell(el as HTMLElement, row, col, rowIdx)
    }
  })
}

onMounted(() => {
  nextTick(() => renderCells())
})

watch(() => props.data, () => {
  nextTick(() => renderCells())
}, { deep: true })

watch(() => props.columns, () => {
  nextTick(() => renderCells())
}, { deep: true })

function isChecked(row: T): boolean {
  if (!props.rowKey) return false
  if (typeof props.rowKey === 'string') {
    return props.checkedRowKeys.includes((row as any)[props.rowKey] as string | number)
  }
  const key = props.rowKey(row)
  return props.checkedRowKeys.includes(key)
}

function toggleCheck(row: T): void {
  if (!props.rowKey) return
  let key: string | number
  if (typeof props.rowKey === 'string') {
    key = (row as any)[props.rowKey] as string | number
  } else {
    key = props.rowKey(row)
  }
  const newKeys = isChecked(row)
    ? props.checkedRowKeys.filter((k) => k !== key)
    : [...props.checkedRowKeys, key]
  emit('update:checkedRowKeys', newKeys)
}

function toggleAll(): void {
  if (allChecked.value) {
    emit('update:checkedRowKeys', [])
  } else {
    const keys = props.data.map((row) => {
      if (typeof props.rowKey === 'string') {
        return (row as any)[props.rowKey] as string | number
      }
      return props.rowKey!(row)
    })
    emit('update:checkedRowKeys', keys)
  }
}

const allChecked = computed(() => {
  if (!props.data.length) return false
  return props.data.every((row) => isChecked(row))
})
</script>

<template>
  <div ref="tableRef" class="hnb-table-wrapper" :style="{ minWidth }" role="region" :aria-label="ariaLabel" tabindex="0">
    <table class="hnb-table">
      <thead>
        <tr>
          <th v-if="selectable" class="hnb-table__check">
            <input type="checkbox" :checked="allChecked" :indeterminate="!allChecked && checkedRowKeys.length > 0" @change="toggleAll" />
          </th>
          <th v-for="col in columns" :key="col.key" :style="col.width ? { width: col.width } : undefined">
            {{ col.title }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="loading">
          <td :colspan="columns.length + (selectable ? 1 : 0)" class="hnb-table__loading">
            <div class="hnb-table__spinner" />
          </td>
        </tr>
        <tr v-else-if="error">
          <td :colspan="columns.length + (selectable ? 1 : 0)" class="hnb-table__state">
            <ErrorState :title="error" :retry-text="errorRetryText" :retry-loading="errorRetryLoading" @retry="emit('errorRetry')" />
          </td>
        </tr>
        <tr v-else-if="!data.length">
          <td :colspan="columns.length + (selectable ? 1 : 0)" class="hnb-table__state">
            <EmptyState :title="emptyTitle" />
          </td>
        </tr>
        <tr v-for="(row, idx) in data" :key="rowKey ? (typeof rowKey === 'string' ? (row as any)[rowKey] : rowKey(row)) : idx" :class="{ 'hnb-table__row--checked': isChecked(row) }">
          <td v-if="selectable" class="hnb-table__check">
            <input type="checkbox" :checked="isChecked(row)" @change="toggleCheck(row)" />
          </td>
          <td v-for="col in columns" :key="col.key">
            <div :data-cell-key="`${idx}::${col.key}`" :ref="() => {}">
              {{ col.render ? '' : ((row as any)[col.key] ?? '-') }}
            </div>
          </td>
        </tr>
      </tbody>
    </table>
    <HNBPagination
      v-if="pagination"
      :page="pagination.page"
      :page-size="pagination.pageSize"
      :total="pagination.total"
      @update:page="(p) => emit('update:page', p)"
      @update:page-size="(ps) => emit('update:pageSize', ps)"
    />
  </div>
</template>

<style scoped>
.hnb-table-wrapper { max-width: 100%; overflow-x: auto; background: var(--hnb-color-bg-surface); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-lg); }
.hnb-table-wrapper:focus-visible { outline: 2px solid var(--hnb-color-focus); outline-offset: 2px; }
.hnb-table { width: 100%; border-collapse: collapse; }
.hnb-table th { padding: 10px 12px; font-size: var(--hnb-font-size-caption); font-weight: var(--hnb-font-weight-semibold); color: var(--hnb-color-text-secondary); text-align: left; border-bottom: 1px solid var(--hnb-color-divider); white-space: nowrap; }
.hnb-table td { padding: 10px 12px; font-size: var(--hnb-font-size-body); color: var(--hnb-color-text-primary); border-bottom: 1px solid var(--hnb-color-divider); }
.hnb-table tbody tr:hover { background: var(--hnb-color-bg-elevated); }
.hnb-table__row--checked { background: color-mix(in srgb, var(--hnb-color-primary) 8%, transparent); }
.hnb-table__check { width: 40px; text-align: center; }
.hnb-table__check input { accent-color: var(--hnb-color-primary); cursor: pointer; }
.hnb-table__state { padding: var(--hnb-space-lg) 0; }
.hnb-table__loading { text-align: center; padding: var(--hnb-space-lg); }
.hnb-table__spinner { display: inline-block; width: 24px; height: 24px; border: 3px solid var(--hnb-color-border); border-top-color: var(--hnb-color-primary); border-radius: 50%; animation: hnb-table-spin 0.6s linear infinite; }
@keyframes hnb-table-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .hnb-table__spinner { animation: none; } }
</style>