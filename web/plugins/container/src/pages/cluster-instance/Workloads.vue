<template>
  <HNBPageShell :title="t('container.workloads.title')" :description="t('container.workloads.desc')">
    <div class="wl__toolbar">
      <HNBSelectInput
        v-model="clusterFilter"
        :options="clusterOptions"
        :placeholder="t('container.workloads.clusterAll')"
        class="wl__filter"
        @update:model-value="loadWorkloads"
      />
      <HNBSelectInput
        v-model="namespaceFilter"
        :options="namespaceOptions"
        :placeholder="t('container.workloads.namespaceAll')"
        class="wl__filter"
        @update:model-value="loadWorkloads"
      />
      <HNBButton variant="ghost" size="small" @click="loadAll">{{ t('container.workloads.refresh') }}</HNBButton>
    </div>

    <HNBTabs v-model="activeTab" :tabs="tabDefs" :ariaLabel="t('container.workloads.tabsLabel')">
      <template v-for="tab in tabDefs" :key="tab.id" #[`panel-${tab.id}`]>
        <HNBTable
          :columns="currentColumns"
          :data="workloadMap[tab.id] ?? []"
          :loading="loadingMap[tab.id]"
          :error="errorMap[tab.id]"
          :empty-title="t('container.workloads.empty')"
          row-key="uid"
          @error-retry="loadWorkloads"
        />
      </template>
    </HNBTabs>
  </HNBPageShell>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  HNBPageShell,
  HNBTable,
  HNBButton,
  HNBSelectInput,
  HNBTabs,
  StatusBadge,
  type HNBTab,
  type StatusSemantic,
  type HNBTableColumn,
  type HNBSelectOption,
} from '@hnb/ui-kit'
import {
  getContainerContextStore,
  listWorkspaceClusters,
  listNamespaces,
  listWorkloads,
  type WorkloadType,
  type ContainerCluster,
  type ContainerNamespace,
} from '../../api/containerApi'

const { t } = useI18n()
const contextStore = getContainerContextStore()

const WORKLOAD_TABS: WorkloadType[] = ['deployment', 'statefulset', 'daemonset', 'job', 'cronjob', 'pod']

const activeTab = ref<WorkloadType>('deployment')
const clusters = ref<ContainerCluster[]>([])
const namespaces = ref<ContainerNamespace[]>([])
const clusterFilter = ref('')
const namespaceFilter = ref('')

const workloadMap = ref<Record<string, Record<string, any>[]>>({})
const loadingMap = ref<Record<string, boolean>>({})
const errorMap = ref<Record<string, string>>({})

for (const t of WORKLOAD_TABS) {
  workloadMap.value[t] = []
  loadingMap.value[t] = false
  errorMap.value[t] = ''
}

const tabDefs = computed<HNBTab[]>(() =>
  WORKLOAD_TABS.map((id) => ({ id, label: t(`container.workloads.tab.${id}`) })),
)

const clusterOptions = computed<HNBSelectOption[]>(() =>
  clusters.value.map((c) => ({
    label: (c.display_name || c.name) + (c.shared ? ' (shared)' : ''),
    value: c.id,
  })),
)

const namespaceOptions = computed<HNBSelectOption[]>(() => {
  const nsMap = new Map<string, string>()
  for (const ns of namespaces.value) {
    nsMap.set(ns.name, ns.name)
  }
  return Array.from(nsMap.entries()).map(([id, name]) => ({ label: name, value: id }))
})

const currentColumns = computed<HNBTableColumn<Record<string, any>>[]>(() => {
  const type = activeTab.value
  const cols: HNBTableColumn[] = [
    { key: 'name', title: t('container.workloads.colName'), width: '200px' },
    { key: 'namespace', title: t('container.workloads.colNamespace'), width: '150px' },
  ]
  switch (type) {
    case 'deployment':
      cols.push({ key: 'desired', title: t('container.workloads.colDesired'), width: '80px' })
      cols.push({ key: 'available', title: t('container.workloads.colAvailable'), width: '80px' })
      break
    case 'statefulset':
      cols.push({ key: 'desired', title: t('container.workloads.colDesired'), width: '80px' })
      cols.push({ key: 'ready', title: t('container.workloads.colReady'), width: '80px' })
      break
    case 'daemonset':
      cols.push({ key: 'desired', title: t('container.workloads.colDesired'), width: '80px' })
      cols.push({ key: 'current', title: t('container.workloads.colCurrent'), width: '80px' })
      break
    case 'job':
      cols.push({ key: 'completions', title: t('container.workloads.colCompletions'), width: '100px' })
      cols.push({ key: 'duration', title: t('container.workloads.colDuration'), width: '100px' })
      break
    case 'cronjob':
      cols.push({ key: 'schedule', title: t('container.workloads.colSchedule'), width: '120px' })
      cols.push({ key: 'lastSchedule', title: t('container.workloads.colLastSchedule'), width: '160px' })
      break
    case 'pod':
      cols.push({ key: 'node', title: t('container.workloads.colNode'), width: '160px' })
      cols.push({ key: 'restarts', title: t('container.workloads.colRestarts'), width: '80px' })
      break
  }
  cols.push({
    key: 'status', title: t('container.workloads.colStatus'), width: '130px',
    render: (row: any) => h(StatusBadge, { semantic: statusSemantic(row.status), label: statusLabel(row.status) }),
  })
  cols.push({
    key: 'images', title: t('container.workloads.colImages'), width: '200px',
    render: (row: any) => (row.images || []).join(', '),
  })
  cols.push({
    key: 'age', title: t('container.workloads.colAge'), width: '140px',
    render: (row: any) => formatAge(row.age),
  })
  return cols
})

function statusSemantic(status: string): StatusSemantic {
  if (['Running', 'Available', 'Ready', 'True'].includes(status)) return 'success'
  if (['Pending', 'Wait', 'Unknown'].includes(status)) return 'warning'
  if (['Failed', 'Error', 'CrashLoopBackOff', 'False'].includes(status)) return 'error'
  return 'default'
}

function statusLabel(status: string): string {
  return status || '-'
}

function formatAge(ts: string): string {
  if (!ts) return '-'
  const diff = Date.now() - new Date(ts).getTime()
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.floor(hours / 24)
  return `${days}d`
}

function normalizeItem(item: any, type: WorkloadType): Record<string, any> {
  const meta = item?.metadata || {}
  const spec = item?.spec || {}
  const status = item?.status || {}
  const containers = spec?.template?.spec?.containers || spec?.containers || []
  const images = containers.map((c: any) => c.image).filter(Boolean)

  const base = {
    uid: meta.uid || meta.name,
    name: meta.name,
    namespace: meta.namespace || '',
    status: statusPhase(status, type),
    images,
    age: meta.creationTimestamp,
  }

  switch (type) {
    case 'deployment':
      return { ...base, desired: spec?.replicas ?? 0, available: status?.availableReplicas ?? 0 }
    case 'statefulset':
      return { ...base, desired: spec?.replicas ?? 0, ready: status?.readyReplicas ?? 0 }
    case 'daemonset':
      return { ...base, desired: status?.desiredNumberScheduled ?? 0, current: status?.currentNumberScheduled ?? 0 }
    case 'job':
      return { ...base, completions: `${status?.succeeded ?? 0}/${spec?.completions ?? 1}`, duration: jobDuration(status) }
    case 'cronjob':
      return { ...base, schedule: spec?.schedule || '-', lastSchedule: status?.lastScheduleTime || '-' }
    case 'pod':
      return { ...base, node: spec?.nodeName || '', restarts: podRestarts(status) }
    default:
      return base
  }
}

function statusPhase(status: any, type: WorkloadType): string {
  if (type === 'pod') return status?.phase || 'Unknown'
  if (type === 'job') {
    if (status?.succeeded > 0) return 'Succeeded'
    if (status?.failed > 0) return 'Failed'
    if (status?.active > 0) return 'Running'
    return 'Pending'
  }
  const conditions = status?.conditions ?? []
  for (const c of conditions) {
    if (c.type === 'Available') return c.status
    if (c.type === 'Ready') return c.status
  }
  return 'Unknown'
}

function jobDuration(status: any): string {
  if (!status?.startTime) return '-'
  if (status?.completionTime) {
    const diff = new Date(status.completionTime).getTime() - new Date(status.startTime).getTime()
    const seconds = Math.floor(diff / 1000)
    if (seconds < 60) return `${seconds}s`
    return `${Math.floor(seconds / 60)}m${seconds % 60}s`
  }
  const diff = Date.now() - new Date(status.startTime).getTime()
  return `${Math.floor(diff / 60000)}m`
}

function podRestarts(status: any): number {
  const containerStatuses = status?.containerStatuses ?? []
  let total = 0
  for (const cs of containerStatuses) {
    total += cs?.restartCount ?? 0
  }
  return total
}

async function loadClusters(): Promise<void> {
  try {
    clusters.value = await listWorkspaceClusters()
  } catch { /* ignore */ }
}

async function loadNamespaces(): Promise<void> {
  try {
    namespaces.value = await listNamespaces({ clusterId: clusterFilter.value || undefined })
  } catch { /* ignore */ }
}

async function loadWorkloads(): Promise<void> {
  const type = activeTab.value
  const clusterId = clusterFilter.value
  if (!clusterId) return

  loadingMap.value[type] = true
  errorMap.value[type] = ''
  try {
    const items = await listWorkloads({
      clusterId,
      type,
      namespace: namespaceFilter.value || undefined,
    })
    workloadMap.value[type] = (items as any[]).map((item) => normalizeItem(item, type))
  } catch (e: any) {
    errorMap.value[type] = e?.message || t('container.workloads.loadError')
    workloadMap.value[type] = []
  } finally {
    loadingMap.value[type] = false
  }
}

async function loadAll(): Promise<void> {
  await Promise.all([loadClusters(), loadNamespaces()])
  if (clusterFilter.value) {
    await loadWorkloads()
  }
}

watch(activeTab, () => {
  if (clusterFilter.value) loadWorkloads()
})

watch(
  () => contextStore.current.spaceId,
  () => {
    clusterFilter.value = ''
    namespaceFilter.value = ''
    for (const type of WORKLOAD_TABS) {
      workloadMap.value[type] = []
      errorMap.value[type] = ''
    }
    loadAll()
  },
)

onMounted(loadAll)
</script>

<style scoped>
.wl__toolbar {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-sm);
  margin-bottom: var(--hnb-space-md);
}
.wl__filter {
  width: 220px;
}
</style>