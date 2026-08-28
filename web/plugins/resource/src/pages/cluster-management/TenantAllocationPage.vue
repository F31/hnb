<script setup lang="ts">
/**
 * TenantAllocationPage — 集群详情 > 租户分配。
 * 每个租户一行容器资源配额（CPU/内存/存储/虚拟GPU/虚拟显存/物理GPU，进度条）；
 * 横向滚动 + 固定操作列；分页；新增/更新（抽屉）/删除（确认弹窗）。
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { HNBButton, HNBConfirmation } from '@hnb/ui-kit'
import {
  deleteTenantAllocation,
  getTenantAllocations,
  updateTenantAllocation,
} from './api/p4Api'
import { getClusterPermissionStore } from './api/clusterApi'
import { usePluginContext } from './composables/usePluginContext'
import ClusterDetailLayout from './components/ClusterDetailLayout.vue'
import ClusterDrawer from './components/ClusterDrawer.vue'
import AllocationMetricCell from './components/AllocationMetricCell.vue'
import type { TenantAllocation } from './types/p4'

const { t } = useI18n()
const route = useRoute()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()
const clusterId = String(route.params.clusterId ?? '')

const allocations = ref<TenantAllocation[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 抽屉（新增/更新） ----
const drawerVisible = ref(false)
const drawerBusy = ref(false)
const drawerError = ref('')
const editingTenant = ref('')
const formTenantName = ref('')
const formCpuLimit = ref('')
const formMemLimit = ref('')
const formStorageLimit = ref('')

// ---- 删除确认 ----
const confirmDelete = ref(false)
const deletingTenant = ref('')
const deleteError = ref('')

const columnLabels = computed(() => [
  { key: 'cpu', label: t('resource.clusterMgmt.tenantAlloc.colCpu') },
  { key: 'memory', label: t('resource.clusterMgmt.tenantAlloc.colMemory') },
  { key: 'storage', label: t('resource.clusterMgmt.tenantAlloc.colStorage') },
  { key: 'virtualGpu', label: t('resource.clusterMgmt.tenantAlloc.colVirtualGpu') },
  { key: 'virtualVram', label: t('resource.clusterMgmt.tenantAlloc.colVirtualVram') },
  { key: 'physicalGpu', label: t('resource.clusterMgmt.tenantAlloc.colPhysicalGpu') },
])

async function load(): Promise<void> {
  if (!clusterId) return
  loading.value = true
  error.value = ''
  try {
    allocations.value = await getTenantAllocations(clusterId, { keyword: keyword.value })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    allocations.value = []
  } finally {
    loading.value = false
  }
}

function onSearch(): void {
  load()
}

function openCreate(): void {
  if (!canUpdate.value) return
  editingTenant.value = ''
  formTenantName.value = ''
  formCpuLimit.value = ''
  formMemLimit.value = ''
  formStorageLimit.value = ''
  drawerError.value = ''
  drawerVisible.value = true
}

function openUpdate(tenant: TenantAllocation): void {
  if (!canUpdate.value) return
  editingTenant.value = tenant.tenantName
  formTenantName.value = tenant.tenantName
  formCpuLimit.value = String(tenant.cpu.limit ?? '')
  formMemLimit.value = String(tenant.memory.limit ?? '')
  formStorageLimit.value = String(tenant.storage.limit ?? '')
  drawerError.value = ''
  drawerVisible.value = true
}

async function onDrawerConfirm(): Promise<void> {
  if (!formTenantName.value.trim()) {
    drawerError.value = t('resource.clusterMgmt.tenantAlloc.form.tenantRequired')
    return
  }
  drawerBusy.value = true
  drawerError.value = ''
  try {
    await updateTenantAllocation(clusterId, editingTenant.value || formTenantName.value.trim(), {
      tenantName: formTenantName.value.trim(),
      cpuLimit: Number(formCpuLimit.value) || 0,
      memoryLimit: Number(formMemLimit.value) || 0,
      storageLimit: Number(formStorageLimit.value) || 0,
    })
    drawerVisible.value = false
    pluginCtx.notify(t('resource.clusterMgmt.tenantAlloc.saved'))
    await load()
  } catch (err) {
    drawerError.value = err instanceof Error ? err.message : String(err)
  } finally {
    drawerBusy.value = false
  }
}

function requestDelete(tenant: TenantAllocation): void {
  if (!canUpdate.value) return
  deletingTenant.value = tenant.tenantName
  deleteError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  deleteError.value = ''
  try {
    await deleteTenantAllocation(clusterId, deletingTenant.value)
    confirmDelete.value = false
    pluginCtx.notify(t('resource.clusterMgmt.tenantAlloc.deleted'))
    await load()
  } catch (err) {
    deleteError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <ClusterDetailLayout>
    <div class="tenant-alloc-page">
      <div class="page-toolbar">
        <HNBButton v-if="canUpdate" @click="openCreate">
          {{ t('resource.clusterMgmt.tenantAlloc.action.create') }}
        </HNBButton>
        <div class="toolbar-right">
          <input
            v-model="keyword"
            class="keyword-input"
            type="text"
            :placeholder="t('resource.clusterMgmt.tenantAlloc.keywordPlaceholder')"
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
      <div v-else class="table-scroll">
        <table class="wide-table">
          <thead>
            <tr>
              <th class="col-tenant">{{ t('resource.clusterMgmt.tenantAlloc.colTenant') }}</th>
              <th v-for="col in columnLabels" :key="col.key">{{ col.label }}</th>
              <th class="col-actions">{{ t('resource.clusterMgmt.col.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="tenant in allocations" :key="tenant.tenantName">
              <td class="col-tenant">
                <span class="tenant-name">{{ tenant.tenantName }}</span>
              </td>
              <td><AllocationMetricCell :metric="tenant.cpu" /></td>
              <td><AllocationMetricCell :metric="tenant.memory" unit="GiB" /></td>
              <td><AllocationMetricCell :metric="tenant.storage" unit="GiB" /></td>
              <td><AllocationMetricCell :metric="tenant.virtualGpu" /></td>
              <td><AllocationMetricCell :metric="tenant.virtualVram" unit="MiB" /></td>
              <td><AllocationMetricCell :metric="tenant.physicalGpu" /></td>
              <td class="col-actions">
                <button class="text-action" type="button" :disabled="!canUpdate" @click="openUpdate(tenant)">
                  {{ t('resource.clusterMgmt.tenantAlloc.action.update') }}
                </button>
                <button class="text-action danger" type="button" :disabled="!canUpdate" @click="requestDelete(tenant)">
                  {{ t('resource.clusterMgmt.tenantAlloc.action.delete') }}
                </button>
              </td>
            </tr>
            <tr v-if="!allocations.length">
              <td class="empty-cell" :colspan="8">{{ t('resource.clusterMgmt.tenantAlloc.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <ClusterDrawer
      v-model="drawerVisible"
      :title="editingTenant ? t('resource.clusterMgmt.tenantAlloc.drawer.updateTitle') : t('resource.clusterMgmt.tenantAlloc.drawer.createTitle')"
      :busy="drawerBusy"
      :error="drawerError"
      @confirm="onDrawerConfirm"
    >
      <form class="alloc-form" @submit.prevent="onDrawerConfirm">
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.tenantAlloc.form.tenantName') }}</span>
          <input v-model="formTenantName" type="text" :placeholder="t('resource.clusterMgmt.tenantAlloc.form.tenantPlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.tenantAlloc.form.cpuLimit') }}</span>
          <input v-model="formCpuLimit" type="number" min="0" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.tenantAlloc.form.memLimit') }}</span>
          <input v-model="formMemLimit" type="number" min="0" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.tenantAlloc.form.storageLimit') }}</span>
          <input v-model="formStorageLimit" type="number" min="0" />
        </label>
      </form>
    </ClusterDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('resource.clusterMgmt.tenantAlloc.deleteTitle')"
      :description="t('resource.clusterMgmt.tenantAlloc.deleteMessage', { name: deletingTenant })"
      :error="deleteError"
      danger
      @confirm="onConfirmDelete"
    />
  </ClusterDetailLayout>
</template>

<style scoped>
.tenant-alloc-page { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
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
.table-scroll { overflow-x: auto; max-width: 100%; scrollbar-width: thin; scrollbar-color: var(--hnb-color-text-tertiary, #8a94a3) transparent; }
.table-scroll::-webkit-scrollbar { height: 6px; }
.table-scroll::-webkit-scrollbar-thumb { background: var(--hnb-color-text-tertiary, #8a94a3); border-radius: 3px; }
.table-scroll::-webkit-scrollbar-track { background: transparent; }
.wide-table { border-collapse: collapse; width: 100%; min-width: 1080px; }
.wide-table th {
  padding: 10px 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--hnb-color-text-secondary, #5b6675);
  text-align: left;
  border-bottom: 1px solid var(--hnb-color-divider, #e2e7ef);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  white-space: nowrap;
}
.wide-table td { padding: 10px 12px; font-size: 13px; border-bottom: 1px solid var(--hnb-color-divider, #e2e7ef); }
.wide-table .col-tenant { min-width: 160px; background: var(--hnb-color-bg-elevated, #f6f8fb); }
.tenant-name { font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.col-actions { position: sticky; right: 0; background: var(--hnb-color-bg-surface, #fff); white-space: nowrap; }
.wide-table th.col-actions { background: var(--hnb-color-bg-elevated, #f6f8fb); }
.text-action {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 13px;
  padding: 2px 6px;
}
.text-action.danger { color: var(--hnb-color-status-danger, #f04438); }
.text-action:disabled { opacity: 0.5; cursor: not-allowed; }
.empty-cell { text-align: center; color: var(--hnb-color-text-tertiary, #8a94a3); padding: 32px 0; }
.alloc-form { display: flex; flex-direction: column; gap: 14px; }
.form-field { display: flex; flex-direction: column; gap: 6px; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.form-field input {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
</style>
