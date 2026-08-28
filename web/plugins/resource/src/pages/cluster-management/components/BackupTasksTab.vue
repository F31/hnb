<script setup lang="ts">
/**
 * BackupTasksTab — 备份任务管理。
 * 列表：备份文件名称/源集群/所属备份策略/开始时间/结束时间/状态/备份进度/执行类型/操作。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { getBackupTasks } from '../api/backupApi'
import { usePluginContext } from '../composables/usePluginContext'
import type { BackupTask } from '../types/backup'

const { t } = useI18n()
const pluginCtx = usePluginContext()

const items = ref<BackupTask[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

function statusBadge(task: BackupTask): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = {
    running: 'default',
    success: 'success',
    failed: 'error',
    pending: 'default',
  }
  return { label: t(`resource.clusterMgmt.backup.task.status.${task.status}`), semantic: map[task.status] ?? 'default' }
}

function execTypeLabel(type: BackupTask['execType']): string {
  return t(`resource.clusterMgmt.backup.task.execType.${type}`)
}

const columns = computed<HNBTableColumn<BackupTask>[]>(() => [
  { key: 'backupFileName', title: t('resource.clusterMgmt.backup.task.colFile'), render: (r) => r.backupFileName || '--' },
  { key: 'sourceCluster', title: t('resource.clusterMgmt.backup.task.colSource'), render: (r) => r.sourceCluster || '--' },
  { key: 'backupPolicy', title: t('resource.clusterMgmt.backup.task.colPolicy'), render: (r) => r.backupPolicy || '--' },
  { key: 'startTime', title: t('resource.clusterMgmt.backup.task.colStart'), render: (r) => r.startTime || '--' },
  { key: 'endTime', title: t('resource.clusterMgmt.backup.task.colEnd'), render: (r) => r.endTime || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.backup.task.colStatus'),
    render: (r) => {
      const b = statusBadge(r)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'progress', title: t('resource.clusterMgmt.backup.task.colProgress'), render: (r) => `${r.progress}%` },
  { key: 'execType', title: t('resource.clusterMgmt.backup.task.colExecType'), render: (r) => execTypeLabel(r.execType) },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (r) => h('button', { type: 'button', class: 'text-action', onClick: () => pluginCtx.notify(t('resource.clusterMgmt.placeholder.notEnabled')) }, t('resource.clusterMgmt.backup.task.action.view')),
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getBackupTasks('', { keyword: keyword.value })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="backup-tab-panel" :aria-label="t('resource.clusterMgmt.backup.tab.backupTasks')">
    <div class="panel-toolbar">
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
      :aria-label="t('resource.clusterMgmt.backup.tab.backupTasks')"
    />
  </section>
</template>

<style scoped>
.backup-tab-panel { display: flex; flex-direction: column; gap: 10px; }
.panel-toolbar { display: flex; align-items: center; justify-content: flex-end; gap: 12px; flex-wrap: wrap; }
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
:deep(.text-action) {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 13px;
  padding: 2px 6px;
}
</style>
