<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { stringify } from 'yaml'
import { HNBButton, HNBConfirmation, HNBPageShell } from '@hnb/ui-kit'
import { listNamespaces, listWorkspaceClusters, type ContainerCluster } from '../../api/containerApi'
import {
  createSecret,
  deleteConfigMap,
  deleteSecret,
  listConfigMaps,
  listSecrets,
  saveConfigMap,
  type ConfigMapItem,
  type SecretItem,
} from '../../api/configApi'
import NetworkDrawer from '../network/NetworkDrawer.vue'

type ConfigTab = 'configMap' | 'secret'
type DataRow = { key: string; value: string }

const { t } = useI18n()
const tabs: ConfigTab[] = ['configMap', 'secret']
const activeTab = ref<ConfigTab>('configMap')
const clusters = ref<ContainerCluster[]>([])
const clusterId = ref('')
const namespaces = ref<string[]>(['argocd', 'default'])
const namespace = ref('argocd')
const searchType = ref('name')
const searchInput = ref('')
const appliedSearch = ref('')
const loading = ref(false)
const loadError = ref('')
const notice = ref('')
const initialized = ref(false)
const configMaps = ref<ConfigMapItem[]>([])
const secrets = ref<SecretItem[]>([])
const page = ref(1)
const pageSize = ref(10)
const jumpPage = ref('')
const columnMenuOpen = ref(false)
const configColumns = ref(['createdAt'])
const secretColumns = ref(['type', 'createdAt'])
const moreMenuKey = ref('')

const clusterOptions = computed(() => clusters.value.map((item) => ({ value: item.id, label: item.display_name || item.name })))
const filteredConfigMaps = computed(() => configMaps.value.filter((item) => !appliedSearch.value || item.name.toLowerCase().includes(appliedSearch.value.toLowerCase())))
const filteredSecrets = computed(() => secrets.value.filter((item) => !appliedSearch.value || item.name.toLowerCase().includes(appliedSearch.value.toLowerCase())))
const filteredItems = computed(() => activeTab.value === 'configMap' ? filteredConfigMaps.value : filteredSecrets.value)
const pageCount = computed(() => Math.max(1, Math.ceil(filteredItems.value.length / pageSize.value)))
const pageStart = computed(() => filteredItems.value.length ? (page.value - 1) * pageSize.value + 1 : 0)
const pageEnd = computed(() => Math.min(page.value * pageSize.value, filteredItems.value.length))
const pageNumbers = computed(() => Array.from({ length: pageCount.value }, (_, index) => index + 1))
const pagedConfigMaps = computed(() => filteredConfigMaps.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const pagedSecrets = computed(() => filteredSecrets.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

function showNotice(message: string): void {
  notice.value = message
  window.setTimeout(() => { if (notice.value === message) notice.value = '' }, 2500)
}

function formatDate(value: string): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function loadResources(): Promise<void> {
  if (!clusterId.value || !namespace.value) return
  loading.value = true
  loadError.value = ''
  try {
    if (activeTab.value === 'configMap') configMaps.value = await listConfigMaps(clusterId.value, namespace.value)
    else secrets.value = await listSecrets(clusterId.value, namespace.value)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.config.loadError')
  } finally {
    loading.value = false
  }
}

async function loadNamespaceOptions(): Promise<void> {
  const items = await listNamespaces({ clusterId: clusterId.value || undefined })
  namespaces.value = Array.from(new Set(['argocd', 'default', ...items.map((item) => item.name)]))
  if (!namespaces.value.includes(namespace.value)) namespace.value = namespaces.value.includes('argocd') ? 'argocd' : namespaces.value[0] || 'default'
}

async function initialize(): Promise<void> {
  try {
    clusters.value = await listWorkspaceClusters()
    clusterId.value = clusters.value[0]?.id ?? ''
    await loadNamespaceOptions()
    initialized.value = true
    await loadResources()
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.config.loadError')
  }
}

function query(): void {
  appliedSearch.value = searchInput.value.trim()
  page.value = 1
  void loadResources()
}

function resetFilters(): void {
  searchType.value = 'name'
  searchInput.value = ''
  appliedSearch.value = ''
  namespace.value = namespaces.value.includes('argocd') ? 'argocd' : namespaces.value[0] || 'default'
  page.value = 1
  columnMenuOpen.value = false
  void loadResources()
}

const formVisible = ref(false)
const formBusy = ref(false)
const formError = ref('')
const editingConfigName = ref('')
const formName = ref('')
const formCluster = ref('')
const formNamespace = ref('')
const formNamespaces = ref<string[]>([])
const secretType = ref('Opaque')
const dataRows = ref<DataRow[]>([{ key: '', value: '' }])
const editingConfigData = ref<Record<string, string>>({})

function openCreate(): void {
  editingConfigName.value = ''
  formName.value = ''
  formCluster.value = clusterId.value
  formNamespace.value = namespace.value
  formNamespaces.value = [...namespaces.value]
  secretType.value = 'Opaque'
  dataRows.value = [{ key: '', value: '' }]
  editingConfigData.value = {}
  formError.value = ''
  formVisible.value = true
}

function openConfigEdit(item: ConfigMapItem): void {
  editingConfigName.value = item.name
  formName.value = item.name
  formCluster.value = clusterId.value
  formNamespace.value = item.namespace
  formNamespaces.value = [...namespaces.value]
  editingConfigData.value = { ...item.data }
  dataRows.value = Object.entries(item.data).map(([key, value]) => ({ key, value }))
  if (!dataRows.value.length) dataRows.value = [{ key: '', value: '' }]
  formError.value = ''
  formVisible.value = true
}

function addDataRow(): void {
  dataRows.value.push({ key: '', value: '' })
}

function removeDataRow(index: number): void {
  if (dataRows.value.length > 1) dataRows.value.splice(index, 1)
}

function focusValue(index: number): void {
  document.getElementById(`config-value-${index}`)?.focus()
}

function dataRecord(): Record<string, string> | null {
  const result: Record<string, string> = {}
  for (const row of dataRows.value) {
    const key = row.key.trim()
    if (!key || Object.prototype.hasOwnProperty.call(result, key)) return null
    result[key] = row.value
  }
  return result
}

async function submitForm(): Promise<void> {
  const data = dataRecord()
  if (!formName.value.trim() || !formCluster.value || !formNamespace.value || !data || (activeTab.value === 'secret' && !secretType.value)) {
    formError.value = t('container.config.validation.required')
    return
  }
  formBusy.value = true
  formError.value = ''
  try {
    if (activeTab.value === 'configMap') {
      await saveConfigMap(formCluster.value, { name: formName.value.trim(), namespace: formNamespace.value, data }, editingConfigName.value || undefined, editingConfigData.value)
    } else {
      await createSecret(formCluster.value, { name: formName.value.trim(), namespace: formNamespace.value, type: secretType.value, stringData: data })
    }
    clusterId.value = formCluster.value
    namespace.value = formNamespace.value
    formVisible.value = false
    showNotice(t('container.config.message.saved'))
    await loadResources()
  } catch (error) {
    formError.value = error instanceof Error ? error.message : String(error)
  } finally {
    formBusy.value = false
  }
}

const yamlVisible = ref(false)
const yamlTitle = ref('')
const yamlContent = ref('')

function showConfigYaml(item: ConfigMapItem): void {
  yamlTitle.value = item.name
  yamlContent.value = stringify({ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: item.name, namespace: item.namespace, creationTimestamp: item.createdAt }, data: item.data })
  yamlVisible.value = true
}

function showSecretYaml(item: SecretItem): void {
  yamlTitle.value = item.name
  yamlContent.value = stringify({ apiVersion: 'v1', kind: 'Secret', metadata: { name: item.name, namespace: item.namespace, creationTimestamp: item.createdAt }, type: item.type, data: Object.fromEntries(item.dataKeys.map((key) => [key, '<redacted>'])) })
  yamlVisible.value = true
  moreMenuKey.value = ''
}

const confirmVisible = ref(false)
const confirmBusy = ref(false)
const confirmError = ref('')
const deleteTarget = ref<{ kind: ConfigTab; name: string; namespace: string; protected?: boolean } | null>(null)

function requestDelete(kind: ConfigTab, item: ConfigMapItem | SecretItem): void {
  if (kind === 'secret' && 'protected' in item && item.protected) return
  deleteTarget.value = { kind, name: item.name, namespace: item.namespace, protected: 'protected' in item ? item.protected : false }
  confirmError.value = ''
  confirmVisible.value = true
  moreMenuKey.value = ''
}

async function confirmDelete(): Promise<void> {
  if (!deleteTarget.value) return
  confirmBusy.value = true
  confirmError.value = ''
  try {
    const target = deleteTarget.value
    if (target.kind === 'configMap') await deleteConfigMap(clusterId.value, target.namespace, target.name)
    else await deleteSecret(clusterId.value, target.namespace, target.name)
    confirmVisible.value = false
    showNotice(t('container.config.message.deleted'))
    await loadResources()
  } catch (error) {
    confirmError.value = error instanceof Error ? error.message : String(error)
  } finally {
    confirmBusy.value = false
  }
}

function changePageSize(event: Event): void {
  pageSize.value = Number((event.target as HTMLSelectElement).value)
  page.value = 1
}

function jumpToPage(): void {
  const target = Number(jumpPage.value)
  if (Number.isInteger(target)) page.value = Math.max(1, Math.min(pageCount.value, target))
  jumpPage.value = ''
}

watch(activeTab, () => {
  searchInput.value = ''
  appliedSearch.value = ''
  page.value = 1
  moreMenuKey.value = ''
  columnMenuOpen.value = false
  if (initialized.value) void loadResources()
})
watch(clusterId, async () => {
  if (!initialized.value) return
  await loadNamespaceOptions()
  await loadResources()
})
watch(namespace, () => { if (initialized.value) { page.value = 1; void loadResources() } })
watch(formCluster, async (value) => {
  if (!formVisible.value || !value) return
  const items = await listNamespaces({ clusterId: value })
  formNamespaces.value = Array.from(new Set(['argocd', 'default', ...items.map((item) => item.name)]))
  if (!formNamespaces.value.includes(formNamespace.value)) formNamespace.value = formNamespaces.value[0] || 'default'
})
watch(pageCount, (count) => { if (page.value > count) page.value = count })
onMounted(initialize)
</script>

<template>
  <HNBPageShell :title="t(`container.config.tabs.${activeTab}`)">
    <template #actions><a class="help-link" href="https://docs.hnb.example.io/container/config" target="_blank" rel="noopener noreferrer">? {{ t('container.config.help') }}</a></template>

    <nav class="config-tabs" role="tablist" :aria-label="t('container.config.title')">
      <button v-for="tab in tabs" :key="tab" type="button" role="tab" :aria-selected="activeTab === tab" :class="{ active: activeTab === tab }" @click="activeTab = tab">{{ t(`container.config.tabs.${tab}`) }}</button>
    </nav>

    <div class="config-toolbar">
      <HNBButton variant="primary" @click="openCreate">{{ t(`container.config.toolbar.${activeTab === 'configMap' ? 'create' : 'addSecret'}`) }}</HNBButton>
      <div class="filters">
        <label><span>{{ t('container.config.toolbar.cluster') }}</span><select v-model="clusterId"><option v-for="item in clusterOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
        <label><span>{{ t('container.config.toolbar.namespace') }}</span><select v-model="namespace"><option v-for="item in namespaces" :key="item" :value="item">{{ item }}</option></select></label>
        <select v-model="searchType" class="search-type"><option value="name">{{ t('container.config.toolbar.name') }}</option></select>
        <input v-model="searchInput" type="search" :placeholder="t('container.config.toolbar.searchPlaceholder')" @keyup.enter="query">
        <HNBButton size="small" @click="query">{{ t('container.config.toolbar.query') }}</HNBButton>
        <button class="icon-button" type="button" :aria-label="t('container.config.toolbar.refresh')" :title="t('container.config.toolbar.refresh')" @click="loadResources">↻</button>
        <button class="icon-button" type="button" :aria-label="t('container.config.toolbar.reset')" :title="t('container.config.toolbar.reset')" @click="resetFilters">⊗</button>
        <div class="column-settings"><button class="icon-button" type="button" :aria-label="t('container.config.toolbar.columns')" :title="t('container.config.toolbar.columns')" @click="columnMenuOpen = !columnMenuOpen">⚙</button><div v-if="columnMenuOpen" class="column-menu"><label v-for="key in (activeTab === 'configMap' ? ['createdAt'] : ['type', 'createdAt'])" :key="key"><input type="checkbox" :checked="(activeTab === 'configMap' ? configColumns : secretColumns).includes(key)" @change="activeTab === 'configMap' ? (configColumns = configColumns.includes(key) ? configColumns.filter((item) => item !== key) : [...configColumns, key]) : (secretColumns = secretColumns.includes(key) ? secretColumns.filter((item) => item !== key) : [...secretColumns, key])"><span>{{ t(`container.config.columns.${key}`) }}</span></label></div></div>
      </div>
    </div>

    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <p v-if="loadError" class="error" role="alert">{{ loadError }}</p>
    <p v-if="loading" class="loading" role="status">{{ t('container.config.loading') }}</p>

    <div v-else class="table-wrap">
      <table v-if="activeTab === 'configMap'" class="config-table"><thead><tr><th>{{ t('container.config.columns.configName') }}</th><th v-if="configColumns.includes('createdAt')">{{ t('container.config.columns.createdAt') }}</th><th>{{ t('container.config.columns.actions') }}</th></tr></thead><tbody><tr v-for="item in pagedConfigMaps" :key="item.name"><td><button class="name-link ellipsis" type="button" :title="item.name" @click="showConfigYaml(item)">{{ item.name }}</button></td><td v-if="configColumns.includes('createdAt')">{{ formatDate(item.createdAt) }}</td><td><div class="row-actions"><button type="button" @click="showConfigYaml(item)">{{ t('container.config.action.yaml') }}</button><button type="button" @click="openConfigEdit(item)">{{ t('container.config.action.edit') }}</button><button type="button" @click="requestDelete('configMap', item)">{{ t('container.config.action.delete') }}</button></div></td></tr><tr v-if="!pagedConfigMaps.length"><td colspan="3" class="empty">{{ t('container.config.empty') }}</td></tr></tbody></table>
      <table v-else class="config-table"><thead><tr><th>{{ t('container.config.columns.secretName') }}</th><th v-if="secretColumns.includes('type')">{{ t('container.config.columns.type') }}</th><th v-if="secretColumns.includes('createdAt')">{{ t('container.config.columns.createdAt') }}</th><th>{{ t('container.config.columns.actions') }}</th></tr></thead><tbody><tr v-for="item in pagedSecrets" :key="item.name"><td><button class="name-link ellipsis" type="button" :title="item.name" @click="showSecretYaml(item)">{{ item.name }}</button></td><td v-if="secretColumns.includes('type')">{{ item.type }}</td><td v-if="secretColumns.includes('createdAt')">{{ formatDate(item.createdAt) }}</td><td><div class="row-actions menu-host"><button type="button" :disabled="item.protected" :title="item.protected ? t('container.config.protected') : ''" @click="requestDelete('secret', item)">{{ t('container.config.action.delete') }}</button><button type="button" @click="moreMenuKey = moreMenuKey === item.name ? '' : item.name">{{ t('container.config.action.more') }}</button><div v-if="moreMenuKey === item.name" class="more-menu"><button type="button" @click="showSecretYaml(item)">{{ t('container.config.action.yaml') }}</button><button type="button" :disabled="item.protected" @click="requestDelete('secret', item)">{{ t('container.config.action.delete') }}</button></div></div></td></tr><tr v-if="!pagedSecrets.length"><td colspan="4" class="empty">{{ t('container.config.empty') }}</td></tr></tbody></table>
    </div>

    <footer v-if="!loading" class="pagination"><span>{{ t('container.config.pagination.range', { start: pageStart, end: pageEnd, total: filteredItems.length }) }}</span><div class="page-buttons"><button type="button" :disabled="page <= 1" @click="page--">‹</button><button v-for="number in pageNumbers" :key="number" type="button" :class="{ active: number === page }" @click="page = number">{{ number }}</button><button type="button" :disabled="page >= pageCount" @click="page++">›</button></div><div class="page-jump"><select :value="pageSize" @change="changePageSize"><option v-for="size in [10, 20, 50]" :key="size" :value="size">{{ t('container.config.pagination.pageSize', { size }) }}</option></select><span>{{ t('container.config.pagination.jump') }}</span><input v-model="jumpPage" type="number" min="1" :max="pageCount" @keyup.enter="jumpToPage"><span>{{ t('container.config.pagination.pageUnit', { pages: pageCount }) }}</span></div></footer>

    <NetworkDrawer v-model="formVisible" :title="t(`container.config.dialog.${activeTab === 'secret' ? 'addSecret' : editingConfigName ? 'editConfig' : 'addConfig'}`)" :busy="formBusy" :error="formError" :close-label="t('container.config.action.close')" :close-on-backdrop="false" :cancel-text="t('container.config.action.cancel')" :confirm-text="t('container.config.action.confirm')" @confirm="submitForm">
      <form class="config-form" @submit.prevent="submitForm">
        <label><span><b>*</b> {{ t(`container.config.form.${activeTab === 'configMap' ? 'configName' : 'secretName'}`) }}</span><input v-model="formName" :disabled="!!editingConfigName" :placeholder="t(`container.config.form.${activeTab === 'configMap' ? 'inputName' : 'inputSecretName'}`)"></label>
        <label><span><b>*</b> {{ t('container.config.form.cluster') }}</span><select v-model="formCluster"><option value="" disabled>{{ t('container.config.form.select') }}</option><option v-for="item in clusterOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
        <label><span><b>*</b> {{ t('container.config.form.namespace') }}</span><select v-model="formNamespace"><option value="" disabled>{{ t('container.config.form.select') }}</option><option v-for="item in formNamespaces" :key="item" :value="item">{{ item }}</option></select></label>
        <label v-if="activeTab === 'secret'"><span><b>*</b> {{ t('container.config.form.secretType') }}</span><select v-model="secretType"><option value="" disabled>{{ t('container.config.form.select') }}</option><option v-for="type in ['Opaque', 'kubernetes.io/tls', 'kubernetes.io/dockerconfigjson', 'kubernetes.io/basic-auth']" :key="type" :value="type">{{ type }}</option></select></label>
        <div class="data-section"><div v-if="activeTab === 'secret'" class="data-label"><b>*</b> {{ t('container.config.form.secretData') }} <span class="help-tip" :title="t('container.config.form.secretDataHelp')">?</span></div><div v-for="(row, index) in dataRows" :key="index" class="data-row"><label><span>{{ t('container.config.form.key') }}</span><input v-model="row.key" :placeholder="t('container.config.form.inputKey')"></label><label><span>{{ t('container.config.form.value') }}</span><textarea :id="`config-value-${index}`" v-model="row.value" rows="3" :placeholder="t('container.config.form.inputValue')" /></label><div class="data-actions"><button type="button" :aria-label="t('container.config.action.edit')" @click="focusValue(index)">✎</button><button v-if="activeTab === 'configMap'" type="button" :disabled="dataRows.length === 1" :aria-label="t('container.config.action.remove')" @click="removeDataRow(index)">⊖</button></div></div><button class="add-row" type="button" @click="addDataRow">⊕ {{ t('container.config.action.add') }}</button></div>
      </form>
    </NetworkDrawer>

    <NetworkDrawer v-model="yamlVisible" :title="yamlTitle" :close-label="t('container.config.action.close')" :cancel-text="t('container.config.action.cancel')" hide-confirm><textarea class="yaml-view" :value="yamlContent" readonly rows="24" /></NetworkDrawer>
    <HNBConfirmation v-model="confirmVisible" :title="t('container.config.confirm.title')" :description="t('container.config.confirm.message', { name: deleteTarget?.name ?? '' })" :loading="confirmBusy" :error="confirmError" :confirm-text="t('container.config.action.confirm')" :cancel-text="t('container.config.action.cancel')" danger @confirm="confirmDelete" />
  </HNBPageShell>
</template>

<style scoped>
.help-link { color: var(--hnb-color-primary); font-size: 13px; text-decoration: none; }
.config-tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--hnb-color-divider); }
.config-tabs button { padding: 9px 18px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--hnb-color-text-secondary); cursor: pointer; }
.config-tabs button.active { border-bottom-color: var(--hnb-color-primary); color: var(--hnb-color-primary); font-weight: 600; }
.config-toolbar { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; }
.filters { display: flex; align-items: flex-end; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
.filters label { display: flex; align-items: center; gap: 6px; color: var(--hnb-color-text-secondary); font-size: 13px; }
.filters select, .filters input, .search-type { min-height: 34px; padding: 6px 9px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); }
.filters input { width: 190px; }.icon-button{width:34px;height:34px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary);cursor:pointer}
.column-settings,.menu-host{position:relative}.column-menu,.more-menu{position:absolute;z-index:20;top:40px;right:0;min-width:150px;padding:7px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-surface);box-shadow:var(--hnb-shadow-3)}.column-menu label{display:flex;padding:6px}.more-menu{top:24px;display:grid}.more-menu button{padding:7px 9px}
.notice,.error,.loading{margin:0;padding:8px 10px;border-radius:var(--hnb-radius-sm);font-size:13px}.notice{color:var(--hnb-color-status-success);background:var(--hnb-color-status-success-surface)}.error{color:var(--hnb-color-status-danger);background:var(--hnb-color-status-danger-surface)}.loading{color:var(--hnb-color-text-secondary)}
.table-wrap{overflow-x:auto;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md)}.config-table{width:100%;min-width:760px;border-collapse:collapse;table-layout:fixed;font-size:13px}.config-table th,.config-table td{padding:11px 12px;border-bottom:1px solid var(--hnb-color-divider);text-align:left}.config-table th{background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary)}.config-table th:last-child,.config-table td:last-child{width:240px}.ellipsis{display:block;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.name-link,.row-actions button,.more-menu button{border:0;background:transparent;color:var(--hnb-color-primary);cursor:pointer;font-size:13px}.row-actions{display:flex;align-items:center;gap:12px}.row-actions button:disabled,.more-menu button:disabled{color:var(--hnb-color-text-tertiary);cursor:not-allowed}.empty{padding:38px!important;text-align:center!important;color:var(--hnb-color-text-tertiary)}
.pagination{display:grid;grid-template-columns:1fr auto 1fr;align-items:center;gap:12px;color:var(--hnb-color-text-secondary);font-size:13px}.page-buttons,.page-jump{display:flex;align-items:center;gap:5px}.page-buttons button{min-width:30px;height:30px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary)}.page-buttons button.active{background:var(--hnb-color-primary);color:var(--hnb-color-text-on-accent)}.page-jump{justify-content:flex-end}.page-jump select,.page-jump input{height:30px;padding:4px 7px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.page-jump input{width:50px}
.config-form{display:flex;flex-direction:column;gap:14px}.config-form>label,.data-row label{display:grid;grid-template-columns:120px 1fr;align-items:start;gap:10px;color:var(--hnb-color-text-secondary);font-size:13px}.config-form input,.config-form select,.config-form textarea{width:100%;box-sizing:border-box;padding:8px 10px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.config-form b,.data-label b{color:var(--hnb-color-status-danger)}.data-section{display:flex;flex-direction:column;gap:12px}.data-label{color:var(--hnb-color-text-secondary);font-size:13px}.help-tip{display:inline-grid;place-items:center;width:16px;height:16px;border:1px solid var(--hnb-color-border);border-radius:50%;cursor:help}.data-row{display:grid;grid-template-columns:1fr;gap:8px;padding:12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm)}.data-actions{display:flex;justify-content:flex-end;gap:6px}.data-actions button,.add-row{border:0;background:transparent;color:var(--hnb-color-primary);cursor:pointer}.data-actions button:disabled{opacity:.45;cursor:not-allowed}.add-row{align-self:flex-start}.yaml-view{width:100%;box-sizing:border-box;padding:12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:#0b1020;color:#d9e2f2;font:12px/1.55 ui-monospace,monospace;resize:vertical}
@media(max-width:900px){.config-toolbar{align-items:stretch;flex-direction:column}.filters{justify-content:flex-start}.pagination{grid-template-columns:1fr}.page-jump{justify-content:flex-start}}
@media(max-width:560px){.config-tabs{overflow-x:auto}.filters label{width:100%;justify-content:space-between}.filters label select,.filters input{flex:1;width:auto}.config-form>label,.data-row label{grid-template-columns:1fr}}
</style>
