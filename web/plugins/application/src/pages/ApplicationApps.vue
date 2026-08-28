<script setup lang="ts">
import { computed, ref, watch, onMounted, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppCreateWizard from './AppCreateWizard.vue'
import ApplicationDrawer from '../components/ApplicationDrawer.vue'
import * as api from '../marketApi'
import { HNBTable } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'

type AppMode = 'monolith' | 'microservice'
type WizardStep = 1 | 2 | 3
type DrawerMode = 'create' | 'edit' | 'detail'
type ConfigItem = {
  id: string
  name: string
  namespace: string
  app: string
  createdAt: string
  version?: string
  description?: string
  labels?: Record<string, string>
  data?: { key: string; value: string }[]
  yamlContent?: string
}
type SecretItem = Omit<ConfigItem, 'data'> & {
  usedByServices?: string[]
  dataKeys?: string[]
}
type VmConfigItem = {
  id: string
  name: string
  project: string
  createdAt: string
  version?: string
  description?: string
  fileNames?: string[]
}

const { t } = useI18n()
const route = useRoute()

const mode = computed<AppMode>(() =>
  route.path.includes('/microservices') ? 'microservice' : 'monolith',
)
const activeTab = ref(mode.value === 'microservice' ? 'groups' : 'apps')

watch(mode, (val) => {
  activeTab.value = val === 'microservice' ? 'groups' : 'apps'
})
const wizardOpen = ref(false)
const wizardStep = ref<WizardStep>(1)
const form = ref({
  name: '',
  description: '',
  image: '',
  replicas: 1,
  port: 8080,
  configProfile: 'default',
})

const configProfiles = ['default', 'production', 'staging'] as const

const activeConfigTab = ref('configmaps')
const configCluster = ref('default')
const configApp = ref('')
const configSearch = ref('')

const configSubTabs = computed(() => [
  { key: 'configmaps', label: t('application.appPages.config.tabConfigmaps') },
  { key: 'secrets', label: t('application.appPages.config.tabSecrets') },
  { key: 'vmConfig', label: t('application.appPages.config.tabVmConfig') },
])

const configItems = ref<ConfigItem[]>([
  { id: '1', name: 'sql', namespace: 'spacefbponvcx', app: 'pass', createdAt: '2026-04-23 09:42:36' },
])

const configDialogOpen = ref(false)
const configDrawerMode = ref<DrawerMode>('create')
const selectedConfig = ref<ConfigItem | null>(null)
const submittingConfig = ref(false)
const configForm = ref({
  createMode: 'visual' as 'visual' | 'yaml',
  yamlContent: '',
  yamlAppId: '',
  yamlFileName: '',
  yamlFileContent: '',
  name: '',
  version: '',
  appId: '',
  description: '',
  labels: [] as { key: string; value: string }[],
  dataMode: 'manual' as 'manual' | 'file',
  data: [{ key: '', value: '' }],
})
const configFormErrors = ref<Record<string, string>>({})

const secretDialogOpen = ref(false)
const secretDrawerMode = ref<DrawerMode>('create')
const selectedSecret = ref<SecretItem | null>(null)
const submittingSecret = ref(false)
const secretForm = ref({
  createMode: 'visual' as 'visual' | 'yaml',
  name: '',
  appId: '',
  description: '',
  labels: [] as { key: string; value: string }[],
  dataMode: 'manual' as 'manual' | 'file',
  data: [] as { key: string; value: string; autoEncode: boolean }[],
  yamlAppId: '',
  yamlFileName: '',
  yamlContent: '',
})
const secretFormErrors = ref<Record<string, string>>({})
const secretYamlLineCount = ref(1)
const secretYamlFileInput = ref<HTMLInputElement | null>(null)
const secretItems = ref<SecretItem[]>([])

function openAddConfigmap() {
  configDrawerMode.value = 'create'
  selectedConfig.value = null
  configDialogOpen.value = true
  configForm.value = {
    createMode: 'visual',
    yamlContent: '',
    yamlAppId: '',
    yamlFileName: '',
    yamlFileContent: '',
    name: '',
    version: '',
    appId: '',
    description: '',
    labels: [],
    dataMode: 'manual',
    data: [{ key: '', value: '' }],
  }
  configFormErrors.value = {}
}

function openConfigDetail(row: ConfigItem) {
  selectedConfig.value = row
  configDrawerMode.value = 'detail'
  configDialogOpen.value = true
}

function openEditConfigmap(row: ConfigItem) {
  openAddConfigmap()
  configDrawerMode.value = 'edit'
  selectedConfig.value = row
  configForm.value.name = row.name
  configForm.value.version = row.version || ''
  configForm.value.appId = row.app === 'pass' ? 'app1' : ''
  configForm.value.description = row.description || ''
  configForm.value.labels = Object.entries(row.labels || {}).map(([key, value]) => ({ key, value }))
  configForm.value.data = row.data?.map(({ key, value }) => ({ key, value })) || [{ key: '', value: '' }]
  if (row.yamlContent) {
    configForm.value.yamlContent = row.yamlContent
    yamlLineCount.value = (row.yamlContent.match(/\n/g) || []).length + 1
  }
}

const configDrawerTitle = computed(() => {
  if (configDrawerMode.value === 'create') return t('application.appPages.config.addConfigmap')
  if (configDrawerMode.value === 'detail') return t('application.appPages.config.viewYaml')
  return `${t('application.appPages.config.edit')} ${t('application.appPages.config.tabConfigmaps')}`
})

function closeConfigDialog() {
  configDialogOpen.value = false
}

function validateConfigForm(): boolean {
  const errors: Record<string, string> = {}
  if (configForm.value.createMode === 'visual') {
    if (!configForm.value.name.trim()) errors.name = t('application.appPages.config.configNameRequired')
    if (!configForm.value.version.trim()) errors.version = t('application.appPages.config.versionRequired')
    if (!configForm.value.appId) errors.appId = t('application.appPages.config.appRequired')
    const keySet = new Set<string>()
    for (let i = 0; i < configForm.value.data.length; i++) {
      const d = configForm.value.data[i]
      if (!d.key.trim()) {
        errors[`data_${i}_key`] = t('application.appPages.config.keyRequired')
      } else if (keySet.has(d.key.trim())) {
        errors[`data_${i}_key`] = t('application.appPages.config.keyDuplicate')
      }
      keySet.add(d.key.trim())
    }
    if (!configForm.value.data.some((d) => d.key.trim())) errors.data = t('application.appPages.config.dataRequired')
  } else {
    if (!configForm.value.yamlAppId) errors.yamlAppId = t('application.appPages.config.appRequired')
    if (!configForm.value.yamlContent.trim()) errors.yamlContent = t('application.appPages.config.yamlContentRequired')
  }
  configFormErrors.value = errors
  return Object.keys(errors).length === 0
}

function submitConfigForm() {
  if (!validateConfigForm()) return
  submittingConfig.value = true
  setTimeout(() => {
    submittingConfig.value = false
    closeConfigDialog()
    // TODO: call API
  }, 800)
}

function addConfigLabel() {
  configForm.value.labels.push({ key: '', value: '' })
}

function removeConfigLabel(index: number) {
  configForm.value.labels.splice(index, 1)
}

function addConfigDataRow() {
  configForm.value.data.push({ key: '', value: '' })
}

function removeConfigDataRow(index: number) {
  configForm.value.data.splice(index, 1)
}

const yamlFileInput = ref<HTMLInputElement | null>(null)
const yamlLineCount = ref(1)

function updateYamlLineCount() {
  yamlLineCount.value = (configForm.value.yamlContent.match(/\n/g) || []).length + 1
}

function browseYamlFile() {
  yamlFileInput.value?.click()
}

function onYamlFileSelected(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const ext = file.name.split('application..').pop()?.toLowerCase()
  if (ext && !['txt', 'conf', 'yaml', 'yml'].includes(ext)) {
    configFormErrors.value.yamlFile = t('application.appPages.config.yamlFileFormatUnsupported')
    return
  }
  if (file.size > 1024 * 1024) {
    configFormErrors.value.yamlFile = t('application.appPages.config.yamlFileTooLarge')
    return
  }
  configFormErrors.value.yamlFile = ''
  const reader = new FileReader()
  reader.onload = () => {
    const content = reader.result as string
    if (configForm.value.yamlContent.trim() && !confirm(t('application.appPages.config.yamlFileOverwriteConfirm'))) return
    configForm.value.yamlFileName = file.name
    configForm.value.yamlFileContent = content
    configForm.value.yamlContent = content
  }
  reader.readAsText(file)
}

function openAddSecret() {
  secretDrawerMode.value = 'create'
  selectedSecret.value = null
  secretDialogOpen.value = true
  secretForm.value = {
    createMode: 'visual',
    name: '', appId: '', description: '', labels: [], dataMode: 'manual',
    data: [{ key: '', value: '', autoEncode: true }],
    yamlAppId: '', yamlFileName: '', yamlContent: '',
  }
  secretFormErrors.value = {}
}

function openSecretDetail(row: SecretItem) {
  selectedSecret.value = row
  secretDrawerMode.value = 'detail'
  secretDialogOpen.value = true
}

function openEditSecret(row: SecretItem) {
  openAddSecret()
  secretDrawerMode.value = 'edit'
  selectedSecret.value = row
  secretForm.value.name = row.name
  secretForm.value.appId = row.app === 'pass' ? 'app1' : ''
  secretForm.value.description = row.description || ''
  secretForm.value.labels = Object.entries(row.labels || {}).map(([key, value]) => ({ key, value }))
  secretForm.value.data = row.dataKeys?.map((key) => ({ key, value: '', autoEncode: true })) || [{ key: '', value: '', autoEncode: true }]
}

const secretDrawerTitle = computed(() => {
  if (secretDrawerMode.value === 'create') return t('application.appPages.config.addSecret')
  if (secretDrawerMode.value === 'detail') return t('application.appPages.config.viewYaml')
  return `${t('application.appPages.config.edit')} ${t('application.appPages.config.tabSecrets')}`
})

function closeSecretDialog() {
  secretDialogOpen.value = false
}

function validateSecretForm(): boolean {
  const errors: Record<string, string> = {}
  if (secretForm.value.createMode === 'visual') {
    if (!secretForm.value.name.trim()) errors.name = t('application.appPages.config.secretNameRequired')
    if (!secretForm.value.appId) errors.appId = t('application.appPages.config.appRequired')
    const keySet = new Set<string>()
    for (let i = 0; i < secretForm.value.data.length; i++) {
      const d = secretForm.value.data[i]
      if (!d.key.trim()) { errors[`sdata_${i}_key`] = t('application.appPages.config.keyRequired') }
      else if (keySet.has(d.key.trim())) { errors[`sdata_${i}_key`] = t('application.appPages.config.keyDuplicate') }
      keySet.add(d.key.trim())
      if (!d.value.trim()) { errors[`sdata_${i}_val`] = t('application.appPages.config.valueRequired') }
    }
    if (!secretForm.value.data.some((d) => d.key.trim())) errors.sdata = t('application.appPages.config.dataRequired')
  } else {
    if (!secretForm.value.yamlAppId) errors.yamlAppId = t('application.appPages.config.appRequired')
    if (!secretForm.value.yamlContent.trim()) errors.yamlContent = t('application.appPages.config.yamlContentRequired')
  }
  secretFormErrors.value = errors
  return Object.keys(errors).length === 0
}

function submitSecretForm() {
  if (!validateSecretForm()) return
  submittingSecret.value = true
  setTimeout(() => {
    submittingSecret.value = false
    closeSecretDialog()
  }, 800)
}

function addSecretLabel() {
  secretForm.value.labels.push({ key: '', value: '' })
}

function removeSecretLabel(index: number) {
  secretForm.value.labels.splice(index, 1)
}

function addSecretDataRow() {
  secretForm.value.data.push({ key: '', value: '', autoEncode: true })
}

function removeSecretDataRow(index: number) {
  secretForm.value.data.splice(index, 1)
}

function updateSecretYamlLineCount() {
  secretYamlLineCount.value = (secretForm.value.yamlContent.match(/\n/g) || []).length + 1
}

function browseSecretYamlFile() {
  secretYamlFileInput.value?.click()
}

function onSecretYamlFileSelected(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const ext = file.name.split('application..').pop()?.toLowerCase()
  if (ext && !['txt', 'conf', 'yaml', 'yml'].includes(ext)) {
    secretFormErrors.value.yamlFile = t('application.appPages.config.yamlFileFormatUnsupported')
    return
  }
  if (file.size > 1024 * 1024) {
    secretFormErrors.value.yamlFile = t('application.appPages.config.yamlFileTooLarge')
    return
  }
  secretFormErrors.value.yamlFile = ''
  const reader = new FileReader()
  reader.onload = () => {
    const content = reader.result as string
    if (secretForm.value.yamlContent.trim() && !confirm(t('application.appPages.config.yamlFileOverwriteConfirm'))) return
    secretForm.value.yamlFileName = file.name
    secretForm.value.yamlContent = content
  }
  reader.readAsText(file)
}

/* ───────────── VM Config ───────────── */
const vmConfigSearch = ref('')
const vmConfigItems = ref<VmConfigItem[]>([
  { id: '1', name: 'vm-config-1', project: 'CLOUD-HCI-Test', createdAt: '2026-04-23 09:42:36' },
])
const vmConfigDialogOpen = ref(false)
const vmConfigDrawerMode = ref<DrawerMode>('create')
const selectedVmConfig = ref<VmConfigItem | null>(null)
const submittingVmConfig = ref(false)
const vmConfigForm = ref({
  name: '', version: '', project: 'CLOUD-HCI-Test', description: '', files: [] as File[],
})
const vmConfigFormErrors = ref<Record<string, string>>({})
const vmConfigFileInput = ref<HTMLInputElement | null>(null)

const configTableColumns = computed<HNBTableColumn<any>[]>(() => [
  { key: 'name', title: t('application.appPages.config.name'), render: (row: ConfigItem) => h('a', { class: 'config-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openConfigDetail(row) } }, row.name) },
  { key: 'namespace', title: t('application.appPages.config.namespace') },
  { key: 'app', title: t('application.appPages.config.application') },
  { key: 'createdAt', title: t('application.appPages.config.createdAt') },
  { key: 'actions', title: t('application.appPages.config.actions'), render: (row: ConfigItem) => h('span', [
    h('a', { class: 'action-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openConfigDetail(row) } }, t('application.appPages.config.viewYaml')),
    ' ',
    h('a', { class: 'action-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openEditConfigmap(row) } }, t('application.appPages.config.edit')),
    ' ',
    h('a', { class: 'action-link danger', href: '#', onClick: (e: Event) => e.preventDefault() }, t('application.appPages.config.delete')),
  ]) },
])

const secretTableColumns = computed<HNBTableColumn<any>[]>(() => [
  { key: 'name', title: t('application.appPages.config.secretName'), render: (row: SecretItem) => h('a', { class: 'config-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openSecretDetail(row) } }, row.name) },
  { key: 'labels', title: t('application.appPages.config.labels'), render: (row: any) => {
    const labels = row.labels || {}
    const keys = Object.keys(labels)
    return keys.length ? keys.map((k) => h('span', { class: 'label-chip' }, `${k}=${labels[k]}`)) : '-'
  }},
  { key: 'namespace', title: t('application.appPages.config.namespace') },
  { key: 'usedByServices', title: t('application.appPages.config.usedByServices'), render: (row: any) => {
    const services = row.usedByServices || []
    return services.length ? services.map((s: string) => h('span', { class: 'service-chip' }, s)) : '-'
  }},
  { key: 'app', title: t('application.appPages.config.application') },
  { key: 'createdAt', title: t('application.appPages.config.createdAt') },
  { key: 'actions', title: t('application.appPages.config.actions'), render: (row: SecretItem) => h('span', [
    h('a', { class: 'action-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openSecretDetail(row) } }, t('application.appPages.config.viewYaml')),
    ' ',
    h('a', { class: 'action-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openEditSecret(row) } }, t('application.appPages.config.edit')),
    ' ',
    h('a', { class: 'action-link danger', href: '#', onClick: (e: Event) => e.preventDefault() }, t('application.appPages.config.delete')),
  ]) },
])

const vmConfigTableColumns = computed<HNBTableColumn<any>[]>(() => [
  { key: 'name', title: t('application.appPages.config.configName'), render: (row: VmConfigItem) => h('a', { class: 'config-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openVmConfigDetail(row) } }, row.name) },
  { key: 'project', title: t('application.appPages.config.project') },
  { key: 'createdAt', title: t('application.appPages.config.createdAt') },
  { key: 'actions', title: t('application.appPages.config.actions'), render: (row: VmConfigItem) => h('span', [
    h('a', { class: 'action-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openVmConfigDetail(row) } }, t('application.appPages.config.view')),
    ' ',
    h('a', { class: 'action-link', href: '#', onClick: (e: Event) => { e.preventDefault(); openEditVmConfig(row) } }, t('application.appPages.config.edit')),
    ' ',
    h('a', { class: 'action-link danger', href: '#', onClick: (e: Event) => e.preventDefault() }, t('application.appPages.config.delete')),
  ]) },
])

function searchVmConfig() {}
function refreshVmConfig() {}

function openAddVmConfig() {
  vmConfigDrawerMode.value = 'create'
  selectedVmConfig.value = null
  vmConfigDialogOpen.value = true
  vmConfigForm.value = { name: '', version: '', project: 'CLOUD-HCI-Test', description: '', files: [] }
  vmConfigFormErrors.value = {}
}

function openVmConfigDetail(row: VmConfigItem) {
  selectedVmConfig.value = row
  vmConfigDrawerMode.value = 'detail'
  vmConfigDialogOpen.value = true
}

function openEditVmConfig(row: VmConfigItem) {
  openAddVmConfig()
  vmConfigDrawerMode.value = 'edit'
  selectedVmConfig.value = row
  vmConfigForm.value.name = row.name
  vmConfigForm.value.version = row.version || ''
  vmConfigForm.value.project = row.project
  vmConfigForm.value.description = row.description || ''
}

const vmConfigDrawerTitle = computed(() => {
  if (vmConfigDrawerMode.value === 'create') return t('application.appPages.config.addVmConfig')
  if (vmConfigDrawerMode.value === 'detail') return t('application.appPages.config.view')
  return `${t('application.appPages.config.edit')} ${t('application.appPages.config.tabVmConfig')}`
})

function closeVmConfigDialog() {
  vmConfigDialogOpen.value = false
}

function validateVmConfigForm(): boolean {
  const errors: Record<string, string> = {}
  if (!vmConfigForm.value.name.trim()) errors.name = t('application.appPages.config.configNameRequired')
  if (!vmConfigForm.value.version.trim()) errors.version = t('application.appPages.config.versionRequired')
  if (!vmConfigForm.value.project) errors.project = t('application.appPages.config.appRequired')
  if (!vmConfigForm.value.files.length) errors.files = t('application.appPages.config.dataRequired')
  else {
    let totalSize = 0; const nameSet = new Set<string>()
    for (const f of vmConfigForm.value.files) {
      totalSize += f.size; const fn = f.name
      if (fn.length < 1 || fn.length > 32) { errors.files = t('application.appPages.config.fileNameLength'); break }
      if (!/^[a-zA-Z0-9][a-zA-Z0-9\-_.]*[a-zA-Z0-9]$/.test(fn)) { errors.files = t('application.appPages.config.fileNameInvalid'); break }
      if (nameSet.has(fn)) { errors.files = t('application.appPages.config.fileNameDuplicate'); break }
      nameSet.add(fn)
    }
    if (!errors.files && totalSize > 1024 * 1024) errors.files = t('application.appPages.config.fileTotalSizeExceeded')
  }
  vmConfigFormErrors.value = errors
  return Object.keys(errors).length === 0
}

function submitVmConfigForm() {
  if (!validateVmConfigForm()) return
  submittingVmConfig.value = true
  setTimeout(() => { submittingVmConfig.value = false; closeVmConfigDialog() }, 800)
}

function browseVmConfigFiles() { vmConfigFileInput.value?.click() }

function onVmConfigFilesSelected(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files || !files.length) return
  const allowed = ['txt', 'conf', 'xml', 'json', 'yaml', 'properties']
  for (const f of Array.from(files)) {
    const ext = f.name.split('application..').pop()?.toLowerCase()
    if (!ext || !allowed.includes(ext)) { vmConfigFormErrors.value.files = t('application.appPages.config.fileFormatUnsupported', { name: f.name }); return }
  }
  vmConfigFormErrors.value.files = ''
  vmConfigForm.value.files.push(...Array.from(files))
}

function removeVmConfigFile(index: number) { vmConfigForm.value.files.splice(index, 1) }

const tabs = computed(() => {
  const base = [
    { key: 'apps', label: t('application.appPages.tabs.apps') },
    { key: 'config', label: t('application.appPages.tabs.config') },
  ]
  if (mode.value === 'microservice') {
    return [{ key: 'groups', label: t('application.appPages.tabs.groups') }, ...base]
  }
  return base
})

const title = computed(() =>
  mode.value === 'microservice'
    ? t('application.appPages.microservice.title')
    : t('application.appPages.monolith.title'),
)
const description = computed(() =>
  mode.value === 'microservice'
    ? t('application.appPages.microservice.desc')
    : t('application.appPages.monolith.desc'),
)
const appKindLabel = computed(() =>
  mode.value === 'microservice'
    ? t('application.appPages.microservice.kind')
    : t('application.appPages.monolith.kind'),
)

const groups = ref<api.AppGroup[]>([])
const groupPageSize = 9
const groupCurrentPage = ref(1)
const totalGroups = ref(0)
const groupLoading = ref(false)
const totalGroupPages = computed(() => Math.max(1, Math.ceil(totalGroups.value / groupPageSize)))
const visibleGroupPages = computed(() => {
  const start = Math.max(1, groupCurrentPage.value - 2)
  const end = Math.min(totalGroupPages.value, groupCurrentPage.value + 2)
  return Array.from({ length: end - start + 1 }, (_, i) => start + i)
})
const pagedGroups = computed(() => {
  const start = (groupCurrentPage.value - 1) * groupPageSize
  return groups.value.slice(start, start + groupPageSize)
})

async function loadGroups() {
  groupLoading.value = true
  try {
    groups.value = await api.listGroups()
    totalGroups.value = groups.value.length
  } catch {
    groups.value = []
    totalGroups.value = 0
  } finally {
    groupLoading.value = false
  }
}

onMounted(loadGroups)

function openWizard() {
  wizardOpen.value = true
  wizardStep.value = 1
}

function closeWizard() {
  wizardOpen.value = false
}

function nextStep() {
  if (wizardStep.value < 3) wizardStep.value = (wizardStep.value + 1) as WizardStep
}

function prevStep() {
  if (wizardStep.value > 1) wizardStep.value = (wizardStep.value - 1) as WizardStep
}

function submitWizard() {
  closeWizard()
}

/* ───────────── App Group ───────────── */
const groupDialogOpen = ref(false)
const submittingGroup = ref(false)
const groupForm = ref({ name: '', project: 'CLOUD-HCI-Test', customNamespace: false, type: 'custom' })
const groupFormErrors = ref<Record<string, string>>({})

function openCreateGroup() {
  groupDialogOpen.value = true
  groupForm.value = { name: '', project: 'CLOUD-HCI-Test', customNamespace: false, type: 'custom' }
  groupFormErrors.value = {}
}

function closeGroupDialog() {
  groupDialogOpen.value = false
}

async function submitGroupForm() {
  const errors: Record<string, string> = {}
  if (!groupForm.value.name.trim()) errors.name = '请输入名称'
  groupFormErrors.value = errors
  if (Object.keys(errors).length) return
  try {
    submittingGroup.value = true
    await api.createGroup({
      name: groupForm.value.name,
      group_type: groupForm.value.type,
    })
    closeGroupDialog()
    await loadGroups()
  } catch (e: any) {
    groupFormErrors.value = { submit: e?.message || '创建失败' }
  } finally {
    submittingGroup.value = false
  }
}
</script>

<template>
  <section class="app-page">
    <header class="app-page__header">
      <div>
        <p class="app-page__eyebrow">{{ appKindLabel }}</p>
        <h1>{{ title }}</h1>
        <p>{{ description }}</p>
      </div>
    </header>

    <div class="app-tabs" role="tablist">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="app-tab"
        :class="{ active: activeTab === tab.key }"
        type="button"
        role="tab"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
      </button>
    </div>

    <section class="app-panel">
      <div v-if="activeTab === 'apps'" class="app-list-panel">
        <div class="panel-toolbar">
          <div>
            <h2>{{ t('application.appPages.apps.title') }}</h2>
            <p>{{ t('application.appPages.apps.desc') }}</p>
          </div>
          <button class="primary-button" type="button" @click="openWizard">
            {{ t('application.appPages.create.button') }}
          </button>
        </div>

        <div class="app-table">
          <div class="app-table__head">
            <span>{{ t('application.appPages.apps.name') }}</span>
            <span>{{ t('application.appPages.apps.status') }}</span>
            <span>{{ t('application.appPages.apps.runtime') }}</span>
            <span>{{ t('application.appPages.apps.updatedAt') }}</span>
          </div>
          <div class="app-empty">
            <strong>{{ t('application.appPages.apps.emptyTitle') }}</strong>
            <p>{{ t('application.appPages.apps.emptyDesc') }}</p>
          </div>
        </div>
      </div>

      <div v-else-if="activeTab === 'groups'" class="app-list-panel">
        <div class="panel-toolbar">
          <div>
            <h2>{{ t('application.appPages.groups.title') }}</h2>
            <p>{{ t('application.appPages.groups.desc') }}</p>
          </div>
          <button class="primary-button" type="button" @click="openCreateGroup">
            {{ t('application.appPages.groups.createButton') }}
          </button>
        </div>

        <div v-if="groupLoading" class="app-empty"><strong>{{ t('application.common.loading') }}</strong></div>
        <div v-else-if="pagedGroups.length" class="group-grid">
          <div v-for="g in pagedGroups" :key="g.id" class="group-card">
            <h3 class="group-name">{{ g.name }}</h3>
            <p class="group-desc">{{ g.description || '-' }}</p>
            <div class="group-meta">
              <span class="group-count">{{ g.app_count }} 个应用</span>
              <span class="group-time">{{ g.updated_at?.slice(0, 10) }}</span>
            </div>
          </div>
        </div>
        <div v-else class="app-empty">
          <strong>{{ t('application.appPages.apps.emptyTitle') }}</strong>
          <p>{{ t('application.appPages.apps.emptyDesc') }}</p>
        </div>

        <div v-if="totalGroupPages > 1" class="pagination-bar">
          <span class="pagination-info">{{ totalGroups }} {{ t('application.marketPage.store.products') }}</span>
          <div class="pagination-controls">
            <button class="page-button" :disabled="groupCurrentPage <= 1" @click="groupCurrentPage--">‹</button>
            <span v-for="p in visibleGroupPages" :key="p" :class="{ active: p === groupCurrentPage }" class="page-num" @click="groupCurrentPage = p">{{ p }}</span>
            <button class="page-button" :disabled="groupCurrentPage >= totalGroupPages" @click="groupCurrentPage++">›</button>
          </div>
        </div>
      </div>

      <div v-else class="config-page">
        <div class="config-header">
          <p class="config-desc">{{ t('application.appPages.config.desc') }}</p>
          <a href="#" class="help-link">? {{ t('application.appPages.config.help') }}</a>
        </div>

        <div class="sub-tabs">
          <button v-for="st in configSubTabs" :key="st.key" :class="{ active: activeConfigTab === st.key }" @click="activeConfigTab = st.key">{{ st.label }}</button>
        </div>

        <div v-if="activeConfigTab === 'configmaps'" class="config-section">
          <div class="config-notice">{{ t('application.appPages.config.notice') }}</div>

          <div class="action-bar">
            <button class="primary-button" type="button" @click="openAddConfigmap">
              <span class="icon">⊕</span> {{ t('application.appPages.config.addConfigmap') }}
            </button>
            <div class="filter-group">
              <label><span>{{ t('application.appPages.config.cluster') }}:</span><select v-model="configCluster"><option>default</option></select></label>
              <label><span>{{ t('application.appPages.config.application') }}:</span><select v-model="configApp"><option value="" disabled selected>{{ t('application.appPages.config.appPlaceholder') }}</option></select></label>
            </div>
            <div class="search-group">
              <input v-model="configSearch" :placeholder="t('application.appPages.config.searchPlaceholder')" />
              <button class="search-btn" type="button">🔍</button>
              <button class="refresh-btn" type="button">↻</button>
            </div>
          </div>

          <HNBTable :columns="configTableColumns" :data="configItems" :empty-title="t('application.appPages.config.noItems')" />
        </div>

        <div v-else-if="activeConfigTab === 'secrets'" class="config-section">
          <div class="config-notice">{{ t('application.appPages.config.secretsNotice') }}</div>
          <div class="action-bar">
            <button class="primary-button" type="button" @click="openAddSecret">
              <span class="icon">⊕</span> {{ t('application.appPages.config.addSecret') }}
            </button>
            <div class="filter-group">
              <label><span>{{ t('application.appPages.config.cluster') }}:</span><select v-model="configCluster"><option>default</option></select></label>
              <label><span>{{ t('application.appPages.config.application') }}:</span><select v-model="configApp"><option value="" disabled selected>{{ t('application.appPages.config.appPlaceholder') }}</option></select></label>
            </div>
            <div class="search-group">
              <input v-model="configSearch" :placeholder="t('application.appPages.config.searchPlaceholder')" />
              <button class="search-btn" type="button">🔍</button>
              <button class="refresh-btn" type="button">↻</button>
            </div>
          </div>
          <HNBTable :columns="secretTableColumns" :data="secretItems" :empty-title="t('application.appPages.config.noSecrets')" />
        </div>

        <div v-else class="config-section">
          <div class="config-notice">{{ t('application.appPages.config.vmConfigNotice') }}</div>
          <div class="action-bar">
            <button class="primary-button" type="button" @click="openAddVmConfig">
              <span class="icon">⊕</span> {{ t('application.appPages.config.addVmConfig') }}
            </button>
            <div class="search-group" style="margin-left:auto">
              <input v-model="vmConfigSearch" :placeholder="t('application.appPages.config.searchPlaceholder')" @keyup.enter="searchVmConfig" />
              <button class="search-btn" type="button" @click="searchVmConfig">🔍</button>
              <button class="refresh-btn" type="button" @click="refreshVmConfig">↻</button>
            </div>
          </div>
          <HNBTable :columns="vmConfigTableColumns" :data="vmConfigItems" :empty-title="t('application.appPages.config.noVmConfigs')" />
        </div>
      </div>
    </section>

    <AppCreateWizard v-if="wizardOpen" :mode="mode" :app-kind-label="appKindLabel" @close="closeWizard" @submit="submitWizard" />

    <ApplicationDrawer
      v-model="configDialogOpen"
      :title="configDrawerTitle"
      :busy="submittingConfig"
      :hide-confirm="configDrawerMode === 'detail'"
      width="680px"
      @confirm="submitConfigForm"
    >
      <dl v-if="configDrawerMode === 'detail' && selectedConfig" class="detail-list">
        <dt>{{ t('application.appPages.config.configName') }}</dt><dd>{{ selectedConfig.name }}</dd>
        <dt>{{ t('application.appPages.config.namespace') }}</dt><dd>{{ selectedConfig.namespace }}</dd>
        <dt>{{ t('application.appPages.config.application') }}</dt><dd>{{ selectedConfig.app }}</dd>
        <dt>{{ t('application.appPages.config.createdAt') }}</dt><dd>{{ selectedConfig.createdAt }}</dd>
        <template v-if="selectedConfig.version"><dt>{{ t('application.appPages.config.version') }}</dt><dd>{{ selectedConfig.version }}</dd></template>
        <template v-if="selectedConfig.description"><dt>{{ t('application.appPages.config.description') }}</dt><dd>{{ selectedConfig.description }}</dd></template>
        <template v-if="Object.keys(selectedConfig.labels || {}).length">
          <dt>{{ t('application.appPages.config.labels') }}</dt><dd>{{ Object.entries(selectedConfig.labels || {}).map(([key, value]) => `${key}=${value}`).join(', ') }}</dd>
        </template>
        <template v-if="selectedConfig.data?.length">
          <dt>{{ t('application.appPages.config.configData') }}</dt>
          <dd><div v-for="entry in selectedConfig.data" :key="entry.key"><strong>{{ entry.key }}</strong>: {{ entry.value }}</div></dd>
        </template>
        <template v-if="selectedConfig.yamlContent">
          <dt>{{ t('application.appPages.config.yamlContent') }}</dt><dd><pre class="detail-yaml">{{ selectedConfig.yamlContent }}</pre></dd>
        </template>
      </dl>
      <div v-else class="drawer-form">
          <div class="segmented">
            <button type="button" :class="{ active: configForm.createMode === 'visual' }" @click="configForm.createMode = 'visual'">{{ t('application.appPages.config.visualMode') }}</button>
            <button type="button" :class="{ active: configForm.createMode === 'yaml' }" @click="configForm.createMode = 'yaml'">{{ t('application.appPages.config.yamlMode') }}</button>
          </div>

          <template v-if="configForm.createMode === 'visual'">
            <label class="dialog-field required">
              <span>{{ t('application.appPages.config.configName') }}</span>
              <input v-model="configForm.name" :placeholder="t('application.appPages.config.configNamePlaceholder')" :class="{ error: configFormErrors.name }" />
              <small v-if="configFormErrors.name" class="error-text">{{ configFormErrors.name }}</small>
            </label>

            <label class="dialog-field required">
              <span>{{ t('application.appPages.config.version') }}</span>
              <input v-model="configForm.version" :placeholder="t('application.appPages.config.versionPlaceholder')" :class="{ error: configFormErrors.version }" />
              <small v-if="configFormErrors.version" class="error-text">{{ configFormErrors.version }}</small>
            </label>

            <label class="dialog-field required">
              <span>{{ t('application.appPages.config.application') }}</span>
              <select v-model="configForm.appId" :class="{ error: configFormErrors.appId }">
                <option value="" disabled>{{ t('application.appPages.config.appPlaceholder') }}</option>
                <option value="app1">pass</option>
              </select>
              <small v-if="configFormErrors.appId" class="error-text">{{ configFormErrors.appId }}</small>
            </label>

            <label class="dialog-field">
              <span>{{ t('application.appPages.config.description') }}</span>
              <textarea v-model="configForm.description" rows="3" />
            </label>

            <div class="dialog-field">
              <span>{{ t('application.appPages.config.labels') }}</span>
              <div v-for="(label, i) in configForm.labels" :key="i" class="inline-row">
                <input v-model="label.key" :placeholder="t('application.appPages.config.labelKey')" />
                <input v-model="label.value" :placeholder="t('application.appPages.config.labelValue')" />
                <button class="icon-btn-sm" type="button" @click="removeConfigLabel(i)">⊖</button>
              </div>
              <button class="add-btn" type="button" @click="addConfigLabel">⊕ {{ t('application.appPages.config.addLabel') }}</button>
            </div>

            <div class="dialog-field required">
              <span>{{ t('application.appPages.config.configData') }}</span>
              <div class="segmented">
                <button type="button" :class="{ active: configForm.dataMode === 'manual' }" @click="configForm.dataMode = 'manual'">{{ t('application.appPages.config.manualInput') }}</button>
                <button type="button" :class="{ active: configForm.dataMode === 'file' }" @click="configForm.dataMode = 'file'">{{ t('application.appPages.config.fileUpload') }}</button>
              </div>

              <template v-if="configForm.dataMode === 'manual'">
                <div v-for="(d, i) in configForm.data" :key="i" class="data-row">
                  <input v-model="d.key" :placeholder="t('application.appPages.config.dataKey')" :class="{ error: configFormErrors[`data_${i}_key`] }" />
                  <textarea v-model="d.value" :placeholder="t('application.appPages.config.dataValue')" rows="2" />
                  <div class="data-actions">
                    <button class="icon-btn-sm" type="button">✎</button>
                    <button class="icon-btn-sm" type="button" @click="removeConfigDataRow(i)">⊖</button>
                  </div>
                  <small v-if="configFormErrors[`data_${i}_key`]" class="error-text">{{ configFormErrors[`data_${i}_key`] }}</small>
                </div>
                <div class="btn-row">
                  <button class="add-btn" type="button" @click="addConfigDataRow">⊕ {{ t('application.appPages.config.addData') }}</button>
                  <button class="secondary-btn-sm" type="button">{{ t('application.appPages.config.quickAdd') }}</button>
                </div>
                <small v-if="configFormErrors.data" class="error-text">{{ configFormErrors.data }}</small>
              </template>

              <template v-else>
                <div class="upload-zone">
                  <p>{{ t('application.appPages.config.uploadHint') }}</p>
                  <button class="secondary-btn-sm" type="button">{{ t('application.appPages.config.selectFile') }}</button>
                </div>
              </template>
            </div>
          </template>

          <template v-else>
            <label class="dialog-field required">
              <span>{{ t('application.appPages.config.application') }}</span>
              <select v-model="configForm.yamlAppId" :class="{ error: configFormErrors.yamlAppId }">
                <option value="" disabled>{{ t('application.appPages.config.appPlaceholder') }}</option>
                <option value="app1">pass</option>
              </select>
              <small class="field-hint">{{ t('application.appPages.config.namespaceHint') }}</small>
              <small v-if="configFormErrors.yamlAppId" class="error-text">{{ configFormErrors.yamlAppId }}</small>
            </label>

            <label class="dialog-field">
              <span>{{ t('application.appPages.config.selectFile') }}</span>
              <div class="file-picker">
                <input :value="configForm.yamlFileName" readonly :placeholder="t('application.appPages.config.filePlaceholder')" />
                <button class="secondary-btn-sm" type="button" @click="browseYamlFile">{{ t('application.appPages.config.browse') }}</button>
              </div>
              <small class="field-hint">{{ t('application.appPages.config.fileHint') }}</small>
              <small v-if="configFormErrors.yamlFile" class="error-text">{{ configFormErrors.yamlFile }}</small>
              <input ref="yamlFileInput" type="file" accept=".txt,.conf,.yaml,.yml" hidden @change="onYamlFileSelected" />
            </label>

            <div class="dialog-field required">
              <span>{{ t('application.appPages.config.yamlContent') }} <small class="field-hint">(YAML)</small></span>
              <div class="yaml-editor-wrapper">
                <div class="yaml-gutter">
                  <div v-for="n in yamlLineCount" :key="n" class="yaml-line-num">{{ n }}</div>
                </div>
                <textarea v-model="configForm.yamlContent" class="yaml-editor" rows="18" :placeholder="t('application.appPages.config.yamlPlaceholder')" @input="updateYamlLineCount" spellcheck="false" />
              </div>
              <small v-if="configFormErrors.yamlContent" class="error-text">{{ configFormErrors.yamlContent }}</small>
            </div>
          </template>
      </div>
    </ApplicationDrawer>

    <ApplicationDrawer
      v-model="secretDialogOpen"
      :title="secretDrawerTitle"
      :busy="submittingSecret"
      :hide-confirm="secretDrawerMode === 'detail'"
      width="680px"
      @confirm="submitSecretForm"
    >
      <dl v-if="secretDrawerMode === 'detail' && selectedSecret" class="detail-list">
        <dt>{{ t('application.appPages.config.secretName') }}</dt><dd>{{ selectedSecret.name }}</dd>
        <dt>{{ t('application.appPages.config.namespace') }}</dt><dd>{{ selectedSecret.namespace }}</dd>
        <dt>{{ t('application.appPages.config.application') }}</dt><dd>{{ selectedSecret.app }}</dd>
        <dt>{{ t('application.appPages.config.createdAt') }}</dt><dd>{{ selectedSecret.createdAt }}</dd>
        <template v-if="selectedSecret.description"><dt>{{ t('application.appPages.config.description') }}</dt><dd>{{ selectedSecret.description }}</dd></template>
        <template v-if="Object.keys(selectedSecret.labels || {}).length">
          <dt>{{ t('application.appPages.config.labels') }}</dt><dd>{{ Object.entries(selectedSecret.labels || {}).map(([key, value]) => `${key}=${value}`).join(', ') }}</dd>
        </template>
        <template v-if="selectedSecret.usedByServices?.length">
          <dt>{{ t('application.appPages.config.usedByServices') }}</dt><dd>{{ selectedSecret.usedByServices.join(', ') }}</dd>
        </template>
        <template v-if="selectedSecret.dataKeys?.length">
          <dt>{{ t('application.appPages.config.secretData') }}</dt>
          <dd><div v-for="key in selectedSecret.dataKeys" :key="key" class="redacted-secret"><span>{{ key }}</span><code>********</code></div></dd>
        </template>
      </dl>
      <div v-else class="drawer-form">
          <div class="segmented">
            <button type="button" :class="{ active: secretForm.createMode === 'visual' }" @click="secretForm.createMode = 'visual'">{{ t('application.appPages.config.visualMode') }}</button>
            <button type="button" :class="{ active: secretForm.createMode === 'yaml' }" @click="secretForm.createMode = 'yaml'">{{ t('application.appPages.config.yamlMode') }}</button>
          </div>

          <template v-if="secretForm.createMode === 'visual'">
            <label class="dialog-field required">
              <span>{{ t('application.appPages.config.secretName') }}</span>
              <input v-model="secretForm.name" :placeholder="t('application.appPages.config.secretNamePlaceholder')" :class="{ error: secretFormErrors.name }" />
              <small v-if="secretFormErrors.name" class="error-text">{{ secretFormErrors.name }}</small>
            </label>
            <label class="dialog-field required">
              <span>{{ t('application.appPages.config.application') }}</span>
              <select v-model="secretForm.appId" :class="{ error: secretFormErrors.appId }">
                <option value="" disabled>{{ t('application.appPages.config.appPlaceholder') }}</option>
                <option value="app1">pass</option>
              </select>
              <small v-if="secretFormErrors.appId" class="error-text">{{ secretFormErrors.appId }}</small>
            </label>
            <label class="dialog-field">
              <span>{{ t('application.appPages.config.description') }}</span>
              <textarea v-model="secretForm.description" rows="3" />
            </label>
            <div class="dialog-field">
              <span>{{ t('application.appPages.config.labels') }}</span>
              <div v-for="(label, i) in secretForm.labels" :key="i" class="inline-row">
                <input v-model="label.key" :placeholder="t('application.appPages.config.labelKey')" />
                <input v-model="label.value" :placeholder="t('application.appPages.config.labelValue')" />
                <button class="icon-btn-sm" type="button" @click="removeSecretLabel(i)">⊖</button>
              </div>
              <button class="add-btn" type="button" @click="addSecretLabel">⊕ {{ t('application.appPages.config.addLabel') }}</button>
            </div>
            <div class="dialog-field required">
              <span>{{ t('application.appPages.config.secretData') }}</span>
              <div class="segmented">
                <button type="button" :class="{ active: secretForm.dataMode === 'manual' }" @click="secretForm.dataMode = 'manual'">{{ t('application.appPages.config.manualInput') }}</button>
                <button type="button" :class="{ active: secretForm.dataMode === 'file' }" @click="secretForm.dataMode = 'file'">{{ t('application.appPages.config.fileUpload') }}</button>
              </div>
              <template v-if="secretForm.dataMode === 'manual'">
                <div v-for="(d, i) in secretForm.data" :key="i" class="data-row">
                  <input v-model="d.key" :placeholder="t('application.appPages.config.dataKey')" :class="{ error: secretFormErrors[`sdata_${i}_key`] }" />
                  <textarea v-model="d.value" :placeholder="t('application.appPages.config.dataValue')" rows="2" :class="{ error: secretFormErrors[`sdata_${i}_val`] }" />
                  <div class="data-actions">
                    <label class="encode-check"><input v-model="d.autoEncode" type="checkbox" />{{ t('application.appPages.config.autoEncode') }}<span class="help-icon" :title="t('application.appPages.config.encodeHint')">?</span></label>
                    <button class="icon-btn-sm" type="button" @click="removeSecretDataRow(i)">⊖</button>
                  </div>
                  <small v-if="secretFormErrors[`sdata_${i}_key`]" class="error-text">{{ secretFormErrors[`sdata_${i}_key`] }}</small>
                  <small v-if="secretFormErrors[`sdata_${i}_val`]" class="error-text">{{ secretFormErrors[`sdata_${i}_val`] }}</small>
                </div>
                <button class="add-btn" type="button" @click="addSecretDataRow">⊕ {{ t('application.appPages.config.addData') }}</button>
                <small v-if="secretFormErrors.sdata" class="error-text">{{ secretFormErrors.sdata }}</small>
              </template>
              <template v-else>
                <div class="upload-zone"><p>{{ t('application.appPages.config.uploadHint') }}</p><button class="secondary-btn-sm" type="button">{{ t('application.appPages.config.selectFile') }}</button></div>
              </template>
            </div>
          </template>

          <template v-else>
            <label class="dialog-field required">
              <span>{{ t('application.appPages.config.application') }}</span>
              <select v-model="secretForm.yamlAppId" :class="{ error: secretFormErrors.yamlAppId }">
                <option value="" disabled>{{ t('application.appPages.config.appPlaceholder') }}</option>
                <option value="app1">pass</option>
              </select>
              <small class="field-hint">{{ t('application.appPages.config.namespaceHint') }}</small>
              <small v-if="secretFormErrors.yamlAppId" class="error-text">{{ secretFormErrors.yamlAppId }}</small>
            </label>
            <label class="dialog-field">
              <span>{{ t('application.appPages.config.selectFile') }}</span>
              <div class="file-picker"><input :value="secretForm.yamlFileName" readonly :placeholder="t('application.appPages.config.filePlaceholder')" /><button class="secondary-btn-sm" type="button" @click="browseSecretYamlFile">{{ t('application.appPages.config.browse') }}</button></div>
              <small class="field-hint">{{ t('application.appPages.config.fileHint') }}</small>
              <small v-if="secretFormErrors.yamlFile" class="error-text">{{ secretFormErrors.yamlFile }}</small>
              <input ref="secretYamlFileInput" type="file" accept=".txt,.conf,.yaml,.yml" hidden @change="onSecretYamlFileSelected" />
            </label>
            <div class="dialog-field required">
              <span>{{ t('application.appPages.config.yamlContent') }} <small class="field-hint">(YAML)</small></span>
              <div class="yaml-editor-wrapper">
                <div class="yaml-gutter"><div v-for="n in secretYamlLineCount" :key="n" class="yaml-line-num">{{ n }}</div></div>
                <textarea v-model="secretForm.yamlContent" class="yaml-editor" rows="18" :placeholder="t('application.appPages.config.yamlPlaceholder')" @input="updateSecretYamlLineCount" spellcheck="false" />
              </div>
              <small v-if="secretFormErrors.yamlContent" class="error-text">{{ secretFormErrors.yamlContent }}</small>
            </div>
          </template>
      </div>
    </ApplicationDrawer>

    <ApplicationDrawer
      v-model="vmConfigDialogOpen"
      :title="vmConfigDrawerTitle"
      :busy="submittingVmConfig"
      :hide-confirm="vmConfigDrawerMode === 'detail'"
      width="680px"
      @confirm="submitVmConfigForm"
    >
      <dl v-if="vmConfigDrawerMode === 'detail' && selectedVmConfig" class="detail-list">
        <dt>{{ t('application.appPages.config.configName') }}</dt><dd>{{ selectedVmConfig.name }}</dd>
        <dt>{{ t('application.appPages.config.project') }}</dt><dd>{{ selectedVmConfig.project }}</dd>
        <dt>{{ t('application.appPages.config.createdAt') }}</dt><dd>{{ selectedVmConfig.createdAt }}</dd>
        <template v-if="selectedVmConfig.version"><dt>{{ t('application.appPages.config.version') }}</dt><dd>{{ selectedVmConfig.version }}</dd></template>
        <template v-if="selectedVmConfig.description"><dt>{{ t('application.appPages.config.description') }}</dt><dd>{{ selectedVmConfig.description }}</dd></template>
        <template v-if="selectedVmConfig.fileNames?.length">
          <dt>{{ t('application.appPages.config.configData') }}</dt><dd>{{ selectedVmConfig.fileNames.join(', ') }}</dd>
        </template>
      </dl>
      <div v-else class="drawer-form">
          <label class="dialog-field required">
            <span>{{ t('application.appPages.config.configName') }}</span>
            <input v-model="vmConfigForm.name" :placeholder="t('application.appPages.config.configNamePlaceholder')" :class="{ error: vmConfigFormErrors.name }" />
            <small v-if="vmConfigFormErrors.name" class="error-text">{{ vmConfigFormErrors.name }}</small>
          </label>
          <label class="dialog-field required">
            <span>{{ t('application.appPages.config.version') }}</span>
            <input v-model="vmConfigForm.version" :placeholder="t('application.appPages.config.versionPlaceholder')" :class="{ error: vmConfigFormErrors.version }" />
            <small v-if="vmConfigFormErrors.version" class="error-text">{{ vmConfigFormErrors.version }}</small>
          </label>
          <label class="dialog-field required">
            <span>{{ t('application.appPages.config.project') }}</span>
            <select v-model="vmConfigForm.project" :class="{ error: vmConfigFormErrors.project }">
              <option>CLOUD-HCI-Test</option>
            </select>
            <small v-if="vmConfigFormErrors.project" class="error-text">{{ vmConfigFormErrors.project }}</small>
          </label>
          <label class="dialog-field">
            <span>{{ t('application.appPages.config.description') }}</span>
            <textarea v-model="vmConfigForm.description" rows="3" />
          </label>
          <div class="dialog-field required">
            <span>{{ t('application.appPages.config.configData') }}</span>
            <button class="add-btn" type="button" @click="browseVmConfigFiles">⊕ {{ t('application.appPages.config.clickUpload') }}</button>
            <input ref="vmConfigFileInput" type="file" multiple accept=".txt,.conf,.xml,.json,.yaml,.properties" hidden @change="onVmConfigFilesSelected" />
            <div class="vm-file-hints">
              <p>{{ t('application.appPages.config.vmFileHint1') }}</p>
              <p>{{ t('application.appPages.config.vmFileHint2') }}</p>
              <p>{{ t('application.appPages.config.vmFileHint3') }}</p>
            </div>
            <div v-for="(f, i) in vmConfigForm.files" :key="i" class="file-row">
              <span class="file-name">{{ f.name }}</span>
              <span class="file-size">{{ (f.size / 1024).toFixed(1) }} KB</span>
              <button class="icon-btn-sm" type="button" @click="removeVmConfigFile(i)">⊖</button>
            </div>
            <small v-if="vmConfigFormErrors.files" class="error-text">{{ vmConfigFormErrors.files }}</small>
          </div>
      </div>
    </ApplicationDrawer>

    <ApplicationDrawer
      v-model="groupDialogOpen"
      :title="t('application.appPages.groups.breadcrumbCurrent')"
      :busy="submittingGroup"
      :error="groupFormErrors.submit"
      width="680px"
      :confirm-text="t('application.appPages.groups.confirm')"
      @confirm="submitGroupForm"
    >
      <div class="drawer-form">
          <div class="group-info">
            <span class="info-icon">!</span>
            <div>
              <p>{{ t('application.appPages.groups.infoLine1') }}</p>
              <p>{{ t('application.appPages.groups.infoLine2') }}</p>
            </div>
          </div>
          <div class="group-form">
            <label class="dialog-field required">
              <span>{{ t('application.appPages.groups.name') }}</span>
              <div class="field-input-wrap"><input v-model="groupForm.name" :placeholder="t('application.appPages.groups.namePlaceholder')" :class="{ error: groupFormErrors.name }" /><span class="q-icon" title="应用组名称">?</span></div>
              <small v-if="groupFormErrors.name" class="error-text">{{ groupFormErrors.name }}</small>
            </label>
            <label class="dialog-field">
              <span>{{ t('application.appPages.groups.project') }}</span>
              <select v-model="groupForm.project"><option>CLOUD-HCI-Test</option></select>
            </label>
            <label class="dialog-field switch-field">
              <span>{{ t('application.appPages.groups.customNamespace') }}</span>
              <div class="toggle-group">
                <span :class="{ active: !groupForm.customNamespace }">{{ t('application.common.no') }}</span>
                <input v-model="groupForm.customNamespace" type="checkbox" />
                <span :class="{ active: groupForm.customNamespace }">{{ t('application.common.yes') }}</span>
              </div>
            </label>
            <div class="dialog-field required">
              <span>{{ t('application.appPages.groups.groupType') }}</span>
              <div class="type-desc">
                <span>{{ t('application.appPages.groups.typeDesc') }}</span>
                <span class="type-warning">{{ t('application.appPages.groups.typeWarning') }}</span>
              </div>
              <div class="type-cards">
                <button v-for="tp in ['springcloud','istio','custom']" :key="tp" class="type-card" :class="{ selected: groupForm.type === tp }" @click="groupForm.type = tp">
                  <span class="type-icon" :class="`icon-${tp}`">
                    <svg v-if="tp === 'springcloud'" width="22" height="22" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="var(--hnb-color-status-success, #12b76a)" stroke-width="1.5"/><path d="M9 12l2 2 4-4" stroke="var(--hnb-color-status-success, #12b76a)" stroke-width="1.5"/></svg>
                    <svg v-else-if="tp === 'istio'" width="22" height="22" viewBox="0 0 24 24" fill="none"><path d="M12 3L3 20h18L12 3z" stroke="var(--hnb-color-status-info, #5bb8f5)" stroke-width="1.5" stroke-linejoin="round"/><path d="M12 3v17" stroke="var(--hnb-color-status-info, #5bb8f5)" stroke-width="1.5"/></svg>
                    <svg v-else width="22" height="22" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="2.5" stroke="var(--hnb-color-text-secondary, #a9b2c2)" stroke-width="1.5"/><path d="M12 5v3M12 16v3M5 12h3M16 12h3" stroke="var(--hnb-color-text-secondary, #a9b2c2)" stroke-width="1.5"/></svg>
                  </span>
                  <span class="type-label">{{ t(`application.appPages.groups.typeLabels.${tp}`) }}</span>
                </button>
              </div>
            </div>
          </div>
      </div>
    </ApplicationDrawer>
  </section>
</template>

<style scoped>
.app-page { min-height: 100%; padding: 24px; color: var(--hnb-color-text-primary, #edeff5); background: var(--hnb-color-bg-void, #0b0f14); }
.app-page__header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 20px; }
.app-page__eyebrow { margin: 0 0 6px; color: var(--hnb-color-primary, #5b8dff); font-size: 12px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
.app-page h1 { margin: 0; font-size: 26px; }
.app-page p { margin: 8px 0 0; color: var(--hnb-color-text-secondary, #a9b2c2); }
.app-tabs { display: flex; gap: 8px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); margin-bottom: 16px; }
.app-tab { position: relative; padding: 12px 16px; border: 0; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); cursor: pointer; font-weight: 600; }
.app-tab.active { color: #fff; }
.app-tab.active::after { content: ''; position: absolute; left: 12px; right: 12px; bottom: -1px; height: 2px; background: var(--hnb-color-primary, #5b8dff); border-radius: 2px; }
.app-panel { background: var(--hnb-color-bg-void, #0b0f14); border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 14px; overflow: hidden; }
.panel-toolbar { display: flex; justify-content: space-between; gap: 16px; padding: 18px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.panel-toolbar h2 { margin: 0; font-size: 18px; }
.panel-toolbar p { margin: 6px 0 0; color: var(--hnb-color-text-secondary, #a9b2c2); }
.primary-button { border-radius: 8px; cursor: pointer; font-weight: 700; }
.primary-button { height: 36px; padding: 0 16px; border: 1px solid var(--hnb-color-primary, #5b8dff); background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.app-table__head { display: grid; grid-template-columns: 2fr 1fr 1fr 1fr; gap: 12px; padding: 12px 18px; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.app-empty { padding: 40px 18px; text-align: center; color: var(--hnb-color-text-secondary, #a9b2c2); }
.app-empty strong { color: var(--hnb-color-text-primary, #edeff5); }
.group-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; padding: 18px; }
.group-card { display: flex; flex-direction: column; gap: 8px; padding: 16px; border: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); border-radius: 10px; background: var(--hnb-color-bg-surface, #101425); cursor: pointer; }
.group-card:hover { border-color: var(--hnb-color-primary, #5b8dff); }
.group-name { margin: 0; font-size: 15px; color: var(--hnb-color-text-primary, #edeff5); font-weight: 600; }
.group-desc { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 12px; line-height: 1.5; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.group-meta { display: flex; justify-content: space-between; margin-top: auto; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 11px; }
.group-count { color: var(--hnb-color-primary, #5b8dff); }
.pagination-bar { display: flex; align-items: center; justify-content: space-between; padding: 12px 18px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; border-top: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); }
.pagination-info { color: var(--hnb-color-text-tertiary, #6b7a8a); }
.pagination-controls { display: flex; align-items: center; gap: 4px; }
.page-button { background: transparent; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 14px; padding: 4px 10px; cursor: pointer; }
.page-button:disabled { opacity: 0.4; cursor: not-allowed; }
.page-button:hover:not(:disabled) { border-color: var(--hnb-color-primary, #5b8dff); color: #fff; }
.page-num { padding: 4px 8px; border-radius: 4px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; cursor: pointer; }
.page-num.active { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.page-num:hover:not(.active) { background: var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); }
/* ───────────── Config Management ───────────── */
.config-page { padding: 0; }
.config-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.config-desc { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; line-height: 1.5; max-width: 80%; }
.help-link { color: var(--hnb-color-primary, #5b8dff); font-size: 13px; text-decoration: none; white-space: nowrap; }
.sub-tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--hnb-color-border, var(--hnb-color-border, #29344a)); margin-bottom: 16px; }
.sub-tabs button { padding: 10px 16px; border: 0; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; font-weight: 600; cursor: pointer; position: relative; }
.sub-tabs button.active { color: var(--hnb-color-text-primary, #edeff5); }
.sub-tabs button.active::after { content: ''; position: absolute; left: 12px; right: 12px; bottom: -1px; height: 2px; background: var(--hnb-color-primary, #5b8dff); border-radius: 2px; }
.config-notice { margin-bottom: 16px; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; line-height: 1.6; }
.action-bar { display: flex; align-items: center; gap: 14px; margin-bottom: 16px; flex-wrap: wrap; }
.action-bar .primary-button { display: flex; align-items: center; gap: 6px; height: 36px; padding: 0 16px; border: 1px solid var(--hnb-color-primary, #5b8dff); background: var(--hnb-color-primary, #5b8dff); color: #fff; border-radius: 8px; font-size: 13px; font-weight: 700; cursor: pointer; }
.action-bar .primary-button .icon { font-size: 16px; font-weight: 700; }
.filter-group { display: flex; align-items: center; gap: 10px; }
.filter-group label { display: flex; align-items: center; gap: 6px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.filter-group select { padding: 6px 10px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.search-group { display: flex; align-items: center; gap: 4px; margin-left: auto; }
.search-group input { width: 180px; padding: 6px 10px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.search-group input::placeholder { color: var(--hnb-color-text-tertiary, #6b7a8a); }
.search-btn { width: 32px; height: 32px; border: 1px solid var(--hnb-color-primary, #5b8dff); border-radius: 6px; background: var(--hnb-color-primary, #5b8dff); color: #fff; cursor: pointer; font-size: 14px; }
.refresh-btn { width: 32px; height: 32px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: transparent; color: var(--hnb-color-text-primary, #edeff5); cursor: pointer; font-size: 16px; }
.config-link { color: var(--hnb-color-primary, #5b8dff); text-decoration: none; font-weight: 600; }
.config-link:hover { text-decoration: underline; }
.action-link { color: var(--hnb-color-primary, #5b8dff); text-decoration: none; font-size: 13px; margin-right: 8px; }
.action-link:hover { text-decoration: underline; }
.action-link.danger { color: var(--hnb-color-status-danger, #f04438); }

.drawer-form { display: flex; flex-direction: column; gap: 18px; }
.detail-list { display: grid; grid-template-columns: minmax(120px, auto) 1fr; gap: 12px 20px; margin: 0; }
.detail-list dt { color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.detail-list dd { min-width: 0; margin: 0; color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; overflow-wrap: anywhere; }
.detail-yaml { margin: 0; padding: 12px; border-radius: 8px; color: #d4d4d4; background: #1e1e1e; white-space: pre-wrap; }
.redacted-secret { display: flex; justify-content: space-between; gap: 16px; }
.dialog-field { display: flex; flex-direction: column; gap: 6px; }
.dialog-field > span { color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; font-weight: 500; }
.dialog-field.required > span::after { content: ' *'; color: var(--hnb-color-status-danger, var(--hnb-color-status-danger, #f04438)); }
.dialog-field input, .dialog-field textarea, .dialog-field select { padding: 8px 12px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.dialog-field input:focus, .dialog-field textarea:focus, .dialog-field select:focus { outline: none; border-color: var(--hnb-color-primary, #5b8dff); }
.dialog-field input.error, .dialog-field select.error { border-color: var(--hnb-color-status-danger, var(--hnb-color-status-danger, #f04438)); }
.dialog-field textarea { resize: vertical; min-height: 60px; }
.error-text { color: var(--hnb-color-status-danger, var(--hnb-color-status-danger, #f04438)); font-size: 11px; }
.segmented { display: flex; gap: 4px; }
.segmented button { flex: 1; padding: 8px 12px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; cursor: pointer; }
.segmented button.active { background: var(--hnb-color-primary, #5b8dff); border-color: var(--hnb-color-primary, #5b8dff); color: #fff; }
.inline-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.inline-row input { flex: 1; padding: 6px 10px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.data-row { display: grid; grid-template-columns: 1fr 2fr auto; gap: 6px; margin-bottom: 8px; position: relative; }
.data-row input { padding: 6px 10px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.data-row input.error { border-color: var(--hnb-color-status-danger, var(--hnb-color-status-danger, #f04438)); }
.data-row textarea { padding: 6px 10px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; resize: vertical; }
.data-actions { display: flex; flex-direction: column; gap: 4px; }
.data-row .error-text { grid-column: 1 / -1; }
.icon-btn-sm { width: 28px; height: 28px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); cursor: pointer; font-size: 14px; display: flex; align-items: center; justify-content: center; }
.add-btn { padding: 6px 14px; border: 0; border-radius: 6px; background: var(--hnb-color-primary, #5b8dff); color: #fff; font-size: 13px; cursor: pointer; display: inline-flex; align-items: center; gap: 4px; }
.btn-row { display: flex; gap: 8px; }
.secondary-btn-sm { padding: 6px 14px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; cursor: pointer; }
.upload-zone { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 20px; border: 2px dashed var(--hnb-color-border, #29344a); border-radius: 10px; background: var(--hnb-color-bg-void, #0b0f14); }
.upload-zone p { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.label-chip { display: inline-block; padding: 2px 8px; margin: 2px; border-radius: 4px; background: var(--hnb-color-bg-elevated, #171d31); color: #8bb5ff; font-size: 11px; white-space: nowrap; }
.service-chip { display: inline-block; padding: 2px 8px; margin: 2px; border-radius: 4px; background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-status-success, var(--hnb-color-status-success, #12b76a)); font-size: 11px; }
.config-table-secrets th, .config-table-secrets td { font-size: 12px; }
.encode-check { display: flex; align-items: center; gap: 4px; padding: 4px 0; font-size: 11px; color: var(--hnb-color-text-secondary, #a9b2c2); cursor: pointer; }
.encode-check input[type="checkbox"] { accent-color: var(--hnb-color-primary, #5b8dff); }
.help-icon { display: inline-flex; align-items: center; justify-content: center; width: 16px; height: 16px; border-radius: 50%; background: var(--hnb-color-border, #29344a); color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 10px; cursor: help; position: relative; }
.vm-file-hints { display: flex; flex-direction: column; gap: 4px; margin: 8px 0; }
.vm-file-hints p { margin: 0; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 11px; line-height: 1.5; }
.file-row { display: flex; align-items: center; gap: 10px; padding: 6px 10px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: var(--hnb-color-bg-surface, #101425); margin-bottom: 4px; }
.file-row .file-name { flex: 1; color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.file-row .file-size { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 11px; }

/* ───────────── App Group Drawer ───────────── */
.group-info { display: flex; gap: 12px; padding: 14px 16px; background: var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); border: 1px solid var(--hnb-color-border, #29344a); border-radius: 10px; }
.info-icon { flex-shrink: 0; width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; border-radius: 50%; background: var(--hnb-color-primary, #5b8dff); color: #fff; font-size: 13px; font-weight: 700; }
.group-info p { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; line-height: 1.5; }
.group-info p + p { margin-top: 4px; }
.group-form { display: flex; flex-direction: column; gap: 18px; }
.field-input-wrap { display: flex; align-items: center; gap: 6px; flex: 1; }
.field-input-wrap input { flex: 1; padding: 8px 12px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; outline: none; }
.field-input-wrap input:focus { border-color: var(--hnb-color-primary, #5b8dff); }
.field-input-wrap input.error { border-color: var(--hnb-color-status-danger, var(--hnb-color-status-danger, #f04438)); }
.field-input-wrap input::placeholder { color: var(--hnb-color-text-tertiary, #6b7a8a); }
.q-icon { width: 18px; height: 18px; display: flex; align-items: center; justify-content: center; border-radius: 50%; border: 1px solid var(--hnb-color-border, #29344a); color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 11px; cursor: help; flex-shrink: 0; }
.switch-field .toggle-group { padding-top: 6px; }
.type-desc { display: flex; flex-direction: row; gap: 4px; align-items: center; flex-wrap: wrap; }
.type-desc span { font-size: 12px; color: var(--hnb-color-text-secondary, #a9b2c2); }
.type-desc .type-warning { color: var(--hnb-color-status-warning, #f79009); }
.type-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-top: 8px; }
.type-card { display: flex; align-items: center; gap: 10px; padding: 14px 16px; border: 2px solid var(--hnb-color-border, #29344a); border-radius: 10px; background: var(--hnb-color-bg-surface, #101425); cursor: pointer; transition: border-color 0.15s; position: relative; }
.type-card:hover { border-color: var(--hnb-color-primary, #5b8dff); }
.type-card.selected { border-color: var(--hnb-color-primary, #5b8dff); }
.type-card.selected::after { content: '✓'; position: absolute; right: 8px; bottom: 8px; width: 18px; height: 18px; display: flex; align-items: center; justify-content: center; border-radius: 50%; background: var(--hnb-color-primary, #5b8dff); color: #fff; font-size: 10px; font-weight: 700; }
.type-icon { flex-shrink: 0; width: 36px; height: 36px; display: flex; align-items: center; justify-content: center; border-radius: 50%; }
.type-icon.icon-springcloud { background: var(--hnb-color-status-success, #12b76a); }
.type-icon.icon-istio { background: var(--hnb-color-status-info, #5bb8f5); }
.type-icon.icon-custom { background: var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); }
.type-label { font-size: 13px; font-weight: 600; color: var(--hnb-color-text-primary, #edeff5); }
.file-picker { display: flex; gap: 8px; }
.file-picker input { flex: 1; padding: 8px 12px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.file-picker input::placeholder { color: var(--hnb-color-text-tertiary, #6b7a8a); }
.field-hint { margin: 0; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 11px; }
.yaml-editor-wrapper { display: flex; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; overflow: hidden; background: #1e1e1e; min-height: 400px; }
.yaml-editor-wrapper:focus-within { border-color: var(--hnb-color-primary, #5b8dff); }
.yaml-gutter { display: flex; flex-direction: column; padding: 10px 0; background: #252526; color: #858585; text-align: right; min-width: 36px; user-select: none; }
.yaml-line-num { padding: 0 8px; font-family: 'Consolas', 'Monaco', monospace; font-size: 12px; line-height: 1.6; }
.yaml-editor { flex: 1; padding: 10px 12px; border: 0; background: transparent; color: #d4d4d4; font-family: 'Consolas', 'Monaco', monospace; font-size: 12px; line-height: 1.6; resize: vertical; outline: none; min-height: 400px; tab-size: 2; }
.yaml-editor::placeholder { color: var(--hnb-color-text-tertiary, #6b7a8a); }
@media (max-width: 768px) {
  .app-page { padding: 16px; }
  .panel-toolbar { flex-direction: column; align-items: stretch; }
}
</style>
