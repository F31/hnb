<script setup lang="ts">
/**
 * ClusterList — 集群管理列表页（资源插件 · cluster-management）。
 *
 * 遵循 V2.5 §10 动态表格：服务端分页、状态字典、行操作 ≤3、危险操作二次确认、
 * 批量操作展示影响范围；写动作统一提交 RuntimeIntent 并跟踪 Operation。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import * as api from './api/clusterApi'
import { clusterPermissions, getClusterNavigate } from './api/clusterApi'
import ClusterStatusBadge from './components/ClusterStatusBadge.vue'
import ClusterSummaryCards from './components/ClusterSummaryCards.vue'
import ClusterRegisterWizard from './components/ClusterRegisterWizard.vue'
import StaleChallengeDialog from './components/StaleChallengeDialog.vue'
import { useStaleSubmit } from './composables/useStaleSubmit'
import { CLUSTER_KIND_OPTIONS, CLUSTER_STATUS_OPTIONS, clusterListColumns } from './schemas/cluster.list'
import { canMutate } from './schemas/cluster.status'
import type { ClusterKind, ClusterListAggregate, ClusterStatus, ClusterSummary, RuntimeIntentRecord } from './types/cluster'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

function routeString(name: string): string {
  const value = route.query[name]
  return typeof value === 'string' ? value : ''
}

function routeNumber(name: string, fallback: number): number {
  const value = Number(routeString(name))
  return Number.isInteger(value) && value > 0 ? value : fallback
}

const clusters = ref<ClusterSummary[]>([])
const loading = ref(false)
const error = ref('')
const query = ref(routeString('keyword'))
const kindFilter = ref<ClusterKind | ''>(routeString('kind') as ClusterKind | '')
const statusFilter = ref<ClusterStatus | ''>(routeString('status') as ClusterStatus | '')
const page = ref(routeNumber('page', 1))
const pageSize = ref(routeNumber('pageSize', 20) === 50 ? 50 : 20)
const total = ref(0)
const summary = ref<ClusterListAggregate | undefined>()
let filterTimer: ReturnType<typeof setTimeout> | null = null

const selected = ref<string[]>([])
const registerOpen = ref(false)
const submitError = ref('')
const lastIntent = ref<RuntimeIntentRecord | null>(null)
const intentOpen = ref(false)

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const visiblePages = computed(() => {
  const pages: number[] = []
  const start = Math.max(1, page.value - 2)
  const end = Math.min(totalPages.value, page.value + 2)
  for (let i = start; i <= end; i++) pages.push(i)
  return pages
})

const canCreate = computed(() => api.hasClusterPermission(clusterPermissions.create))
const canUpdate = computed(() => api.hasClusterPermission(clusterPermissions.update))
const canDelete = computed(() => api.hasClusterPermission(clusterPermissions.delete))

const selectedClusters = computed(() =>
  clusters.value.filter((c) => selected.value.includes(c.clusterId)),
)

function syncRoute(): void {
  void router.replace({
    query: {
      ...route.query,
      keyword: query.value || undefined,
      kind: kindFilter.value || undefined,
      status: statusFilter.value || undefined,
      page: page.value > 1 ? String(page.value) : undefined,
      pageSize: pageSize.value !== 20 ? String(pageSize.value) : undefined,
    },
  })
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const res = await api.listClusters({
      page: page.value,
      pageSize: pageSize.value,
      keyword: query.value,
      kind: kindFilter.value,
      status: statusFilter.value,
    })
    clusters.value = res.items
    total.value = res.total
    summary.value = res.summary
    selected.value = selected.value.filter((id) => res.items.some((c) => c.clusterId === id))
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    clusters.value = []
    summary.value = undefined
  } finally {
    loading.value = false
  }
}

function refresh(): void {
  load()
}

watch([query, kindFilter, statusFilter], () => {
  page.value = 1
  // Inputs change on every keystroke; debounce to prevent out-of-order,
  // unnecessary list queries against the Read Model.
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    filterTimer = null
    syncRoute()
    void load()
  }, 300)
})

onMounted(load)
onBeforeUnmount(() => {
  if (filterTimer) clearTimeout(filterTimer)
})

function goPage(p: number): void {
  if (p < 1 || p > totalPages.value || p === page.value) return
  page.value = p
  syncRoute()
  load()
}

function changePageSize(size: number): void {
  pageSize.value = size
  page.value = 1
  syncRoute()
  load()
}

function goDetail(clusterId: string): void {
  getClusterNavigate()(`/resource/clusters/${encodeURIComponent(clusterId)}`)
}

function goOperation(operationId: string): void {
  getClusterNavigate()(`/resource/operations/${encodeURIComponent(operationId)}`)
}

function openRegister(): void {
  registerOpen.value = true
}

function onSubmitted(record: RuntimeIntentRecord): void {
  // 注册向导已在其内联视图展示 Operation 进度（闭环），这里只需刷新列表，
  // 让新接入的集群以 REGISTERING/PROVISIONING 状态出现在列表首部。
  void record
  submitError.value = ''
  refresh()
}

// ---------------------------------------------------------------------------
// 确认弹窗（V2.5 §12.3：危险操作二次确认 + 影响范围）
// ---------------------------------------------------------------------------
interface ConfirmOptions {
  title: string
  message: string
  items?: string[]
  danger?: boolean
  confirmText: string
}
const confirmOptions = ref<ConfirmOptions | null>(null)
let confirmResolver: ((ok: boolean) => void) | null = null

function requestConfirm(options: ConfirmOptions): Promise<boolean> {
  confirmOptions.value = options
  return new Promise((resolve) => {
    confirmResolver = resolve
  })
}

function resolveConfirm(ok: boolean): void {
  const resolver = confirmResolver
  confirmResolver = null
  confirmOptions.value = null
  resolver?.(ok)
}

// ---------------------------------------------------------------------------
// 写动作：升级 / 解除纳管（RuntimeIntent，跟踪 Operation）
//  - 升级需选择目标版本（desiredVersion）
//  - STALE 目标提交会收到 409 STALE challenge，需风险确认后携带 riskConfirmation 重提
// ---------------------------------------------------------------------------
const mutationError = ref('')
const mutatingId = ref('')

// 升级版本选择弹窗
const upgradeModal = ref<{ open: boolean; cluster: ClusterSummary | null; version: string }>({
  open: false, cluster: null, version: '',
})

// STALE 风险确认弹窗（共享实现，见 composables/useStaleSubmit + StaleChallengeDialog）
const { staleChallenge, staleActionLabel, submit: submitWithStaleChallenge, resolveStaleConfirm } = useStaleSubmit()

function openUpgrade(cluster: ClusterSummary): void {
  upgradeModal.value = { open: true, cluster, version: '' }
}

async function confirmUpgrade(): Promise<void> {
  const { cluster, version } = upgradeModal.value
  upgradeModal.value = { open: false, cluster: null, version: '' }
  if (!cluster) return
  const desiredVersion = version.trim()
  if (!desiredVersion) return
  mutationError.value = ''
  mutatingId.value = cluster.clusterId
  try {
    const result = await submitWithStaleChallenge(
      api.buildUpgradeIntent(cluster, desiredVersion),
      t('resource.clusterMgmt.action.upgrade'),
    )
    if (result !== 'cancelled') {
      lastIntent.value = result
      intentOpen.value = true
      refresh()
    }
  } catch (err) {
    mutationError.value = err instanceof Error ? err.message : String(err)
  } finally {
    mutatingId.value = ''
  }
}

async function deleteClusters(targets: ClusterSummary[]): Promise<void> {
  if (!targets.length) return
  const confirmed = await requestConfirm({
    title: t('resource.clusterMgmt.confirm.deleteTitle'),
    message: t('resource.clusterMgmt.confirm.deleteMessage', { count: targets.length }),
    items: targets.map((c) => c.displayName),
    danger: true,
    confirmText: t('resource.clusterMgmt.action.delete'),
  })
  if (!confirmed) return
  mutationError.value = ''
  if (targets.length > 1) {
    try {
      const receipt = await api.submitBatchDeleteRuntimeTargets(targets.map((cluster) => cluster.clusterId))
      selected.value = []
      mutationError.value = `${t('resource.clusterMgmt.action.delete')}：${receipt.batch.status} (${receipt.batch.total_children})`
      refresh()
    } catch (err) {
      mutationError.value = err instanceof Error ? err.message : String(err)
    }
    return
  }
  const submitted: RuntimeIntentRecord[] = []
  const failures: string[] = []
  for (const cluster of targets) {
    try {
      const result = await submitWithStaleChallenge(
        api.buildDeleteIntent(cluster),
        t('resource.clusterMgmt.action.delete'),
      )
      if (result !== 'cancelled') submitted.push(result)
    } catch (err) {
      const detail = err instanceof Error ? err.message : String(err)
      failures.push(`${cluster.displayName}: ${detail}`)
    }
  }
  if (submitted.length) {
    // The latest receipt keeps the existing Operation Center navigation while
    // all prior operations remain discoverable in that center.
    lastIntent.value = submitted[submitted.length - 1]
    intentOpen.value = true
    selected.value = []
    refresh()
  }
  if (failures.length) {
    mutationError.value = failures.join('；')
  }
}

function deleteOne(cluster: ClusterSummary): void {
  deleteClusters([cluster])
}

function deleteSelected(): void {
  deleteClusters(selectedClusters.value)
}

function canMutateRow(status: ClusterStatus): boolean {
  return canMutate(status)
}

const selectedAll = computed<boolean>({
  get: () => selected.value.length > 0 && clusters.value.every((c) => selected.value.includes(c.clusterId)),
  set: (value: boolean) => {
    selected.value = value ? clusters.value.map((c) => c.clusterId) : []
  },
})
</script>

<template>
  <section class="cluster-page">
    <header class="page-header">
      <div>
        <h1>{{ t('resource.clusterMgmt.title') }}</h1>
        <p>{{ t('resource.clusterMgmt.desc') }}</p>
      </div>
      <button v-if="canCreate" class="primary-button" type="button" @click="openRegister">
        {{ t('resource.clusterMgmt.action.register') }}
      </button>
    </header>

    <ClusterSummaryCards v-if="!loading && !error" :data="clusters" :summary="summary" />

    <div class="toolbar">
      <input
        v-model="query"
        class="search-input"
        :placeholder="t('resource.clusterMgmt.filter.keyword')"
      />
      <select v-model="kindFilter" class="filter-select">
        <option v-for="opt in CLUSTER_KIND_OPTIONS" :key="opt.value" :value="opt.value">
          {{ t(opt.labelKey) }}
        </option>
      </select>
      <select v-model="statusFilter" class="filter-select">
        <option v-for="opt in CLUSTER_STATUS_OPTIONS" :key="opt.value" :value="opt.value">
          {{ t(opt.labelKey) }}
        </option>
      </select>
      <button v-if="canDelete" class="secondary-button danger" type="button" :disabled="!selected.length" @click="deleteSelected">
        {{ t('resource.clusterMgmt.action.batchDelete') }}
      </button>
      <button class="secondary-button" type="button" @click="refresh">{{ t('resource.clusterMgmt.action.refresh') }}</button>
    </div>

    <p v-if="mutationError" class="mutation-error" role="alert">{{ mutationError }}</p>

    <div v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</div>

    <div v-else-if="error" class="panel-status error" role="alert">
      <span>{{ error }}</span>
      <button class="retry-button" type="button" @click="refresh">{{ t('resource.clusterMgmt.action.retry') }}</button>
    </div>

    <div v-else-if="!clusters.length" class="panel-status empty">
      <p>{{ t('resource.clusterMgmt.empty.list') }}</p>
      <button v-if="canCreate" class="primary-button" type="button" @click="openRegister">
        {{ t('resource.clusterMgmt.action.register') }}
      </button>
    </div>

    <div v-else class="table-card">
      <table class="hnb-table">
        <thead>
          <tr>
            <th v-if="canDelete" class="col-check"><input v-model="selectedAll" type="checkbox" /></th>
            <th v-for="col in clusterListColumns" :key="col.key" :style="col.width ? { width: col.width } : undefined">
              {{ t(col.titleKey) }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="cluster in clusters" :key="cluster.clusterId">
            <td v-if="canDelete" class="col-check">
              <input v-model="selected" type="checkbox" :value="cluster.clusterId" />
            </td>
            <td>
              <button class="name-link" type="button" @click="goDetail(cluster.clusterId)">
                {{ cluster.displayName }}
              </button>
            </td>
            <td>{{ t(`resource.clusterMgmt.kind.${cluster.kind}`) }}</td>
            <td>
              <span v-if="cluster.source === 'imported'" class="source-pill imported">{{ t('resource.clusterMgmt.source.imported') }}</span>
              <span v-else class="source-pill created">{{ t('resource.clusterMgmt.source.created') }}</span>
            </td>
            <td><ClusterStatusBadge :status="cluster.status" /></td>
            <td>{{ cluster.runtimeVersion || '-' }}</td>
            <td>{{ cluster.nodeCount }}</td>
            <td>{{ cluster.updatedAt }}</td>
            <td class="row-actions">
              <router-link class="text-action" :to="`/resource/clusters/${encodeURIComponent(cluster.clusterId)}`">
                {{ t('resource.clusterMgmt.action.view') }}
              </router-link>
              <button
                v-if="canUpdate"
                class="text-action"
                type="button"
                :disabled="!canMutateRow(cluster.status) || !cluster.expectedVersion"
                @click="openUpgrade(cluster)"
              >
                {{ t('resource.clusterMgmt.action.upgrade') }}
              </button>
              <button
                v-if="canDelete"
                class="text-action danger"
                type="button"
                :disabled="!canMutateRow(cluster.status) || !cluster.expectedVersion"
                @click="deleteOne(cluster)"
              >
                {{ t('resource.clusterMgmt.action.delete') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="totalPages > 1" class="pagination-bar">
        <span class="pagination-info">{{ total }} {{ t('resource.clusterMgmt.pagination.items') }}</span>
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
            <option :value="20">20 / {{ t('resource.clusterMgmt.pagination.page') }}</option>
            <option :value="50">50 / {{ t('resource.clusterMgmt.pagination.page') }}</option>
          </select>
        </div>
      </div>
    </div>

    <ClusterRegisterWizard v-model="registerOpen" @submitted="onSubmitted" />

    <!-- 危险操作确认 -->
    <div v-if="confirmOptions" class="modal-mask" role="dialog" aria-modal="true">
      <div class="modal-card small">
        <header class="modal-header">
          <h2>{{ confirmOptions.title }}</h2>
        </header>
        <div class="modal-body">
          <p>{{ confirmOptions.message }}</p>
          <ul v-if="confirmOptions.items?.length" class="impact-list">
            <li v-for="item in confirmOptions.items" :key="item">{{ item }}</li>
          </ul>
        </div>
        <footer class="modal-footer">
          <button class="secondary-button" type="button" @click="resolveConfirm(false)">{{ t('resource.clusterMgmt.common.cancel') }}</button>
          <button class="danger-button" type="button" @click="resolveConfirm(true)">{{ confirmOptions.confirmText }}</button>
        </footer>
      </div>
    </div>

    <!-- 操作已提交（进入 Operation Center） -->
    <div v-if="intentOpen && lastIntent" class="modal-mask" role="dialog" aria-modal="true">
      <div class="modal-card small">
        <header class="modal-header">
          <h2>{{ t('resource.clusterMgmt.operation.submittedTitle') }}</h2>
        </header>
        <div class="modal-body">
          <p>{{ t('resource.clusterMgmt.operation.submittedDesc') }}</p>
          <dl class="intent-info">
            <dt>{{ t('resource.clusterMgmt.operation.intentId') }}</dt>
            <dd>{{ lastIntent.intentId }}</dd>
            <dt>{{ t('resource.clusterMgmt.operation.status') }}</dt>
            <dd>{{ lastIntent.status }}</dd>
            <template v-if="lastIntent.operationId">
              <dt>{{ t('resource.clusterMgmt.operation.operationId') }}</dt>
              <dd>{{ lastIntent.operationId }}</dd>
            </template>
          </dl>
        </div>
        <footer class="modal-footer">
          <button class="secondary-button" type="button" @click="intentOpen = false">
            {{ t('resource.clusterMgmt.operation.close') }}
          </button>
          <button
            v-if="lastIntent.operationId"
            class="primary-button"
            type="button"
            @click="goOperation(lastIntent.operationId)"
          >
            {{ t('resource.clusterMgmt.operation.track') }}
          </button>
        </footer>
      </div>
    </div>

    <!-- 升级：目标版本选择 -->
    <div v-if="upgradeModal.open && upgradeModal.cluster" class="modal-mask" role="dialog" aria-modal="true">
      <div class="modal-card small">
        <header class="modal-header">
          <h2>{{ t('resource.clusterMgmt.confirm.upgradeTitle') }}</h2>
        </header>
        <div class="modal-body">
          <p>{{ t('resource.clusterMgmt.confirm.upgradeMessage', { name: upgradeModal.cluster.displayName }) }}</p>
          <dl class="intent-info">
            <dt>{{ t('resource.clusterMgmt.form.targetVersion') }}</dt>
            <dd>{{ upgradeModal.cluster.runtimeVersion || '-' }}</dd>
          </dl>
          <label class="upgrade-version-field">
            <span>{{ t('resource.clusterMgmt.confirm.upgradeTargetVersion') }}</span>
            <input
              v-model="upgradeModal.version"
              class="version-input"
              :placeholder="t('resource.clusterMgmt.confirm.upgradeTargetVersionPlaceholder')"
            />
          </label>
        </div>
        <footer class="modal-footer">
          <button class="secondary-button" type="button" @click="upgradeModal.open = false">
            {{ t('resource.clusterMgmt.common.cancel') }}
          </button>
          <button
            class="primary-button"
            type="button"
            :disabled="!upgradeModal.version.trim()"
            @click="confirmUpgrade"
          >
            {{ t('resource.clusterMgmt.action.upgrade') }}
          </button>
        </footer>
      </div>
    </div>

    <!-- STALE 风险确认 -->
    <StaleChallengeDialog
      :challenge="staleChallenge"
      :action-label="staleActionLabel"
      @confirm="resolveStaleConfirm(true)"
      @cancel="resolveStaleConfirm(false)"
    />
  </section>
</template>

<style scoped>
.cluster-page {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-md);
  color: var(--hnb-color-text-primary);
}
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--hnb-space-md);
}
.page-header h1 { margin: 0; font-size: var(--hnb-font-size-title); }
.page-header p { margin: var(--hnb-space-xs) 0 0; color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-body); }
.toolbar {
  display: flex;
  gap: var(--hnb-space-sm);
  align-items: center;
  flex-wrap: wrap;
}
.search-input {
  flex: 1;
  min-width: 220px;
  padding: 8px 12px;
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-elevated);
  color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-body);
}
.filter-select {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-elevated);
  color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-body);
}
.primary-button {
  padding: 8px 18px;
  border: 0;
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-primary);
  color: #fff;
  cursor: pointer;
  font-size: var(--hnb-font-size-body);
}
.primary-button:disabled { opacity: 0.55; cursor: not-allowed; }
.secondary-button {
  padding: 8px 18px;
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-surface);
  color: var(--hnb-color-text-primary);
  cursor: pointer;
  font-size: var(--hnb-font-size-body);
}
.secondary-button:disabled { opacity: 0.55; cursor: not-allowed; }
.secondary-button.danger { color: var(--hnb-color-status-danger); border-color: var(--hnb-color-status-danger); }
.danger-button {
  padding: 8px 18px;
  border: 0;
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-status-danger);
  color: #fff;
  cursor: pointer;
  font-size: var(--hnb-font-size-body);
}
.mutation-error { margin: 0; color: var(--hnb-color-status-danger); font-size: var(--hnb-font-size-body); }
.panel-status { padding: var(--hnb-space-xl); text-align: center; color: var(--hnb-color-text-tertiary); }
.panel-status.error { color: var(--hnb-color-status-danger); display: flex; flex-direction: column; gap: var(--hnb-space-sm); align-items: center; }
.panel-status.empty { display: flex; flex-direction: column; gap: var(--hnb-space-md); align-items: center; }
.retry-button {
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-surface);
  color: var(--hnb-color-text-primary);
  padding: 4px 12px;
  cursor: pointer;
}
.table-card {
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-lg);
  background: var(--hnb-color-bg-surface);
  overflow-x: auto;
}
.hnb-table { width: 100%; border-collapse: collapse; font-size: var(--hnb-font-size-body); }
.hnb-table th {
  text-align: left;
  font-weight: var(--hnb-font-weight-semibold);
  color: var(--hnb-color-text-secondary);
  font-size: var(--hnb-font-size-caption);
  padding: var(--hnb-space-sm) var(--hnb-space-md);
  border-bottom: 1px solid var(--hnb-color-divider);
  white-space: nowrap;
}
.hnb-table td {
  padding: var(--hnb-space-sm) var(--hnb-space-md);
  border-bottom: 1px solid var(--hnb-color-divider);
  white-space: nowrap;
}
.col-check { width: 36px; }
.name-link {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary);
  cursor: pointer;
  font-size: var(--hnb-font-size-body);
  padding: 0;
  font-weight: var(--hnb-font-weight-semibold);
}
.source-pill {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: var(--hnb-font-size-caption);
}
.source-pill.created { background: color-mix(in srgb, var(--hnb-color-status-info) 14%, transparent); color: var(--hnb-color-status-info); }
.source-pill.imported { background: color-mix(in srgb, var(--hnb-color-text-tertiary) 16%, transparent); color: var(--hnb-color-text-secondary); }
.row-actions { display: flex; gap: var(--hnb-space-sm); align-items: center; }
.text-action {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary);
  cursor: pointer;
  padding: 0;
  font-size: var(--hnb-font-size-body);
  text-decoration: none;
}
.text-action.danger { color: var(--hnb-color-status-danger); }
.text-action:disabled { opacity: 0.45; cursor: not-allowed; }
.pagination-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--hnb-space-sm) var(--hnb-space-md);
}
.pagination-info { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-tertiary); }
.pagination-controls { display: flex; gap: var(--hnb-space-xs); align-items: center; }
.page-button, .page-num {
  min-width: 28px;
  height: 28px;
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-sm);
  background: var(--hnb-color-bg-surface);
  color: var(--hnb-color-text-primary);
  cursor: pointer;
  font-size: var(--hnb-font-size-caption);
}
.page-num.active { background: var(--hnb-color-primary); border-color: var(--hnb-color-primary); color: #fff; }
.page-button:disabled { opacity: 0.45; cursor: not-allowed; }
.page-size {
  margin-left: var(--hnb-space-sm);
  padding: 4px 6px;
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-sm);
  background: var(--hnb-color-bg-elevated);
  color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-caption);
}
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 48px 16px;
  z-index: 1000;
}
.modal-card {
  width: 100%;
  max-width: 480px;
  background: var(--hnb-color-bg-surface);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-lg);
  box-shadow: var(--hnb-shadow-4);
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-md);
  padding: var(--hnb-space-lg);
}
.modal-card.small { max-width: 400px; }
.modal-header h2 { margin: 0; font-size: var(--hnb-font-size-title); }
.modal-body { display: flex; flex-direction: column; gap: var(--hnb-space-sm); }
.modal-body p { margin: 0; color: var(--hnb-color-text-secondary); }
.modal-footer { display: flex; justify-content: flex-end; gap: var(--hnb-space-sm); }
.impact-list {
  margin: 0;
  padding-left: var(--hnb-space-md);
  max-height: 160px;
  overflow-y: auto;
  font-size: var(--hnb-font-size-body);
}
.intent-info { display: grid; grid-template-columns: 120px 1fr; gap: var(--hnb-space-xs) var(--hnb-space-md); margin: 0; }
.intent-info dt { color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.intent-info dd { margin: 0; word-break: break-all; }
.upgrade-version-field {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs);
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-secondary);
}
.version-input {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-sm);
  background: var(--hnb-color-bg-elevated);
  color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-body);
  font-family: inherit;
}
.stale-impact { margin: 0; color: var(--hnb-color-text-tertiary); font-size: var(--hnb-font-size-caption); }
.stale-ack {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-sm);
  font-size: var(--hnb-font-size-body);
  color: var(--hnb-color-text-secondary);
}
.stale-ack input { width: 16px; height: 16px; accent-color: var(--hnb-color-primary); }
@media (max-width: 768px) {
  .page-header { flex-direction: column; }
}
</style>
