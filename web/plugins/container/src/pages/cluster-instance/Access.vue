<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { stringify } from 'yaml'
import { HNBButton, HNBConfirmation, HNBPageShell } from '@hnb/ui-kit'
import { listNamespaces, listWorkspaceClusters, type ContainerCluster } from '../../api/containerApi'
import {
  deleteAccessIngress,
  deleteAccessNetworkPolicy,
  deleteAccessService,
  deleteMetalLBPool,
  ingressResource,
  listAccessIngresses,
  listAccessNetworkPolicies,
  listAccessServices,
  listMetalLBPools,
  metalLBResource,
  networkPolicyResource,
  saveMetalLBPool,
  serviceResource,
  validIpRange,
  type AccessIngress,
  type AccessNetworkPolicy,
  type AccessService,
  type MetalLBPool,
} from '../../api/accessApi'
import NetworkDrawer from '../network/NetworkDrawer.vue'

type AccessTab = 'service' | 'ingress' | 'metallb' | 'networkPolicy'
type AccessItem = AccessService | AccessIngress | MetalLBPool | AccessNetworkPolicy

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const tabs: AccessTab[] = ['service', 'ingress', 'metallb', 'networkPolicy']
const routeTab = String(route.query.tab ?? '') as AccessTab
const pathTab: AccessTab = route.path.includes('/ingress/') ? 'ingress' : route.path.includes('/network-policy/') ? 'networkPolicy' : 'service'
const activeTab = ref<AccessTab>(tabs.includes(routeTab) ? routeTab : pathTab)

const clusters = ref<ContainerCluster[]>([])
const namespaces = ref<string[]>(['default', 'argocd'])
const clusterId = ref('')
const serviceNamespace = ref('default')
const ingressNamespace = ref('argocd')
const policyNamespace = ref('argocd')
const searchType = ref('name')
const searchInput = ref('')
const appliedSearch = ref('')
const loading = ref(false)
const loadError = ref('')
const notice = ref('')
const metalLBInstalled = ref(true)

const services = ref<AccessService[]>([])
const ingresses = ref<AccessIngress[]>([])
const metalPools = ref<MetalLBPool[]>([])
const policies = ref<AccessNetworkPolicy[]>([])
const page = ref(1)
const pageSize = ref(10)
const jumpPage = ref('')
const moreMenuKey = ref('')
const columnMenuOpen = ref(false)
const ingressColumns = ref(['rules', 'createdAt'])
const metalColumns = ref(['description', 'startIp', 'endIp', 'availableIps', 'usedIps', 'createdAt'])

const clusterOptions = computed(() => clusters.value.map((item) => ({ value: item.id, label: item.display_name || item.name })))
const namespaceOptions = computed(() => Array.from(new Set(['default', 'argocd', ...namespaces.value])))
const currentNamespace = computed(() => activeTab.value === 'service' ? serviceNamespace.value : activeTab.value === 'ingress' ? ingressNamespace.value : policyNamespace.value)
const namespaceModel = computed({
  get: () => currentNamespace.value,
  set: (value: string) => {
    if (activeTab.value === 'service') serviceNamespace.value = value
    else if (activeTab.value === 'ingress') ingressNamespace.value = value
    else policyNamespace.value = value
  },
})

function includesSearch(...values: unknown[]): boolean {
  const needle = appliedSearch.value.toLowerCase()
  return !needle || values.some((value) => String(value ?? '').toLowerCase().includes(needle))
}

const filteredServices = computed(() => services.value.filter((item) => includesSearch(item.name)))
const filteredIngresses = computed(() => ingresses.value.filter((item) => searchType.value === 'rules'
  ? includesSearch(...item.rules.map((rule) => `${rule.path}:${rule.serviceName}`))
  : includesSearch(item.name)))
const filteredMetalPools = computed(() => metalPools.value.filter((item) => searchType.value === 'description'
  ? includesSearch(item.description) : searchType.value === 'startIp' ? includesSearch(item.startIp) : includesSearch(item.name)))
const filteredPolicies = computed(() => policies.value.filter((item) => includesSearch(item.name)))
const pageCount = computed(() => Math.max(1, Math.ceil(filteredServices.value.length / pageSize.value)))
const pagedServices = computed(() => filteredServices.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const pageStart = computed(() => filteredServices.value.length ? (page.value - 1) * pageSize.value + 1 : 0)
const pageEnd = computed(() => Math.min(page.value * pageSize.value, filteredServices.value.length))
const pageNumbers = computed(() => Array.from({ length: pageCount.value }, (_, index) => index + 1))

function showNotice(message: string): void {
  notice.value = message
  window.setTimeout(() => { if (notice.value === message) notice.value = '' }, 2500)
}

async function loadActive(): Promise<void> {
  if (!clusterId.value) return
  loading.value = true
  loadError.value = ''
  try {
    if (activeTab.value === 'service') services.value = await listAccessServices(clusterId.value, serviceNamespace.value)
    else if (activeTab.value === 'ingress') ingresses.value = await listAccessIngresses(clusterId.value, ingressNamespace.value)
    else if (activeTab.value === 'metallb') {
      metalLBInstalled.value = true
      try {
        metalPools.value = await listMetalLBPools(clusterId.value)
      } catch (error) {
        if ((error as { status?: unknown } | null)?.status === 404) {
          metalLBInstalled.value = false
          metalPools.value = []
        } else throw error
      }
    }
    else policies.value = await listAccessNetworkPolicies(clusterId.value, policyNamespace.value)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.access.loadError')
  } finally {
    loading.value = false
  }
}

async function initialize(): Promise<void> {
  try {
    clusters.value = await listWorkspaceClusters()
    clusterId.value = clusters.value[0]?.id ?? ''
    const items = await listNamespaces({ clusterId: clusterId.value || undefined })
    namespaces.value = Array.from(new Set(['default', 'argocd', ...items.map((item) => item.name)]))
    await loadActive()
    if (route.query.saved === '1') showNotice(t('container.access.message.saved'))
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.access.loadError')
  }
}

function query(): void {
  appliedSearch.value = searchInput.value.trim()
  page.value = 1
  void loadActive()
}

function resetFilters(): void {
  searchType.value = 'name'
  searchInput.value = ''
  appliedSearch.value = ''
  if (activeTab.value === 'ingress') ingressNamespace.value = 'argocd'
  else if (activeTab.value === 'networkPolicy') policyNamespace.value = 'argocd'
  page.value = 1
  void loadActive()
}

function createResource(): void {
  if (activeTab.value === 'metallb') {
    openMetalCreate()
    return
  }
  const segment = activeTab.value === 'networkPolicy' ? 'network-policy' : activeTab.value
  void router.push({ path: `/container/instances/access/${segment}/create`, query: { cluster: clusterId.value, namespace: currentNamespace.value } })
}

function editResource(kind: 'service' | 'ingress' | 'networkPolicy', name: string, namespace: string): void {
  const segment = kind === 'networkPolicy' ? 'network-policy' : kind
  void router.push({ path: `/container/instances/access/${segment}/${encodeURIComponent(name)}/edit`, query: { cluster: clusterId.value, namespace } })
}

function formatDate(value: string): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

const detailVisible = ref(false)
const detailTitle = ref('')
const detailRows = ref<Array<{ label: string; value: string }>>([])
function showDetail(item: AccessItem): void {
  detailTitle.value = item.name
  if ('ports' in item) detailRows.value = [
    { label: t('container.access.columns.accessType'), value: t(`container.access.serviceType.${item.type}`) },
    { label: t('container.access.columns.clusterIp'), value: item.clusterIp || 'None' },
    { label: t('container.access.detail.ports'), value: item.ports.map((port) => `${port.port}:${port.targetPort} ${port.protocol}`).join('\n') },
    { label: t('container.access.detail.selector'), value: Object.entries(item.selector).map(([key, value]) => `${key}=${value}`).join(', ') || '--' },
  ]
  else if ('rules' in item) detailRows.value = [
    { label: t('container.access.columns.namespace'), value: item.namespace },
    { label: t('container.access.detail.rules'), value: item.rules.map((rule) => `${rule.host}${rule.path} : ${rule.serviceName}:${rule.servicePort}`).join('\n') },
  ]
  else if ('startIp' in item) detailRows.value = [
    { label: t('container.access.columns.description'), value: item.description || '--' },
    { label: t('container.access.columns.startIp'), value: item.startIp }, { label: t('container.access.columns.endIp'), value: item.endIp },
    { label: t('container.access.columns.availableIps'), value: String(item.availableIps) }, { label: t('container.access.columns.usedIps'), value: String(item.usedIps) },
  ]
  else detailRows.value = [
    { label: t('container.access.columns.namespace'), value: item.namespace }, { label: t('container.access.columns.policyTypes'), value: item.policyTypes.join(', ') },
    { label: t('container.access.columns.description'), value: item.description || '--' },
    { label: t('container.access.detail.selector'), value: Object.entries(item.matchLabels).map(([key, value]) => `${key}=${value}`).join(', ') },
  ]
  detailVisible.value = true
  moreMenuKey.value = ''
}

const yamlVisible = ref(false)
const yamlContent = ref('')
function showYaml(item: AccessService | AccessIngress | AccessNetworkPolicy): void {
  yamlContent.value = stringify('ports' in item ? serviceResource(item) : 'rules' in item ? ingressResource(item) : networkPolicyResource(item))
  yamlVisible.value = true
  moreMenuKey.value = ''
}

const metalVisible = ref(false)
const metalBusy = ref(false)
const metalError = ref('')
const metalValidation = ref('')
const editingMetal = ref('')
const metalForm = ref({ name: '', description: '', startIp: '', endIp: '' })

function openMetalCreate(): void {
  editingMetal.value = ''
  metalForm.value = { name: '', description: '', startIp: '', endIp: '' }
  metalError.value = ''
  metalValidation.value = ''
  metalVisible.value = true
}

function openMetalEdit(item: MetalLBPool): void {
  editingMetal.value = item.name
  metalForm.value = { name: item.name, description: item.description, startIp: item.startIp, endIp: item.endIp }
  metalError.value = ''
  metalValidation.value = ''
  metalVisible.value = true
}

function validateMetal(): boolean {
  const valid = validIpRange(metalForm.value.startIp.trim(), metalForm.value.endIp.trim())
  metalValidation.value = t(`container.access.validation.${valid ? 'ipValid' : 'ipRange'}`)
  return valid
}

async function saveMetal(): Promise<void> {
  if (!metalForm.value.name.trim() || !validateMetal()) {
    metalError.value = !metalForm.value.name.trim() ? t('container.access.validation.required') : t('container.access.validation.ipRange')
    return
  }
  metalBusy.value = true
  metalError.value = ''
  try {
    await saveMetalLBPool(clusterId.value, { ...metalForm.value, availableIps: 0, usedIps: 0, createdAt: '' }, editingMetal.value || undefined)
    metalVisible.value = false
    showNotice(t('container.access.message.saved'))
    await loadActive()
  } catch (error) {
    metalError.value = error instanceof Error ? error.message : String(error)
  } finally {
    metalBusy.value = false
  }
}

const confirmVisible = ref(false)
const confirmBusy = ref(false)
const confirmError = ref('')
const deleteTarget = ref<{ tab: AccessTab; name: string; namespace: string } | null>(null)
function requestDelete(tab: AccessTab, name: string, namespace = ''): void {
  deleteTarget.value = { tab, name, namespace }
  confirmError.value = ''
  confirmVisible.value = true
  moreMenuKey.value = ''
}

async function confirmDelete(): Promise<void> {
  if (!deleteTarget.value) return
  confirmBusy.value = true
  try {
    const target = deleteTarget.value
    if (target.tab === 'service') await deleteAccessService(clusterId.value, target.namespace, target.name)
    else if (target.tab === 'ingress') await deleteAccessIngress(clusterId.value, target.namespace, target.name)
    else if (target.tab === 'metallb') await deleteMetalLBPool(clusterId.value, target.name)
    else await deleteAccessNetworkPolicy(clusterId.value, target.namespace, target.name)
    confirmVisible.value = false
    showNotice(t('container.access.message.deleted'))
    await loadActive()
  } catch (error) {
    confirmError.value = error instanceof Error ? error.message : String(error)
  } finally {
    confirmBusy.value = false
  }
}

function changePageSize(event: Event): void { pageSize.value = Number((event.target as HTMLSelectElement).value); page.value = 1 }
function jumpToPage(): void { const target = Number(jumpPage.value); if (Number.isInteger(target)) page.value = Math.max(1, Math.min(pageCount.value, target)); jumpPage.value = '' }

watch(activeTab, () => {
  searchInput.value = ''
  appliedSearch.value = ''
  searchType.value = 'name'
  page.value = 1
  moreMenuKey.value = ''
  columnMenuOpen.value = false
  void router.replace({ query: { tab: activeTab.value } })
  void loadActive()
})
watch(clusterId, (value, oldValue) => { if (oldValue && value !== oldValue) void loadActive() })
watch(pageCount, (count) => { if (page.value > count) page.value = count })
onMounted(initialize)
</script>

<template>
  <HNBPageShell :title="t('container.access.title')">
    <template #actions><a class="help-link" href="https://docs.hnb.example.io/container/access" target="_blank" rel="noopener noreferrer">? {{ t('container.access.help') }}</a></template>
    <nav class="access-tabs" role="tablist" :aria-label="t('container.access.title')"><button v-for="tab in tabs" :key="tab" type="button" role="tab" :aria-selected="activeTab === tab" :class="{ active: activeTab === tab }" @click="activeTab = tab">{{ t(`container.access.tabs.${tab}`) }}</button></nav>
    <div class="access-toolbar">
      <HNBButton :disabled="activeTab === 'metallb' && !metalLBInstalled" @click="createResource">{{ t(`container.access.toolbar.${activeTab === 'service' ? 'addService' : activeTab === 'networkPolicy' ? 'add' : activeTab === 'ingress' ? 'new' : 'create'}`) }}</HNBButton>
      <div class="filters">
        <label><span>{{ t('container.access.toolbar.cluster') }}</span><select v-model="clusterId"><option v-for="item in clusterOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
        <label v-if="activeTab !== 'metallb'"><span>{{ t(activeTab === 'ingress' ? 'container.access.toolbar.namespaceColon' : 'container.access.toolbar.namespace') }}</span><select v-model="namespaceModel"><option v-for="item in namespaceOptions" :key="item" :value="item">{{ item }}</option></select></label>
        <select v-if="activeTab === 'ingress' || activeTab === 'metallb'" v-model="searchType" class="search-type"><option value="name">{{ t('container.access.toolbar.name') }}</option><option v-if="activeTab === 'ingress'" value="rules">{{ t('container.access.columns.rules') }}</option><option v-if="activeTab === 'metallb'" value="description">{{ t('container.access.toolbar.description') }}</option><option v-if="activeTab === 'metallb'" value="startIp">{{ t('container.access.toolbar.startIp') }}</option></select>
        <input v-model="searchInput" type="search" :placeholder="t(activeTab === 'service' || activeTab === 'networkPolicy' ? 'container.access.toolbar.namePlaceholder' : 'container.access.toolbar.searchPlaceholder')" @keyup.enter="query">
        <HNBButton size="small" @click="query">{{ t('container.access.toolbar.query') }}</HNBButton>
        <button class="icon-button" type="button" :aria-label="t('container.access.toolbar.refresh')" :title="t('container.access.toolbar.refresh')" @click="loadActive">↻</button>
        <button v-if="activeTab === 'ingress' || activeTab === 'metallb'" class="icon-button" type="button" :aria-label="t('container.access.toolbar.reset')" :title="t('container.access.toolbar.reset')" @click="resetFilters">⊗</button>
        <div v-if="activeTab === 'ingress' || activeTab === 'metallb'" class="column-settings"><button class="icon-button" type="button" :aria-label="t('container.access.toolbar.columns')" @click="columnMenuOpen = !columnMenuOpen">⚙</button><div v-if="columnMenuOpen" class="column-menu"><label v-for="key in (activeTab === 'ingress' ? ['rules', 'createdAt'] : ['description', 'startIp', 'endIp', 'availableIps', 'usedIps', 'createdAt'])" :key="key"><input type="checkbox" :checked="(activeTab === 'ingress' ? ingressColumns : metalColumns).includes(key)" @change="activeTab === 'ingress' ? (ingressColumns = ingressColumns.includes(key) ? ingressColumns.filter((item) => item !== key) : [...ingressColumns, key]) : (metalColumns = metalColumns.includes(key) ? metalColumns.filter((item) => item !== key) : [...metalColumns, key])"><span>{{ t(`container.access.columns.${key}`) }}</span></label></div></div>
      </div>
    </div>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p><p v-if="loadError" class="error" role="alert">{{ loadError }}</p><p v-if="loading" class="loading" role="status">{{ t('container.access.loading') }}</p>

    <div v-else class="table-wrap">
      <table v-if="activeTab === 'service'" class="access-table"><thead><tr><th>{{ t('container.access.columns.name') }}</th><th>{{ t('container.access.columns.accessType') }}</th><th>{{ t('container.access.columns.clusterIp') }}</th><th>{{ t('container.access.columns.internalConnection') }}</th><th>{{ t('container.access.columns.createdAt') }}</th><th>{{ t('container.access.columns.actions') }}</th></tr></thead><tbody><tr v-for="item in pagedServices" :key="item.name"><td><button class="name-link ellipsis" type="button" :title="item.name" @click="showDetail(item)">{{ item.name }}</button></td><td>{{ t(`container.access.serviceType.${item.type}`) }}</td><td>{{ item.clusterIp || 'None' }}</td><td><span v-for="port in item.ports" :key="`${port.name}-${port.port}`" class="stack-line">{{ port.port }}:{{ port.targetPort }} {{ port.protocol }}</span></td><td>{{ formatDate(item.createdAt) }}</td><td><div class="row-actions"><button type="button" @click="showYaml(item)">{{ t('container.access.action.yaml') }}</button><button type="button" @click="editResource('service', item.name, item.namespace)">{{ t('container.access.action.edit') }}</button><button type="button" @click="requestDelete('service', item.name, item.namespace)">{{ t('container.access.action.delete') }}</button></div></td></tr><tr v-if="!pagedServices.length"><td colspan="6" class="empty">{{ t('container.access.empty') }}</td></tr></tbody></table>

      <table v-else-if="activeTab === 'ingress'" class="access-table"><thead><tr><th>{{ t('container.access.columns.name') }}</th><th v-if="ingressColumns.includes('rules')">{{ t('container.access.columns.rules') }}</th><th v-if="ingressColumns.includes('createdAt')">{{ t('container.access.columns.createdAt') }}</th><th>{{ t('container.access.columns.actions') }}</th></tr></thead><tbody><tr v-for="item in filteredIngresses" :key="item.name"><td><button class="name-link ellipsis" type="button" :title="item.name" @click="showDetail(item)">{{ item.name }}</button></td><td v-if="ingressColumns.includes('rules')"><span v-for="rule in item.rules" :key="`${rule.host}-${rule.path}`" class="stack-line">{{ rule.path }} : {{ rule.serviceName }}</span></td><td v-if="ingressColumns.includes('createdAt')">{{ formatDate(item.createdAt) }}</td><td><div class="row-actions menu-host"><button type="button" @click="showDetail(item)">{{ t('container.access.action.detail') }}</button><button type="button" @click="moreMenuKey = moreMenuKey === `ingress:${item.name}` ? '' : `ingress:${item.name}`">{{ t('container.access.action.more') }}</button><div v-if="moreMenuKey === `ingress:${item.name}`" class="more-menu"><button type="button" @click="showYaml(item)">{{ t('container.access.action.yaml') }}</button><button type="button" @click="editResource('ingress', item.name, item.namespace)">{{ t('container.access.action.edit') }}</button><button type="button" @click="requestDelete('ingress', item.name, item.namespace)">{{ t('container.access.action.delete') }}</button></div></div></td></tr><tr v-if="!filteredIngresses.length"><td colspan="4" class="empty">{{ t('container.access.empty') }}</td></tr></tbody></table>

      <table v-else-if="activeTab === 'metallb'" class="access-table"><thead><tr><th>{{ t('container.access.columns.name') }}</th><th v-if="metalColumns.includes('description')">{{ t('container.access.columns.description') }}</th><th v-if="metalColumns.includes('startIp')">{{ t('container.access.columns.startIp') }}</th><th v-if="metalColumns.includes('endIp')">{{ t('container.access.columns.endIp') }}</th><th v-if="metalColumns.includes('availableIps')">{{ t('container.access.columns.availableIps') }}</th><th v-if="metalColumns.includes('usedIps')">{{ t('container.access.columns.usedIps') }}</th><th v-if="metalColumns.includes('createdAt')">{{ t('container.access.columns.createdAt') }}</th><th>{{ t('container.access.columns.actions') }}</th></tr></thead><tbody><tr v-for="item in filteredMetalPools" :key="item.name"><td><button class="name-link ellipsis" type="button" :title="item.name" @click="showDetail(item)">{{ item.name }}</button></td><td v-if="metalColumns.includes('description')"><span class="ellipsis" :title="item.description">{{ item.description || '--' }}</span></td><td v-if="metalColumns.includes('startIp')">{{ item.startIp }}</td><td v-if="metalColumns.includes('endIp')">{{ item.endIp }}</td><td v-if="metalColumns.includes('availableIps')">{{ item.availableIps }}</td><td v-if="metalColumns.includes('usedIps')">{{ item.usedIps }}</td><td v-if="metalColumns.includes('createdAt')">{{ formatDate(item.createdAt) }}</td><td><div class="row-actions"><button type="button" @click="showDetail(item)">{{ t('container.access.action.detail') }}</button><button type="button" @click="openMetalEdit(item)">{{ t('container.access.action.edit') }}</button><button type="button" @click="requestDelete('metallb', item.name)">{{ t('container.access.action.delete') }}</button></div></td></tr><tr v-if="!filteredMetalPools.length"><td colspan="8" class="empty">{{ t(metalLBInstalled ? 'container.access.empty' : 'container.access.metallbNotInstalled') }}</td></tr></tbody></table>

      <table v-else class="access-table"><thead><tr><th>{{ t('container.access.columns.name') }}</th><th>{{ t('container.access.columns.namespace') }}</th><th>{{ t('container.access.columns.policyTypes') }}</th><th>{{ t('container.access.columns.description') }}</th><th>{{ t('container.access.columns.createdAt') }}</th><th>{{ t('container.access.columns.actions') }}</th></tr></thead><tbody><tr v-for="item in filteredPolicies" :key="item.name"><td><button class="name-link ellipsis" type="button" :title="item.name" @click="showDetail(item)">{{ item.name }}</button></td><td>{{ item.namespace }}</td><td>{{ item.policyTypes.join(', ') }}</td><td><span class="ellipsis" :title="item.description">{{ item.description || '--' }}</span></td><td>{{ formatDate(item.createdAt) }}</td><td><div class="row-actions"><button type="button" @click="showYaml(item)">{{ t('container.access.action.yaml') }}</button><button type="button" @click="editResource('networkPolicy', item.name, item.namespace)">{{ t('container.access.action.edit') }}</button><button type="button" @click="requestDelete('networkPolicy', item.name, item.namespace)">{{ t('container.access.action.delete') }}</button></div></td></tr><tr v-if="!filteredPolicies.length"><td colspan="6" class="empty">{{ t('container.access.empty') }}</td></tr></tbody></table>
    </div>

    <footer v-if="activeTab === 'service' && !loading" class="pagination"><span>{{ t('container.access.pagination.range', { start: pageStart, end: pageEnd, total: filteredServices.length }) }}</span><div class="page-buttons"><button type="button" :disabled="page <= 1" @click="page--">‹</button><button v-for="number in pageNumbers" :key="number" type="button" :class="{ active: number === page }" @click="page = number">{{ number }}</button><button type="button" :disabled="page >= pageCount" @click="page++">›</button></div><div class="page-jump"><select :value="pageSize" @change="changePageSize"><option v-for="size in [10, 20, 50]" :key="size" :value="size">{{ t('container.access.pagination.pageSize', { size }) }}</option></select><span>{{ t('container.access.pagination.jump') }}</span><input v-model="jumpPage" type="number" min="1" :max="pageCount" @keyup.enter="jumpToPage"><span>{{ t('container.access.pagination.pageUnit', { pages: pageCount }) }}</span></div></footer>

    <NetworkDrawer v-model="detailVisible" :title="detailTitle || t('container.access.drawer.detail')" hide-confirm><dl class="detail-list"><div v-for="row in detailRows" :key="row.label"><dt>{{ row.label }}</dt><dd>{{ row.value }}</dd></div></dl></NetworkDrawer>
    <NetworkDrawer v-model="yamlVisible" :title="t('container.access.drawer.yaml')" hide-confirm><textarea class="yaml-view" :value="yamlContent" readonly rows="30" /></NetworkDrawer>
    <NetworkDrawer v-model="metalVisible" :title="t(`container.access.drawer.${editingMetal ? 'metalEdit' : 'metalCreate'}`)" :busy="metalBusy" :error="metalError" @confirm="saveMetal"><form class="metal-form" @submit.prevent="saveMetal"><label><span>{{ t('container.access.form.cluster') }} *</span><select v-model="clusterId"><option value="">{{ t('container.access.form.select') }}</option><option v-for="item in clusterOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label><label><span>{{ t('container.access.form.networkName') }} * ?</span><input v-model="metalForm.name" :disabled="!!editingMetal" :placeholder="t('container.access.form.input')"></label><label><span>{{ t('container.access.columns.startIp') }} * ?</span><input v-model="metalForm.startIp" :placeholder="t('container.access.form.input')"></label><label><span>{{ t('container.access.columns.endIp') }} * ?</span><input v-model="metalForm.endIp" :placeholder="t('container.access.form.input')"></label><label><span>{{ t('container.access.form.description') }}</span><div class="counter"><textarea v-model="metalForm.description" maxlength="200" rows="4" :placeholder="t('container.access.form.input')" /><small>{{ metalForm.description.length }}/200</small></div></label><HNBButton class="validate-button" type="button" @click="validateMetal">{{ t('container.access.action.validate') }}</HNBButton><p v-if="metalValidation" :class="{ valid: validIpRange(metalForm.startIp, metalForm.endIp) }">{{ metalValidation }}</p></form></NetworkDrawer>
    <HNBConfirmation v-model="confirmVisible" :title="t('container.access.confirm.title')" :description="t('container.access.confirm.message', { name: deleteTarget?.name ?? '' })" :loading="confirmBusy" :error="confirmError" :confirm-text="t('container.access.action.confirm')" :cancel-text="t('container.access.action.cancel')" danger @confirm="confirmDelete" />
  </HNBPageShell>
</template>

<style scoped>
.hnb-page-shell { padding: 18px 20px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-surface); }
.help-link { color: var(--hnb-color-primary); font-size: 13px; text-decoration: none; }
.access-tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--hnb-color-divider); }
.access-tabs button { padding: 9px 18px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--hnb-color-text-secondary); cursor: pointer; }
.access-tabs button.active { border-bottom-color: var(--hnb-color-primary); color: var(--hnb-color-primary); font-weight: 600; }
.access-toolbar { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; }
.filters { display: flex; align-items: flex-end; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
.filters label { display: flex; align-items: center; gap: 6px; color: var(--hnb-color-text-secondary); font-size: 13px; }
.filters select, .filters input, .search-type { min-height: 34px; padding: 6px 9px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); }
.filters input { width: 190px; }
.icon-button { width: 34px; height: 34px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-secondary); cursor: pointer; }
.column-settings { position: relative; }
.column-menu { position: absolute; z-index: 20; top: 40px; right: 0; min-width: 170px; padding: 7px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-surface); box-shadow: var(--hnb-shadow-3); }
.column-menu label { display: flex; padding: 6px; }
.notice, .error, .loading { margin: 0; padding: 8px 10px; border-radius: var(--hnb-radius-sm); font-size: 13px; }
.notice { color: var(--hnb-color-status-success); background: var(--hnb-color-status-success-surface); }.error { color: var(--hnb-color-status-danger); background: var(--hnb-color-status-danger-surface); }.loading { color: var(--hnb-color-text-secondary); }
.table-wrap { overflow-x: auto; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }
.access-table { width: 100%; min-width: 900px; border-collapse: collapse; table-layout: fixed; font-size: 13px; }
.access-table th, .access-table td { padding: 11px 12px; border-bottom: 1px solid var(--hnb-color-divider); text-align: left; vertical-align: middle; }
.access-table th { background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-secondary); white-space: nowrap; }.access-table th:first-child,.access-table td:first-child{width:190px}.access-table th:last-child,.access-table td:last-child{width:220px}
.ellipsis { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.name-link, .row-actions button { padding: 0; border: 0; background: transparent; color: var(--hnb-color-primary); cursor: pointer; font-size: 13px; text-align: left; }
.stack-line { display: block; line-height: 1.7; white-space: nowrap; }
.row-actions { display: flex; align-items: center; gap: 10px; white-space: nowrap; }.menu-host{position:relative}
.more-menu { position: absolute; z-index: 10; top: 24px; right: 0; display: grid; min-width: 110px; padding: 6px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-surface); box-shadow: var(--hnb-shadow-2); }.more-menu button{padding:7px 9px}
.empty { padding: 38px !important; text-align: center !important; color: var(--hnb-color-text-tertiary); }
.pagination { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 12px; color: var(--hnb-color-text-secondary); font-size: 13px; }.page-buttons,.page-jump{display:flex;align-items:center;gap:5px}.page-buttons button{min-width:30px;height:30px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary)}.page-buttons button.active{background:var(--hnb-color-primary);color:var(--hnb-color-text-on-accent)}.page-jump{justify-content:flex-end}.page-jump select,.page-jump input{height:30px;padding:4px 7px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.page-jump input{width:50px}
.detail-list { display: grid; gap: 12px; margin: 0; }.detail-list div{padding-bottom:10px;border-bottom:1px solid var(--hnb-color-divider)}.detail-list dt{color:var(--hnb-color-text-secondary);font-size:12px}.detail-list dd{margin:5px 0 0;white-space:pre-line;overflow-wrap:anywhere}
.yaml-view { width: 100%; min-height: calc(100vh - 150px); box-sizing: border-box; padding: 12px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: #0b1020; color: #d9e2f2; font: 12px/1.55 ui-monospace, monospace; resize: none; }
.metal-form { display: flex; flex-direction: column; gap: 14px; }.metal-form label{display:flex;flex-direction:column;gap:6px;color:var(--hnb-color-text-secondary);font-size:13px}.metal-form input,.metal-form select,.metal-form textarea{width:100%;box-sizing:border-box;padding:8px 10px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.counter{position:relative}.counter small{position:absolute;right:8px;bottom:7px;color:var(--hnb-color-text-tertiary)}.validate-button{align-self:flex-start}.metal-form p{margin:0;color:var(--hnb-color-status-danger);font-size:13px}.metal-form p.valid{color:var(--hnb-color-status-success)}
@media(max-width:900px){.access-toolbar{align-items:stretch;flex-direction:column}.filters{justify-content:flex-start}.pagination{grid-template-columns:1fr}.page-jump{justify-content:flex-start}}
@media(max-width:560px){.hnb-page-shell{padding:14px 12px}.access-tabs{overflow-x:auto}.access-tabs button{white-space:nowrap;padding:8px 12px}.filters label{width:100%;justify-content:space-between}.filters label select,.filters input{flex:1;width:auto}}
</style>
