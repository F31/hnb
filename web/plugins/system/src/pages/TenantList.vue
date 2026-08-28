<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBToolbar, HNBTable, HNBButton, HNBFormField, HNBSelectInput, HNBPageState, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import * as api from '../systemApi'
import { apiGet } from '../systemApi'

const { t } = useI18n()

const tenants = ref<api.TenantRecord[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
const search = ref('')
const statusFilter = ref('all')
const page = ref(1)
const pageSize = ref(10)
const showForm = ref(false)
const editing = ref(false)
const form = ref({ id: '', name: '', display_name: '', status: 'active' })
const showCreateForm = ref(false)
const showWsView = ref(false)
const wsSearch = ref('')
const allWorkspaces = ref<api.WorkspaceRecord[]>([])
const filteredWorkspaces = computed(() => {
  let list = allWorkspaces.value
  if (wsSearch.value) {
    const q = wsSearch.value.toLowerCase()
    list = list.filter((w) => w.name.toLowerCase().includes(q) || (w.display_name || '').toLowerCase().includes(q))
  }
  return list
})
const wsColumns: HNBTableColumn[] = [
  { key: 'name', title: t('system.tenants.wsName'), render: (row) => h('strong', row.name) },
  { key: 'display_name', title: t('system.tenants.wsDisplayName'), render: (row) => row.display_name || '-' },
  { key: 'tenant_id', title: '租户' },
  { key: 'status', title: t('system.tenants.wsStatus'), render: (row) => h(StatusBadge, { semantic: row.is_active ? 'success' : 'error', label: row.is_active ? t('system.tenants.active') : t('system.tenants.disabled') }) },
  { key: 'created_at', title: t('system.tenants.wsCreatedAt'), render: (row) => new Date(row.created_at).toLocaleDateString() },
]

async function loadWorkspaces() {
  wsLoading.value = true
  wsError.value = ''
  try {
    allWorkspaces.value = await api.listWorkspaces()
  } catch (e: any) {
    wsError.value = e?.message || '加载工作空间失败'
  } finally {
    wsLoading.value = false
  }
}
const createForm = ref({
  name: '', code: '',
  quota: { cpu: 0, memory: 0, storage: 0, vgpu: 0, vram: 0, gpu: 0 },
})
const createErrors = ref<Record<string, string>>({})
const saving = ref(false)
const showDetail = ref(false)
const detailTenant = ref<api.TenantRecord | null>(null)
const workspaces = ref<api.WorkspaceRecord[]>([])
const wsLoading = ref(false)
const wsError = ref('')
const wsForm = ref({ name: '', display_name: '' })
const showWsForm = ref(false)
const expandedWs = ref<string | null>(null)
const wsQuotas = ref<Record<string, { cpu: number; memory: number; storage: number; vgpu: number; vram: number; gpu: number }>>({})
const wsQuotaSaving = ref<Record<string, boolean>>({})
const wsQuotaMsg = ref<Record<string, string>>({})
const tenantQuotaData = ref<api.Quota>({ cpu: 0, memory: 0, storage: 0, vgpu: 0, vram: 0, gpu: 0 })
const clusterBindDialog = ref(false)
const clusterList = ref<{ id: string; name: string }[]>([])
const selectedCluster = ref('')
const bindingWsId = ref('')
const wsClusters = ref<Record<string, api.ClusterRecord[]>>({})
const wsMembers = ref<Record<string, api.RoleBindingRecord[]>>({})
const allUsers = ref<api.UserRecord[]>([])
const allRoles = ref<api.RoleRecord[]>([])
const memberDialog = ref(false)
const memberForm = ref({ user_id: '', role_id: '' })
const memberWsId = ref('')
const memberSaving = ref(false)

async function bindCluster(workspaceId: string) {
  bindingWsId.value = workspaceId
  try {
    const res = await apiGet<any[]>('/api/v1/clusters')
    clusterList.value = (res || []).map((c: any) => ({ id: c.id || c.name, name: c.display_name || c.name }))
    selectedCluster.value = ''
    clusterBindDialog.value = true
  } catch {
    clusterList.value = []
    clusterBindDialog.value = true
  }
}

async function confirmBindCluster() {
  if (!bindingWsId.value || !selectedCluster.value) return
  try {
    await api.bindWorkspaceCluster(bindingWsId.value, selectedCluster.value)
    clusterBindDialog.value = false
    await loadWsClusters(bindingWsId.value)
  } catch (e: any) {
    console.error('bind cluster failed', e)
  }
}

async function loadWsClusters(workspaceId: string) {
  try {
    wsClusters.value[workspaceId] = await api.listWorkspaceClusters(workspaceId)
  } catch {
    wsClusters.value[workspaceId] = []
  }
}

async function unbindWsCluster(workspaceId: string, clusterId: string) {
  try {
    await api.unbindWorkspaceCluster(workspaceId, clusterId)
    await loadWsClusters(workspaceId)
  } catch (e: any) {
    console.error('unbind cluster failed', e)
  }
}

async function loadWsMembers(workspaceId: string) {
  try {
    const all = await api.listRoleBindings()
    wsMembers.value[workspaceId] = (all.items || []).filter((b) => b.scope === 'workspace' && b.scope_id === workspaceId)
  } catch {
    wsMembers.value[workspaceId] = []
  }
}

function openAddMember(workspaceId: string) {
  memberWsId.value = workspaceId
  memberForm.value = { user_id: '', role_id: '' }
  memberDialog.value = true
}

async function confirmAddMember() {
  if (!memberForm.value.user_id || !memberForm.value.role_id) return
  memberSaving.value = true
  try {
    await api.bindRole({ user_id: memberForm.value.user_id, role_id: memberForm.value.role_id, scope: 'workspace', scope_id: memberWsId.value })
    memberDialog.value = false
    await loadWsMembers(memberWsId.value)
  } catch (e: any) {
    console.error('add member failed', e)
  } finally {
    memberSaving.value = false
  }
}

async function removeMember(userId: string, workspaceId: string) {
  try {
    await api.unbindRole(userId, 'workspace', workspaceId)
    await loadWsMembers(workspaceId)
  } catch (e: any) {
    console.error('remove member failed', e)
  }
}

function getUserName(userId: string): string {
  return allUsers.value.find((u) => u.id === userId)?.username || userId
}

function getRoleName(roleId: string): string {
  return allRoles.value.find((r) => r.id === roleId)?.display_name || allRoles.value.find((r) => r.id === roleId)?.name || roleId
}

async function loadTenants() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.listTenants(page.value, pageSize.value)
    tenants.value = res.items
    total.value = res.total
  } catch (e: any) {
    error.value = e?.message || t('system.tenants.loadError')
  } finally {
    loading.value = false
  }
}

const pagination = computed(() => ({ page: page.value, pageSize: pageSize.value, total: total.value }))

const filteredTenants = computed(() => {
  let list = tenants.value
  if (search.value) {
    const q = search.value.toLowerCase()
    list = list.filter((t) => t.name.toLowerCase().includes(q) || t.display_name.toLowerCase().includes(q))
  }
  if (statusFilter.value !== 'all') {
    list = list.filter((t) => t.status === statusFilter.value)
  }
  return list
})

function onPage(p: number) { page.value = p; loadTenants() }
function onPageSize(ps: number) { pageSize.value = ps; page.value = 1; loadTenants() }

const columns: HNBTableColumn[] = [
  { key: 'name', title: t('system.tenants.colName'), render: (row) => h('strong', row.name) },
  { key: 'display_name', title: t('system.tenants.colDisplayName') },
  {
    key: 'status', title: t('system.tenants.colStatus'),
    render: (row) => h(StatusBadge, { semantic: row.status === 'active' ? 'success' : 'error', label: row.status === 'active' ? t('system.tenants.active') : t('system.tenants.suspended') }),
  },
  { key: 'created_at', title: t('system.tenants.colCreatedAt'), render: (row) => new Date(row.created_at).toLocaleDateString() },
  {
    key: 'actions', title: t('system.tenants.colActions'),
    render: (row) => { const tnt = row as unknown as api.TenantRecord; return h('div', { style: 'display:flex;gap:8px;align-items:center' }, [
      h(HNBButton, { size: 'small', onClick: () => openDetail(tnt) }, () => t('system.tenants.manage')),
      h(HNBButton, { size: 'small', variant: 'ghost', onClick: () => openEdit(tnt) }, () => t('system.tenants.edit')),
      h(HNBButton, { size: 'small', variant: 'ghost', onClick: () => toggleStatus(tnt) }, () => tnt.status === 'active' ? t('system.tenants.suspend') : t('system.tenants.activate')),
      h(HNBButton, { size: 'small', variant: 'danger', onClick: () => confirmDelete(tnt) }, () => t('system.tenants.delete')),
    ]) },
  },
]

function openCreate() {
  createForm.value = {
    name: '', code: '',
    quota: { cpu: 0, memory: 0, storage: 0, vgpu: 0, vram: 0, gpu: 0 },
  }
  createErrors.value = {}
  showCreateForm.value = true
}

function openEdit(tenant: api.TenantRecord) {
  editing.value = true
  form.value = { id: tenant.id, name: tenant.name, display_name: tenant.display_name, status: tenant.status }
  showForm.value = true
}

async function saveTenant() {
  error.value = ''
  try {
    if (editing.value) {
      await api.updateTenant(form.value.id, { display_name: form.value.display_name, status: form.value.status })
    } else {
      await api.createTenant({ name: form.value.name, display_name: form.value.display_name || undefined })
    }
    showForm.value = false
    await loadTenants()
  } catch (e: any) {
    error.value = e?.message || t('system.tenants.saveError')
  }
}

async function saveCreateTenant() {
  const errs: Record<string, string> = {}
  if (!createForm.value.name.trim()) errs.name = t('system.tenants.nameRequired')
  if (!createForm.value.code.trim()) errs.code = t('system.tenants.codeRequired')
  else if (!/^[a-z0-9-]+$/.test(createForm.value.code)) errs.code = t('system.tenants.codeInvalid')
  createErrors.value = errs
  if (Object.keys(errs).length) return

  saving.value = true
  try {
    await api.createTenant({ name: createForm.value.code, display_name: createForm.value.name, quota: createForm.value.quota })
    showCreateForm.value = false
    await loadTenants()
  } catch (e: any) {
    createErrors.value = { submit: e?.message || t('system.tenants.saveError') }
  } finally {
    saving.value = false
  }
}

async function confirmDelete(tenant: api.TenantRecord) {
  if (tenants.value.length <= 1) {
    error.value = t('system.tenants.cannotDeleteLast')
    return
  }
  if (!confirm(t('system.tenants.confirmDelete', { name: tenant.name }))) return
  error.value = ''
  try {
    await api.deleteTenant(tenant.id)
    await loadTenants()
  } catch (e: any) {
    error.value = e?.message || t('system.tenants.deleteError')
  }
}

async function toggleStatus(tenant: api.TenantRecord) {
  error.value = ''
  try {
    const newStatus = tenant.status === 'active' ? 'suspended' : 'active'
    await api.updateTenant(tenant.id, { status: newStatus })
    await loadTenants()
  } catch (e: any) {
    error.value = e?.message || t('system.tenants.toggleError')
  }
}

async function openDetail(tenant: api.TenantRecord) {
  detailTenant.value = tenant
  workspaces.value = []
  wsError.value = ''
  showDetail.value = true
  wsLoading.value = true
  try {
    const [wsList, tenantQuota, userList, roleList] = await Promise.all([
      api.listTenantWorkspaces(tenant.id),
      api.getTenantQuota(tenant.id).catch(() => ({ cpu: 0, memory: 0, storage: 0, vgpu: 0, vram: 0, gpu: 0 })),
      api.listUsers().catch(() => ({ items: [], total: 0 })),
      api.listRoles().catch(() => []),
    ])
    allUsers.value = (userList as any)?.items || (userList as any) || []
    allRoles.value = roleList
    workspaces.value = wsList
    tenantQuotaData.value = tenantQuota
    const q: Record<string, any> = {}
    const c: Record<string, api.ClusterRecord[]> = {}
    const m: Record<string, api.RoleBindingRecord[]> = {}
    for (const w of wsList) {
      const wsQuota = await api.getWorkspaceQuota(w.id).catch(() => ({ cpu: 0, memory: 0, storage: 0, vgpu: 0, vram: 0, gpu: 0 }))
      q[w.id] = wsQuota
      c[w.id] = await api.listWorkspaceClusters(w.id).catch(() => [])
      m[w.id] = []
    }
    const allBindings = await api.listRoleBindings().catch(() => ({ items: [], total: 0 }))
    for (const b of (allBindings.items || [])) {
      if (b.scope === 'workspace' && b.scope_id && m[b.scope_id] !== undefined) {
        m[b.scope_id].push(b)
      }
    }
    wsQuotas.value = q
    wsClusters.value = c
    wsMembers.value = m
  } catch (e: any) {
    wsError.value = e?.message || t('system.tenants.workspaceLoadError')
  } finally {
    wsLoading.value = false
  }
}

async function createWorkspace() {
  if (!detailTenant.value) return
  wsError.value = ''
  try {
    await api.createTenantWorkspace(detailTenant.value.id, { name: wsForm.value.name, display_name: wsForm.value.display_name || undefined })
    wsForm.value = { name: '', display_name: '' }
    showWsForm.value = false
    workspaces.value = await api.listTenantWorkspaces(detailTenant.value.id)
  } catch (e: any) {
    wsError.value = e?.message || t('system.tenants.workspaceCreateError')
  }
}

async function saveWsQuota(workspaceId: string) {
  const quota = wsQuotas.value[workspaceId]
  if (!quota) return
  wsQuotaSaving.value[workspaceId] = true
  wsQuotaMsg.value[workspaceId] = ''
  try {
    await api.updateWorkspaceQuota(workspaceId, quota)
    wsQuotaMsg.value[workspaceId] = t('system.tenants.quotaSaved')
  } catch (e: any) {
    wsQuotaMsg.value[workspaceId] = e?.message || t('system.tenants.quotaSaveError')
  } finally {
    wsQuotaSaving.value[workspaceId] = false
  }
}

function clearWsQuotaMsg(workspaceId: string) {
  wsQuotaMsg.value[workspaceId] = ''
}

onMounted(loadTenants)
</script>

<template>
  <HNBPageShell :title="t('system.tenants.title')" :description="t('system.tenants.desc')">
    <template #actions>
      <HNBButton variant="ghost" size="medium" @click="loadWorkspaces(); showWsView = !showWsView">{{ t('system.tenants.workspaces') }}</HNBButton>
      <HNBButton variant="primary" size="medium" @click="openCreate">{{ t('system.tenants.create') }}</HNBButton>
    </template>
    <template v-if="!showWsView">
      <HNBToolbar>
        <input v-model="search" :placeholder="t('system.tenants.searchPlaceholder')" class="toolbar-input" />
        <HNBSelectInput v-model="statusFilter" :options="[
          { label: t('system.tenants.statusAll'), value: 'all' },
          { label: t('system.tenants.statusActive'), value: 'active' },
          { label: t('system.tenants.statusSuspended'), value: 'suspended' },
        ]" />
      </HNBToolbar>
      <HNBTable :columns="columns" :data="filteredTenants" :loading="loading" :pagination="pagination" row-key="id" :error="error" :empty-title="t('system.tenants.empty')" @update:page="onPage" @update:page-size="onPageSize" @error-retry="loadTenants" />
    </template>
    <template v-else>
      <HNBToolbar>
        <input v-model="wsSearch" :placeholder="t('system.tenants.wsNamePlaceholder')" class="toolbar-input" />
      </HNBToolbar>
      <HNBTable :columns="wsColumns" :data="filteredWorkspaces" :loading="wsLoading" row-key="id" :error="wsError" :empty-title="t('system.tenants.noWorkspaces')" @error-retry="loadWorkspaces" />
    </template>
  </HNBPageShell>

  <div v-if="showForm" class="modal-mask" @click.self="showForm = false">
    <div class="modal-card">
      <div class="modal-header">
        <h2>{{ editing ? t('system.tenants.editTenant') : t('system.tenants.createTenant') }}</h2>
        <button class="icon-button" @click="showForm = false">×</button>
      </div>
      <div class="modal-body form-grid">
        <HNBFormField :label="t('system.tenants.name')" required>
          <input v-model="form.name" :readonly="editing" class="form-input" :placeholder="t('system.tenants.namePlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.tenants.displayName')">
          <input v-model="form.display_name" class="form-input" :placeholder="t('system.tenants.displayNamePlaceholder')" />
        </HNBFormField>
        <HNBFormField v-if="editing" :label="t('system.tenants.status')">
          <HNBSelectInput v-model="form.status" :options="[
            { label: t('system.tenants.active'), value: 'active' },
            { label: t('system.tenants.suspended'), value: 'suspended' },
          ]" />
        </HNBFormField>
      </div>
      <div class="modal-actions">
        <HNBButton @click="showForm = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :disabled="!form.name" @click="saveTenant">{{ t('system.common.confirm') }}</HNBButton>
      </div>
    </div>
  </div>

  <!-- 创建租户弹窗 -->
  <div v-if="showCreateForm" class="modal-mask" @click.self="showCreateForm = false">
    <div class="modal-card wide">
      <div class="modal-header">
        <h2>{{ t('system.tenants.createTenant') }}</h2>
        <button class="icon-button" @click="showCreateForm = false">×</button>
      </div>
      <div class="modal-body">
        <div class="tcc__cards">
          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.tenants.basicInfo') }}</h2>
            </div>
            <div class="tcc__form">
              <HNBFormField :label="t('system.tenants.name')" required input-id="tcc-name" :error="createErrors.name">
                <input id="tcc-name" v-model="createForm.name" class="tcc__input" :placeholder="t('system.tenants.namePlaceholder')" />
              </HNBFormField>
              <HNBFormField :label="t('system.tenants.code')" required input-id="tcc-code" :error="createErrors.code">
                <input id="tcc-code" v-model="createForm.code" class="tcc__input" :placeholder="t('system.tenants.codePlaceholder')" />
              </HNBFormField>
            </div>
          </section>
          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.tenants.quotaStats') }}</h2>
            </div>
            <div class="tcc__quota-form-grid">
              <div class="tcc__quota-field">
                <label class="tcc__quota-label"><span class="tcc__required">*</span>CPU</label>
                <div class="tcc__number-input-wrap">
                  <input v-model.number="createForm.quota.cpu" type="number" class="tcc__number-input" min="0" />
                  <span class="tcc__unit-badge">核</span>
                </div>
              </div>
              <div class="tcc__quota-field">
                <label class="tcc__quota-label"><span class="tcc__required">*</span>内存</label>
                <div class="tcc__number-input-wrap">
                  <input v-model.number="createForm.quota.memory" type="number" class="tcc__number-input" min="0" />
                  <span class="tcc__unit-badge">Gi</span>
                </div>
              </div>
              <div class="tcc__quota-field">
                <label class="tcc__quota-label"><span class="tcc__required">*</span>存储</label>
                <div class="tcc__number-input-wrap">
                  <input v-model.number="createForm.quota.storage" type="number" class="tcc__number-input" min="0" />
                  <span class="tcc__unit-badge">Gi</span>
                </div>
              </div>
              <div class="tcc__quota-field">
                <label class="tcc__quota-label"><span class="tcc__required">*</span>虚拟GPU</label>
                <div class="tcc__number-input-wrap">
                  <input v-model.number="createForm.quota.vgpu" type="number" class="tcc__number-input" min="0" />
                  <span class="tcc__unit-badge">%</span>
                </div>
              </div>
              <div class="tcc__quota-field">
                <label class="tcc__quota-label"><span class="tcc__required">*</span>虚拟显存</label>
                <div class="tcc__number-input-wrap">
                  <input v-model.number="createForm.quota.vram" type="number" class="tcc__number-input" min="0" />
                  <span class="tcc__unit-badge">MB</span>
                </div>
              </div>
              <div class="tcc__quota-field">
                <label class="tcc__quota-label"><span class="tcc__required">*</span>GPU</label>
                <div class="tcc__number-input-wrap">
                  <input v-model.number="createForm.quota.gpu" type="number" class="tcc__number-input" min="0" />
                  <span class="tcc__unit-badge">块</span>
                </div>
              </div>
            </div>
          </section>
        </div>
        <div v-if="createErrors.submit" class="tcc__submit-error">{{ createErrors.submit }}</div>
      </div>
      <div class="modal-actions">
        <HNBButton @click="showCreateForm = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :loading="saving" @click="saveCreateTenant">{{ t('system.common.confirm') }}</HNBButton>
      </div>
    </div>
  </div>

  <div v-if="showDetail && detailTenant" class="modal-mask" @click.self="showDetail = false">
    <div class="modal-card wide">
      <div class="modal-header">
        <h2>{{ detailTenant.display_name }} ({{ detailTenant.name }})</h2>
        <button class="icon-button" @click="showDetail = false">×</button>
      </div>
      <div class="modal-body">
        <div class="tcc__cards">
          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.tenants.detailInfo') }}</h2>
            </div>
            <div class="tcc__detail-grid">
              <div class="tcc__detail-row">
                <span class="tcc__detail-label">{{ t('system.tenants.detailStatus') }}</span>
                <StatusBadge :semantic="detailTenant.status === 'active' ? 'success' : 'error'" :label="detailTenant.status === 'active' ? t('system.tenants.active') : t('system.tenants.suspended')" />
              </div>
              <div class="tcc__detail-row">
                <span class="tcc__detail-label">{{ t('system.tenants.detailCode') }}</span>
                <span>{{ detailTenant.name }}</span>
              </div>
              <div class="tcc__detail-row">
                <span class="tcc__detail-label">{{ t('system.tenants.detailCreatedAt') }}</span>
                <span>{{ new Date(detailTenant.created_at).toLocaleString() }}</span>
              </div>
            </div>
          </section>

          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.tenants.quotaStats') }}</h2>
            </div>
            <div class="tcc__quota-stats">
              <div class="tcc__stat-item"><span class="tcc__required">*</span>CPU <span class="tcc__stat-value">{{ tenantQuotaData.cpu || 0 }}</span> <span class="tcc__stat-unit">核</span></div>
              <div class="tcc__stat-item"><span class="tcc__required">*</span>内存 <span class="tcc__stat-value">{{ tenantQuotaData.memory || 0 }}</span> <span class="tcc__stat-unit">Gi</span></div>
              <div class="tcc__stat-item"><span class="tcc__required">*</span>存储 <span class="tcc__stat-value">{{ tenantQuotaData.storage || 0 }}</span> <span class="tcc__stat-unit">Gi</span></div>
              <div class="tcc__stat-item"><span class="tcc__required">*</span>虚拟GPU <span class="tcc__stat-value">{{ tenantQuotaData.vgpu || 0 }}</span> <span class="tcc__stat-unit">%</span></div>
              <div class="tcc__stat-item"><span class="tcc__required">*</span>虚拟显存 <span class="tcc__stat-value">{{ tenantQuotaData.vram || 0 }}</span> <span class="tcc__stat-unit">MB</span></div>
              <div class="tcc__stat-item"><span class="tcc__required">*</span>GPU <span class="tcc__stat-value">{{ tenantQuotaData.gpu || 0 }}</span> <span class="tcc__stat-unit">块</span></div>
            </div>
          </section>

          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.tenants.workspaces') }} ({{ workspaces.length }})</h2>
              <HNBButton variant="primary" size="small" @click="showWsForm = !showWsForm">{{ t('system.tenants.createWorkspace') }}</HNBButton>
            </div>
            <div v-if="wsError" class="tcc__ws-error">{{ wsError }}</div>
            <div v-if="showWsForm" class="tcc__ws-form">
              <HNBFormField :label="t('system.tenants.wsNamePlaceholder')" required input-id="ws-name">
                <input id="ws-name" v-model="wsForm.name" class="tcc__input" :placeholder="t('system.tenants.wsNamePlaceholder')" />
              </HNBFormField>
              <HNBFormField :label="t('system.tenants.wsDisplayNamePlaceholder')" input-id="ws-dname">
                <input id="ws-dname" v-model="wsForm.display_name" class="tcc__input" :placeholder="t('system.tenants.wsDisplayNamePlaceholder')" />
              </HNBFormField>
              <HNBButton variant="primary" size="small" :disabled="!wsForm.name" @click="createWorkspace">{{ t('system.common.confirm') }}</HNBButton>
            </div>
            <div v-if="wsLoading" class="tcc__ws-empty">{{ t('system.tenants.loading') }}</div>
            <div v-else-if="!workspaces.length && !wsError" class="tcc__ws-empty">{{ t('system.tenants.noWorkspaces') }}</div>
            <div v-else class="tcc__ws-grid">
              <div v-for="ws in workspaces" :key="ws.id" class="tcc__ws-card" :class="{ 'tcc__ws-card--expanded': expandedWs === ws.id }" @click="expandedWs = expandedWs === ws.id ? null : ws.id">
                <div class="tcc__ws-card-header">
                  <strong class="tcc__ws-card-name">{{ ws.display_name || ws.name }}</strong>
                  <StatusBadge :semantic="ws.is_active ? 'success' : 'error'" :label="ws.is_active ? t('system.tenants.active') : t('system.tenants.disabled')" />
                </div>
                <div class="tcc__ws-card-body">
                  <div class="tcc__ws-card-row">
                    <span class="tcc__ws-card-label">{{ t('system.tenants.wsDisplayName') }}</span>
                    <span>{{ ws.display_name || '-' }}</span>
                  </div>
                  <div class="tcc__ws-card-row">
                    <span class="tcc__ws-card-label">{{ t('system.tenants.wsCreatedAt') }}</span>
                    <span>{{ new Date(ws.created_at).toLocaleDateString() }}</span>
                  </div>
                  <div v-if="expandedWs === ws.id" class="tcc__ws-card-detail">
                    <div class="tcc__ws-card-detail-row">
                      <span class="tcc__ws-card-label">ID</span>
                      <span class="tcc__ws-card-mono">{{ ws.id }}</span>
                    </div>
                    <div class="tcc__ws-card-detail-row">
                      <span class="tcc__ws-card-label">{{ t('system.tenants.detailCode') }}</span>
                      <span>{{ ws.name }}</span>
                    </div>
                    <div class="tcc__ws-card-actions">
                      <HNBButton size="small" variant="ghost" @click.stop="bindCluster(ws.id)">绑定集群</HNBButton>
                    </div>
                    <div v-if="wsClusters[ws.id]?.length" class="tcc__ws-card-clusters">
                      <div class="tcc__ws-card-quota-title">已绑定集群</div>
                      <div class="tcc__cluster-list">
                        <div v-for="cl in wsClusters[ws.id]" :key="cl.id" class="tcc__cluster-item">
                          <span class="tcc__cluster-name">{{ cl.display_name || cl.name }}</span>
                          <span class="tcc__cluster-status" :class="cl.status">{{ cl.status }}</span>
                          <button class="tcc__cluster-unbind" @click.stop="unbindWsCluster(ws.id, cl.id)" title="解绑">×</button>
                        </div>
                      </div>
                    </div>
                    <div class="tcc__ws-card-members">
                      <div class="tcc__ws-card-section-header">
                        <span class="tcc__ws-card-quota-title">{{ t('system.tenants.wsMembers') }}</span>
                        <HNBButton size="small" variant="ghost" @click.stop="openAddMember(ws.id)">{{ t('system.tenants.addMember') }}</HNBButton>
                      </div>
                      <div v-if="wsMembers[ws.id]?.length" class="tcc__member-list">
                        <div v-for="b in wsMembers[ws.id]" :key="b.id" class="tcc__member-item">
                          <span class="tcc__member-name">{{ getUserName(b.user_id) }}</span>
                          <span class="tcc__member-role">{{ getRoleName(b.role_id) }}</span>
                          <button class="tcc__member-remove" @click.stop="removeMember(b.user_id, ws.id)" title="移除">×</button>
                        </div>
                      </div>
                      <p v-else class="tcc__member-empty">{{ t('system.tenants.noMembers') }}</p>
                    </div>
                    <div class="tcc__ws-card-quota">
                      <div class="tcc__ws-card-quota-title">{{ t('system.tenants.quotaTitle') }}</div>
                      <div class="tcc__ws-card-quota-grid">
                        <div class="tcc__quota-field">
                          <label class="tcc__quota-label">CPU</label>
                          <div class="tcc__number-input-wrap">
                            <input v-model.number="wsQuotas[ws.id].cpu" type="number" class="tcc__number-input" min="0" @input="clearWsQuotaMsg(ws.id)" />
                            <span class="tcc__unit-badge">核</span>
                          </div>
                        </div>
                        <div class="tcc__quota-field">
                          <label class="tcc__quota-label">内存</label>
                          <div class="tcc__number-input-wrap">
                            <input v-model.number="wsQuotas[ws.id].memory" type="number" class="tcc__number-input" min="0" @input="clearWsQuotaMsg(ws.id)" />
                            <span class="tcc__unit-badge">Gi</span>
                          </div>
                        </div>
                        <div class="tcc__quota-field">
                          <label class="tcc__quota-label">存储</label>
                          <div class="tcc__number-input-wrap">
                            <input v-model.number="wsQuotas[ws.id].storage" type="number" class="tcc__number-input" min="0" @input="clearWsQuotaMsg(ws.id)" />
                            <span class="tcc__unit-badge">Gi</span>
                          </div>
                        </div>
                        <div class="tcc__quota-field">
                          <label class="tcc__quota-label">虚拟GPU</label>
                          <div class="tcc__number-input-wrap">
                            <input v-model.number="wsQuotas[ws.id].vgpu" type="number" class="tcc__number-input" min="0" @input="clearWsQuotaMsg(ws.id)" />
                            <span class="tcc__unit-badge">%</span>
                          </div>
                        </div>
                        <div class="tcc__quota-field">
                          <label class="tcc__quota-label">虚拟显存</label>
                          <div class="tcc__number-input-wrap">
                            <input v-model.number="wsQuotas[ws.id].vram" type="number" class="tcc__number-input" min="0" @input="clearWsQuotaMsg(ws.id)" />
                            <span class="tcc__unit-badge">MB</span>
                          </div>
                        </div>
                        <div class="tcc__quota-field">
                          <label class="tcc__quota-label">GPU</label>
                          <div class="tcc__number-input-wrap">
                            <input v-model.number="wsQuotas[ws.id].gpu" type="number" class="tcc__number-input" min="0" @input="clearWsQuotaMsg(ws.id)" />
                            <span class="tcc__unit-badge">块</span>
                          </div>
                        </div>
                      </div>
                      <div class="tcc__ws-quota-footer">
                        <span v-if="wsQuotaMsg[ws.id]" class="tcc__ws-quota-msg" :class="{ 'tcc__ws-quota-msg--error': wsQuotaMsg[ws.id] !== t('system.tenants.quotaSaved') }">{{ wsQuotaMsg[ws.id] }}</span>
                        <HNBButton size="small" variant="secondary" :loading="wsQuotaSaving[ws.id]" @click.stop="saveWsQuota(ws.id)">{{ t('system.tenants.quotaSave') }}</HNBButton>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  </div>

  <!-- 添加成员弹窗 -->
  <div v-if="memberDialog" class="modal-mask" @click.self="memberDialog = false">
    <div class="modal-card small">
      <div class="modal-header">
        <h2>{{ t('system.tenants.addMember') }}</h2>
        <button class="icon-button" @click="memberDialog = false">×</button>
      </div>
      <div class="modal-body">
        <div class="tcc__dialog-field">
          <label class="tcc__dialog-label">{{ t('system.tenants.memberUser') }}</label>
          <select v-model="memberForm.user_id" class="tcc__select">
            <option value="" disabled>{{ t('system.tenants.selectUser') }}</option>
            <option v-for="u in allUsers" :key="u.id" :value="u.id">{{ u.username }}{{ u.display_name ? ' (' + u.display_name + ')' : '' }}</option>
          </select>
        </div>
        <div class="tcc__dialog-field">
          <label class="tcc__dialog-label">{{ t('system.tenants.memberRole') }}</label>
          <select v-model="memberForm.role_id" class="tcc__select">
            <option value="" disabled>{{ t('system.tenants.selectRole') }}</option>
            <option v-for="r in allRoles" :key="r.id" :value="r.id">{{ r.display_name || r.name }}</option>
          </select>
        </div>
      </div>
      <div class="modal-actions">
        <HNBButton :disabled="memberSaving" @click="memberDialog = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :loading="memberSaving" :disabled="!memberForm.user_id || !memberForm.role_id" @click="confirmAddMember">{{ t('system.tenants.addMember') }}</HNBButton>
      </div>
    </div>
  </div>

  <!-- 绑定集群弹窗 -->
  <div v-if="clusterBindDialog" class="modal-mask" @click.self="clusterBindDialog = false">
    <div class="modal-card small">
      <div class="modal-header">
        <h2>绑定集群</h2>
        <button class="icon-button" @click="clusterBindDialog = false">×</button>
      </div>
      <div class="modal-body form-grid">
        <HNBSelectInput v-model="selectedCluster" :options="clusterList.map((c) => ({ label: c.name, value: c.id }))" :placeholder="'选择集群'" />
        <p v-if="!clusterList.length" class="text-muted">暂无可用集群</p>
      </div>
      <div class="modal-actions">
        <HNBButton @click="clusterBindDialog = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :disabled="!selectedCluster" @click="confirmBindCluster">确定</HNBButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.toolbar-input { height:34px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary);min-width:220px;box-sizing:border-box; }
.error-banner { padding:10px 16px;border-radius:var(--hnb-radius-md);background:rgba(240,68,56,0.12);color:var(--hnb-color-status-danger, #f04438);font-size:13px; }
.text-muted { color:var(--hnb-color-text-tertiary); }
.modal-mask { position:fixed;inset:0;z-index:1000;display:flex;justify-content:flex-end;background:rgba(0,0,0,0.58); }
.modal-card { width:min(480px,100%);height:100%;display:flex;flex-direction:column;background:var(--hnb-color-bg-surface);border-left:1px solid var(--hnb-color-border);box-shadow:-24px 0 80px rgba(0,0,0,0.35); }
.modal-card.wide { width:min(680px,100%); }
.modal-header { display:flex;justify-content:space-between;align-items:flex-start;gap:16px;padding:20px;border-bottom:1px solid var(--hnb-color-border); }
.modal-header h2 { margin:0;font-size:21px;color:var(--hnb-color-text-primary); }
.icon-button { width:32px;height:32px;border:1px solid var(--hnb-color-border);border-radius:8px;background:transparent;color:var(--hnb-color-text-primary);cursor:pointer;font-size:20px; }
.modal-body { flex:1;overflow:auto;padding:20px; }
.form-grid { display:grid;gap:14px; }
.form-input { width:100%;height:34px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary);box-sizing:border-box; }
.modal-actions { display:flex;justify-content:flex-end;gap:10px;padding:16px 20px;border-top:1px solid var(--hnb-color-border); }
.tcc__detail-grid { display:flex;flex-direction:column;gap:12px; }
.tcc__detail-row { display:flex;align-items:center;gap:16px; }
.tcc__detail-label { min-width:80px;font-size:13px;color:var(--hnb-color-text-secondary);font-weight:600; }
.tcc__ws-error { margin-bottom:12px;padding:8px 12px;background:var(--hnb-color-status-danger-surface);border:1px solid var(--hnb-color-status-danger);border-radius:var(--hnb-radius-md);color:var(--hnb-color-status-danger);font-size:13px; }
.tcc__ws-form { display:flex;gap:8px;align-items:center;margin-bottom:12px;padding:12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-elevated); }
.tcc__ws-empty { text-align:center;color:var(--hnb-color-text-tertiary);padding:24px 12px;font-size:13px; }
.tcc__ws-grid { display:flex;flex-direction:column;gap:10px;margin-top:12px; }
.tcc__ws-card { border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-lg);padding:14px;transition:border-color 0.2s; }
.tcc__ws-card:hover { border-color:var(--hnb-color-primary); }
.tcc__ws-card-header { display:flex;align-items:center;justify-content:space-between;margin-bottom:10px; }
.tcc__ws-card-name { font-size:14px;color:var(--hnb-color-text-primary); }
.tcc__ws-card-body { display:flex;flex-direction:column;gap:6px; }
.tcc__ws-card-row { display:flex;align-items:center;gap:8px;font-size:13px; }
.tcc__ws-card-label { color:var(--hnb-color-text-tertiary);min-width:80px; }
.tcc__ws-card--expanded { border-color:var(--hnb-color-primary);background:var(--hnb-color-bg-elevated); }
.tcc__ws-card-detail { margin-top:10px;padding-top:10px;border-top:1px solid var(--hnb-color-divider);display:flex;flex-direction:column;gap:6px; }
.tcc__ws-card-detail-row { display:flex;align-items:center;gap:8px;font-size:13px; }
.tcc__ws-card-mono { font-family:monospace;font-size:12px;color:var(--hnb-color-text-tertiary); }
.tcc__ws-card-actions { display:flex;gap:8px;margin-bottom:12px; }
.tcc__ws-card-quota { margin-top:12px;padding-top:12px;border-top:1px solid var(--hnb-color-divider); }
.tcc__ws-card-quota-title { font-size:13px;font-weight:600;color:var(--hnb-color-text-primary);margin-bottom:10px; }
.tcc__ws-card-quota-grid { display:grid;grid-template-columns:1fr 1fr;gap:10px; }
.tcc__quota-field { display:flex;flex-direction:column;gap:4px; }
.tcc__quota-label { font-size:12px;color:var(--hnb-color-text-secondary); }
.tcc__number-input-wrap { display:flex;align-items:center; }
.tcc__number-input { height:30px;padding:0 8px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md) 0 0 var(--hnb-radius-md);font-size:13px;color:var(--hnb-color-text-primary);background:var(--hnb-color-bg-surface);outline:none;width:100%;box-sizing:border-box; }
.tcc__number-input:focus { border-color:var(--hnb-color-primary); }
.tcc__unit-badge { display:flex;align-items:center;padding:0 8px;height:30px;background:var(--hnb-color-bg-elevated);border:1px solid var(--hnb-color-border);border-left:none;border-radius:0 var(--hnb-radius-md) var(--hnb-radius-md) 0;font-size:12px;color:var(--hnb-color-text-secondary); }
.tcc__ws-quota-footer { display:flex;align-items:center;justify-content:flex-end;gap:10px;margin-top:12px; }
.tcc__ws-quota-msg { font-size:12px;color:var(--hnb-color-status-success); }
.tcc__ws-quota-msg--error { color:var(--hnb-color-status-danger); }
.tcc__cards { display:flex;flex-direction:column;gap:20px; }
.tcc__card { background:var(--hnb-color-bg-surface);border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-lg);padding:20px; }
.tcc__card-title-row { display:flex;align-items:center;gap:10px;margin-bottom:16px; }
.tcc__card-accent { width:3px;height:18px;background:var(--hnb-color-primary);border-radius:2px;flex-shrink:0; }
.tcc__card-title { margin:0;font-size:15px;font-weight:600;color:var(--hnb-color-text-primary); }
.tcc__form { display:flex;flex-direction:column;gap:14px; }
.tcc__required { color:var(--hnb-color-status-danger);margin-right:2px; }
.tcc__input { height:36px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);font-size:14px;color:var(--hnb-color-text-primary);background:var(--hnb-color-bg-surface);outline:none;width:100%;box-sizing:border-box; }
.tcc__input:focus { border-color:var(--hnb-color-primary);box-shadow:0 0 0 2px color-mix(in srgb,var(--hnb-color-primary) 30%,transparent); }
.tcc__textarea { height:72px;padding:8px 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);font-size:14px;color:var(--hnb-color-text-primary);background:var(--hnb-color-bg-surface);outline:none;resize:vertical;font-family:inherit;width:100%;box-sizing:border-box; }
.tcc__textarea:focus { border-color:var(--hnb-color-primary);box-shadow:0 0 0 2px color-mix(in srgb,var(--hnb-color-primary) 30%,transparent); }
.tcc__textarea-count { text-align:right;font-size:12px;color:var(--hnb-color-text-tertiary);margin-top:2px; }
.tcc__field-error { font-size:12px;color:var(--hnb-color-status-danger); }
.tcc__segmented { display:inline-flex;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);overflow:hidden; }
.tcc__seg-btn { padding:6px 18px;border:none;background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary);font-size:14px;cursor:pointer;transition:all 0.2s; }
.tcc__seg-btn.active { background:var(--hnb-color-primary);color:var(--hnb-color-text-on-accent); }
.tcc__seg-btn:not(.active):hover { background:var(--hnb-color-bg-elevated); }
.tcc__date-row { display:flex;align-items:center;gap:8px;margin-top:8px; }
.tcc__date-input { width:160px; }
.tcc__date-sep { color:var(--hnb-color-text-tertiary);font-size:14px; }
.tcc__quota-stats { display:flex;gap:20px;flex-wrap:wrap; }
.tcc__quota-form-grid { display:grid;grid-template-columns:1fr 1fr;gap:14px; }
.tcc__ws-card-clusters { margin-top:12px;padding:10px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface); }
.tcc__cluster-list { display:flex;flex-direction:column;gap:6px;margin-top:6px; }
.tcc__cluster-item { display:flex;align-items:center;gap:8px;padding:4px 8px;border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg); }
.tcc__cluster-name { flex:1;font-size:13px;color:var(--hnb-color-text-primary); }
.tcc__cluster-status { font-size:11px;padding:1px 6px;border-radius:99px;text-transform:uppercase; }
.tcc__cluster-status.online { background:rgba(0,200,83,0.15);color:var(--hnb-color-status-success, #12b76a); }
.tcc__cluster-status.offline { background:rgba(240,68,56,0.15);color:var(--hnb-color-status-danger, #f04438); }
.tcc__cluster-status.unknown { background:rgba(255,193,7,0.15);color:var(--hnb-color-status-warning, #f79009); }
.tcc__cluster-unbind { background:none;border:none;color:var(--hnb-color-text-tertiary);cursor:pointer;font-size:16px;line-height:1;padding:0 2px; }
.tcc__ws-card-members { margin-top:12px;padding:10px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface); }
.tcc__ws-card-section-header { display:flex;justify-content:space-between;align-items:center;margin-bottom:6px; }
.tcc__member-list { display:flex;flex-direction:column;gap:4px; }
.tcc__member-item { display:flex;align-items:center;gap:8px;padding:4px 8px;border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg); }
.tcc__member-name { flex:1;font-size:13px;color:var(--hnb-color-text-primary); }
.tcc__member-role { font-size:11px;padding:1px 6px;border-radius:99px;background:rgba(100,181,246,0.15);color:var(--hnb-color-status-info, #5bb8f5); }
.tcc__member-remove { background:none;border:none;color:var(--hnb-color-text-tertiary);cursor:pointer;font-size:16px;line-height:1;padding:0 2px; }
.tcc__member-empty { font-size:13px;color:var(--hnb-color-text-tertiary);margin:4px 0; }
.tcc__select { height:34px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary);min-width:160px;width:100%;box-sizing:border-box; }
.tcc__dialog-field { margin-bottom:16px; }
.tcc__dialog-label { display:block;font-size:12px;font-weight:500;color:var(--hnb-color-text-secondary);margin-bottom:4px; }
.tcc__stat-item { font-size:13px;color:var(--hnb-color-text-secondary);white-space:nowrap; }
.tcc__stat-value { font-size:15px;font-weight:600;color:var(--hnb-color-text-primary);margin:0 2px; }
.tcc__stat-unit { color:var(--hnb-color-text-tertiary); }
.tcc__submit-error { margin-top:12px;padding:8px 12px;background:var(--hnb-color-status-danger-surface);border:1px solid var(--hnb-color-status-danger);border-radius:var(--hnb-radius-md);color:var(--hnb-color-status-danger);font-size:13px; }
</style>