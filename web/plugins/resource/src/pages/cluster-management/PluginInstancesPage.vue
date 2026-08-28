<script setup lang="ts">
/**
 * PluginInstancesPage — 集群详情 > 插件管理（OpenSpec plugin-management）。
 * 插件实例列表（状态圆点）+ 查看参数YAML（只读代码视图）+ 新建/更新抽屉
 * （插件与版本联动下拉、values YAML 校验）+ 删除确认。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  HNBTable,
  HNBTableActions,
  HNBConfirmation,
  HNBDialog,
  StatusBadge,
  HNBButton,
} from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTablePagination, HNBTableAction } from '@hnb/ui-kit'
import {
  createPluginInstance,
  deletePluginInstance,
  getPluginInstances,
  getPluginVersionCatalog,
  updatePluginInstance,
} from './api/p4Api'
import { getClusterPermissionStore } from './api/clusterApi'
import { usePluginContext } from './composables/usePluginContext'
import ClusterDetailLayout from './components/ClusterDetailLayout.vue'
import ClusterDrawer from './components/ClusterDrawer.vue'
import YamlEditor from './components/YamlEditor.vue'
import { validateYaml } from './utils/yaml'
import type { PluginInstance, PluginVersionCatalog } from './types/p4'

const { t } = useI18n()
const route = useRoute()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()
const clusterId = String(route.params.clusterId ?? '')

const items = ref<PluginInstance[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

const pagination = computed<HNBTablePagination>(() => ({ page: page.value, pageSize: pageSize.value, total: total.value }))

function statusBadge(inst: PluginInstance): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = { running: 'success', abnormal: 'error', unknown: 'default' }
  return { label: t(`resource.clusterMgmt.pluginInst.status.${inst.status}`), semantic: map[inst.status] ?? 'default' }
}

// ---- YAML 查看 ----
const yamlVisible = ref(false)
const yamlContent = ref('')

// ---- 抽屉（新建/更新） ----
const drawerVisible = ref(false)
const drawerBusy = ref(false)
const drawerError = ref('')
const editingApp = ref('')
const formAppName = ref('')
const formPluginName = ref('')
const formPluginVersion = ref('')
const formValues = ref('')
const catalog = ref<PluginVersionCatalog[]>([])

// ---- 删除确认 ----
const confirmDelete = ref(false)
const deletingApp = ref('')
const deleteError = ref('')

const columns = computed<HNBTableColumn<PluginInstance>[]>(() => [
  { key: 'applicationName', title: t('resource.clusterMgmt.pluginInst.colApp'), render: (row) => row.applicationName || '--' },
  { key: 'description', title: t('resource.clusterMgmt.pluginInst.colDesc'), render: (row) => row.description || '--' },
  { key: 'pluginName', title: t('resource.clusterMgmt.pluginInst.colPlugin'), render: (row) => row.pluginName || '--' },
  { key: 'pluginVersion', title: t('resource.clusterMgmt.pluginInst.colVersion'), render: (row) => row.pluginVersion || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.pluginInst.colStatus'),
    render: (row) => {
      const b = statusBadge(row)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'createdAt', title: t('resource.clusterMgmt.pluginInst.colCreatedAt'), render: (row) => row.createdAt || '--' },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (row) => {
      const actions: HNBTableAction[] = [
        { label: t('resource.clusterMgmt.pluginInst.action.viewYaml'), key: 'viewYaml' },
        { label: t('resource.clusterMgmt.pluginInst.action.update'), key: 'update' },
        { label: t('resource.clusterMgmt.pluginInst.action.delete'), key: 'delete', variant: 'danger' },
      ]
      return h(HNBTableActions, {
        actions,
        onAction: (key: string) => {
          if (key === 'viewYaml') openYaml(row)
          else if (key === 'update') openUpdate(row)
          else if (key === 'delete') requestDelete(row)
        },
      })
    },
  },
])

const pluginOptions = computed(() => catalog.value.map((c) => ({ label: c.pluginName, value: c.pluginName })))
const versionOptions = computed(() => {
  const entry = catalog.value.find((c) => c.pluginName === formPluginName.value)
  return (entry?.versions ?? []).map((v) => ({ label: v, value: v }))
})

async function loadCatalog(): Promise<void> {
  try {
    catalog.value = await getPluginVersionCatalog(clusterId)
  } catch {
    catalog.value = []
  }
}

function openYaml(row: PluginInstance): void {
  yamlContent.value = row.valuesYaml ?? ''
  yamlVisible.value = true
}

function openCreate(): void {
  if (!canUpdate.value) return
  editingApp.value = ''
  formAppName.value = ''
  const firstPlugin = catalog.value[0]
  formPluginName.value = firstPlugin?.pluginName ?? ''
  formPluginVersion.value = firstPlugin?.versions[0] ?? ''
  formValues.value = ''
  drawerError.value = ''
  drawerVisible.value = true
}

function openUpdate(row: PluginInstance): void {
  if (!canUpdate.value) return
  editingApp.value = row.applicationName
  formAppName.value = row.applicationName
  formPluginName.value = row.pluginName
  formPluginVersion.value = row.pluginVersion
  formValues.value = row.valuesYaml ?? ''
  drawerError.value = ''
  drawerVisible.value = true
}

function onPluginChange(): void {
  formPluginVersion.value = versionOptions.value[0]?.value ?? ''
}

async function onDrawerConfirm(): Promise<void> {
  if (!formAppName.value.trim()) {
    drawerError.value = t('resource.clusterMgmt.pluginInst.form.appRequired')
    return
  }
  if (!formPluginName.value || !formPluginVersion.value) {
    drawerError.value = t('resource.clusterMgmt.pluginInst.form.pluginRequired')
    return
  }
  const yamlErr = validateYaml(formValues.value)
  if (yamlErr) {
    drawerError.value = yamlErr
    return
  }
  drawerBusy.value = true
  drawerError.value = ''
  try {
    const payload = {
      applicationName: formAppName.value.trim(),
      pluginName: formPluginName.value,
      pluginVersion: formPluginVersion.value,
      values: formValues.value,
    }
    if (editingApp.value) {
      await updatePluginInstance(clusterId, editingApp.value, payload)
    } else {
      await createPluginInstance(clusterId, payload)
    }
    drawerVisible.value = false
    pluginCtx.notify(t('resource.clusterMgmt.pluginInst.saved'))
    await load()
  } catch (err) {
    drawerError.value = err instanceof Error ? err.message : String(err)
  } finally {
    drawerBusy.value = false
  }
}

function requestDelete(row: PluginInstance): void {
  if (!canUpdate.value) return
  deletingApp.value = row.applicationName
  deleteError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  deleteError.value = ''
  try {
    await deletePluginInstance(clusterId, deletingApp.value)
    confirmDelete.value = false
    pluginCtx.notify(t('resource.clusterMgmt.pluginInst.deleted'))
    await load()
  } catch (err) {
    deleteError.value = err instanceof Error ? err.message : String(err)
  }
}

async function load(): Promise<void> {
  if (!clusterId) return
  loading.value = true
  error.value = ''
  try {
    const res = await getPluginInstances(clusterId, {
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value,
    })
    items.value = res.items
    total.value = res.total
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function onSearch(): void {
  page.value = 1
  load()
}

function onPage(p: number): void {
  page.value = p
  load()
}

function onPageSize(ps: number): void {
  pageSize.value = ps
  page.value = 1
  load()
}

onMounted(() => {
  loadCatalog()
  load()
})
</script>

<template>
  <ClusterDetailLayout>
    <div class="plugin-page">
      <div class="page-toolbar">
        <HNBButton v-if="canUpdate" @click="openCreate">
          {{ t('resource.clusterMgmt.pluginInst.action.create') }}
        </HNBButton>
        <div class="toolbar-right">
          <input
            v-model="keyword"
            class="keyword-input"
            type="text"
            :placeholder="t('resource.clusterMgmt.pluginInst.keywordPlaceholder')"
            @keyup.enter="onSearch"
          />
          <button class="secondary-button" type="button" @click="onSearch">
            {{ t('resource.clusterMgmt.action.query') }}
          </button>
          <button class="secondary-button" type="button" @click="load">
            {{ t('resource.clusterMgmt.action.refresh') }}
          </button>
        </div>
      </div>

      <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
      <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
      <HNBTable
        v-else
        :columns="columns"
        :data="items"
        :pagination="pagination"
        :empty-title="t('resource.clusterMgmt.pluginInst.empty')"
        min-width="1080px"
        :aria-label="t('resource.clusterMgmt.pluginInst.title')"
        @update:page="onPage"
        @update:page-size="onPageSize"
      />
    </div>

    <HNBDialog v-model="yamlVisible" :title="t('resource.clusterMgmt.pluginInst.yamlTitle')">
      <YamlEditor :model-value="yamlContent" readonly :rows="16" />
    </HNBDialog>

    <ClusterDrawer
      v-model="drawerVisible"
      :title="editingApp ? t('resource.clusterMgmt.pluginInst.drawer.updateTitle') : t('resource.clusterMgmt.pluginInst.drawer.createTitle')"
      :busy="drawerBusy"
      :error="drawerError"
      @confirm="onDrawerConfirm"
    >
      <form class="plugin-form" @submit.prevent="onDrawerConfirm">
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.pluginInst.form.appName') }}</span>
          <input v-model="formAppName" type="text" :placeholder="t('resource.clusterMgmt.pluginInst.form.appPlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.pluginInst.form.pluginName') }}</span>
          <select v-model="formPluginName" @change="onPluginChange">
            <option v-for="opt in pluginOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.pluginInst.form.pluginVersion') }}</span>
          <select v-model="formPluginVersion">
            <option v-for="opt in versionOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
        </label>
        <YamlEditor v-model="formValues" :label="t('resource.clusterMgmt.pluginInst.form.values')" :rows="12" placeholder="replicas: 1" />
      </form>
    </ClusterDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('resource.clusterMgmt.pluginInst.deleteTitle')"
      :description="t('resource.clusterMgmt.pluginInst.deleteMessage', { name: deletingApp })"
      :error="deleteError"
      danger
      @confirm="onConfirmDelete"
    />
  </ClusterDetailLayout>
</template>

<style scoped>
.plugin-page { display: flex; flex-direction: column; gap: 10px; }
.page-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.toolbar-right { display: flex; gap: 8px; flex-wrap: wrap; }
.keyword-input {
  padding: 6px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  min-width: 220px;
}
.secondary-button {
  padding: 7px 14px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  cursor: pointer;
}
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.plugin-form { display: flex; flex-direction: column; gap: 14px; }
.form-field { display: flex; flex-direction: column; gap: 6px; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.form-field input, .form-field select {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
</style>
