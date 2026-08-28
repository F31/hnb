<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, StatusBadge } from '@hnb/ui-kit'
import { listNamespaces, listWorkspaceClusters, type ContainerCluster } from '../../api/containerApi'
import { listWorkloadEvents, type WorkloadEvent } from '../../api/eventsApi'
import { listLogPods, listLogWorkloads, type LogWorkload, type LogWorkloadType } from '../../api/logsApi'

const { t } = useI18n()
const workloadTypes: LogWorkloadType[] = ['deployment', 'statefulset', 'daemonset', 'job', 'cronjob']
const clusters = ref<ContainerCluster[]>([])
const namespaces = ref<string[]>([])
const workloads = ref<LogWorkload[]>([])
const events = ref<WorkloadEvent[]>([])
const clusterId = ref('')
const namespace = ref('')
const workloadType = ref<LogWorkloadType>('deployment')
const workloadName = ref('')
const loading = ref(false)
const loadError = ref('')
const page = ref(1)
const pageSize = ref(10)
const jumpPage = ref('')
let sequence = 0

const clusterOptions = computed(() => clusters.value.map((item) => ({ value: item.id, label: item.display_name || item.name })))
const pageCount = computed(() => Math.max(1, Math.ceil(events.value.length / pageSize.value)))
const pageStart = computed(() => events.value.length ? (page.value - 1) * pageSize.value + 1 : 0)
const pageEnd = computed(() => Math.min(page.value * pageSize.value, events.value.length))
const pageNumbers = computed(() => Array.from({ length: pageCount.value }, (_, index) => index + 1))
const pagedEvents = computed(() => events.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

function formatDate(value: string): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function eventSemantic(type: string): 'success' | 'warning' | 'default' {
  if (type.toLowerCase() === 'normal') return 'success'
  if (type.toLowerCase() === 'warning') return 'warning'
  return 'default'
}

async function onClusterChange(): Promise<void> {
  const current = ++sequence
  namespaces.value = []
  workloads.value = []
  events.value = []
  namespace.value = ''
  workloadName.value = ''
  loadError.value = ''
  if (!clusterId.value) return
  loading.value = true
  try {
    const items = await listNamespaces({ clusterId: clusterId.value })
    if (current !== sequence) return
    namespaces.value = Array.from(new Set(['argocd', 'default', ...items.map((item) => item.name)]))
    namespace.value = namespaces.value.includes('argocd') ? 'argocd' : namespaces.value[0] || ''
    await loadWorkloads()
  } catch (error) {
    if (current === sequence) loadError.value = error instanceof Error ? error.message : t('container.events.error.namespaces')
  } finally {
    if (current === sequence) loading.value = false
  }
}

async function loadWorkloads(): Promise<void> {
  const current = ++sequence
  workloads.value = []
  events.value = []
  workloadName.value = ''
  page.value = 1
  loadError.value = ''
  if (!clusterId.value || !namespace.value) return
  loading.value = true
  try {
    const items = await listLogWorkloads(clusterId.value, namespace.value, workloadType.value)
    if (current !== sequence) return
    workloads.value = items
    workloadName.value = items.find((item) => item.name === 'argocd-dex-server')?.name ?? items[0]?.name ?? ''
    if (workloadName.value) await loadEvents()
  } catch (error) {
    if (current === sequence) loadError.value = error instanceof Error ? error.message : t('container.events.error.workloads')
  } finally {
    if (current === sequence) loading.value = false
  }
}

async function loadEvents(): Promise<void> {
  const current = ++sequence
  events.value = []
  page.value = 1
  loadError.value = ''
  const workload = workloads.value.find((item) => item.name === workloadName.value)
  if (!workload || !clusterId.value || !namespace.value) {
    loading.value = false
    return
  }
  loading.value = true
  try {
    const hasPodSelector = Object.keys(workload.selector).length > 0 || Boolean(workload.pods?.length)
    const pods = hasPodSelector ? await listLogPods(clusterId.value, namespace.value, workload) : []
    if (current !== sequence) return
    const items = await listWorkloadEvents(clusterId.value, namespace.value, [workload.name, ...pods.map((pod) => pod.name)])
    if (current !== sequence) return
    events.value = items
  } catch (error) {
    if (current === sequence) loadError.value = error instanceof Error ? error.message : t('container.events.error.events')
  } finally {
    if (current === sequence) loading.value = false
  }
}

function changePageSize(event: Event): void { pageSize.value = Number((event.target as HTMLSelectElement).value); page.value = 1 }
function jumpToPage(): void { const target = Number(jumpPage.value); if (Number.isInteger(target)) page.value = Math.max(1, Math.min(pageCount.value, target)); jumpPage.value = '' }

async function initialize(): Promise<void> {
  try {
    clusters.value = await listWorkspaceClusters()
    clusterId.value = clusters.value[0]?.id ?? ''
    await onClusterChange()
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.events.error.clusters')
  }
}

watch(pageCount, (count) => { if (page.value > count) page.value = count })
onMounted(initialize)
</script>

<template>
  <HNBPageShell :title="t('container.events.title')">
    <template #actions><a class="help-link" href="https://docs.hnb.example.io/container/events" target="_blank" rel="noopener noreferrer">? {{ t('container.events.help') }}</a></template>
    <section class="query-panel" :aria-label="t('container.events.queryTitle')">
      <label><span>{{ t('container.events.field.cluster') }}</span><select v-model="clusterId" @change="onClusterChange"><option value="" disabled>{{ t('container.events.placeholder') }}</option><option v-for="item in clusterOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
      <label><span>{{ t('container.events.field.namespace') }}</span><select v-model="namespace" :disabled="!clusterId" @change="loadWorkloads"><option value="" disabled>{{ t('container.events.placeholder') }}</option><option v-for="item in namespaces" :key="item" :value="item">{{ item }}</option></select></label>
      <label><span>{{ t('container.events.field.workloadType') }}</span><select v-model="workloadType" :disabled="!namespace" @change="loadWorkloads"><option v-for="type in workloadTypes" :key="type" :value="type">{{ t(`container.events.workloadType.${type}`) }}</option></select></label>
      <label><span>{{ t('container.events.field.workload') }}</span><select v-model="workloadName" :disabled="!workloads.length" @change="loadEvents"><option value="">{{ t('container.events.placeholder') }}</option><option v-for="item in workloads" :key="item.name" :value="item.name">{{ item.name }}</option></select></label>
      <button class="refresh-button" type="button" :disabled="loading || !workloadName" :aria-label="t('container.events.refresh')" :title="t('container.events.refresh')" @click="loadEvents">↻</button>
    </section>
    <p v-if="loadError" class="error" role="alert">{{ loadError }}</p><p v-if="loading" class="loading" role="status">{{ t('container.events.loading') }}</p>
    <div class="table-wrap"><table class="event-table"><thead><tr><th>{{ t('container.events.columns.updatedAt') }}</th><th>{{ t('container.events.columns.type') }}</th><th>{{ t('container.events.columns.object') }}</th><th>{{ t('container.events.columns.reason') }}</th><th>{{ t('container.events.columns.message') }}</th></tr></thead><tbody><tr v-for="item in pagedEvents" :key="item.id"><td>{{ formatDate(item.updatedAt) }}</td><td><StatusBadge :label="item.type" :semantic="eventSemantic(item.type)" /></td><td><span class="ellipsis" :title="item.object">{{ item.object }}</span></td><td>{{ item.reason || '--' }}</td><td><span class="event-message" :title="item.message">{{ item.message || '--' }}</span></td></tr><tr v-if="!loading && !pagedEvents.length"><td colspan="5" class="empty">{{ t('container.events.empty') }}</td></tr></tbody></table></div>
    <footer v-if="!loading && events.length" class="pagination"><span>{{ t('container.events.pagination.range', { start: pageStart, end: pageEnd, total: events.length }) }}</span><div class="page-buttons"><button type="button" :disabled="page <= 1" @click="page--">‹</button><button v-for="number in pageNumbers" :key="number" type="button" :class="{ active: number === page }" @click="page = number">{{ number }}</button><button type="button" :disabled="page >= pageCount" @click="page++">›</button></div><div class="page-jump"><select :value="pageSize" @change="changePageSize"><option v-for="size in [10, 20, 50]" :key="size" :value="size">{{ t('container.events.pagination.pageSize', { size }) }}</option></select><span>{{ t('container.events.pagination.jump') }}</span><input v-model="jumpPage" type="number" min="1" :max="pageCount" @keyup.enter="jumpToPage"><span>{{ t('container.events.pagination.pageUnit', { pages: pageCount }) }}</span></div></footer>
  </HNBPageShell>
</template>

<style scoped>
.help-link{color:var(--hnb-color-primary);font-size:13px;text-decoration:none}.query-panel{display:grid;grid-template-columns:repeat(4,minmax(150px,1fr)) 34px;align-items:end;gap:12px;padding:14px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface)}.query-panel label{display:flex;flex-direction:column;gap:6px;color:var(--hnb-color-text-secondary);font-size:13px}.query-panel select{min-height:34px;padding:6px 9px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.refresh-button{width:34px;height:34px;border:1px solid var(--hnb-color-border);border-radius:50%;background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary);cursor:pointer}.refresh-button:disabled{cursor:not-allowed;opacity:.5}.error,.loading{margin:0;padding:8px 10px;border-radius:var(--hnb-radius-sm);font-size:13px}.error{color:var(--hnb-color-status-danger);background:var(--hnb-color-status-danger-surface)}.loading{color:var(--hnb-color-text-secondary)}.table-wrap{overflow-x:auto;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md)}.event-table{width:100%;min-width:960px;border-collapse:collapse;table-layout:fixed;font-size:13px}.event-table th,.event-table td{padding:11px 12px;border-bottom:1px solid var(--hnb-color-divider);text-align:left;vertical-align:top}.event-table th{background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary);white-space:nowrap}.event-table th:nth-child(1){width:170px}.event-table th:nth-child(2){width:100px}.event-table th:nth-child(3){width:190px}.event-table th:nth-child(4){width:130px}.ellipsis{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.event-message{display:-webkit-box;overflow:hidden;overflow-wrap:anywhere;-webkit-box-orient:vertical;-webkit-line-clamp:2;line-height:1.55}.empty{padding:48px!important;text-align:center!important;color:var(--hnb-color-text-tertiary)}.pagination{display:grid;grid-template-columns:1fr auto 1fr;align-items:center;gap:12px;color:var(--hnb-color-text-secondary);font-size:13px}.page-buttons,.page-jump{display:flex;align-items:center;gap:5px}.page-buttons button{min-width:30px;height:30px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary)}.page-buttons button.active{background:var(--hnb-color-primary);color:var(--hnb-color-text-on-accent)}.page-jump{justify-content:flex-end}.page-jump select,.page-jump input{height:30px;padding:4px 7px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.page-jump input{width:50px}@media(max-width:900px){.query-panel{grid-template-columns:1fr 1fr}.pagination{grid-template-columns:1fr}.page-jump{justify-content:flex-start}}@media(max-width:560px){.query-panel{grid-template-columns:1fr}.refresh-button{justify-self:end}}
</style>
