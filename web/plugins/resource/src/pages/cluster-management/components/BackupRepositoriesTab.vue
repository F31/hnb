<script setup lang="ts">
/**
 * BackupRepositoriesTab — 备份存储仓库。
 * 顶部「创建备份存储仓库」；列表：名称/集群/访问地址/地域/存储桶/可用性/强制路径风格/操作。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBButton, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { createBackupRepository, getBackupRepositories } from '../api/backupApi'
import { getClusterPermissionStore } from '../api/clusterApi'
import { usePluginContext } from '../composables/usePluginContext'
import ClusterDrawer from './ClusterDrawer.vue'
import type { BackupRepository } from '../types/backup'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()

const items = ref<BackupRepository[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 抽屉（创建） ----
const drawerVisible = ref(false)
const drawerBusy = ref(false)
const drawerError = ref('')
const formName = ref('')
const formAccessUrl = ref('')
const formRegion = ref('')
const formBucket = ref('')
const formForcePathStyle = ref(true)

function availabilityBadge(r: BackupRepository): { label: string; semantic: `success` | `error` } {
  return r.availability === 'available'
    ? { label: t('resource.clusterMgmt.backup.repo.available'), semantic: 'success' as const }
    : { label: t('resource.clusterMgmt.backup.repo.unavailable'), semantic: 'error' as const }
}

const columns = computed<HNBTableColumn<BackupRepository>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.backup.repo.colName'), render: (r) => r.name || '--' },
  { key: 'cluster', title: t('resource.clusterMgmt.backup.repo.colCluster'), render: (r) => r.cluster || '--' },
  { key: 'accessUrl', title: t('resource.clusterMgmt.backup.repo.colAccess'), render: (r) => r.accessUrl || '--' },
  { key: 'region', title: t('resource.clusterMgmt.backup.repo.colRegion'), render: (r) => r.region || '--' },
  { key: 'bucket', title: t('resource.clusterMgmt.backup.repo.colBucket'), render: (r) => r.bucket || '--' },
  {
    key: 'availability',
    title: t('resource.clusterMgmt.backup.repo.colAvailability'),
    render: (r) => {
      const b = availabilityBadge(r)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  {
    key: 'forcePathStyle',
    title: t('resource.clusterMgmt.backup.repo.colForcePath'),
    render: (r) => (r.forcePathStyle ? t('resource.clusterMgmt.common.yes') : t('resource.clusterMgmt.common.no')),
  },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: () => {
      const actions: HNBTableAction[] = [{ label: t('resource.clusterMgmt.backup.repo.action.edit'), key: 'edit' }]
      return h(HNBTableActions, { actions, disabled: !canUpdate.value, onAction: () => pluginCtx.notify(t('resource.clusterMgmt.placeholder.notEnabled')) })
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getBackupRepositories('', { keyword: keyword.value })
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
  formAccessUrl.value = ''
  formRegion.value = ''
  formBucket.value = ''
  formForcePathStyle.value = true
  drawerError.value = ''
  drawerVisible.value = true
}

async function onDrawerConfirm(): Promise<void> {
  if (!formName.value.trim() || !formBucket.value.trim()) {
    drawerError.value = t('resource.clusterMgmt.backup.repo.form.required')
    return
  }
  drawerBusy.value = true
  drawerError.value = ''
  try {
    await createBackupRepository('', {
      name: formName.value.trim(),
      cluster: 'graphify',
      accessUrl: formAccessUrl.value.trim() || 's3://backup.local',
      region: formRegion.value.trim() || 'cn-east-1',
      bucket: formBucket.value.trim(),
      availability: 'available',
      forcePathStyle: formForcePathStyle.value,
    })
    drawerVisible.value = false
    pluginCtx.notify(t('resource.clusterMgmt.backup.repo.created'))
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
  <section class="backup-tab-panel" :aria-label="t('resource.clusterMgmt.backup.tab.repositories')">
    <div class="panel-toolbar">
      <HNBButton v-if="canUpdate" @click="openCreate">
        {{ t('resource.clusterMgmt.backup.repo.action.create') }}
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
      min-width="1180px"
      :aria-label="t('resource.clusterMgmt.backup.tab.repositories')"
    />

    <ClusterDrawer
      v-model="drawerVisible"
      :title="t('resource.clusterMgmt.backup.repo.drawer.title')"
      :busy="drawerBusy"
      :error="drawerError"
      @confirm="onDrawerConfirm"
    >
      <form class="backup-form" @submit.prevent="onDrawerConfirm">
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.repo.form.name') }}</span>
          <input v-model="formName" type="text" :placeholder="t('resource.clusterMgmt.backup.repo.form.namePlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.repo.form.access') }}</span>
          <input v-model="formAccessUrl" type="text" placeholder="s3://backup.local" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.repo.form.region') }}</span>
          <input v-model="formRegion" type="text" placeholder="cn-east-1" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.backup.repo.form.bucket') }}</span>
          <input v-model="formBucket" type="text" :placeholder="t('resource.clusterMgmt.backup.repo.form.bucketPlaceholder')" />
        </label>
        <label class="form-field switch-field">
          <input v-model="formForcePathStyle" type="checkbox" />
          <span>{{ t('resource.clusterMgmt.backup.repo.form.forcePath') }}</span>
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
.switch-field { flex-direction: row; align-items: center; gap: 8px; }
.switch-field input { width: 16px; height: 16px; accent-color: var(--hnb-color-primary, #2f6fed); }
</style>
