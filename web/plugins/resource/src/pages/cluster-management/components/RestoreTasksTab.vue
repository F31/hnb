<script setup lang="ts">
/**
 * RestoreTasksTab — 恢复任务管理。
 * 顶部「执行恢复任务」；列表：名称/目标集群/目标命名空间/恢复的文件/状态/恢复进度/描述/创建时间/操作。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBButton, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { createRestoreTask, getRestoreTasks } from '../api/backupApi'
import { getClusterPermissionStore } from '../api/clusterApi'
import { usePluginContext } from '../composables/usePluginContext'
import ClusterDrawer from './ClusterDrawer.vue'
import type { RestoreTask } from '../types/backup'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()

const items = ref<RestoreTask[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 抽屉（执行恢复任务） ----
const drawerVisible = ref(false)
const drawerBusy = ref(false)
const drawerError = ref('')
const formName = ref('')
const formTargetCluster = ref('')
const formTargetNamespace = ref('')
const formRestoredFile = ref('')

function statusBadge(task: RestoreTask): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = {
    running: 'default',
    success: 'success',
    failed: 'error',
    pending: 'default',
  }
  return { label: t(`resource.clusterMgmt.backup.restore.status.${task.status}`), semantic: map[task.status] ?? 'default' }
}

const columns = computed<HNBTableColumn<RestoreTask>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.backup.restore.colName'), render: (r) => r.name || '--' },
  { key: 'targetCluster', title: t('resource.clusterMgmt.backup.restore.colCluster'), render: (r) => r.targetCluster || '--' },
  { key: 'targetNamespace', title: t('resource.clusterMgmt.backup.restore.colNamespace'), render: (r) => r.targetNamespace || '--' },
  { key: 'restoredFile', title: t('resource.clusterMgmt.backup.restore.colFile'), render: (r) => r.restoredFile || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.backup.restore.colStatus'),
    render: (r) => {
      const b = statusBadge(r)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'progress', title: t('resource.clusterMgmt.backup.restore.colProgress'), render: (r) => `${r.progress}%` },
  { key: 'description', title: t('resource.clusterMgmt.backup.restore.colDesc'), render: (r) => r.description || '--' },
  { key: 'createdAt', title: t('resource.clusterMgmt.backup.restore.colCreated'), render: (r) => r.createdAt || '--' },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (r) => {
      const actions: HNBTableAction[] = [{ label: t('resource.clusterMgmt.backup.restore.action.view'), key: 'view' }]
      return h(HNBTableActions, { actions, onAction: () => pluginCtx.notify(t('resource.clusterMgmt.placeholder.notEnabled')) })
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getRestoreTasks('', { keyword: keyword.value })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  if (!canUpdate.value) return
  formName.value = ''
  formTargetCluster.value = ''
  formTargetNamespace.value = ''
  formRestoredFile.value = ''
  drawerError.value = ''
  drawerVisible.value = true
}

async function onDrawerConfirm(): Promise<void> {
  if (!formName.value.trim() || !formRestoredFile.value.trim()) {
    drawerError.value = t('resource.clusterMgmt.backup.restore.form.required')
    return
  }
  drawerBusy.value = true
  drawerError.value = ''
  const now = new Date().toISOString().slice(0, 19).replace('T', ' ')
  try {
    await createRestoreTask('', {
      name: formName.value.trim(),
      targetCluster: formTargetCluster.value.trim() || 'graphify',
      targetNamespace: formTargetNamespace.value.trim() || 'default',
      restoredFile: formRestoredFile.value.trim(),
      status: 'pending',
      progress: 0,
      description: '',
      createdAt: now,
    })
    drawerVisible.value = false
    pluginCtx.notify(t('resource.clusterMgmt.backup.restore.submitted'))
    await load()
  } catch (err) {
    drawerError.value = err instanceof Error ? err.message : String(err)
  } finally {
    drawerBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="backup-tab-panel" :aria-label="t('resource.clusterMgmt.backup.tab.restoreTasks')">
    <div class="panel-toolbar">
      <HNBButton v-if="canUpdate" @click="openCreate">
        {{ t('resource.clusterMgmt.backup.restore.action.execute') }}
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

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('resource.clusterMgmt.backup.empty')"
      min-width="1280px"
      :aria-label="t('resource.clusterMgmt.backup.tab.restoreTasks')"
    />

    <ClusterDrawer
      v-model="drawerVisible"
      :title="t('resource.clusterMgmt.backup.restore.drawer.title')"
      :busy="drawerBusy"
      :error="drawerError"
      @confirm="onDrawerConfirm"
    >
      <form class="backup-form" @submit.prevent="onDrawerConfirm">
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.restore.form.name') }}</span>
          <input v-model="formName" type="text" :placeholder="t('resource.clusterMgmt.backup.restore.form.namePlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.restore.form.cluster') }}</span>
          <input v-model="formTargetCluster" type="text" placeholder="graphify" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.restore.form.namespace') }}</span>
          <input v-model="formTargetNamespace" type="text" placeholder="default" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.restore.form.file') }}</span>
          <input v-model="formRestoredFile" type="text" :placeholder="t('resource.clusterMgmt.backup.restore.form.filePlaceholder')" />
        </label>
      </form>
    </ClusterDrawer>
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
.form-field input {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
</style>
