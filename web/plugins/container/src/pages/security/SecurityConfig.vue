<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton, HNBPageShell, HNBTabs, type HNBTab } from '@hnb/ui-kit'
import {
  getVulnerabilityDatabaseRecords,
  getVulnerabilityScanProjects,
  saveVulnerabilityScanRules,
  uploadVulnerabilityDatabase,
} from '../../api/securityApi'
import type { VulnerabilityDatabaseRecord, VulnerabilityScanProject, VulnerabilityScanRule } from '../../api/securityTypes'
import NetworkDrawer from '../network/NetworkDrawer.vue'

type ConfigTab = 'database' | 'scan'
type SortKey = 'updatedBy' | 'createdAt' | 'updatedAt'

const { t } = useI18n()
const activeTab = ref<ConfigTab>('database')
const tabs = computed<HNBTab[]>(() => [
  { id: 'database', label: t('container.security.configCenter.tabs.database') },
  { id: 'scan', label: t('container.security.configCenter.tabs.scan') },
])
const loading = ref(false)
const pageError = ref('')

const fileInput = ref<HTMLInputElement | null>(null)
const uploadVisible = ref(false)
const selectedFile = ref<File | null>(null)
const dragOver = ref(false)
const uploadBusy = ref(false)
const uploadError = ref('')
const uploadSuccess = ref('')
const records = ref<VulnerabilityDatabaseRecord[]>([])
const sortKey = ref<SortKey>('updatedAt')
const sortDirection = ref<'asc' | 'desc'>('desc')

const sortedRecords = computed(() => [...records.value].sort((left, right) => {
  const result = String(left[sortKey.value]).localeCompare(String(right[sortKey.value]))
  return sortDirection.value === 'asc' ? result : -result
}))

function acceptFile(file?: File | null): void {
  uploadError.value = ''
  uploadSuccess.value = ''
  if (!file) return
  if (!file.name.toLowerCase().endsWith('.tgz')) {
    selectedFile.value = null
    uploadError.value = t('container.security.configCenter.database.extensionError')
    return
  }
  selectedFile.value = file
}

function onFileChange(event: Event): void {
  acceptFile((event.target as HTMLInputElement).files?.[0])
}

function onDrop(event: DragEvent): void {
  dragOver.value = false
  acceptFile(event.dataTransfer?.files?.[0])
}

function cancelUpload(): void {
  selectedFile.value = null
  uploadError.value = ''
  uploadSuccess.value = ''
  if (fileInput.value) fileInput.value.value = ''
}

function openUpload(): void {
  cancelUpload()
  uploadVisible.value = true
}

async function confirmUpload(): Promise<void> {
  if (!selectedFile.value || uploadBusy.value) return
  uploadBusy.value = true
  uploadError.value = ''
  try {
    const record = await uploadVulnerabilityDatabase(selectedFile.value)
    records.value.unshift(record)
    selectedFile.value = null
    if (fileInput.value) fileInput.value.value = ''
    uploadVisible.value = false
    uploadSuccess.value = t('container.security.configCenter.database.uploadSuccess')
  } catch (error) {
    uploadError.value = error instanceof Error ? error.message : String(error)
  } finally {
    uploadBusy.value = false
  }
}

function changeSort(key: SortKey): void {
  if (sortKey.value === key) sortDirection.value = sortDirection.value === 'asc' ? 'desc' : 'asc'
  else { sortKey.value = key; sortDirection.value = 'asc' }
}

const projects = ref<VulnerabilityScanProject[]>([])
const autoFilter = ref('')
const scheduledFilter = ref('')
const searchInput = ref('')
const appliedSearch = ref('')
const selectedIds = ref<string[]>([])

const filteredProjects = computed(() => projects.value.filter((item) => {
  if (autoFilter.value && String(item.autoScan) !== autoFilter.value) return false
  if (scheduledFilter.value && String(item.scheduledScan) !== scheduledFilter.value) return false
  return !appliedSearch.value || item.name.toLowerCase().includes(appliedSearch.value.toLowerCase())
}))
const allSelected = computed(() => Boolean(filteredProjects.value.length) && filteredProjects.value.every((item) => selectedIds.value.includes(item.id)))

function queryProjects(): void { appliedSearch.value = searchInput.value.trim() }
function toggleAll(): void {
  const visible = filteredProjects.value.map((item) => item.id)
  if (allSelected.value) selectedIds.value = selectedIds.value.filter((id) => !visible.includes(id))
  else selectedIds.value = Array.from(new Set([...selectedIds.value, ...visible]))
}

function toggleProject(id: string): void {
  selectedIds.value = selectedIds.value.includes(id) ? selectedIds.value.filter((item) => item !== id) : [...selectedIds.value, id]
}

const ruleVisible = ref(false)
const ruleBusy = ref(false)
const ruleError = ref('')
const ruleTargetIds = ref<string[]>([])
const rule = ref<VulnerabilityScanRule>({ autoScan: false, scheduledScan: false, frequency: '', scanTime: '' })

function openRuleDialog(ids: string[]): void {
  if (!ids.length) { pageError.value = t('container.security.configCenter.scan.selectFirst'); return }
  const first = projects.value.find((item) => item.id === ids[0])
  ruleTargetIds.value = [...ids]
  rule.value = first ? { autoScan: first.autoScan, scheduledScan: first.scheduledScan, frequency: first.frequency, scanTime: first.scanTime } : { autoScan: false, scheduledScan: false, frequency: '', scanTime: '' }
  ruleError.value = ''
  pageError.value = ''
  ruleVisible.value = true
}

async function saveRules(): Promise<void> {
  if (rule.value.scheduledScan && (!rule.value.frequency || !rule.value.scanTime)) { ruleError.value = t('container.security.configCenter.scan.scheduleRequired'); return }
  ruleBusy.value = true
  ruleError.value = ''
  try {
    await saveVulnerabilityScanRules(ruleTargetIds.value, rule.value)
    for (const item of projects.value) if (ruleTargetIds.value.includes(item.id)) Object.assign(item, rule.value)
    ruleVisible.value = false
  } catch (error) {
    ruleError.value = error instanceof Error ? error.message : String(error)
  } finally {
    ruleBusy.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  pageError.value = ''
  try {
    const [recordItems, projectItems] = await Promise.all([getVulnerabilityDatabaseRecords(), getVulnerabilityScanProjects()])
    records.value = recordItems
    projects.value = projectItems
    selectedIds.value = []
  } catch (error) {
    pageError.value = error instanceof Error ? error.message : t('container.security.configCenter.loadError')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <HNBPageShell :title="t('container.security.configCenter.title')">
    <template #actions><a class="help-link" href="https://docs.hnb.example.io/container/security/configuration" target="_blank" rel="noopener noreferrer">? {{ t('container.security.configCenter.help') }}</a></template>
    <p v-if="pageError" class="error" role="alert">{{ pageError }}</p><p v-if="loading" class="loading" role="status">{{ t('container.security.loading') }}</p>
    <HNBTabs v-else v-model="activeTab" :tabs="tabs" :ariaLabel="t('container.security.configCenter.title')">
      <template #panel-database>
        <div class="database-toolbar"><HNBButton variant="primary" @click="openUpload">{{ t('container.security.configCenter.database.updateTitle') }}</HNBButton><p v-if="uploadSuccess" class="success" role="status">{{ uploadSuccess }}</p></div>
        <section class="config-section records"><h3>{{ t('container.security.configCenter.database.recordsTitle') }}</h3><div class="table-wrap"><table><thead><tr><th>{{ t('container.security.configCenter.database.columns.version') }}</th><th><button type="button" @click="changeSort('updatedBy')">{{ t('container.security.configCenter.database.columns.user') }} ⇅</button></th><th><button type="button" @click="changeSort('createdAt')">{{ t('container.security.configCenter.database.columns.createdAt') }} ⇅</button></th><th><button type="button" @click="changeSort('updatedAt')">{{ t('container.security.configCenter.database.columns.updatedAt') }} ⇅</button></th></tr></thead><tbody><tr v-for="item in sortedRecords" :key="`${item.version}-${item.createdAt}`"><td>{{ item.version }}</td><td>{{ item.updatedBy }}</td><td>{{ new Date(item.createdAt).toLocaleString() }}</td><td>{{ new Date(item.updatedAt).toLocaleString() }}</td></tr><tr v-if="!sortedRecords.length"><td colspan="4" class="empty">{{ t('container.security.empty') }}</td></tr></tbody></table></div></section>
      </template>
      <template #panel-scan>
        <div class="scan-toolbar"><HNBButton variant="primary" @click="openRuleDialog(selectedIds)">{{ t('container.security.configCenter.scan.settings') }}</HNBButton><div><select v-model="autoFilter"><option value="">{{ t('container.security.configCenter.scan.autoPlaceholder') }}</option><option value="true">{{ t('container.security.configCenter.yes') }}</option><option value="false">{{ t('container.security.configCenter.no') }}</option></select><select v-model="scheduledFilter"><option value="">{{ t('container.security.configCenter.scan.scheduledPlaceholder') }}</option><option value="true">{{ t('container.security.configCenter.yes') }}</option><option value="false">{{ t('container.security.configCenter.no') }}</option></select><input v-model="searchInput" type="search" :placeholder="t('container.security.configCenter.scan.searchPlaceholder')" @keyup.enter="queryProjects"><HNBButton size="small" @click="queryProjects">{{ t('container.config.toolbar.query') }}</HNBButton><button class="icon-button" type="button" :title="t('container.security.protection.refresh')" @click="load">↻</button></div></div>
        <div class="table-wrap"><table><thead><tr><th class="check-cell"><input type="checkbox" :checked="allSelected" :aria-label="t('container.security.configCenter.scan.selectAll')" @change="toggleAll"></th><th>{{ t('container.security.configCenter.scan.columns.project') }}</th><th>{{ t('container.security.configCenter.scan.columns.auto') }}</th><th>{{ t('container.security.configCenter.scan.columns.frequency') }}</th><th>{{ t('container.security.configCenter.scan.columns.time') }}</th><th>{{ t('container.security.configCenter.scan.columns.actions') }}</th></tr></thead><tbody><tr v-for="item in filteredProjects" :key="item.id"><td class="check-cell"><input type="checkbox" :checked="selectedIds.includes(item.id)" :aria-label="item.name" @change="toggleProject(item.id)"></td><td>{{ item.name }}</td><td>{{ t(`container.security.configCenter.${item.autoScan ? 'yes' : 'no'}`) }}</td><td>{{ item.frequency ? t(`container.security.configCenter.scan.frequency.${item.frequency}`) : '--' }}</td><td>{{ item.scanTime || '--' }}</td><td><button class="text-action" type="button" @click="openRuleDialog([item.id])">{{ t('container.security.configCenter.scan.settings') }}</button></td></tr><tr v-if="!filteredProjects.length"><td colspan="6" class="empty">{{ t('container.security.empty') }}</td></tr></tbody></table></div>
      </template>
    </HNBTabs>
    <NetworkDrawer v-model="uploadVisible" :title="t('container.security.configCenter.database.updateTitle')" :busy="uploadBusy" :error="uploadError" :confirm-disabled="!selectedFile" @cancel="cancelUpload" @confirm="confirmUpload"><ol class="steps"><li><span class="step-index">1</span><div><h4>{{ t('container.security.configCenter.database.download') }}</h4><p>{{ t('container.security.configCenter.database.downloadAddress') }} <a href="https://github.com/aquasecurity/trivy-db/releases" target="_blank" rel="noopener noreferrer">https://github.com/aquasecurity/trivy-db/releases</a> {{ t('container.security.configCenter.database.latestFile') }}</p></div></li><li><span class="step-index">2</span><div class="step-content"><h4>{{ t('container.security.configCenter.database.upload') }}</h4><label class="upload-zone" :class="{ dragging: dragOver }" @dragover.prevent="dragOver = true" @dragleave.prevent="dragOver = false" @drop.prevent="onDrop"><input ref="fileInput" type="file" accept=".tgz" @change="onFileChange"><span v-if="selectedFile">{{ selectedFile.name }}</span><span v-else><strong>{{ t('container.security.configCenter.database.clickUpload') }}</strong> / {{ t('container.security.configCenter.database.dragUpload') }}</span></label><small>{{ t('container.security.configCenter.database.formatHint') }}</small></div></li></ol></NetworkDrawer>
    <NetworkDrawer v-model="ruleVisible" :title="t('container.security.configCenter.scan.dialogTitle')" :busy="ruleBusy" :error="ruleError" @confirm="saveRules"><form class="rule-form" @submit.prevent="saveRules"><label class="check-row"><input v-model="rule.autoScan" type="checkbox"><span>{{ t('container.security.configCenter.scan.enableAuto') }}</span></label><label class="check-row"><input v-model="rule.scheduledScan" type="checkbox"><span>{{ t('container.security.configCenter.scan.enableScheduled') }}</span></label><label><span>{{ t('container.security.configCenter.scan.columns.frequency') }}</span><select v-model="rule.frequency" :disabled="!rule.scheduledScan"><option value="">{{ t('container.security.configCenter.select') }}</option><option value="daily">{{ t('container.security.configCenter.scan.frequency.daily') }}</option><option value="weekly">{{ t('container.security.configCenter.scan.frequency.weekly') }}</option><option value="monthly">{{ t('container.security.configCenter.scan.frequency.monthly') }}</option></select></label><label><span>{{ t('container.security.configCenter.scan.columns.time') }}</span><input v-model="rule.scanTime" type="time" :disabled="!rule.scheduledScan"></label></form></NetworkDrawer>
  </HNBPageShell>
</template>

<style scoped>
.help-link{color:var(--hnb-color-primary);font-size:13px;text-decoration:none}.error{margin:0;color:var(--hnb-color-status-danger);font-size:13px}.success{margin:0;color:var(--hnb-color-status-success);font-size:13px}.loading{color:var(--hnb-color-text-secondary)}.config-section{display:flex;flex-direction:column;gap:14px}.config-section h3,.config-section h4{margin:0;color:var(--hnb-color-text-primary)}.steps{list-style:none;margin:0;padding:0;display:flex;flex-direction:column}.steps li{position:relative;display:grid;grid-template-columns:32px 1fr;gap:12px;padding-bottom:24px}.steps li:not(:last-child)::before{content:'';position:absolute;left:15px;top:28px;bottom:0;border-left:1px solid var(--hnb-color-border)}.step-index{z-index:1;display:grid;place-items:center;width:30px;height:30px;border-radius:50%;background:var(--hnb-color-primary);color:var(--hnb-color-text-on-accent);font-weight:600}.steps p{color:var(--hnb-color-text-secondary);font-size:13px;line-height:1.6}.steps a{color:var(--hnb-color-primary)}.step-content{display:flex;flex-direction:column;gap:9px}.upload-zone{position:relative;display:grid;place-items:center;min-height:130px;border:1px dashed var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary);cursor:pointer}.upload-zone.dragging{border-color:var(--hnb-color-primary)}.upload-zone input{position:absolute;width:1px;height:1px;opacity:0}.upload-zone strong{color:var(--hnb-color-primary)}.step-content small{color:var(--hnb-color-text-tertiary)}.upload-actions{display:flex;gap:10px;margin-left:44px}.records{margin-top:18px}.table-wrap{overflow-x:auto;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md)}table{width:100%;min-width:720px;border-collapse:collapse;table-layout:fixed;font-size:13px}th,td{padding:11px 12px;border-bottom:1px solid var(--hnb-color-divider);text-align:left}th{background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary)}th button,.text-action{padding:0;border:0;background:transparent;color:inherit;cursor:pointer}.text-action{color:var(--hnb-color-primary)}.empty{padding:42px!important;text-align:center!important;color:var(--hnb-color-text-tertiary)}.scan-toolbar{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:14px}.scan-toolbar>div{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.scan-toolbar select,.scan-toolbar input,.rule-form select,.rule-form input[type=time]{min-height:34px;padding:6px 9px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.icon-button{width:34px;height:34px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary)}.check-cell{width:48px;text-align:center}.rule-form{display:flex;flex-direction:column;gap:14px}.rule-form>label:not(.check-row){display:grid;grid-template-columns:150px 1fr;align-items:center;gap:12px}.check-row{display:flex;align-items:center;gap:9px}@media(max-width:768px){.scan-toolbar{align-items:stretch;flex-direction:column}.upload-actions{margin-left:0}.rule-form>label:not(.check-row){grid-template-columns:1fr}}
.database-toolbar{display:flex;align-items:center;gap:12px}.steps h4{margin:0;color:var(--hnb-color-text-primary)}
</style>
