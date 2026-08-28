<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton, HNBPageShell } from '@hnb/ui-kit'
import { listNamespaces, listWorkspaceClusters, type ContainerCluster } from '../../api/containerApi'
import {
  fetchContainerLogs,
  listLogPods,
  listLogWorkloads,
  type LogPod,
  type LogWorkload,
  type LogWorkloadType,
} from '../../api/logsApi'

interface LogTabState {
  id: number
  name: string
  clusterId: string
  namespace: string
  workloadType: LogWorkloadType
  workloadName: string
  podName: string
  container: string
  tailLines: number
  autoRefresh: boolean
  namespaces: string[]
  workloads: LogWorkload[]
  pods: LogPod[]
  containers: string[]
  logs: string
  loading: boolean
  error: string
  sequence: number
}

const { t } = useI18n()
const clusters = ref<ContainerCluster[]>([])
const nextTabId = ref(1)
const tabs = ref<LogTabState[]>([])
const activeTabId = ref(0)
const pageError = ref('')
const notice = ref('')
const viewerEl = ref<HTMLElement | null>(null)
const fullscreen = ref(false)
let pollingTimer: number | undefined

const workloadTypes: LogWorkloadType[] = ['deployment', 'statefulset', 'daemonset', 'job', 'cronjob']
const activeTab = computed(() => tabs.value.find((tab) => tab.id === activeTabId.value) ?? tabs.value[0])
const clusterOptions = computed(() => clusters.value.map((cluster) => ({ value: cluster.id, label: cluster.display_name || cluster.name })))

function makeTab(): LogTabState {
  const id = nextTabId.value++
  return {
    id, name: t('container.logs.tabName', { number: id }), clusterId: '', namespace: '', workloadType: 'deployment',
    workloadName: '', podName: '', container: '', tailLines: 100, autoRefresh: false,
    namespaces: [], workloads: [], pods: [], containers: [], logs: '', loading: false, error: '', sequence: 0,
  }
}

function addTab(): void {
  const tab = makeTab()
  tabs.value.push(tab)
  activeTabId.value = tab.id
}

function closeTab(id: number): void {
  const index = tabs.value.findIndex((tab) => tab.id === id)
  if (index < 0) return
  tabs.value.splice(index, 1)
  if (!tabs.value.length) {
    addTab()
    return
  }
  if (activeTabId.value === id) activeTabId.value = tabs.value[Math.min(index, tabs.value.length - 1)].id
}

function resetAfterCluster(tab: LogTabState): void {
  tab.namespace = ''
  tab.workloadName = ''
  tab.podName = ''
  tab.container = ''
  tab.namespaces = []
  tab.workloads = []
  tab.pods = []
  tab.containers = []
  tab.logs = ''
  tab.error = ''
}

function resetAfterNamespace(tab: LogTabState): void {
  tab.workloadName = ''
  tab.podName = ''
  tab.container = ''
  tab.workloads = []
  tab.pods = []
  tab.containers = []
  tab.logs = ''
  tab.error = ''
}

async function onClusterChange(tab: LogTabState): Promise<void> {
  resetAfterCluster(tab)
  if (!tab.clusterId) return
  const sequence = ++tab.sequence
  try {
    const items = await listNamespaces({ clusterId: tab.clusterId })
    if (sequence !== tab.sequence) return
    tab.namespaces = Array.from(new Set(['default', 'argocd', ...items.map((item) => item.name)]))
  } catch (error) {
    tab.error = error instanceof Error ? error.message : t('container.logs.error.namespaces')
  }
}

async function loadWorkloads(tab: LogTabState): Promise<void> {
  resetAfterNamespace(tab)
  if (!tab.clusterId || !tab.namespace) return
  const sequence = ++tab.sequence
  tab.loading = true
  try {
    const items = await listLogWorkloads(tab.clusterId, tab.namespace, tab.workloadType)
    if (sequence !== tab.sequence) return
    tab.workloads = items
  } catch (error) {
    tab.error = error instanceof Error ? error.message : t('container.logs.error.workloads')
  } finally {
    if (sequence === tab.sequence) tab.loading = false
  }
}

async function loadPods(tab: LogTabState): Promise<void> {
  tab.podName = ''
  tab.container = ''
  tab.pods = []
  tab.containers = []
  tab.logs = ''
  const workload = tab.workloads.find((item) => item.name === tab.workloadName)
  if (!workload) return
  const sequence = ++tab.sequence
  tab.loading = true
  try {
    const items = await listLogPods(tab.clusterId, tab.namespace, workload)
    if (sequence !== tab.sequence) return
    tab.pods = items
  } catch (error) {
    tab.error = error instanceof Error ? error.message : t('container.logs.error.pods')
  } finally {
    if (sequence === tab.sequence) tab.loading = false
  }
}

function onPodChange(tab: LogTabState): void {
  tab.container = ''
  tab.logs = ''
  tab.containers = [...(tab.pods.find((pod) => pod.name === tab.podName)?.containers ?? [])]
}

function onContainerChange(tab: LogTabState): void {
  tab.logs = ''
  if (tab.container) tab.name = tab.container
}

function queryReady(tab: LogTabState): boolean {
  return Boolean(tab.clusterId && tab.namespace && tab.workloadName && tab.podName && tab.container)
}

async function refreshLogs(tab: LogTabState): Promise<void> {
  if (!queryReady(tab) || tab.loading) return
  const sequence = ++tab.sequence
  tab.loading = true
  tab.error = ''
  try {
    const text = await fetchContainerLogs({
      clusterId: tab.clusterId, namespace: tab.namespace, pod: tab.podName, container: tab.container, tailLines: tab.tailLines,
    })
    if (sequence !== tab.sequence) return
    tab.logs = text
  } catch (error) {
    tab.error = error instanceof Error ? error.message : t('container.logs.error.logs')
  } finally {
    if (sequence === tab.sequence) tab.loading = false
  }
}

function downloadLogs(tab: LogTabState): void {
  if (!tab.logs || !queryReady(tab)) return
  const blobUrl = URL.createObjectURL(new Blob([tab.logs], { type: 'text/plain;charset=utf-8' }))
  const anchor = document.createElement('a')
  anchor.href = blobUrl
  anchor.download = `${tab.namespace}-${tab.podName}-${tab.container}.log`
  anchor.click()
  URL.revokeObjectURL(blobUrl)
  showNotice(t('container.logs.message.downloaded'))
}

async function copyLogs(tab: LogTabState): Promise<void> {
  if (!tab.logs) return
  try {
    if (!navigator.clipboard?.writeText) throw new Error('clipboard API unavailable')
    await navigator.clipboard.writeText(tab.logs)
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = tab.logs
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    textarea.remove()
  }
  showNotice(t('container.logs.message.copied'))
}

async function toggleFullscreen(): Promise<void> {
  if (!viewerEl.value) return
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await viewerEl.value.requestFullscreen()
  } catch (error) {
    activeTab.value.error = error instanceof Error ? error.message : String(error)
  }
}

function showNotice(message: string): void {
  notice.value = message
  window.setTimeout(() => { if (notice.value === message) notice.value = '' }, 2200)
}

async function initialize(): Promise<void> {
  tabs.value = [makeTab()]
  activeTabId.value = tabs.value[0].id
  try {
    clusters.value = await listWorkspaceClusters()
  } catch (error) {
    pageError.value = error instanceof Error ? error.message : t('container.logs.error.clusters')
  }
  pollingTimer = window.setInterval(() => {
    for (const tab of tabs.value) if (tab.autoRefresh && queryReady(tab) && !tab.loading) void refreshLogs(tab)
  }, 5000)
  document.addEventListener('fullscreenchange', onFullscreenChange)
}

function onFullscreenChange(): void {
  fullscreen.value = Boolean(document.fullscreenElement)
}

onMounted(initialize)
onBeforeUnmount(() => {
  if (pollingTimer) window.clearInterval(pollingTimer)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
})
</script>

<template>
  <HNBPageShell :title="t('container.logs.title')">
    <template #actions><a class="help-link" href="https://docs.hnb.example.io/container/logs" target="_blank" rel="noopener noreferrer">? {{ t('container.logs.help') }}</a></template>
    <nav class="log-tabs" role="tablist" :aria-label="t('container.logs.title')">
      <button v-for="tab in tabs" :key="tab.id" class="log-tab" :class="{ active: activeTabId === tab.id }" type="button" role="tab" :aria-selected="activeTabId === tab.id" @click="activeTabId = tab.id"><span>{{ tab.name }}</span><span class="tab-close" role="button" :aria-label="t('container.logs.closeTab')" @click.stop="closeTab(tab.id)">×</span></button>
      <button class="add-tab" type="button" :aria-label="t('container.logs.newTab')" :title="t('container.logs.newTab')" @click="addTab">+</button>
    </nav>

    <p v-if="pageError" class="log-error" role="alert">{{ pageError }}</p>
    <p v-if="notice" class="log-notice" role="status">{{ notice }}</p>

    <template v-if="activeTab">
      <section class="query-panel" :aria-label="activeTab.name">
        <label><span>{{ t('container.logs.field.cluster') }}</span><select v-model="activeTab.clusterId" @change="onClusterChange(activeTab)"><option value="">{{ t('container.logs.placeholder') }}</option><option v-for="cluster in clusterOptions" :key="cluster.value" :value="cluster.value">{{ cluster.label }}</option></select></label>
        <label><span>{{ t('container.logs.field.namespace') }}</span><select v-model="activeTab.namespace" :disabled="!activeTab.clusterId" @change="loadWorkloads(activeTab)"><option value="">{{ t('container.logs.placeholder') }}</option><option v-for="namespace in activeTab.namespaces" :key="namespace" :value="namespace">{{ namespace }}</option></select></label>
        <label><span>{{ t('container.logs.field.workloadType') }}</span><select v-model="activeTab.workloadType" :disabled="!activeTab.namespace" @change="loadWorkloads(activeTab)"><option v-for="type in workloadTypes" :key="type" :value="type">{{ t(`container.logs.workloadType.${type}`) }}</option></select></label>
        <label><span>{{ t('container.logs.field.workload') }}</span><select v-model="activeTab.workloadName" :disabled="!activeTab.workloads.length" @change="loadPods(activeTab)"><option value="">{{ t('container.logs.placeholder') }}</option><option v-for="workload in activeTab.workloads" :key="workload.name" :value="workload.name">{{ workload.name }}</option></select></label>
        <label><span>{{ t('container.logs.field.pod') }}</span><select v-model="activeTab.podName" :disabled="!activeTab.pods.length" @change="onPodChange(activeTab)"><option value="">{{ t('container.logs.placeholder') }}</option><option v-for="pod in activeTab.pods" :key="pod.name" :value="pod.name">{{ pod.name }}</option></select></label>
        <label><span>{{ t('container.logs.field.container') }}</span><select v-model="activeTab.container" :disabled="!activeTab.containers.length" @change="onContainerChange(activeTab)"><option value="">{{ t('container.logs.placeholder') }}</option><option v-for="container in activeTab.containers" :key="container" :value="container">{{ container }}</option></select></label>
        <label><span>{{ t('container.logs.field.tailLines') }}</span><select v-model.number="activeTab.tailLines"><option :value="100">100</option><option :value="200">200</option><option :value="500">500</option></select></label>
        <div class="query-actions"><label class="auto-refresh"><input v-model="activeTab.autoRefresh" type="checkbox"><span>{{ t('container.logs.autoRefresh') }}</span></label><HNBButton :loading="activeTab.loading" :disabled="!queryReady(activeTab)" @click="refreshLogs(activeTab)">{{ t('container.logs.action.refresh') }}</HNBButton><HNBButton variant="secondary" :disabled="!activeTab.logs || !queryReady(activeTab)" @click="downloadLogs(activeTab)">{{ t('container.logs.action.download') }}</HNBButton></div>
      </section>

      <section ref="viewerEl" class="log-viewer" :class="{ fullscreen }" aria-live="polite">
        <div class="viewer-actions"><button type="button" :disabled="!activeTab.logs" :aria-label="t('container.logs.action.copy')" :title="t('container.logs.action.copy')" @click="copyLogs(activeTab)">⧉</button><button type="button" :aria-label="t(`container.logs.action.${fullscreen ? 'exitFullscreen' : 'fullscreen'}`)" :title="t(`container.logs.action.${fullscreen ? 'exitFullscreen' : 'fullscreen'}`)" @click="toggleFullscreen">⛶</button></div>
        <p v-if="activeTab.error" class="viewer-error" role="alert">{{ activeTab.error }}</p>
        <pre v-if="activeTab.logs">{{ activeTab.logs }}</pre>
      </section>
    </template>
  </HNBPageShell>
</template>

<style scoped>
.hnb-page-shell { min-height: calc(100vh - 100px); padding: 18px 20px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-surface); }
.help-link { color: var(--hnb-color-primary); font-size: 13px; text-decoration: none; }
.log-tabs { display: flex; align-items: stretch; gap: 2px; min-width: 0; overflow-x: auto; border-bottom: 1px solid var(--hnb-color-divider); }
.log-tab, .add-tab { flex: 0 0 auto; padding: 8px 12px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--hnb-color-text-secondary); cursor: pointer; }
.log-tab { display: inline-flex; align-items: center; gap: 9px; max-width: 200px; }
.log-tab > span:first-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.log-tab.active { border-bottom-color: var(--hnb-color-primary); color: var(--hnb-color-primary); font-weight: 600; }
.tab-close { font-size: 16px; line-height: 1; }
.add-tab { font-size: 20px; }
.log-error, .log-notice { margin: 0; padding: 8px 10px; border-radius: var(--hnb-radius-sm); font-size: 13px; }
.log-error { color: var(--hnb-color-status-danger); background: var(--hnb-color-status-danger-surface); }
.log-notice { color: var(--hnb-color-status-success); background: var(--hnb-color-status-success-surface); }
.query-panel { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px 16px; padding: 16px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-elevated); }
.query-panel > label { display: flex; flex-direction: column; gap: 6px; color: var(--hnb-color-text-secondary); font-size: 13px; }
.query-panel select { width: 100%; min-height: 34px; padding: 7px 9px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); }
.query-panel select:disabled { opacity: .55; cursor: not-allowed; }
.query-actions { display: flex; align-items: flex-end; justify-content: flex-end; gap: 9px; min-width: 0; }
.auto-refresh { display: inline-flex; align-items: center; gap: 7px; min-height: 34px; white-space: nowrap; color: var(--hnb-color-text-secondary); font-size: 13px; }
.auto-refresh input { accent-color: var(--hnb-color-primary); }
.log-viewer { position: relative; flex: 1; min-height: 520px; max-height: calc(100vh - 330px); overflow: auto; border: 1px solid #29344a; border-radius: var(--hnb-radius-md); background: #080c14; color: #d8e1ef; }
.log-viewer.fullscreen { max-height: none; border-radius: 0; }
.viewer-actions { position: sticky; z-index: 2; top: 8px; right: 8px; display: flex; justify-content: flex-end; gap: 5px; height: 0; padding-right: 8px; }
.viewer-actions button { width: 30px; height: 30px; border: 1px solid #31405a; border-radius: 4px; background: #121a28; color: #aebbd0; cursor: pointer; }
.viewer-actions button:disabled { opacity: .4; cursor: not-allowed; }
.log-viewer pre { min-width: max-content; margin: 0; padding: 18px 48px 18px 18px; font: 12px/1.65 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; white-space: pre; }
.viewer-error { margin: 14px; color: var(--hnb-color-status-danger); font: 12px/1.5 ui-monospace, monospace; }
@media (max-width: 1050px) { .query-panel { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 620px) { .hnb-page-shell { padding: 14px 12px; }.query-panel { grid-template-columns: 1fr; }.query-actions { justify-content: flex-start; flex-wrap: wrap; }.log-viewer { min-height: 440px; max-height: none; } }
</style>
