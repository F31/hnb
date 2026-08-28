<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { parse, stringify } from 'yaml'
import { HNBButton, HNBConfirmation } from '@hnb/ui-kit'
import { listNamespaces, listWorkspaceClusters, type ContainerCluster } from '../../api/containerApi'
import {
  createPersistentVolume,
  createPersistentVolumeClaim,
  createStorageClass,
  deletePersistentVolume,
  deletePersistentVolumeClaim,
  deleteStorageClass,
  listPersistentVolumeClaims,
  listPersistentVolumes,
  listStorageClasses,
  updateStorageLabels,
  type PersistentVolume,
  type PersistentVolumeClaim,
  type StorageAccessMode,
  type StorageClassInfo,
  type StorageResourceKind,
} from '../../api/storageApi'
import NetworkDrawer from '../network/NetworkDrawer.vue'

type StorageTab = 'pv' | 'pvc' | 'storageClass'
type LabelRow = { key: string; value: string }

const { t } = useI18n()
const route = useRoute()
const tabs: StorageTab[] = ['pv', 'pvc', 'storageClass']
const queryValue = (value: unknown): string => Array.isArray(value) ? String(value[0] ?? '') : String(value ?? '')
const requestedCluster = queryValue(route.query.cluster) || queryValue(route.query.target)
const requestedOffering = queryValue(route.query.offering)
const requestedStorageClass = queryValue(route.query.storageClass)
const requestedNamespace = queryValue(route.query.namespace)
const activeTab = ref<StorageTab>(requestedOffering || requestedStorageClass ? 'storageClass' : requestedNamespace ? 'pvc' : 'pv')

const clusters = ref<ContainerCluster[]>([])
const namespaces = ref<string[]>(['default'])
const selectedCluster = ref('')
const selectedNamespace = ref(requestedNamespace || 'default')
const nameInput = ref(requestedStorageClass)
const appliedName = ref(requestedStorageClass)

const persistentVolumes = ref<PersistentVolume[]>([])
const persistentVolumeClaims = ref<PersistentVolumeClaim[]>([])
const storageClasses = ref<StorageClassInfo[]>([])
const loading = ref(false)
const loadError = ref('')
const notice = ref('')

const page = ref(1)
const pageSize = ref(10)
const jumpPage = ref('')
const sortDescending = ref(true)
const columnMenuOpen = ref(false)
const moreMenuKey = ref('')
const storageClassDetailVisible = ref(false)
const selectedStorageClass = ref<StorageClassInfo | null>(null)

const visibleColumns = ref<Record<StorageTab, string[]>>({
  pv: ['capacity', 'status', 'accessMode', 'reclaimPolicy', 'service', 'createdAt'],
  pvc: ['status', 'capacity', 'accessMode', 'storageClass', 'volume', 'namespace', 'service', 'createdAt'],
  storageClass: ['provisioner', 'reclaimPolicy', 'availability', 'poolPolicy', 'createdAt'],
})

const columnOptions = computed(() => {
  const keys = activeTab.value === 'pv'
    ? ['capacity', 'status', 'accessMode', 'reclaimPolicy', 'service', 'createdAt']
    : activeTab.value === 'pvc'
      ? ['status', 'capacity', 'accessMode', 'storageClass', 'volume', 'namespace', 'service', 'createdAt']
      : ['provisioner', 'reclaimPolicy', 'availability', 'poolPolicy', 'createdAt']
  return keys.map((key) => ({ key, label: t(`container.storage.columns.${key}`) }))
})

const currentTitle = computed(() => t(`container.storage.tabs.${activeTab.value}`))
const storageClassDetailTitle = computed(() => selectedStorageClass.value
  ? `${t('container.storage.tabs.storageClass')}: ${selectedStorageClass.value.name}`
  : t('container.storage.tabs.storageClass'))
const clusterOptions = computed(() => clusters.value.map((cluster) => ({
  value: cluster.id,
  label: cluster.display_name || cluster.name,
})))
const namespaceOptions = computed(() => Array.from(new Set(['default', ...namespaces.value])))

function matchesName(name: string): boolean {
  return !appliedName.value || name.toLowerCase().includes(appliedName.value.toLowerCase())
}

function sortByCreated<T extends { createdAt: string }>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const delta = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
    return sortDescending.value ? -delta : delta
  })
}

const filteredPvs = computed(() => sortByCreated(persistentVolumes.value.filter((item) => matchesName(item.name))))
const filteredPvcs = computed(() => sortByCreated(persistentVolumeClaims.value.filter((item) =>
  matchesName(item.name) && (!selectedNamespace.value || item.namespace === selectedNamespace.value),
)))
const filteredStorageClasses = computed(() => sortByCreated(storageClasses.value.filter((item) => matchesName(item.name))))
const currentTotal = computed(() => activeTab.value === 'pv' ? filteredPvs.value.length : filteredPvcs.value.length)
const pageCount = computed(() => Math.max(1, Math.ceil(currentTotal.value / pageSize.value)))
const pageStart = computed(() => currentTotal.value ? (page.value - 1) * pageSize.value + 1 : 0)
const pageEnd = computed(() => Math.min(page.value * pageSize.value, currentTotal.value))
const pagedPvs = computed(() => filteredPvs.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const pagedPvcs = computed(() => filteredPvcs.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

const pageTokens = computed<Array<number | 'ellipsis'>>(() => {
  const total = pageCount.value
  if (total <= 7) return Array.from({ length: total }, (_, index) => index + 1)
  const values: Array<number | 'ellipsis'> = [1]
  const start = Math.max(2, page.value - 1)
  const end = Math.min(total - 1, page.value + 1)
  if (start > 2) values.push('ellipsis')
  for (let value = start; value <= end; value += 1) values.push(value)
  if (end < total - 1) values.push('ellipsis')
  values.push(total)
  return values
})

function isVisible(tab: StorageTab, key: string): boolean {
  return visibleColumns.value[tab].includes(key)
}

function toggleColumn(key: string): void {
  const current = visibleColumns.value[activeTab.value]
  visibleColumns.value[activeTab.value] = current.includes(key)
    ? current.filter((item) => item !== key)
    : [...current, key]
}

function applyQuery(): void {
  appliedName.value = nameInput.value.trim()
  page.value = 1
}

function toggleStorageClassDetail(item: StorageClassInfo): void {
  if (storageClassDetailVisible.value && selectedStorageClass.value?.name === item.name) {
    storageClassDetailVisible.value = false
    return
  }
  selectedStorageClass.value = item
  storageClassDetailVisible.value = true
}

function toggleSort(): void {
  sortDescending.value = !sortDescending.value
  page.value = 1
}

function goPage(next: number): void {
  page.value = Math.min(pageCount.value, Math.max(1, next))
}

function changePageSize(event: Event): void {
  pageSize.value = Number((event.target as HTMLSelectElement).value)
  page.value = 1
}

function jumpToPage(): void {
  const target = Number(jumpPage.value)
  if (Number.isInteger(target)) goPage(target)
  jumpPage.value = ''
}

async function loadNamespaceOptions(): Promise<void> {
  try {
    const data = await listNamespaces({ clusterId: selectedCluster.value || undefined })
    namespaces.value = Array.from(new Set(['default', ...data.map((item) => item.name)]))
    if (!namespaces.value.includes(selectedNamespace.value)) selectedNamespace.value = 'default'
  } catch {
    namespaces.value = ['default']
  }
}

async function loadStorage(): Promise<void> {
  if (!selectedCluster.value) return
  loading.value = true
  loadError.value = ''
  try {
    const [pvs, pvcs, classes] = await Promise.all([
      listPersistentVolumes(selectedCluster.value),
      listPersistentVolumeClaims(selectedCluster.value),
      listStorageClasses(selectedCluster.value),
    ])
    persistentVolumes.value = pvs
    persistentVolumeClaims.value = pvcs
    storageClasses.value = classes
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.storage.loadError')
  } finally {
    loading.value = false
  }
}

async function refresh(): Promise<void> {
  await Promise.all([loadNamespaceOptions(), loadStorage()])
}

async function initialize(): Promise<void> {
  try {
    clusters.value = await listWorkspaceClusters()
    selectedCluster.value = clusters.value.some((cluster) => cluster.id === requestedCluster)
      ? requestedCluster
      : clusters.value[0]?.id ?? ''
    await loadNamespaceOptions()
    await loadStorage()
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.storage.loadError')
  }
}

watch(activeTab, () => {
  page.value = 1
  nameInput.value = ''
  appliedName.value = ''
  columnMenuOpen.value = false
  moreMenuKey.value = ''
})

watch(selectedCluster, async (value, oldValue) => {
  if (oldValue && value !== oldValue) await refresh()
})

watch(selectedNamespace, () => { page.value = 1 })
watch(pageCount, (count) => { if (page.value > count) page.value = count })
onMounted(initialize)

const createVisible = ref(false)
const createKind = ref<StorageTab>('pv')
const createMode = ref<'form' | 'yaml'>('form')
const createBusy = ref(false)
const createError = ref('')
const yamlInput = ref('')
const formLabels = ref<LabelRow[]>([])
const form = ref({
  name: '', namespace: 'default', preset: false, externalStorage: 'preset-nfs', storageSystem: 'NFS',
  capacity: 20, unit: 'Gi', server: '', path: '', accessMode: 'ReadWriteOnce' as StorageAccessMode,
  alias: '', description: '', storageClass: '', nfsVersion: '4.1', reclaimPolicy: 'Retain',
})

const createTitle = computed(() => t(`container.storage.drawer.${createKind.value === 'pv' ? 'pvTitle' : createKind.value === 'pvc' ? 'pvcTitle' : 'storageClassTitle'}`))

function yamlTemplate(kind: StorageTab): string {
  if (kind === 'pv') return 'apiVersion: v1\nkind: PersistentVolume\nmetadata:\n  name: pv-example\nspec:\n  capacity:\n    storage: 20Gi\n  accessModes:\n    - ReadWriteOnce\n  persistentVolumeReclaimPolicy: Retain\n  nfs:\n    server: 10.0.0.10\n    path: /data/example\n'
  return 'apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: pvc-example\n  namespace: default\nspec:\n  storageClassName: sc-nfs-standard\n  accessModes:\n    - ReadWriteOnce\n  resources:\n    requests:\n      storage: 20Gi\n'
}

function openCreate(): void {
  createKind.value = activeTab.value
  createMode.value = 'form'
  createError.value = ''
  formLabels.value = []
  form.value = {
    name: '', namespace: selectedNamespace.value || 'default', preset: false, externalStorage: 'preset-nfs', storageSystem: 'NFS',
    capacity: 20, unit: 'Gi', server: '', path: '', accessMode: 'ReadWriteOnce', alias: '', description: '',
    storageClass: storageClasses.value[0]?.name ?? '', nfsVersion: '4.1', reclaimPolicy: 'Retain',
  }
  yamlInput.value = createKind.value === 'storageClass' ? '' : yamlTemplate(createKind.value)
  createVisible.value = true
}

function addFormLabel(): void {
  formLabels.value.push({ key: '', value: '' })
}

function labelRecord(rows: LabelRow[]): Record<string, string> {
  const result: Record<string, string> = {}
  for (const row of rows) {
    const key = row.key.trim()
    if (!key) continue
    if (key in result) throw new Error(t('container.storage.validation.duplicateLabel'))
    result[key] = row.value.trim()
  }
  return result
}

function formStorageLocation(): { server: string; path: string } {
  if (!form.value.preset) return { server: form.value.server.trim(), path: form.value.path.trim() }
  return form.value.externalStorage === 'preset-sds'
    ? { server: 'sds.hnb.local', path: '/exports/sds' }
    : { server: 'nfs.hnb.local', path: '/exports/default' }
}

function buildFormResource(): Record<string, unknown> {
  const labels = labelRecord(formLabels.value)
  const capacity = `${form.value.capacity}${form.value.unit}`
  if (!selectedCluster.value || !form.value.name.trim()) throw new Error(t('container.storage.validation.required'))
  if (createKind.value === 'pv') {
    const location = formStorageLocation()
    if (!location.server || !location.path) throw new Error(t('container.storage.validation.required'))
    return {
      apiVersion: 'v1', kind: 'PersistentVolume', metadata: { name: form.value.name.trim(), labels },
      spec: {
        capacity: { storage: capacity }, accessModes: [form.value.accessMode], persistentVolumeReclaimPolicy: 'Retain',
        nfs: { server: location.server, path: location.path },
      },
    }
  }
  if (createKind.value === 'pvc') {
    if (!form.value.namespace || !form.value.storageClass) throw new Error(t('container.storage.validation.required'))
    return {
      apiVersion: 'v1', kind: 'PersistentVolumeClaim',
      metadata: {
        name: form.value.name.trim(), namespace: form.value.namespace, labels,
        annotations: { 'hnb.io/alias': form.value.alias.trim(), 'hnb.io/description': form.value.description.trim() },
      },
      spec: { storageClassName: form.value.storageClass, accessModes: [form.value.accessMode], resources: { requests: { storage: capacity } } },
    }
  }
  const location = formStorageLocation()
  if (!location.server || !location.path || !form.value.nfsVersion) throw new Error(t('container.storage.validation.required'))
  const name = form.value.name.trim().startsWith('sc-') ? form.value.name.trim() : `sc-${form.value.name.trim()}`
  return {
    apiVersion: 'storage.k8s.io/v1', kind: 'StorageClass', metadata: { name, labels }, provisioner: 'nfs.csi.k8s.io',
    parameters: { server: location.server, share: location.path, mountOptions: `nfsvers=${form.value.nfsVersion}` },
    reclaimPolicy: form.value.reclaimPolicy, allowVolumeExpansion: true, volumeBindingMode: 'Immediate',
  }
}

async function submitCreate(): Promise<void> {
  createBusy.value = true
  createError.value = ''
  try {
    let resource: Record<string, any>
    if (createMode.value === 'yaml' && createKind.value !== 'storageClass') {
      if (!yamlInput.value.trim()) throw new Error(t('container.storage.validation.yamlRequired'))
      resource = parse(yamlInput.value)
      const expected = createKind.value === 'pv' ? 'PersistentVolume' : 'PersistentVolumeClaim'
      if (!resource || resource.kind !== expected || !resource.metadata?.name) throw new Error(t('container.storage.validation.yamlInvalid'))
    } else {
      resource = buildFormResource()
    }
    if (createKind.value === 'pv') await createPersistentVolume(selectedCluster.value, resource)
    else if (createKind.value === 'pvc') {
      const namespace = String(resource.metadata?.namespace || form.value.namespace || 'default')
      await createPersistentVolumeClaim(selectedCluster.value, namespace, resource)
    } else await createStorageClass(selectedCluster.value, resource)
    createVisible.value = false
    showNotice(t('container.storage.message.created'))
    await loadStorage()
  } catch (error) {
    createError.value = error instanceof Error ? error.message : String(error)
  } finally {
    createBusy.value = false
  }
}

function showNotice(message: string): void {
  notice.value = message
  window.setTimeout(() => { if (notice.value === message) notice.value = '' }, 2500)
}

function pvResource(item: PersistentVolume): Record<string, unknown> {
  return {
    apiVersion: 'v1', kind: 'PersistentVolume', metadata: { name: item.name, labels: item.labels, creationTimestamp: item.createdAt },
    spec: {
      capacity: { storage: item.capacity }, accessModes: item.accessModes, persistentVolumeReclaimPolicy: item.reclaimPolicy,
      storageClassName: item.storageClassName,
      ...(item.claimName ? { claimRef: { name: item.claimName, namespace: item.claimNamespace } } : {}),
    }, status: { phase: item.status, capacity: { storage: item.capacity } },
  }
}

function pvcResource(item: PersistentVolumeClaim): Record<string, unknown> {
  return {
    apiVersion: 'v1', kind: 'PersistentVolumeClaim', metadata: { name: item.name, namespace: item.namespace, labels: item.labels, creationTimestamp: item.createdAt },
    spec: { accessModes: item.accessModes, storageClassName: item.storageClassName, volumeName: item.volumeName, resources: { requests: { storage: item.capacity } } },
    status: { phase: item.status, capacity: { storage: item.capacity } },
  }
}

function storageClassResource(item: StorageClassInfo): Record<string, unknown> {
  return {
    apiVersion: 'storage.k8s.io/v1', kind: 'StorageClass', metadata: { name: item.name, labels: item.labels, creationTimestamp: item.createdAt },
    provisioner: item.provisioner, parameters: item.parameters, reclaimPolicy: item.reclaimPolicy, allowVolumeExpansion: item.allowVolumeExpansion,
  }
}

const yamlVisible = ref(false)
const yamlContent = ref('')
function showYaml(item: PersistentVolume | PersistentVolumeClaim | StorageClassInfo, kind: StorageResourceKind): void {
  const resource = kind === 'pv' ? pvResource(item as PersistentVolume) : kind === 'pvc' ? pvcResource(item as PersistentVolumeClaim) : storageClassResource(item as StorageClassInfo)
  yamlContent.value = stringify(resource)
  yamlVisible.value = true
  moreMenuKey.value = ''
}

const labelsVisible = ref(false)
const labelsBusy = ref(false)
const labelsError = ref('')
const labelRows = ref<LabelRow[]>([])
const labelTarget = ref<{ kind: StorageResourceKind; name: string; namespace: string } | null>(null)

function manageLabels(item: PersistentVolume | PersistentVolumeClaim | StorageClassInfo, kind: StorageResourceKind): void {
  labelTarget.value = { kind, name: item.name, namespace: 'namespace' in item ? item.namespace : '' }
  labelRows.value = Object.entries(item.labels).map(([key, value]) => ({ key, value }))
  labelsError.value = ''
  labelsVisible.value = true
  moreMenuKey.value = ''
}

async function saveLabels(): Promise<void> {
  if (!labelTarget.value) return
  labelsBusy.value = true
  labelsError.value = ''
  try {
    await updateStorageLabels(selectedCluster.value, labelTarget.value.kind, labelTarget.value.name, labelRecord(labelRows.value), labelTarget.value.namespace)
    labelsVisible.value = false
    showNotice(t('container.storage.message.labelsSaved'))
    await loadStorage()
  } catch (error) {
    labelsError.value = error instanceof Error ? error.message : String(error)
  } finally {
    labelsBusy.value = false
  }
}

const confirmVisible = ref(false)
const confirmBusy = ref(false)
const confirmError = ref('')
const confirmTarget = ref<{ kind: StorageResourceKind; name: string; namespace: string } | null>(null)

function requestDelete(kind: StorageResourceKind, name: string, namespace = ''): void {
  confirmTarget.value = { kind, name, namespace }
  confirmError.value = ''
  confirmVisible.value = true
  moreMenuKey.value = ''
}

async function confirmOperation(): Promise<void> {
  if (!confirmTarget.value) return
  confirmBusy.value = true
  confirmError.value = ''
  try {
    if (confirmTarget.value.kind === 'pv') await deletePersistentVolume(selectedCluster.value, confirmTarget.value.name)
    else if (confirmTarget.value.kind === 'pvc') await deletePersistentVolumeClaim(selectedCluster.value, confirmTarget.value.namespace, confirmTarget.value.name)
    else await deleteStorageClass(selectedCluster.value, confirmTarget.value.name)
    confirmVisible.value = false
    showNotice(t('container.storage.message.deleted'))
    await loadStorage()
  } catch (error) {
    confirmError.value = error instanceof Error ? error.message : String(error)
  } finally {
    confirmBusy.value = false
  }
}

const confirmTitle = computed(() => t('container.storage.confirm.deleteTitle'))
const confirmMessage = computed(() => t('container.storage.confirm.deleteMessage', { name: confirmTarget.value?.name ?? '' }))

function statusText(status: string): string {
  return ['Bound', 'Released', 'Available', 'Pending', 'Failed'].includes(status) ? t(`container.storage.status.${status}`) : status
}

function accessModeText(modes: StorageAccessMode[]): string {
  return modes.map((mode) => t(`container.storage.accessMode.${mode}`)).join(', ') || '--'
}

function serviceText(service: string, namespace: string): string {
  return service ? `${service}: ${namespace || 'default'}` : '--'
}

function formatDate(value: string): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>

<template>
  <section class="storage-page">
    <header class="storage-header">
      <h2>{{ currentTitle }}</h2>
      <a class="help-link" href="https://docs.hnb.example.io/container/storage" target="_blank" rel="noopener noreferrer">? {{ t('container.storage.help') }}</a>
    </header>

    <nav class="storage-tabs" role="tablist" :aria-label="t('container.storage.title')">
      <button v-for="tab in tabs" :key="tab" type="button" role="tab" :aria-selected="activeTab === tab" :class="{ active: activeTab === tab }" @click="activeTab = tab">
        {{ t(`container.storage.tabs.${tab}`) }}
      </button>
    </nav>

    <div class="storage-toolbar">
      <HNBButton @click="openCreate">{{ t('container.storage.toolbar.create') }}</HNBButton>
      <div class="storage-filters">
        <label class="compact-field">
          <span>{{ t('container.storage.toolbar.cluster') }}</span>
          <select v-model="selectedCluster">
            <option v-for="cluster in clusterOptions" :key="cluster.value" :value="cluster.value">{{ cluster.label }}</option>
          </select>
        </label>
        <label v-if="activeTab === 'pvc'" class="compact-field">
          <span>{{ t('container.storage.toolbar.namespace') }}</span>
          <select v-model="selectedNamespace">
            <option v-for="namespace in namespaceOptions" :key="namespace" :value="namespace">{{ namespace }}</option>
          </select>
        </label>
        <input v-model="nameInput" class="search-input" type="search" :placeholder="t('container.storage.toolbar.namePlaceholder')" @keyup.enter="applyQuery">
        <HNBButton size="small" @click="applyQuery">{{ t('container.storage.toolbar.query') }}</HNBButton>
        <button class="icon-button" type="button" :title="t('container.storage.toolbar.refresh')" :aria-label="t('container.storage.toolbar.refresh')" @click="refresh">↻</button>
        <div class="column-settings">
          <button class="icon-button" type="button" :title="t('container.storage.toolbar.columns')" :aria-label="t('container.storage.toolbar.columns')" @click="columnMenuOpen = !columnMenuOpen">⚙</button>
          <div v-if="columnMenuOpen" class="column-menu">
            <label v-for="column in columnOptions" :key="column.key">
              <input type="checkbox" :checked="isVisible(activeTab, column.key)" @change="toggleColumn(column.key)">
              <span>{{ column.label }}</span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <p v-if="requestedOffering" class="storage-context">
      {{ t('container.storage.offeringContext', { offering: requestedOffering }) }}
    </p>

    <p v-if="notice" class="storage-notice" role="status">{{ notice }}</p>
    <p v-if="loadError" class="storage-error" role="alert">{{ loadError }}</p>
    <p v-if="loading" class="storage-loading" role="status">{{ t('container.storage.loading') }}</p>

    <div v-else class="storage-table-wrap">
      <table v-if="activeTab === 'pv'" class="storage-table">
        <thead><tr>
          <th>{{ t('container.storage.columns.name') }}</th>
          <th v-if="isVisible('pv', 'capacity')">{{ t('container.storage.columns.capacity') }}</th>
          <th v-if="isVisible('pv', 'status')">{{ t('container.storage.columns.status') }}</th>
          <th v-if="isVisible('pv', 'accessMode')">{{ t('container.storage.columns.accessMode') }}</th>
          <th v-if="isVisible('pv', 'reclaimPolicy')">{{ t('container.storage.columns.reclaimPolicy') }}</th>
          <th v-if="isVisible('pv', 'service')">{{ t('container.storage.columns.service') }}</th>
          <th v-if="isVisible('pv', 'createdAt')"><button class="sort-button" type="button" @click="toggleSort">{{ t('container.storage.columns.createdAt') }} {{ sortDescending ? '↓' : '↑' }}</button></th>
          <th>{{ t('container.storage.columns.actions') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="item in pagedPvs" :key="item.name">
            <td><span class="ellipsis" :title="item.name">{{ item.name }}</span></td>
            <td v-if="isVisible('pv', 'capacity')">{{ item.capacity }}</td>
            <td v-if="isVisible('pv', 'status')"><span class="status"><i :class="`status-dot status-dot--${item.status.toLowerCase()}`" />{{ statusText(item.status) }}</span></td>
            <td v-if="isVisible('pv', 'accessMode')"><span class="ellipsis" :title="accessModeText(item.accessModes)">{{ accessModeText(item.accessModes) }}</span></td>
            <td v-if="isVisible('pv', 'reclaimPolicy')">{{ item.reclaimPolicy }}</td>
            <td v-if="isVisible('pv', 'service')"><span class="ellipsis" :title="serviceText(item.service, item.claimNamespace)">{{ serviceText(item.service, item.claimNamespace) }}</span></td>
            <td v-if="isVisible('pv', 'createdAt')">{{ formatDate(item.createdAt) }}</td>
            <td><div class="row-actions">
              <button type="button" :disabled="item.status === 'Bound'" @click="manageLabels(item, 'pv')">{{ t('container.storage.action.labels') }}</button>
              <button type="button" @click="showYaml(item, 'pv')">{{ t('container.storage.action.yaml') }}</button>
              <button type="button" :disabled="item.status === 'Bound'" @click="requestDelete('pv', item.name)">{{ t('container.storage.action.delete') }}</button>
            </div></td>
          </tr>
          <tr v-if="!pagedPvs.length"><td class="empty-cell" colspan="8">{{ t('container.storage.empty') }}</td></tr>
        </tbody>
      </table>

      <table v-else-if="activeTab === 'pvc'" class="storage-table">
        <thead><tr>
          <th>{{ t('container.storage.columns.name') }}</th>
          <th v-if="isVisible('pvc', 'status')">{{ t('container.storage.columns.status') }}</th>
          <th v-if="isVisible('pvc', 'capacity')">{{ t('container.storage.columns.capacity') }}</th>
          <th v-if="isVisible('pvc', 'accessMode')">{{ t('container.storage.columns.accessMode') }}</th>
          <th v-if="isVisible('pvc', 'storageClass')">{{ t('container.storage.columns.storageClass') }}</th>
          <th v-if="isVisible('pvc', 'volume')">{{ t('container.storage.columns.volume') }}</th>
          <th v-if="isVisible('pvc', 'namespace')">{{ t('container.storage.columns.namespace') }}</th>
          <th v-if="isVisible('pvc', 'service')">{{ t('container.storage.columns.service') }}</th>
          <th v-if="isVisible('pvc', 'createdAt')"><button class="sort-button" type="button" @click="toggleSort">{{ t('container.storage.columns.createdAt') }} {{ sortDescending ? '↓' : '↑' }}</button></th>
          <th>{{ t('container.storage.columns.actions') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="item in pagedPvcs" :key="`${item.namespace}/${item.name}`">
            <td><span class="ellipsis" :title="item.name">{{ item.name }}</span></td>
            <td v-if="isVisible('pvc', 'status')"><span class="status"><i :class="`status-dot status-dot--${item.status.toLowerCase()}`" />{{ statusText(item.status) }}</span></td>
            <td v-if="isVisible('pvc', 'capacity')">{{ item.capacity }}</td>
            <td v-if="isVisible('pvc', 'accessMode')"><span class="ellipsis" :title="accessModeText(item.accessModes)">{{ accessModeText(item.accessModes) }}</span></td>
            <td v-if="isVisible('pvc', 'storageClass')"><span class="ellipsis" :title="item.storageClassName">{{ item.storageClassName || '--' }}</span></td>
            <td v-if="isVisible('pvc', 'volume')"><span class="ellipsis" :title="item.volumeName">{{ item.volumeName || '--' }}</span></td>
            <td v-if="isVisible('pvc', 'namespace')">{{ item.namespace }}</td>
            <td v-if="isVisible('pvc', 'service')"><span class="ellipsis" :title="serviceText(item.service, item.namespace)">{{ serviceText(item.service, item.namespace) }}</span></td>
            <td v-if="isVisible('pvc', 'createdAt')">{{ formatDate(item.createdAt) }}</td>
            <td><div class="row-actions row-actions--menu">
              <button type="button" @click="requestDelete('pvc', item.name, item.namespace)">{{ t('container.storage.action.delete') }}</button>
              <button type="button" @click="moreMenuKey = moreMenuKey === `pvc:${item.namespace}:${item.name}` ? '' : `pvc:${item.namespace}:${item.name}`">{{ t('container.storage.action.more') }}</button>
              <div v-if="moreMenuKey === `pvc:${item.namespace}:${item.name}`" class="more-menu">
                <button type="button" @click="manageLabels(item, 'pvc')">{{ t('container.storage.action.labels') }}</button>
                <button type="button" @click="showYaml(item, 'pvc')">{{ t('container.storage.action.yaml') }}</button>
              </div>
            </div></td>
          </tr>
          <tr v-if="!pagedPvcs.length"><td class="empty-cell" colspan="10">{{ t('container.storage.empty') }}</td></tr>
        </tbody>
      </table>

      <table v-else class="storage-table">
        <thead><tr>
          <th class="expand-column" :aria-label="t('container.storage.columns.expand')" />
          <th>{{ t('container.storage.columns.name') }}</th>
          <th v-if="isVisible('storageClass', 'provisioner')">{{ t('container.storage.columns.provisioner') }}</th>
          <th v-if="isVisible('storageClass', 'reclaimPolicy')">{{ t('container.storage.columns.reclaimPolicy') }}</th>
          <th v-if="isVisible('storageClass', 'availability')">{{ t('container.storage.columns.availability') }}</th>
          <th v-if="isVisible('storageClass', 'poolPolicy')">{{ t('container.storage.columns.poolPolicy') }}</th>
          <th v-if="isVisible('storageClass', 'createdAt')"><button class="sort-button" type="button" @click="toggleSort">{{ t('container.storage.columns.createdAt') }} {{ sortDescending ? '↓' : '↑' }}</button></th>
          <th>{{ t('container.storage.columns.actions') }}</th>
        </tr></thead>
        <tbody v-for="item in filteredStorageClasses" :key="item.name">
          <tr>
            <td><button class="expand-button" type="button" :aria-expanded="storageClassDetailVisible && selectedStorageClass?.name === item.name" @click="toggleStorageClassDetail(item)">›</button></td>
            <td><span class="ellipsis" :title="item.name">{{ item.name }}</span></td>
            <td v-if="isVisible('storageClass', 'provisioner')"><span class="ellipsis" :title="item.provisioner">{{ item.provisioner }}</span></td>
            <td v-if="isVisible('storageClass', 'reclaimPolicy')">{{ item.reclaimPolicy }}</td>
            <td v-if="isVisible('storageClass', 'availability')">{{ t(`container.storage.availability.${item.allowVolumeExpansion ? 'expandable' : 'fixed'}`) }}</td>
            <td v-if="isVisible('storageClass', 'poolPolicy')">{{ item.poolPolicy || '--' }}</td>
            <td v-if="isVisible('storageClass', 'createdAt')">{{ formatDate(item.createdAt) }}</td>
            <td><div class="row-actions row-actions--menu">
              <button type="button" @click="manageLabels(item, 'storageClass')">{{ t('container.storage.action.labels') }}</button>
              <button type="button" @click="showYaml(item, 'storageClass')">{{ t('container.storage.action.yaml') }}</button>
              <button type="button" @click="moreMenuKey = moreMenuKey === `sc:${item.name}` ? '' : `sc:${item.name}`">{{ t('container.storage.action.more') }}</button>
              <div v-if="moreMenuKey === `sc:${item.name}`" class="more-menu"><button type="button" @click="requestDelete('storageClass', item.name)">{{ t('container.storage.action.delete') }}</button></div>
            </div></td>
          </tr>
        </tbody>
        <tbody v-if="!filteredStorageClasses.length"><tr><td class="empty-cell" colspan="8">{{ t('container.storage.empty') }}</td></tr></tbody>
      </table>
    </div>

    <footer v-if="activeTab !== 'storageClass' && !loading" class="storage-pagination">
      <span>{{ t('container.storage.pagination.range', { start: pageStart, end: pageEnd, total: currentTotal }) }}</span>
      <div class="page-buttons">
        <button type="button" :disabled="page <= 1" :aria-label="t('container.storage.pagination.previous')" @click="goPage(page - 1)">‹</button>
        <template v-for="(token, index) in pageTokens" :key="`${token}-${index}`">
          <span v-if="token === 'ellipsis'">…</span>
          <button v-else type="button" :class="{ active: token === page }" @click="goPage(token)">{{ token }}</button>
        </template>
        <button type="button" :disabled="page >= pageCount" :aria-label="t('container.storage.pagination.next')" @click="goPage(page + 1)">›</button>
      </div>
      <div class="page-jump">
        <select :value="pageSize" @change="changePageSize"><option v-for="size in [10, 20, 50]" :key="size" :value="size">{{ t('container.storage.pagination.pageSize', { size }) }}</option></select>
        <span>{{ t('container.storage.pagination.jump') }}</span>
        <input v-model="jumpPage" type="number" min="1" :max="pageCount" @keyup.enter="jumpToPage">
        <span>{{ t('container.storage.pagination.pageUnit', { pages: pageCount }) }}</span>
      </div>
    </footer>

    <NetworkDrawer v-model="createVisible" :title="createTitle" :busy="createBusy" :error="createError" @confirm="submitCreate">
      <form class="storage-form" @submit.prevent="submitCreate">
        <div v-if="createKind !== 'storageClass'" class="form-mode" role="tablist">
          <button type="button" role="tab" :aria-selected="createMode === 'form'" :class="{ active: createMode === 'form' }" @click="createMode = 'form'">{{ t(`container.storage.mode.${createKind === 'pv' ? 'pv' : 'storageClass'}`) }}</button>
          <button type="button" role="tab" :aria-selected="createMode === 'yaml'" :class="{ active: createMode === 'yaml' }" @click="createMode = 'yaml'">{{ t('container.storage.mode.yaml') }}</button>
        </div>
        <div v-if="createKind === 'pvc' && createMode === 'form'" class="form-tip">{{ t('container.storage.pvcTip') }}</div>
        <template v-if="createMode === 'form' || createKind === 'storageClass'">
          <label v-if="createKind !== 'storageClass'" class="form-field"><span>{{ t('container.storage.form.name') }} *</span><input v-model="form.name" type="text"></label>
          <label v-else class="form-field"><span>{{ t('container.storage.form.name') }} *</span><div class="prefixed-input"><span>sc-</span><input v-model="form.name" type="text"></div></label>
          <label class="form-field"><span>{{ t('container.storage.form.cluster') }} *</span><select v-model="selectedCluster"><option disabled value="">{{ t('container.storage.form.select') }}</option><option v-for="cluster in clusterOptions" :key="cluster.value" :value="cluster.value">{{ cluster.label }}</option></select></label>
          <label v-if="createKind === 'pvc'" class="form-field"><span>{{ t('container.storage.form.namespace') }} *</span><select v-model="form.namespace"><option disabled value="">{{ t('container.storage.form.select') }}</option><option v-for="namespace in namespaceOptions" :key="namespace" :value="namespace">{{ namespace }}</option></select></label>
          <template v-if="createKind !== 'pvc'">
            <label class="switch-field"><span>{{ t('container.storage.form.preset') }}</span><input v-model="form.preset" type="checkbox" role="switch" :aria-checked="form.preset"></label>
            <label v-if="form.preset" class="form-field"><span>{{ t('container.storage.form.externalStorage') }}</span><select v-model="form.externalStorage"><option value="preset-nfs">{{ t('container.storage.form.externalNfs') }}</option><option value="preset-sds">{{ t('container.storage.form.externalSds') }}</option></select></label>
            <label v-else class="form-field"><span>{{ t('container.storage.form.storageSystem') }}</span><select v-model="form.storageSystem"><option>NFS</option></select></label>
          </template>
          <label v-if="createKind === 'pvc'" class="form-field"><span>{{ t('container.storage.form.alias') }}</span><input v-model="form.alias" type="text" maxlength="64" :placeholder="t('container.storage.form.aliasPlaceholder')"></label>
          <label v-if="createKind === 'pvc'" class="form-field"><span>{{ t('container.storage.form.description') }}</span><textarea v-model="form.description" rows="3" /></label>
          <label v-if="createKind === 'pvc'" class="form-field"><span>{{ t('container.storage.form.storageClass') }} *</span><select v-model="form.storageClass"><option disabled value="">{{ t('container.storage.form.select') }}</option><option v-for="item in storageClasses" :key="item.name" :value="item.name">{{ item.name }}</option></select></label>
          <label v-if="createKind !== 'pvc' && !form.preset" class="form-field"><span>{{ createKind === 'storageClass' ? t('container.storage.form.nfsServer') : t('container.storage.form.server') }} *</span><input v-model="form.server" type="text" :placeholder="createKind === 'storageClass' ? t('container.storage.form.nfsServerPlaceholder') : ''"></label>
          <label v-if="createKind !== 'pvc' && !form.preset" class="form-field"><span>{{ t('container.storage.form.path') }} *</span><input v-model="form.path" type="text"></label>
          <label v-if="createKind === 'storageClass'" class="form-field"><span>{{ t('container.storage.form.nfsVersion') }} *</span><select v-model="form.nfsVersion"><option disabled value="">{{ t('container.storage.form.select') }}</option><option value="3">3</option><option value="4.1">4.1</option></select></label>
          <div v-if="createKind !== 'storageClass'" class="form-field"><span>{{ t('container.storage.form.capacity') }} *</span><div class="capacity-field"><input v-model.number="form.capacity" type="number" min="1"><select v-model="form.unit"><option>Gi</option><option>Ti</option></select></div></div>
          <div v-if="createKind !== 'storageClass'" class="form-field"><span>{{ t('container.storage.form.accessMode') }}</span><div class="segmented"><button v-for="mode in (['ReadWriteOnce', 'ReadWriteMany', 'ReadOnlyMany'] as StorageAccessMode[])" :key="mode" type="button" :disabled="createKind === 'pvc' && mode === 'ReadOnlyMany'" :class="{ active: form.accessMode === mode }" @click="form.accessMode = mode">{{ t(`container.storage.accessMode.${mode}`) }}</button></div></div>
          <div v-else class="form-field"><span>{{ t('container.storage.form.reclaimPolicy') }}</span><div class="segmented"><button type="button" :class="{ active: form.reclaimPolicy === 'Retain' }" @click="form.reclaimPolicy = 'Retain'">{{ t('container.storage.form.retain') }}</button><button type="button" :class="{ active: form.reclaimPolicy === 'Delete' }" @click="form.reclaimPolicy = 'Delete'">{{ t('container.storage.form.remove') }}</button></div></div>
          <div class="form-field"><span>{{ t('container.storage.form.labels') }}</span><div v-for="(label, index) in formLabels" :key="index" class="label-row"><input v-model="label.key" :placeholder="t('container.storage.form.labelKey')"><input v-model="label.value" :placeholder="t('container.storage.form.labelValue')"><button type="button" @click="formLabels.splice(index, 1)">×</button></div><button class="add-label" type="button" @click="addFormLabel">{{ t('container.storage.action.add') }}</button></div>
        </template>
        <label v-else class="form-field"><span>{{ t('container.storage.form.yaml') }}</span><textarea v-model="yamlInput" class="yaml-editor" rows="22" spellcheck="false" /></label>
      </form>
    </NetworkDrawer>

    <NetworkDrawer v-model="labelsVisible" :title="t('container.storage.drawer.labelsTitle')" :busy="labelsBusy" :error="labelsError" @confirm="saveLabels">
      <div class="storage-form"><div class="form-field"><span>{{ t('container.storage.form.labels') }}</span><div v-for="(label, index) in labelRows" :key="index" class="label-row"><input v-model="label.key" :placeholder="t('container.storage.form.labelKey')"><input v-model="label.value" :placeholder="t('container.storage.form.labelValue')"><button type="button" @click="labelRows.splice(index, 1)">×</button></div><button class="add-label" type="button" @click="labelRows.push({ key: '', value: '' })">{{ t('container.storage.action.add') }}</button></div></div>
    </NetworkDrawer>

    <NetworkDrawer v-model="yamlVisible" :title="t('container.storage.drawer.yamlTitle')" hide-confirm>
      <textarea class="yaml-editor yaml-editor--readonly" :value="yamlContent" rows="30" readonly spellcheck="false" />
    </NetworkDrawer>

    <NetworkDrawer v-model="storageClassDetailVisible" :title="storageClassDetailTitle" hide-confirm>
      <dl v-if="selectedStorageClass" class="detail-list">
        <div><dt>{{ t('container.storage.details.parameters') }}</dt><dd>{{ Object.entries(selectedStorageClass.parameters).map(([key, value]) => `${key}=${value}`).join(', ') || '--' }}</dd></div>
        <div><dt>{{ t('container.storage.details.labels') }}</dt><dd>{{ Object.entries(selectedStorageClass.labels).map(([key, value]) => `${key}=${value}`).join(', ') || '--' }}</dd></div>
      </dl>
    </NetworkDrawer>

    <HNBConfirmation v-model="confirmVisible" :title="confirmTitle" :description="confirmMessage" :loading="confirmBusy" :error="confirmError" :confirm-text="t('container.storage.action.confirm')" :cancel-text="t('container.storage.action.cancel')" danger @confirm="confirmOperation" />
  </section>
</template>

<style scoped>
.storage-page { display: flex; flex-direction: column; gap: 14px; min-width: 0; padding: 18px 20px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); }
.storage-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.storage-header h2 { margin: 0; font-size: 18px; }
.help-link { color: var(--hnb-color-primary); font-size: 13px; text-decoration: none; white-space: nowrap; }
.storage-tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--hnb-color-divider); }
.storage-tabs button { padding: 9px 18px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--hnb-color-text-secondary); cursor: pointer; }
.storage-tabs button.active { border-bottom-color: var(--hnb-color-primary); color: var(--hnb-color-primary); font-weight: 600; }
.storage-toolbar { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; }
.storage-filters { display: flex; align-items: flex-end; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
.compact-field { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--hnb-color-text-secondary); }
.compact-field select, .search-input { min-height: 34px; padding: 6px 10px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); }
.search-input { width: 190px; }
.icon-button { width: 34px; height: 34px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-secondary); cursor: pointer; font-size: 17px; }
.column-settings { position: relative; }
.column-menu { position: absolute; z-index: 20; top: 40px; right: 0; min-width: 180px; padding: 8px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-surface); box-shadow: var(--hnb-shadow-3); }
.column-menu label { display: flex; gap: 8px; align-items: center; padding: 6px 8px; font-size: 13px; cursor: pointer; }
.storage-notice, .storage-error, .storage-loading { margin: 0; padding: 8px 10px; border-radius: var(--hnb-radius-sm); font-size: 13px; }
.storage-context { margin: 0; color: var(--hnb-color-text-secondary); font-size: 13px; }
.storage-notice { color: var(--hnb-color-status-success); background: var(--hnb-color-status-success-surface); }
.storage-error { color: var(--hnb-color-status-danger); background: var(--hnb-color-status-danger-surface); }
.storage-loading { color: var(--hnb-color-text-secondary); }
.storage-table-wrap { overflow-x: auto; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }
.storage-table { width: 100%; min-width: 1080px; border-collapse: collapse; table-layout: fixed; font-size: 13px; }
.storage-table th, .storage-table td { padding: 11px 12px; border-bottom: 1px solid var(--hnb-color-divider); text-align: left; vertical-align: middle; }
.storage-table th { background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-secondary); font-weight: 600; white-space: nowrap; }
.storage-table tr:last-child td { border-bottom: 0; }
.storage-table th:first-child, .storage-table td:first-child { width: 190px; }
.storage-table th:last-child, .storage-table td:last-child { width: 260px; }
.storage-table .expand-column, .storage-table td:has(.expand-button) { width: 42px; }
.ellipsis { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sort-button { padding: 0; border: 0; background: transparent; color: inherit; font: inherit; font-weight: 600; cursor: pointer; }
.status { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
.status-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--hnb-color-text-tertiary); }
.status-dot--bound { background: var(--hnb-color-status-warning); }
.status-dot--released, .status-dot--available { background: var(--hnb-color-status-success); }
.status-dot--pending { background: var(--hnb-color-status-info); }
.status-dot--failed { background: var(--hnb-color-status-danger); }
.row-actions { display: flex; align-items: center; gap: 10px; white-space: nowrap; }
.row-actions button { padding: 0; border: 0; background: transparent; color: var(--hnb-color-primary); cursor: pointer; font-size: 13px; }
.row-actions button:disabled { color: var(--hnb-color-text-tertiary); cursor: not-allowed; }
.row-actions--menu { position: relative; }
.more-menu { position: absolute; z-index: 10; top: 24px; right: 0; display: grid; min-width: 110px; padding: 6px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-surface); box-shadow: var(--hnb-shadow-2); }
.more-menu button { padding: 7px 9px; text-align: left; }
.expand-button { border: 0; background: transparent; color: var(--hnb-color-text-secondary); cursor: pointer; font-size: 20px; transition: transform var(--hnb-duration-fast); }
.expand-button[aria-expanded="true"] { transform: rotate(90deg); }
.detail-list { display: grid; gap: 12px; margin: 0; }
.detail-list div { padding-bottom: 10px; border-bottom: 1px solid var(--hnb-color-divider); }
.detail-list dt { color: var(--hnb-color-text-secondary); font-size: 12px; }
.detail-list dd { margin: 5px 0 0; overflow-wrap: anywhere; }
.empty-cell { padding: 40px !important; text-align: center !important; color: var(--hnb-color-text-tertiary); }
.storage-pagination { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 14px; color: var(--hnb-color-text-secondary); font-size: 13px; }
.page-buttons, .page-jump { display: flex; align-items: center; gap: 5px; }
.page-buttons button { min-width: 30px; height: 30px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); cursor: pointer; }
.page-buttons button.active { border-color: var(--hnb-color-primary); background: var(--hnb-color-primary); color: var(--hnb-color-text-on-accent); }
.page-buttons button:disabled { opacity: 0.45; cursor: not-allowed; }
.page-jump { justify-content: flex-end; }
.page-jump select, .page-jump input { height: 30px; padding: 4px 7px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); }
.page-jump input { width: 52px; }
.storage-form { display: flex; flex-direction: column; gap: 14px; }
.form-mode { display: flex; gap: 4px; border-bottom: 1px solid var(--hnb-color-divider); }
.form-mode button { padding: 8px 14px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--hnb-color-text-secondary); cursor: pointer; }
.form-mode button.active { border-bottom-color: var(--hnb-color-primary); color: var(--hnb-color-primary); }
.form-tip { padding: 10px 12px; border-radius: var(--hnb-radius-sm); background: var(--hnb-color-status-info-surface); color: var(--hnb-color-text-secondary); font-size: 12px; line-height: 1.6; }
.form-field { display: flex; flex-direction: column; gap: 6px; color: var(--hnb-color-text-secondary); font-size: 13px; }
.form-field input, .form-field select, .form-field textarea, .capacity-field input, .capacity-field select { width: 100%; box-sizing: border-box; padding: 8px 10px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); }
.switch-field { display: flex; align-items: center; justify-content: space-between; color: var(--hnb-color-text-secondary); font-size: 13px; }
.switch-field input { width: 38px; height: 20px; accent-color: var(--hnb-color-primary); }
.capacity-field { display: grid; grid-template-columns: 1fr 90px; gap: 8px; }
.prefixed-input { display: flex; align-items: center; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); overflow: hidden; }
.prefixed-input span { padding: 0 0 0 10px; color: var(--hnb-color-text-secondary); }
.prefixed-input input { border: 0; }
.segmented { display: grid; grid-template-columns: repeat(3, 1fr); gap: 6px; }
.segmented button { padding: 8px 6px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-secondary); cursor: pointer; }
.segmented button.active { border-color: var(--hnb-color-primary); background: var(--hnb-color-primary); color: var(--hnb-color-text-on-accent); }
.segmented button:disabled { opacity: 0.45; cursor: not-allowed; }
.label-row { display: grid; grid-template-columns: 1fr 1fr 30px; gap: 6px; }
.label-row button, .add-label { border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: transparent; color: var(--hnb-color-primary); cursor: pointer; }
.add-label { align-self: flex-start; padding: 6px 10px; }
.yaml-editor { width: 100%; box-sizing: border-box; padding: 12px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: #0b1020; color: #d9e2f2; font: 12px/1.55 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; resize: vertical; }
.yaml-editor--readonly { min-height: calc(100vh - 150px); resize: none; }
@media (max-width: 900px) {
  .storage-toolbar { align-items: stretch; flex-direction: column; }
  .storage-filters { justify-content: flex-start; }
  .storage-pagination { grid-template-columns: 1fr; }
  .page-buttons, .page-jump { justify-content: flex-start; flex-wrap: wrap; }
}
@media (max-width: 560px) {
  .storage-page { padding: 14px 12px; }
  .storage-tabs { overflow-x: auto; }
  .storage-tabs button { padding: 8px 12px; white-space: nowrap; }
  .compact-field { width: 100%; justify-content: space-between; }
  .compact-field select, .search-input { flex: 1; width: auto; }
  .segmented { grid-template-columns: 1fr; }
}
</style>
