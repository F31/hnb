<script setup lang="ts">
/**
 * OperationList — Operation Center 列表页（Resource 插件 · cluster-management）。
 *
 * 数据来自 apiserver Operation BFF 服务端分页/过滤/精确总数；行点击进入
 * Operation 详情（L3 组件）跟踪步骤与进度。写动作仅存在于详情页。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { listOperations } from './api/operationApi'
import { getClusterNavigate } from './api/clusterApi'
import { OPERATION_STATUS_OPTIONS, operationListColumns } from './schemas/operation.list'
import type { OperationStatus, OperationSummary } from './types/operation'

const { t } = useI18n()

const items = ref<OperationSummary[]>([])
const loading = ref(false)
const error = ref('')
const statusFilter = ref<OperationStatus | ''>('')
const typeFilter = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const visiblePages = computed(() => {
  const pages: number[] = []
  const start = Math.max(1, page.value - 2)
  const end = Math.min(totalPages.value, page.value + 2)
  for (let i = start; i <= end; i++) pages.push(i)
  return pages
})

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const res = await listOperations({
      page: page.value,
      pageSize: pageSize.value,
      status: statusFilter.value,
      type: typeFilter.value || undefined,
    })
    items.value = res.items
    total.value = res.pagination?.total ?? res.items.length
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function refresh(): void {
  load()
}

function goPage(p: number): void {
  if (p < 1 || p > totalPages.value || p === page.value) return
  page.value = p
  load()
}

function changePageSize(size: number): void {
  pageSize.value = size
  page.value = 1
  load()
}

function goDetail(operationId: string): void {
  getClusterNavigate()(`/resource/operations/${encodeURIComponent(operationId)}`)
}

watch([statusFilter, typeFilter], () => {
  page.value = 1
  load()
})

onMounted(load)
</script>

<template>
  <section class="operation-page">
    <header class="page-header">
      <div>
        <h1>{{ t('resource.operationCenter.title') }}</h1>
        <p>{{ t('resource.operationCenter.desc') }}</p>
      </div>
    </header>

    <div class="toolbar">
      <select v-model="statusFilter" class="filter-select">
        <option v-for="opt in OPERATION_STATUS_OPTIONS" :key="opt.value" :value="opt.value">
          {{ t(opt.labelKey) }}
        </option>
      </select>
      <input
        v-model="typeFilter"
        class="search-input"
        :placeholder="t('resource.operationCenter.filter.type')"
      />
      <button class="secondary-button" type="button" @click="refresh">{{ t('resource.operationCenter.action.refresh') }}</button>
    </div>

    <div v-if="loading" class="panel-status" role="status">{{ t('resource.operationCenter.common.loading') }}</div>
    <div v-else-if="error" class="panel-status error" role="alert">
      <span>{{ error }}</span>
      <button class="retry-button" type="button" @click="refresh">{{ t('resource.operationCenter.action.retry') }}</button>
    </div>
    <div v-else-if="!items.length" class="panel-status empty">
      <p>{{ t('resource.operationCenter.empty.list') }}</p>
    </div>

    <div v-else class="table-card">
      <table class="hnb-table">
        <thead>
          <tr>
            <th v-for="col in operationListColumns" :key="col.key" :style="col.width ? { width: col.width } : undefined">
              {{ t(col.titleKey) }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="op in items" :key="op.operationId">
            <td>
              <button class="name-link" type="button" @click="goDetail(op.operationId)">
                {{ op.operationId }}
              </button>
            </td>
            <td>{{ t(`resource.operationCenter.type.${op.type}`) }}</td>
            <td>{{ op.targetId }}</td>
            <td><span class="status-badge" :data-status="op.status">{{ t(`resource.operationCenter.status.${op.status}`) }}</span></td>
            <td>{{ op.progress.percent }}%</td>
            <td>{{ op.createdAt }}</td>
            <td class="row-actions">
              <router-link class="text-action" :to="`/resource/operations/${encodeURIComponent(op.operationId)}`">
                {{ t('resource.operationCenter.action.view') }}
              </router-link>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="totalPages > 1" class="pagination-bar">
        <span class="pagination-info">{{ total }} {{ t('resource.operationCenter.pagination.items') }}</span>
        <div class="pagination-controls">
          <button class="page-button" :disabled="page <= 1" @click="goPage(page - 1)">‹</button>
          <button
            v-for="p in visiblePages"
            :key="p"
            class="page-num"
            :class="{ active: p === page }"
            @click="goPage(p)"
          >
            {{ p }}
          </button>
          <button class="page-button" :disabled="page >= totalPages" @click="goPage(page + 1)">›</button>
          <select class="page-size" :value="pageSize" @change="changePageSize(Number(($event.target as HTMLSelectElement).value))">
            <option :value="20">20 / {{ t('resource.operationCenter.pagination.page') }}</option>
            <option :value="50">50 / {{ t('resource.operationCenter.pagination.page') }}</option>
          </select>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.operation-page { display: flex; flex-direction: column; gap: var(--hnb-space-md); color: var(--hnb-color-text-primary); }
.page-header h1 { margin: 0; font-size: var(--hnb-font-size-title); }
.page-header p { margin: var(--hnb-space-xs) 0 0; color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-body); }
.toolbar { display: flex; gap: var(--hnb-space-sm); align-items: center; flex-wrap: wrap; }
.search-input {
  flex: 1; min-width: 160px; padding: 8px 12px; border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-body);
}
.filter-select {
  padding: 8px 10px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); font-size: var(--hnb-font-size-body);
}
.secondary-button {
  padding: 8px 18px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); cursor: pointer; font-size: var(--hnb-font-size-body);
}
.panel-status { padding: var(--hnb-space-xl); text-align: center; color: var(--hnb-color-text-tertiary); }
.panel-status.error { color: var(--hnb-color-status-danger); display: flex; flex-direction: column; gap: var(--hnb-space-sm); align-items: center; }
.panel-status.empty { display: flex; flex-direction: column; gap: var(--hnb-space-md); align-items: center; }
.retry-button {
  border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-surface);
  color: var(--hnb-color-text-primary); padding: 4px 12px; cursor: pointer;
}
.table-card { border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-lg); background: var(--hnb-color-bg-surface); overflow-x: auto; }
.hnb-table { width: 100%; border-collapse: collapse; font-size: var(--hnb-font-size-body); }
.hnb-table th {
  text-align: left; font-weight: var(--hnb-font-weight-semibold); color: var(--hnb-color-text-secondary);
  font-size: var(--hnb-font-size-caption); padding: var(--hnb-space-sm) var(--hnb-space-md);
  border-bottom: 1px solid var(--hnb-color-divider); white-space: nowrap;
}
.hnb-table td { padding: var(--hnb-space-sm) var(--hnb-space-md); border-bottom: 1px solid var(--hnb-color-divider); white-space: nowrap; }
.name-link {
  border: 0; background: transparent; color: var(--hnb-color-primary); cursor: pointer; font-size: var(--hnb-font-size-body);
  padding: 0; font-weight: var(--hnb-font-weight-semibold);
}
.status-badge {
  display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: var(--hnb-font-size-caption);
  background: color-mix(in srgb, var(--hnb-color-text-tertiary) 16%, transparent); color: var(--hnb-color-text-secondary);
}
.status-badge[data-status='succeeded'] { background: color-mix(in srgb, var(--hnb-color-status-success) 14%, transparent); color: var(--hnb-color-status-success); }
.status-badge[data-status='failed'] { background: color-mix(in srgb, var(--hnb-color-status-danger) 14%, transparent); color: var(--hnb-color-status-danger); }
.status-badge[data-status='in_progress'] { background: color-mix(in srgb, var(--hnb-color-status-info) 14%, transparent); color: var(--hnb-color-status-info); }
.row-actions { display: flex; gap: var(--hnb-space-sm); align-items: center; }
.text-action { border: 0; background: transparent; color: var(--hnb-color-primary); cursor: pointer; padding: 0; font-size: var(--hnb-font-size-body); text-decoration: none; }
.pagination-bar { display: flex; align-items: center; justify-content: space-between; padding: var(--hnb-space-sm) var(--hnb-space-md); }
.pagination-info { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-tertiary); }
.pagination-controls { display: flex; gap: var(--hnb-space-xs); align-items: center; }
.page-button, .page-num {
  min-width: 28px; height: 28px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm);
  background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); cursor: pointer; font-size: var(--hnb-font-size-caption);
}
.page-num.active { background: var(--hnb-color-primary); border-color: var(--hnb-color-primary); color: #fff; }
.page-button:disabled { opacity: 0.45; cursor: not-allowed; }
.page-size { margin-left: var(--hnb-space-sm); padding: 4px 6px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); font-size: var(--hnb-font-size-caption); }
</style>
