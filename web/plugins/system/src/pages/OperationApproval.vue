<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBTable, HNBButton, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTablePagination, HNBSelectOption } from '@hnb/ui-kit'
import * as api from '../systemApi'
const { t } = useI18n()

const loading = ref(false)
const error = ref('')
const operations = ref<any[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const statusFilter = ref('pending_approval')
const actionDialog = ref(false)
const actionType = ref<'approve' | 'reject'>('approve')
const actionOperationId = ref('')
const actionReason = ref('')
const actionSubmitting = ref(false)
const actionError = ref('')

const statusOptions: HNBSelectOption[] = [
  { label: t('system.approvals.statusAll'), value: '' },
  { label: t('system.approvals.statusPending'), value: 'pending_approval' },
  { label: t('system.approvals.statusQueued'), value: 'queued' },
  { label: t('system.approvals.statusRunning'), value: 'in_progress' },
  { label: t('system.approvals.statusSucceeded'), value: 'succeeded' },
  { label: t('system.approvals.statusFailed'), value: 'failed' },
  { label: t('system.approvals.statusCancelled'), value: 'cancelled' },
]

const pagination = computed<HNBTablePagination>(() => ({
  page: page.value,
  pageSize: pageSize.value,
  total: total.value,
}))

const columns: HNBTableColumn[] = [
  { key: 'type', title: t('system.approvals.colType'), width: '120px', render: (row) => row.type || '-' },
  { key: 'target', title: t('system.approvals.colTarget'), render: (row) => `${row.targetKind || ''} / ${row.targetId || ''}` },
  {
    key: 'status', title: t('system.approvals.colStatus'), width: '140px',
    render: (row) => h(StatusBadge, { semantic: statusSemantic(row.status), label: statusLabel(row.status) }),
  },
  { key: 'createdAt', title: t('system.approvals.colRequested'), width: '170px', render: (row) => formatTime(row.createdAt) },
  {
    key: 'actions', title: t('system.approvals.colActions'), width: '180px',
    render: (row) => h('div', { style: 'display:flex;gap:6px' }, [
      allowedActions(row).includes('approve')
        ? h(HNBButton, { size: 'small', variant: 'primary', onClick: () => openActionDialog(row.operationId, 'approve') }, () => t('system.approvals.approve'))
        : null,
      allowedActions(row).includes('reject')
        ? h(HNBButton, { size: 'small', variant: 'danger', onClick: () => openActionDialog(row.operationId, 'reject') }, () => t('system.approvals.reject'))
        : null,
    ]),
  },
]

function statusSemantic(s: string): 'success' | 'warning' | 'error' | 'info' | 'processing' | 'default' {
  const map: Record<string, 'success' | 'warning' | 'error' | 'info' | 'processing' | 'default'> = {
    pending_approval: 'warning',
    pending: 'warning',
    queued: 'info',
    queued_offline: 'info',
    in_progress: 'processing',
    succeeded: 'success',
    failed: 'error',
    cancelled: 'default',
  }
  return map[s] || 'default'
}

function statusLabel(s: string): string {
  return t(`system.approvals.status_${s}`) || s
}

function formatTime(ts: string): string {
  if (!ts) return '-'
  try {
    const d = new Date(ts)
    return d.toLocaleString()
  } catch { return ts }
}

async function loadOperations() {
  loading.value = true
  error.value = ''
  try {
    const params: Record<string, string> = { page: String(page.value), pageSize: String(pageSize.value) }
    if (statusFilter.value) params.status = statusFilter.value
    const raw = await api.apiGet<any>('/api/v1/operations', { params })
    operations.value = raw?.items || []
    total.value = raw?.pagination?.total || 0
  } catch (e: any) {
    error.value = e?.message || t('system.approvals.loadError')
    operations.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) {
  page.value = p
  loadOperations()
}

function onPageSizeChange(size: number) {
  pageSize.value = size
  page.value = 1
  loadOperations()
}

function onStatusChange() {
  page.value = 1
  loadOperations()
}

function openActionDialog(opId: string, action: 'approve' | 'reject') {
  actionType.value = action
  actionOperationId.value = opId
  actionReason.value = ''
  actionError.value = ''
  actionSubmitting.value = false
  actionDialog.value = true
}

async function submitAction() {
  actionSubmitting.value = true
  actionError.value = ''
  try {
    const body = actionReason.value ? { reason: actionReason.value } : undefined
    if (actionType.value === 'approve') {
      await api.apiPost(`/api/v1/operations/${actionOperationId.value}/actions/approve`, body)
    } else {
      await api.apiPost(`/api/v1/operations/${actionOperationId.value}/actions/reject`, body)
    }
    actionDialog.value = false
    await loadOperations()
  } catch (e: any) {
    actionError.value = e?.message || t('system.approvals.actionError')
  } finally {
    actionSubmitting.value = false
  }
}

function allowedActions(row: any): string[] {
  return row.allowedActions || []
}

onMounted(loadOperations)
</script>
<template>
  <HNBPageShell :title="t('system.approvals.title')" :description="t('system.approvals.desc')">
    <div class="tcc__cards">
      <section class="tcc__card">
        <div class="tcc__card-title-row">
          <span class="tcc__card-accent" />
          <h2 class="tcc__card-title">{{ t('system.approvals.listTitle') }}</h2>
        </div>
        <div class="tcc__toolbar">
          <select v-model="statusFilter" class="tcc__select" @change="onStatusChange">
            <option v-for="opt in statusOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
        </div>
        <div class="tcc__table-wrap">
          <HNBTable
            :columns="columns"
            :data="operations"
            :loading="loading"
            :error="error"
            :pagination="pagination"
            empty-title="暂无待审批操作"
            @update:page="onPageChange"
            @update:page-size="onPageSizeChange"
            @error-retry="loadOperations"
          />
        </div>
      </section>
    </div>
  </HNBPageShell>

  <div v-if="actionDialog" class="modal-mask" @click.self="actionDialog = false">
    <div class="modal-card small">
      <div class="modal-header">
        <h2>{{ actionType === 'approve' ? t('system.approvals.approveTitle') : t('system.approvals.rejectTitle') }}</h2>
        <button class="icon-button" @click="actionDialog = false">×</button>
      </div>
      <div class="modal-body">
        <p class="tcc__dialog-desc">{{ actionType === 'approve' ? t('system.approvals.approveDesc') : t('system.approvals.rejectDesc') }}</p>
        <div class="tcc__dialog-field">
          <label class="tcc__dialog-label">{{ t('system.approvals.reason') }}</label>
          <textarea v-model="actionReason" class="tcc__textarea" :placeholder="t('system.approvals.reasonPlaceholder')" rows="3" />
        </div>
        <p v-if="actionError" class="tcc__dialog-error">{{ actionError }}</p>
      </div>
      <div class="modal-actions">
        <HNBButton :disabled="actionSubmitting" @click="actionDialog = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :loading="actionSubmitting" @click="submitAction">{{ actionType === 'approve' ? t('system.approvals.approve') : t('system.approvals.reject') }}</HNBButton>
      </div>
    </div>
  </div>
</template>
<style scoped>
.tcc__cards { display:flex;flex-direction:column;gap:20px; }
.tcc__card { background:var(--hnb-color-bg-surface);border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-lg);padding:20px; }
.tcc__card-title-row { display:flex;align-items:center;gap:10px;margin-bottom:16px; }
.tcc__card-accent { width:3px;height:18px;background:var(--hnb-color-primary);border-radius:2px;flex-shrink:0; }
.tcc__card-title { margin:0;font-size:15px;font-weight:600;color:var(--hnb-color-text-primary); }
.tcc__toolbar { display:flex;gap:12px;margin-bottom:16px; }
.tcc__select { height:34px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary);min-width:160px;box-sizing:border-box; }
.tcc__table-wrap { margin-bottom:16px; }
.modal-mask { position:fixed;inset:0;z-index:1000;display:flex;justify-content:flex-end;background:rgba(0,0,0,0.58); }
.modal-card { width:min(480px,100%);height:100%;display:flex;flex-direction:column;background:var(--hnb-color-bg-surface);border-left:1px solid var(--hnb-color-border);box-shadow:-24px 0 80px rgba(0,0,0,0.35); }
.modal-card.small { width:min(420px,100%); }
.modal-header { display:flex;justify-content:space-between;align-items:flex-start;gap:16px;padding:20px;border-bottom:1px solid var(--hnb-color-border); }
.modal-header h2 { margin:0;font-size:21px;color:var(--hnb-color-text-primary); }
.icon-button { background:none;border:none;font-size:24px;color:var(--hnb-color-text-tertiary);cursor:pointer;padding:0;line-height:1; }
.modal-body { flex:1;overflow-y:auto;padding:20px; }
.modal-actions { display:flex;justify-content:flex-end;gap:8px;padding:16px 20px;border-top:1px solid var(--hnb-color-border); }
.tcc__dialog-desc { margin:0 0 16px;font-size:13px;color:var(--hnb-color-text-tertiary); }
.tcc__dialog-field { margin-bottom:16px; }
.tcc__dialog-label { display:block;font-size:12px;font-weight:500;color:var(--hnb-color-text-secondary);margin-bottom:4px; }
.tcc__textarea { width:100%;padding:8px 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg);color:var(--hnb-color-text-primary);font-size:13px;box-sizing:border-box;resize:vertical; }
.tcc__dialog-error { margin:0;padding:8px 12px;border-radius:var(--hnb-radius-md);background:rgba(240,68,56,0.12);color:var(--hnb-color-status-danger, #f04438);font-size:13px; }
</style>
