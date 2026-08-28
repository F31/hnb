<script setup lang="ts">
/**
 * SubnetRequestsTab — 网段申请审批。
 * 列表：申请编号/申请命名空间/申请CIDR/状态/申请时间/操作；
 * 待审批：审批通过（自动创建子网并绑定）/ 拒绝。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBTableActions, HNBConfirmation } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { approveSubnetRequest, getSubnetRequests, rejectSubnetRequest } from '../api/networkApi'
import { getClusterPermissionStore } from '../api/clusterApi'
import { usePluginContext } from '../composables/usePluginContext'
import type { SubnetRequest } from '../types/network'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()

const items = ref<SubnetRequest[]>([])
const loading = ref(true)
const error = ref('')

const canApprove = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 拒绝确认 ----
const confirmReject = ref(false)
const rejectTarget = ref<SubnetRequest | null>(null)
const actionError = ref('')

function statusBadge(r: SubnetRequest): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = { pending: 'default', approved: 'success', rejected: 'error' }
  return { label: t(`resource.clusterMgmt.network.request.status.${r.status}`), semantic: map[r.status] ?? 'default' }
}

const columns = computed<HNBTableColumn<SubnetRequest>[]>(() => [
  { key: 'id', title: t('resource.clusterMgmt.network.request.colId'), render: (r) => r.id || '--' },
  { key: 'namespace', title: t('resource.clusterMgmt.network.request.colNamespace'), render: (r) => r.namespace || '--' },
  { key: 'requestedCidr', title: t('resource.clusterMgmt.network.request.colCidr'), render: (r) => r.requestedCidr || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.network.request.colStatus'),
    render: (r) => {
      const b = statusBadge(r)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'requestedAt', title: t('resource.clusterMgmt.network.request.colTime'), render: (r) => r.requestedAt || '--' },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (r) => {
      if (r.status !== 'pending') {
        return h('span', { class: 'empty-action' }, '--')
      }
      const actions: HNBTableAction[] = [
        { label: t('resource.clusterMgmt.network.request.action.approve'), key: 'approve' },
        { label: t('resource.clusterMgmt.network.request.action.reject'), key: 'reject', variant: 'danger' },
      ]
      return h(HNBTableActions, { actions, disabled: !canApprove.value, onAction: (key: string) => {
        if (key === 'approve') onApprove(r)
        else if (key === 'reject') requestReject(r)
      }})
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getSubnetRequests()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

async function onApprove(r: SubnetRequest): Promise<void> {
  actionError.value = ''
  try {
    await approveSubnetRequest(r)
    pluginCtx.notify(t('resource.clusterMgmt.network.request.approved', { id: r.id }))
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

function requestReject(r: SubnetRequest): void {
  if (!canApprove.value) return
  rejectTarget.value = r
  actionError.value = ''
  confirmReject.value = true
}

async function onConfirmReject(): Promise<void> {
  const target = rejectTarget.value
  if (!target) return
  actionError.value = ''
  try {
    await rejectSubnetRequest(target)
    confirmReject.value = false
    pluginCtx.notify(t('resource.clusterMgmt.network.request.rejected', { id: target.id }))
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('resource.clusterMgmt.network.tab.requests')">
    <div class="panel-toolbar">
      <button class="secondary-button" type="button" @click="load">
        {{ t('resource.clusterMgmt.action.refresh') }}
      </button>
    </div>

    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>
    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('resource.clusterMgmt.network.empty')"
      min-width="900px"
      :aria-label="t('resource.clusterMgmt.network.tab.requests')"
    />

    <HNBConfirmation
      v-model="confirmReject"
      :title="t('resource.clusterMgmt.network.request.rejectTitle')"
      :description="t('resource.clusterMgmt.network.request.rejectMessage', { id: rejectTarget?.id ?? '' })"
      :error="actionError"
      danger
      @confirm="onConfirmReject"
    />
  </section>
</template>

<style scoped>
.network-tab-panel { display: flex; flex-direction: column; gap: 10px; }
.panel-toolbar { display: flex; justify-content: flex-end; }
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
.empty-action { color: var(--hnb-color-text-tertiary, #8a94a3); }
</style>
