<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBToolbar, HNBTable, HNBButton, HNBSelectInput, HNBFormField, StatusBadge, HNBDetailPanel } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import * as api from '../systemApi'

const { t } = useI18n()

const roles = ref<api.RoleRecord[]>([])
const users = ref<api.UserRecord[]>([])
const bindings = ref<api.RoleBindingRecord[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const scopeFilter = ref('all')
const showDetail = ref(false)
const detailRole = ref<api.RoleRecord | null>(null)
const showBindDialog = ref(false)
const bindForm = ref({ user_id: '', scope: 'global', scope_id: '' })
const bindRole = ref<api.RoleRecord | null>(null)
const showCreateForm = ref(false)
const createForm = ref({ name: '', display_name: '', scope: 'global', verbs: '', resources: '' })

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const [r, uRes, bRes] = await Promise.all([api.listRoles(), api.listUsers(), api.listRoleBindings()])
    roles.value = r
    users.value = uRes.items
    bindings.value = bRes.items
  } catch (e: any) {
    error.value = e?.message || t('system.roles.loadError')
  } finally {
    loading.value = false
  }
}

const filteredRoles = computed(() => {
  let list = roles.value
  if (search.value) {
    const q = search.value.toLowerCase()
    list = list.filter((r) => r.name.toLowerCase().includes(q) || (r.display_name || '').toLowerCase().includes(q))
  }
  if (scopeFilter.value !== 'all') {
    list = list.filter((r) => r.scope === scopeFilter.value)
  }
  return list
})

function getRoleBindings(roleId: string): api.RoleBindingRecord[] {
  return bindings.value.filter((b) => b.role_id === roleId)
}

function getUserName(userId: string): string {
  return users.value.find((u) => u.id === userId)?.username || userId
}

const columns: HNBTableColumn[] = [
  { key: 'name', title: t('system.roles.colName'), render: (row) => h('strong', row.name) },
  { key: 'display_name', title: t('system.roles.colDisplayName'), render: (row) => row.display_name || '-' },
  { key: 'scope', title: t('system.roles.colScope'), render: (row) => h(StatusBadge, { semantic: row.scope === 'global' ? 'info' : 'processing', label: row.scope }) },
  { key: 'built_in', title: t('system.roles.colBuiltIn'), render: (row) => h(StatusBadge, { semantic: row.built_in ? 'success' : 'warning', label: row.built_in ? t('system.roles.builtIn') : t('system.roles.custom') }) },
  { key: 'users', title: t('system.roles.colUsers'), render: (row) => String(getRoleBindings(row.id).length) },
  {
    key: 'actions', title: t('system.roles.colActions'),
    render: (row) => { const r = row as unknown as api.RoleRecord; return h('div', { style: 'display:flex;gap:8px;align-items:center' }, [
      h(HNBButton, { size: 'small', variant: 'ghost', onClick: () => openDetail(r) }, () => t('system.roles.view')),
      h(HNBButton, { size: 'small', variant: 'ghost', onClick: () => openBindDialog(r) }, () => t('system.roles.assign')),
      !r.built_in ? h(HNBButton, { size: 'small', variant: 'danger', onClick: () => confirmDeleteRole(r) }, () => t('system.roles.delete')) : null,
    ]) },
  },
]

function openCreateRole() {
  createForm.value = { name: '', display_name: '', scope: 'global', verbs: '', resources: '' }
  showCreateForm.value = true
}

async function saveCreateRole() {
  error.value = ''
  try {
    const verbs = createForm.value.verbs.split(',').map((v) => v.trim()).filter(Boolean)
    const resources = createForm.value.resources.split(',').map((r: string) => r.trim()).filter(Boolean)
    if (!verbs.length) verbs.push('*')
    if (!resources.length) resources.push('*')
    await api.createRole({
      name: createForm.value.name,
      display_name: createForm.value.display_name || undefined,
      scope: createForm.value.scope,
      verbs,
      resources,
    })
    showCreateForm.value = false
    await loadData()
  } catch (e: any) {
    error.value = e?.message || t('system.roles.createError')
  }
}

async function confirmDeleteRole(role: api.RoleRecord) {
  if (!confirm(t('system.roles.confirmDelete', { name: role.display_name || role.name }))) return
  error.value = ''
  try {
    await api.deleteRole(role.id)
    await loadData()
  } catch (e: any) {
    error.value = e?.message || t('system.roles.deleteError')
  }
}

function openDetail(role: api.RoleRecord) {
  detailRole.value = role
  showDetail.value = true
}

function openBindDialog(role: api.RoleRecord) {
  bindRole.value = role
  bindForm.value = { user_id: '', scope: role.scope, scope_id: '' }
  showBindDialog.value = true
}

async function addBinding() {
  if (!bindRole.value || !bindForm.value.user_id) return
  error.value = ''
  try {
    await api.bindRole({
      user_id: bindForm.value.user_id,
      role_id: bindRole.value.id,
      scope: bindForm.value.scope,
      scope_id: bindForm.value.scope_id || undefined,
    })
    showBindDialog.value = false
    await loadData()
  } catch (e: any) {
    error.value = e?.message || t('system.roles.bindError')
  }
}

async function removeBinding(binding: api.RoleBindingRecord) {
  error.value = ''
  try {
    await api.unbindRole(binding.user_id, binding.scope, binding.scope_id || '')
    await loadData()
  } catch (e: any) {
    error.value = e?.message || t('system.roles.unbindError')
  }
}

const detailItems = computed(() => {
  if (!detailRole.value) return []
  return [
    { label: t('system.roles.name'), value: detailRole.value.name },
    { label: t('system.roles.displayName'), value: detailRole.value.display_name || '-' },
    { label: t('system.roles.scope'), value: detailRole.value.scope },
    { label: t('system.roles.builtIn'), value: detailRole.value.built_in ? t('system.roles.yes') : t('system.roles.no') },
  ]
})

onMounted(loadData)
</script>

<template>
  <HNBPageShell :title="t('system.roles.title')" :description="t('system.roles.desc')">
    <template #actions>
      <HNBButton variant="primary" size="medium" @click="openCreateRole">{{ t('system.roles.createRole') }}</HNBButton>
    </template>
    <HNBToolbar>
      <input v-model="search" :placeholder="t('system.roles.searchPlaceholder')" class="toolbar-input" />
      <HNBSelectInput v-model="scopeFilter" :options="[
        { label: t('system.roles.scopeAll'), value: 'all' },
        { label: t('system.roles.scopeGlobal'), value: 'global' },
        { label: t('system.roles.scopeWorkspace'), value: 'workspace' },
        { label: t('system.roles.scopeCluster'), value: 'cluster' },
        { label: t('system.roles.scopeProject'), value: 'project' },
      ]" />
    </HNBToolbar>
    <HNBTable :columns="columns" :data="filteredRoles" :loading="loading" row-key="id" :error="error" :empty-title="t('system.roles.empty')" @error-retry="loadData" />
  </HNBPageShell>

  <div v-if="showCreateForm" class="modal-mask" @click.self="showCreateForm = false">
    <div class="modal-card">
      <div class="modal-header">
        <h2>{{ t('system.roles.createRole') }}</h2>
        <button class="icon-button" @click="showCreateForm = false">×</button>
      </div>
      <div class="modal-body form-grid">
        <HNBFormField :label="t('system.roles.roleName')" required>
          <input v-model="createForm.name" class="form-input" :placeholder="t('system.roles.roleNamePlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.roles.displayNameField')">
          <input v-model="createForm.display_name" class="form-input" :placeholder="t('system.roles.displayNamePlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.roles.scope')" required>
          <HNBSelectInput v-model="createForm.scope" :options="[
            { label: t('system.roles.scopeGlobal'), value: 'global' },
            { label: t('system.roles.scopeWorkspace'), value: 'workspace' },
            { label: t('system.roles.scopeCluster'), value: 'cluster' },
            { label: t('system.roles.scopeProject'), value: 'project' },
          ]" />
        </HNBFormField>
        <HNBFormField :label="t('system.roles.verbsField')" :help="t('system.roles.verbsHelp')">
          <input v-model="createForm.verbs" class="form-input" :placeholder="t('system.roles.verbsPlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.roles.resourcesField')" :help="t('system.roles.resourcesHelp')">
          <input v-model="createForm.resources" class="form-input" :placeholder="t('system.roles.resourcesPlaceholder')" />
        </HNBFormField>
      </div>
      <div class="modal-actions">
        <HNBButton @click="showCreateForm = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :disabled="!createForm.name" @click="saveCreateRole">{{ t('system.common.confirm') }}</HNBButton>
      </div>
    </div>
  </div>

  <div v-if="showDetail && detailRole" class="modal-mask" @click.self="showDetail = false">
    <div class="modal-card wide">
      <div class="modal-header">
        <h2>{{ detailRole.display_name || detailRole.name }}</h2>
        <button class="icon-button" @click="showDetail = false">×</button>
      </div>
      <div class="modal-body">
        <div class="tcc__cards">
          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.roles.basicInfo') }}</h2>
            </div>
            <div class="tcc__detail-grid">
              <div class="tcc__detail-row" v-for="item in detailItems" :key="item.label">
                <span class="tcc__detail-label">{{ item.label }}</span>
                <span>{{ item.value ?? '-' }}</span>
              </div>
            </div>
          </section>
          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.roles.permissions') }}</h2>
            </div>
            <div v-if="detailRole.rules && detailRole.rules.length" class="tcc__perm-list">
              <div v-for="(rule, i) in detailRole.rules" :key="i" class="tcc__perm-card">
                <div class="tcc__perm-header">{{ t('system.roles.rule') }} {{ i + 1 }}</div>
                <div class="tcc__perm-row"><span class="tcc__perm-label">{{ t('system.roles.verbs') }}</span><span v-for="v in rule.verbs" :key="v" class="tcc__perm-tag">{{ v }}</span></div>
                <div class="tcc__perm-row"><span class="tcc__perm-label">{{ t('system.roles.resources') }}</span><span v-for="r in rule.resources" :key="r" class="tcc__perm-tag">{{ r }}</span></div>
              </div>
            </div>
            <p v-else class="tcc__ws-empty">{{ t('system.roles.noPermissions') }}</p>
          </section>
          <section class="tcc__card">
            <div class="tcc__card-title-row">
              <span class="tcc__card-accent" />
              <h2 class="tcc__card-title">{{ t('system.roles.boundUsers') }} ({{ getRoleBindings(detailRole.id).length }})</h2>
            </div>
            <table class="tcc__binding-table">
              <thead><tr><th>{{ t('system.roles.selectUser') }}</th><th>Scope</th><th></th></tr></thead>
              <tbody>
                <tr v-for="b in getRoleBindings(detailRole.id)" :key="b.id">
                  <td><strong>{{ getUserName(b.user_id) }}</strong></td>
                  <td>{{ b.scope }}{{ b.scope_id ? '/' + b.scope_id : '' }}</td>
                  <td><HNBButton variant="danger" size="small" @click="removeBinding(b)">{{ t('system.roles.unbind') }}</HNBButton></td>
                </tr>
                <tr v-if="!getRoleBindings(detailRole.id).length"><td colspan="3" class="tcc__ws-empty">{{ t('system.roles.noBoundUsers') }}</td></tr>
              </tbody>
            </table>
          </section>
        </div>
      </div>
    </div>
  </div>

  <div v-if="showBindDialog && bindRole" class="modal-mask" @click.self="showBindDialog = false">
    <div class="modal-card">
      <div class="modal-header">
        <h2>{{ t('system.roles.assignRole') }} - {{ bindRole.display_name || bindRole.name }}</h2>
        <button class="icon-button" @click="showBindDialog = false">×</button>
      </div>
      <div class="modal-body form-grid">
        <HNBSelectInput v-model="bindForm.user_id" :options="users.map((u) => ({ label: u.username + (u.display_name ? ' (' + u.display_name + ')' : ''), value: u.id }))" :placeholder="t('system.roles.selectUser')" />
        <HNBSelectInput v-model="bindForm.scope" :options="[
          { label: t('system.roles.scopeGlobal'), value: 'global' },
          { label: t('system.roles.scopeWorkspace'), value: 'workspace' },
          { label: t('system.roles.scopeCluster'), value: 'cluster' },
          { label: t('system.roles.scopeProject'), value: 'project' },
        ]" />
        <input v-model="bindForm.scope_id" class="form-input" :placeholder="t('system.roles.scopeIdPlaceholder')" />
      </div>
      <div class="modal-actions">
        <HNBButton @click="showBindDialog = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :disabled="!bindForm.user_id" @click="addBinding">{{ t('system.common.confirm') }}</HNBButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.toolbar-input { height:34px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary);min-width:220px;box-sizing:border-box; }
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
.tcc__cards { display:flex;flex-direction:column;gap:20px; }
.tcc__card { background:var(--hnb-color-bg-surface);border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-lg);padding:20px; }
.tcc__card-title-row { display:flex;align-items:center;gap:10px;margin-bottom:16px; }
.tcc__card-accent { width:3px;height:18px;background:var(--hnb-color-primary);border-radius:2px;flex-shrink:0; }
.tcc__card-title { margin:0;font-size:15px;font-weight:600;color:var(--hnb-color-text-primary); }
.tcc__detail-grid { display:flex;flex-direction:column;gap:12px; }
.tcc__detail-row { display:flex;align-items:center;gap:16px;font-size:13px; }
.tcc__detail-label { min-width:80px;color:var(--hnb-color-text-secondary);font-weight:600; }
.tcc__perm-list { display:flex;flex-direction:column;gap:10px; }
.tcc__perm-card { padding:12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-lg);background:var(--hnb-color-bg-elevated); }
.tcc__perm-header { font-weight:600;margin-bottom:8px;color:var(--hnb-color-text-secondary);font-size:12px; }
.tcc__perm-row { display:flex;align-items:center;gap:8px;margin-bottom:6px;font-size:13px; }
.tcc__perm-label { color:var(--hnb-color-text-secondary);font-size:12px; }
.tcc__perm-tag { display:inline-block;padding:2px 8px;border-radius:999px;background:var(--hnb-color-primary);color:#fff;font-size:11px; }
.tcc__binding-table { width:100%;border-collapse:collapse;margin-top:8px; }
.tcc__binding-table th { padding:10px 12px;font-size:12px;font-weight:600;color:var(--hnb-color-text-secondary);text-align:left;border-bottom:1px solid var(--hnb-color-divider); }
.tcc__binding-table td { padding:10px 12px;font-size:13px;color:var(--hnb-color-text-primary);border-bottom:1px solid var(--hnb-color-divider); }
.tcc__ws-empty { text-align:center;color:var(--hnb-color-text-tertiary);padding:24px 12px;font-size:13px; }
</style>