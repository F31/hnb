<script setup lang="ts">
/**
 * BackupPoliciesTab — 备份策略管理。
 * 列表：名称/集群/状态/备份方式/下次备份时间/创建时间/操作；
 * 顶部「创建备份策略」；操作：执行备份/编辑/删除。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBConfirmation, HNBButton, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import {
  createBackupPolicy,
  deleteBackupPolicy,
  executeBackup,
  getBackupPolicies,
  updateBackupPolicy,
} from '../api/backupApi'
import { getClusterPermissionStore } from '../api/clusterApi'
import { usePluginContext } from '../composables/usePluginContext'
import ClusterDrawer from './ClusterDrawer.vue'
import type { BackupPolicy } from '../types/backup'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()

const items = ref<BackupPolicy[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 抽屉（创建/编辑） ----
const drawerVisible = ref(false)
const drawerBusy = ref(false)
const drawerError = ref('')
const editingName = ref('')
const formName = ref('')
const formCluster = ref('')
const formMethod = ref('全量')

const methodOptions = ['全量', '增量', '差异']

// ---- 删除/执行确认 ----
const confirmDelete = ref(false)
const deleteTarget = ref('')
const actionError = ref('')

function statusBadge(p: BackupPolicy): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = { enabled: 'success', disabled: 'default', unknown: 'error' }
  return { label: t(`resource.clusterMgmt.backup.policy.status.${p.status}`), semantic: map[p.status] ?? 'default' }
}

const columns = computed<HNBTableColumn<BackupPolicy>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.backup.policy.colName'), render: (r) => r.name || '--' },
  { key: 'cluster', title: t('resource.clusterMgmt.backup.policy.colCluster'), render: (r) => r.cluster || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.backup.policy.colStatus'),
    render: (r) => {
      const b = statusBadge(r)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'backupMethod', title: t('resource.clusterMgmt.backup.policy.colMethod'), render: (r) => r.backupMethod || '--' },
  { key: 'nextBackupAt', title: t('resource.clusterMgmt.backup.policy.colNext'), render: (r) => r.nextBackupAt || '--' },
  { key: 'createdAt', title: t('resource.clusterMgmt.backup.policy.colCreated'), render: (r) => r.createdAt || '--' },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (r) => {
      const actions: HNBTableAction[] = [
        { label: t('resource.clusterMgmt.backup.policy.action.execute'), key: 'execute' },
        { label: t('resource.clusterMgmt.backup.policy.action.edit'), key: 'edit' },
        { label: t('resource.clusterMgmt.backup.policy.action.delete'), key: 'delete', variant: 'danger' },
      ]
      return h(HNBTableActions, {
        actions,
        disabled: !canUpdate.value,
        onAction: (key: string) => {
          if (key === 'execute') onExecute(r)
          else if (key === 'edit') openEdit(r)
          else if (key === 'delete') requestDelete(r)
        },
      })
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getBackupPolicies('', { keyword: keyword.value })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  if (!canUpdate.value) return
  editingName.value = ''
  formName.value = ''
  formCluster.value = ''
  formMethod.value = '全量'
  drawerError.value = ''
  drawerVisible.value = true
}

function openEdit(p: BackupPolicy): void {
  if (!canUpdate.value) return
  editingName.value = p.name
  formName.value = p.name
  formCluster.value = p.cluster
  formMethod.value = p.backupMethod
  drawerError.value = ''
  drawerVisible.value = true
}

async function onDrawerConfirm(): Promise<void> {
  if (!formName.value.trim()) {
    drawerError.value = t('resource.clusterMgmt.backup.policy.form.nameRequired')
    return
  }
  drawerBusy.value = true
  drawerError.value = ''
  const now = new Date().toISOString().slice(0, 19).replace('T', ' ')
  const payload: BackupPolicy = {
    name: formName.value.trim(),
    cluster: formCluster.value.trim() || 'graphify',
    status: 'enabled',
    backupMethod: formMethod.value,
    nextBackupAt: '--',
    createdAt: editingName.value ? (items.value.find((i) => i.name === editingName.value)?.createdAt ?? now) : now,
  }
  try {
    if (editingName.value) await updateBackupPolicy('', editingName.value, payload)
    else await createBackupPolicy('', payload)
    drawerVisible.value = false
    await load()
  } catch (err) {
    drawerError.value = err instanceof Error ? err.message : String(err)
  } finally {
    drawerBusy.value = false
  }
}

async function onExecute(p: BackupPolicy): Promise<void> {
  actionError.value = ''
  try {
    await executeBackup('', p.name)
    pluginCtx.notify(t('resource.clusterMgmt.backup.policy.executed'))
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

function requestDelete(p: BackupPolicy): void {
  if (!canUpdate.value) return
  deleteTarget.value = p.name
  actionError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  actionError.value = ''
  try {
    await deleteBackupPolicy('', deleteTarget.value)
    confirmDelete.value = false
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <section class="backup-tab-panel" :aria-label="t('resource.clusterMgmt.backup.tab.backupPolicies')">
    <div class="panel-toolbar">
      <HNBButton v-if="canUpdate" @click="openCreate">
        {{ t('resource.clusterMgmt.backup.policy.action.create') }}
      </HNBButton>
      <div class="toolbar-right">
        <input
          v-model="keyword"
          class="keyword-input"
          type="text"
          :placeholder="t('resource.clusterMgmt.backup.keywordPlaceholder')"
          @keyup.enter="load"
        />
        <button class="secondary-button" type="button" @click="load">
          {{ t('resource.clusterMgmt.action.query') }}
        </button>
        <button class="secondary-button" type="button" @click="load">
          {{ t('resource.clusterMgmt.action.refresh') }}
        </button>
      </div>
    </div>

    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>
    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('resource.clusterMgmt.backup.empty')"
      min-width="1080px"
      :aria-label="t('resource.clusterMgmt.backup.tab.backupPolicies')"
    />

    <ClusterDrawer
      v-model="drawerVisible"
      :title="editingName ? t('resource.clusterMgmt.backup.policy.drawer.editTitle') : t('resource.clusterMgmt.backup.policy.drawer.createTitle')"
      :busy="drawerBusy"
      :error="drawerError"
      @confirm="onDrawerConfirm"
    >
      <form class="backup-form" @submit.prevent="onDrawerConfirm">
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.policy.form.name') }}</span>
          <input v-model="formName" type="text" :placeholder="t('resource.clusterMgmt.backup.policy.form.namePlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.policy.form.cluster') }}</span>
          <input v-model="formCluster" type="text" placeholder="graphify" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.policy.form.method') }}</span>
          <select v-model="formMethod">
            <option v-for="m in methodOptions" :key="m" :value="m">{{ m }}</option>
          </select>
        </label>
      </form>
    </ClusterDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('resource.clusterMgmt.backup.policy.deleteTitle')"
      :description="t('resource.clusterMgmt.backup.policy.deleteMessage', { name: deleteTarget })"
      :error="actionError"
      danger
      @confirm="onConfirmDelete"
    />
  </section>
</template>

<style scoped>
.backup-tab-panel { display: flex; flex-direction: column; gap: 10px; }
.panel-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
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
.backup-form { display: flex; flex-direction: column; gap: 14px; }
.form-field { display: flex; flex-direction: column; gap: 6px; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.form-field input, .form-field select {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
</style>
