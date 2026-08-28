<template>
  <HNBPageShell :title="t('container.namespaces.title')" :description="t('container.namespaces.desc')">
    <div class="ns__toolbar">
      <HNBSelectInput
        v-model="clusterFilter"
        :options="clusterOptions"
        :placeholder="t('container.namespaces.clusterAll')"
        class="ns__filter"
        @update:model-value="loadNamespaces"
      />
      <HNBButton variant="ghost" size="small" @click="loadAll">{{ t('container.namespaces.refresh') }}</HNBButton>
      <HNBButton variant="primary" size="small" @click="openCreate">{{ t('container.namespaces.create') }}</HNBButton>
    </div>

    <HNBTable
      :columns="columns"
      :data="namespaces"
      :loading="loading"
      :error="error"
      :pagination="pagination"
      :empty-title="t('container.namespaces.empty')"
      row-key="id"
      @error-retry="loadAll"
    />
  </HNBPageShell>

  <NetworkDrawer
    v-model="showDialog"
    :title="isEditing ? t('container.namespaces.editTitle') : t('container.namespaces.createTitle')"
    :busy="saving"
    :error="dialogError"
    @cancel="resetDialog"
    @confirm="submitForm"
  >
    <form class="ns__form" @submit.prevent="submitForm">
      <HNBFormField :label="t('container.namespaces.nameLabel')" required input-id="ns-name">
        <input
          id="ns-name"
          v-model="form.name"
          class="ns__input"
          :placeholder="t('container.namespaces.namePlaceholder')"
          :disabled="isEditing"
        />
      </HNBFormField>
      <HNBFormField :label="t('container.namespaces.descLabel')" input-id="ns-desc">
        <input
          id="ns-desc"
          v-model="form.description"
          class="ns__input"
          :placeholder="t('container.namespaces.descPlaceholder')"
        />
      </HNBFormField>
      <HNBFormField :label="t('container.namespaces.clusterLabel')" input-id="ns-cluster">
        <HNBSelectInput
          id="ns-cluster"
          v-model="form.cluster_id"
          :options="clusterOptions"
          :placeholder="t('container.namespaces.clusterPlaceholder')"
        />
      </HNBFormField>

      <div class="ns__quota-section">
        <h4 class="ns__quota-title">{{ t('container.namespaces.quotaTitle') }}</h4>
        <div class="ns__quota-grid">
          <QuotaField
            :label="t('container.namespaces.cpuLabel')"
            unit="Core"
            v-model="form.quota.cpu"
            :remaining="remainingQuota.cpu"
          />
          <QuotaField
            :label="t('container.namespaces.memoryLabel')"
            unit="Gi"
            v-model="form.quota.memory"
            :remaining="remainingQuota.memory"
          />
          <QuotaField
            :label="t('container.namespaces.storageLabel')"
            unit="Gi"
            v-model="form.quota.storage"
            :remaining="remainingQuota.storage"
          />
          <QuotaField
            :label="t('container.namespaces.vgpuLabel')"
            unit="%"
            v-model="form.quota.vgpu"
            :remaining="remainingQuota.vgpu"
          />
          <QuotaField
            :label="t('container.namespaces.vramLabel')"
            unit="MB"
            v-model="form.quota.vram"
            :remaining="remainingQuota.vram"
          />
          <QuotaField
            :label="t('container.namespaces.gpuLabel')"
            unit="块"
            v-model="form.quota.gpu"
            :remaining="remainingQuota.gpu"
          />
        </div>
      </div>

      <div v-if="isEditing" class="ns__members-section">
        <h4 class="ns__quota-title">{{ t('container.namespaces.membersTitle') }}</h4>
        <div class="ns__member-list">
          <div v-for="m in members" :key="m.subject_id" class="ns__member-row">
            <span class="ns__member-name">{{ m.display_name || m.username }}</span>
            <span class="ns__member-email">{{ m.email }}</span>
            <HNBButton size="small" variant="danger" @click="removeMember(m.subject_id)">{{ t('container.namespaces.remove') }}</HNBButton>
          </div>
          <div v-if="members.length === 0" class="ns__member-empty">{{ t('container.namespaces.noMembers') }}</div>
        </div>
        <div class="ns__member-add">
          <HNBSelectInput v-model="newMemberSubjectId" :options="availableUsersOptions" :placeholder="t('container.namespaces.addMemberPlaceholder')" class="ns__member-select" />
          <HNBButton size="small" variant="primary" :disabled="!newMemberSubjectId" @click="addMember">{{ t('container.namespaces.addMember') }}</HNBButton>
        </div>
      </div>
    </form>
  </NetworkDrawer>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  HNBPageShell,
  HNBTable,
  HNBButton,
  HNBSelectInput,
  HNBFormField,
  StatusBadge,
  type StatusSemantic,
  type HNBTableColumn,
  type HNBTablePagination,
  type HNBSelectOption,
} from '@hnb/ui-kit'
import {
  getContainerContextStore,
  listWorkspaceClusters,
  listNamespaces,
  getNamespace,
  createNamespace,
  updateNamespace,
  deleteNamespace,
  getNamespaceQuotaRemaining,
  listNamespaceMembers,
  addNamespaceMember,
  removeNamespaceMember,
  listTenantUsers,
  type ContainerNamespace,
  type ContainerNamespaceQuota,
  type ContainerCluster,
  type NamespaceMember,
  type TenantUser,
} from '../../api/containerApi'
import NetworkDrawer from '../network/NetworkDrawer.vue'

const { t } = useI18n()
const contextStore = getContainerContextStore()

const QuotaField = defineComponent({
  props: { label: String, unit: String, modelValue: Number, remaining: { type: Number, default: undefined } },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => {
      const label = props.label || ''
      const unit = props.unit || ''
      const remaining = props.remaining
      const remainingText = remaining != null ? `(剩余: ${remaining}${unit})` : ''
      return h('div', { class: 'ns__quota-field' }, [
        h('label', { class: 'ns__quota-field-label' }, `${label} (${unit})`),
        h('div', { class: 'ns__quota-field-row' }, [
          h('input', {
            class: 'ns__input',
            type: 'number',
            min: 0,
            placeholder: '0',
            value: props.modelValue ?? '',
            onInput: (e: any) => emit('update:modelValue', e.target.value ? Number(e.target.value) : undefined),
          }),
          remainingText ? h('span', { class: 'ns__quota-remaining' }, remainingText) : null,
        ]),
      ])
    }
  },
})

const namespaces = ref<ContainerNamespace[]>([])
const clusters = ref<ContainerCluster[]>([])
const clusterFilter = ref('')
const loading = ref(false)
const error = ref('')

const showDialog = ref(false)
const isEditing = ref(false)
const editingId = ref('')
const saving = ref(false)
const dialogError = ref('')
const form = ref<{
  name: string
  description: string
  cluster_id: string
  quota: ContainerNamespaceQuota
}>({
  name: '',
  description: '',
  cluster_id: '',
  quota: {},
})
const remainingQuota = ref<ContainerNamespaceQuota>({})
const members = ref<NamespaceMember[]>([])
const tenantUsers = ref<TenantUser[]>([])
const newMemberSubjectId = ref('')

const availableUsersOptions = computed<HNBSelectOption[]>(() => {
  const existing = new Set(members.value.map((m) => m.subject_id))
  return tenantUsers.value
    .filter((u) => !existing.has(u.subject_id))
    .map((u) => ({ label: (u.display_name || u.username) + (u.email ? ` (${u.email})` : ''), value: u.subject_id }))
})

const clusterOptions = computed<HNBSelectOption[]>(() => [
  { label: t('container.namespaces.clusterAll'), value: '' },
  ...clusters.value.map((c) => ({
    label: (c.display_name || c.name) + (c.shared ? ' (shared)' : ''),
    value: c.id,
  })),
])

const clusterName = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {}
  for (const c of clusters.value) map[c.id] = (c.display_name || c.name) + (c.shared ? ' (shared)' : '')
  return map
})

function statusSemantic(status: string): StatusSemantic {
  if (status === 'active') return 'success'
  if (status === 'suspended') return 'warning'
  return 'default'
}

function statusLabel(status: string): string {
  if (status === 'active') return t('container.namespaces.active')
  if (status === 'suspended') return t('container.namespaces.suspended')
  if (status === 'deleted') return t('container.namespaces.deleted')
  return status
}

function fmt(v: number | undefined | null, suffix = ''): string {
  if (v == null || v === 0) return '-'
  return `${v}${suffix}`
}

const columns = computed<HNBTableColumn<ContainerNamespace>[]>(() => [
  { key: 'name', title: t('container.namespaces.colName'), width: '140px' },
  {
    key: 'status', title: t('container.namespaces.colStatus'), width: '100px',
    render: (row) => h(StatusBadge, { semantic: statusSemantic(row.status), label: statusLabel(row.status) }),
  },
  {
    key: 'cluster_id', title: t('container.namespaces.colCluster'), width: '140px',
    render: (row) => (row.cluster_id ? clusterName.value[row.cluster_id] || '-' : '-'),
  },
  {
    key: 'cpu', title: t('container.namespaces.colCPU'), width: '80px',
    render: (row) => fmt(row.quota?.cpu),
  },
  {
    key: 'memory', title: t('container.namespaces.colMemory'), width: '80px',
    render: (row) => fmt(row.quota?.memory, 'Gi'),
  },
  {
    key: 'storage', title: t('container.namespaces.colStorage'), width: '80px',
    render: (row) => fmt(row.quota?.storage, 'Gi'),
  },
  {
    key: 'vgpu', title: t('container.namespaces.colVGPU'), width: '80px',
    render: (row) => fmt(row.quota?.vgpu, '%'),
  },
  {
    key: 'vram', title: t('container.namespaces.colVRAM'), width: '80px',
    render: (row) => fmt(row.quota?.vram, 'MB'),
  },
  {
    key: 'gpu', title: t('container.namespaces.colGPU'), width: '80px',
    render: (row) => fmt(row.quota?.gpu, '块'),
  },
  {
    key: 'created_at', title: t('container.namespaces.colCreated'), width: '160px',
    render: (row) => new Date(row.created_at).toLocaleString(),
  },
  {
    key: 'actions', title: t('container.namespaces.colActions'), width: '120px',
    render: (row) => h('div', { style: 'display:flex;gap:8px;' }, [
      h(HNBButton, { size: 'small', variant: 'secondary', onClick: () => openEdit(row) }, () => t('container.namespaces.edit')),
      h(HNBButton, { size: 'small', variant: 'danger', onClick: () => confirmDelete(row) }, () => t('container.namespaces.delete')),
    ]),
  },
])

const pagination = computed<HNBTablePagination | undefined>(() => {
  if (namespaces.value.length === 0) return undefined
  return { page: 1, pageSize: 20, total: namespaces.value.length }
})

async function loadClusters(): Promise<void> {
  try {
    clusters.value = await listWorkspaceClusters()
  } catch { /* ignore */ }
}

async function loadNamespaces(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    namespaces.value = await listNamespaces({ clusterId: clusterFilter.value || undefined })
  } catch (e: any) {
    error.value = e?.message || t('container.namespaces.loadError')
    namespaces.value = []
  } finally {
    loading.value = false
  }
}

async function loadAll(): Promise<void> {
  await Promise.all([loadClusters(), loadNamespaces()])
}

async function fetchQuotaRemaining(): Promise<void> {
  try {
    remainingQuota.value = await getNamespaceQuotaRemaining()
  } catch { remainingQuota.value = {} }
}

async function loadMembers(nsId: string): Promise<void> {
  try {
    members.value = await listNamespaceMembers(nsId)
  } catch { members.value = [] }
}

async function loadTenantUsers(): Promise<void> {
  try {
    tenantUsers.value = await listTenantUsers()
  } catch { tenantUsers.value = [] }
}

async function addMember(): Promise<void> {
  if (!newMemberSubjectId.value || !editingId.value) return
  try {
    await addNamespaceMember(editingId.value, newMemberSubjectId.value)
    newMemberSubjectId.value = ''
    await loadMembers(editingId.value)
  } catch (e: any) {
    dialogError.value = e?.message || t('container.namespaces.addMemberError')
  }
}

async function removeMember(subjectId: string): Promise<void> {
  if (!editingId.value) return
  try {
    await removeNamespaceMember(editingId.value, subjectId)
    await loadMembers(editingId.value)
  } catch (e: any) {
    dialogError.value = e?.message || t('container.namespaces.removeMemberError')
  }
}

function openCreate(): void {
  isEditing.value = false
  editingId.value = ''
  form.value = { name: '', description: '', cluster_id: '', quota: {} }
  dialogError.value = ''
  showDialog.value = true
  fetchQuotaRemaining()
}

async function openEdit(ns: ContainerNamespace): Promise<void> {
  isEditing.value = true
  editingId.value = ns.id
  try {
    const detail = await getNamespace(ns.id)
    form.value = {
      name: detail.name,
      description: detail.description || '',
      cluster_id: detail.cluster_id || '',
      quota: { ...(detail.quota || {}) },
    }
  } catch {
    form.value = {
      name: ns.name,
      description: ns.description || '',
      cluster_id: ns.cluster_id || '',
      quota: { ...(ns.quota || {}) },
    }
  }
  dialogError.value = ''
  showDialog.value = true
  newMemberSubjectId.value = ''
  fetchQuotaRemaining()
  loadMembers(ns.id)
  loadTenantUsers()
}

function resetDialog(): void {
  showDialog.value = false
  form.value = { name: '', description: '', cluster_id: '', quota: {} }
  dialogError.value = ''
}

async function submitForm(): Promise<void> {
  if (!isEditing.value) {
    const name = form.value.name.trim()
    if (!name) {
      dialogError.value = t('container.namespaces.nameRequired')
      return
    }
  }
  saving.value = true
  dialogError.value = ''
  try {
    if (isEditing.value) {
      await updateNamespace(editingId.value, {
        description: form.value.description.trim() || undefined,
        cluster_id: form.value.cluster_id || undefined,
        quota: cleanQuota(form.value.quota),
      })
    } else {
      await createNamespace({
        name: form.value.name.trim(),
        description: form.value.description.trim() || undefined,
        cluster_id: form.value.cluster_id || undefined,
        quota: cleanQuota(form.value.quota),
      })
    }
    showDialog.value = false
    resetDialog()
    await loadNamespaces()
  } catch (e: any) {
    dialogError.value = e?.message || t('container.namespaces.saveError')
  } finally {
    saving.value = false
  }
}

function cleanQuota(q: ContainerNamespaceQuota): ContainerNamespaceQuota | undefined {
  const result: ContainerNamespaceQuota = {}
  if (q.cpu != null && q.cpu > 0) result.cpu = q.cpu
  if (q.memory != null && q.memory > 0) result.memory = q.memory
  if (q.storage != null && q.storage > 0) result.storage = q.storage
  if (q.vgpu != null && q.vgpu > 0) result.vgpu = q.vgpu
  if (q.vram != null && q.vram > 0) result.vram = q.vram
  if (q.gpu != null && q.gpu > 0) result.gpu = q.gpu
  if (Object.keys(result).length === 0) return undefined
  return result
}

async function confirmDelete(ns: ContainerNamespace): Promise<void> {
  if (!window.confirm(t('container.namespaces.deleteConfirm', { name: ns.name }))) return
  try {
    await deleteNamespace(ns.id)
    await loadNamespaces()
  } catch (e: any) {
    error.value = e?.message || t('container.namespaces.deleteError')
  }
}

watch(
  () => contextStore.current.spaceId,
  () => {
    clusterFilter.value = ''
    loadAll()
  },
)

onMounted(loadAll)
</script>

<style scoped>
.ns__toolbar {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-sm);
  margin-bottom: var(--hnb-space-md);
}
.ns__filter {
  width: 220px;
}
.ns__form {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-md);
  color: var(--hnb-color-text-primary);
}
.ns__form :deep(.ns__input) {
  width: 100%;
  min-height: 36px;
  padding: 8px 10px;
  font-size: var(--hnb-font-size-body);
  line-height: 1.4;
  color: var(--hnb-color-text-primary);
  background: var(--hnb-color-bg-elevated);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-sm);
  box-sizing: border-box;
  -webkit-appearance: none;
  appearance: none;
  box-shadow: none;
  transition: border-color var(--hnb-duration-fast), box-shadow var(--hnb-duration-fast);
}
.ns__form :deep(.ns__input:focus) {
  outline: none;
  border-color: var(--hnb-color-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--hnb-color-primary) 24%, transparent);
}
.ns__form :deep(.ns__input:disabled) {
  color: var(--hnb-color-text-tertiary);
  background: color-mix(in srgb, var(--hnb-color-bg-elevated) 72%, var(--hnb-color-bg-surface));
  opacity: 1;
  cursor: not-allowed;
}
.ns__form :deep(.ns__input::placeholder) {
  color: var(--hnb-color-text-tertiary);
}
.ns__form :deep(.hnb-form-field__label),
.ns__form :deep(.ns__quota-field-label) {
  color: var(--hnb-color-text-secondary);
  font-weight: var(--hnb-font-weight-semibold);
}
.ns__form :deep(.hnb-select-input) {
  width: 100%;
  min-width: 0;
  height: 36px;
  padding: 0 10px;
  color: var(--hnb-color-text-primary);
  background: var(--hnb-color-bg-elevated);
  border-color: var(--hnb-color-border);
  border-radius: var(--hnb-radius-sm);
  box-shadow: none;
}
.ns__form :deep(.hnb-select-input:focus) {
  outline: none;
  border-color: var(--hnb-color-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--hnb-color-primary) 24%, transparent);
}
.ns__quota-section {
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  padding: var(--hnb-space-md);
}
.ns__quota-title {
  margin: 0 0 var(--hnb-space-sm);
  font-size: var(--hnb-font-size-caption);
  font-weight: var(--hnb-font-weight-semibold);
  color: var(--hnb-color-text-primary);
}
.ns__quota-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--hnb-space-sm);
}
.ns__quota-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ns__quota-field-label {
  font-size: var(--hnb-font-size-caption);
}
.ns__quota-field-row {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-xs);
}
.ns__quota-remaining {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-tertiary);
  white-space: nowrap;
}
.ns__members-section {
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  padding: var(--hnb-space-md);
}
.ns__member-list {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs);
  margin-bottom: var(--hnb-space-sm);
}
.ns__member-row {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-sm);
  padding: var(--hnb-space-xs) 0;
}
.ns__member-name {
  flex: 1;
  font-size: var(--hnb-font-size-body);
  color: var(--hnb-color-text-primary);
}
.ns__member-email {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-tertiary);
}
.ns__member-empty {
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-tertiary);
  padding: var(--hnb-space-xs) 0;
}
.ns__member-add {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-sm);
}
.ns__member-select {
  flex: 1;
}
</style>
