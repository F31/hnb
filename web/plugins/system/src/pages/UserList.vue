<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBToolbar, HNBTable, HNBButton, HNBFormField, HNBSelectInput, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import * as api from '../systemApi'

const { t } = useI18n()

const users = ref<api.UserRecord[]>([])
const total = ref(0)
const roles = ref<api.RoleRecord[]>([])
const bindings = ref<api.RoleBindingRecord[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const statusFilter = ref('all')
const page = ref(1)
const pageSize = ref(10)
const checkedKeys = ref<(string | number)[]>([])

const showForm = ref(false)
const editing = ref(false)
const form = ref({ id: '', username: '', display_name: '', email: '', phone: '', password: '', is_active: true })
const phoneError = ref('')
const passwordReset = ref({ id: '', username: '', password: '', show: false })
const showRoleBindings = ref(false)
const roleBindingUser = ref<api.UserRecord | null>(null)
const roleBindingForm = ref({ role_id: '', scope: 'global', scope_id: '' })

async function loadUsers() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.listUsers(page.value, pageSize.value)
    console.log('[UserList] listUsers raw result:', res, '| items:', res?.items, '| total:', res?.total)
    users.value = res.items || []
    total.value = res.total ?? 0
    console.log('[UserList] users.value now:', users.value.length, 'items')
  } catch (e: any) {
    console.error('[UserList] listUsers error:', e)
    error.value = e?.message || t('system.users.loadError')
  } finally {
    loading.value = false
  }
}

async function loadRoles() {
  try {
    roles.value = await api.listRoles()
  } catch { /* roles are supplementary */ }
}

async function loadBindings() {
  try {
    const res = await api.listRoleBindings()
    bindings.value = res.items
  } catch { /* bindings are supplementary */ }
}

const pagination = computed(() => ({ page: page.value, pageSize: pageSize.value, total: total.value }))

const filteredUsers = computed(() => {
  let list = users.value
  if (search.value) {
    const q = search.value.toLowerCase()
    list = list.filter((u) => u.username.toLowerCase().includes(q) || (u.display_name || '').toLowerCase().includes(q) || (u.email || '').toLowerCase().includes(q))
  }
  if (statusFilter.value !== 'all') {
    const active = statusFilter.value === 'active'
    list = list.filter((u) => u.is_active === active)
  }
  return list
})

function onPage(p: number) { page.value = p; loadUsers() }
function onPageSize(ps: number) { pageSize.value = ps; page.value = 1; loadUsers() }

function getUserBindings(userId: string): api.RoleBindingRecord[] {
  return (bindings.value || []).filter((b) => b.user_id === userId)
}

function getRoleName(roleId: string): string {
  return roles.value.find((r) => r.id === roleId)?.display_name || roleId
}

const columns: HNBTableColumn[] = [
  { key: 'username', title: t('system.users.colUsername'), render: (row) => h('strong', row.username) },
  { key: 'display_name', title: t('system.users.colDisplayName'), render: (row) => row.display_name || '-' },
  { key: 'email', title: t('system.users.colEmail'), render: (row) => row.email || '-' },
  { key: 'phone', title: t('system.users.phone'), render: (row) => row.phone || '-' },
  { key: 'source', title: t('system.users.colSource'), render: (row) => h(StatusBadge, { semantic: 'info', label: row.source }) },
  { key: 'status', title: t('system.users.colStatus'), render: (row) => h(StatusBadge, { semantic: row.is_active ? 'success' : 'error', label: row.is_active ? t('system.users.active') : t('system.users.disabled') }) },
  {
    key: 'roles', title: t('system.users.colRoles'),
    render: (row) => {
      const userBindings = getUserBindings(row.id)
      if (!userBindings.length) return h('span', { class: 'text-muted' }, '-')
      return h('div', { style: 'display:flex;flex-wrap:wrap;gap:4px' }, userBindings.map((b) =>
        h('span', { style: 'display:inline-block;padding:2px 8px;border-radius:999px;background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary);font-size:11px' }, getRoleName(b.role_id))
      ))
    },
  },
  { key: 'created_at', title: t('system.users.colCreatedAt'), render: (row) => new Date(row.created_at).toLocaleDateString() },
  {
    key: 'actions', title: t('system.users.colActions'),
    render: (row) => { const u = row as unknown as api.UserRecord; return h('div', { style: 'display:flex;gap:8px;align-items:center' }, [
      h(HNBButton, { size: 'small', variant: 'ghost', onClick: () => openEdit(u) }, () => t('system.users.edit')),
      h(HNBButton, { size: 'small', variant: 'ghost', disabled: !u.is_active, onClick: () => openResetPassword(u) }, () => t('system.users.resetPassword')),
      h(HNBButton, { size: 'small', variant: 'ghost', onClick: () => openRoleBindings(u) }, () => t('system.users.assignRoles')),
      users.value.length > 1 ? h(HNBButton, { size: 'small', variant: 'danger', onClick: () => confirmDelete(u) }, () => t('system.users.delete')) : null,
    ]) },
  },
]

async function batchDelete() {
  if (!checkedKeys.value.length) return
  if (users.value.length <= checkedKeys.value.length) {
    error.value = t('system.users.cannotDeleteLast')
    return
  }
  if (!confirm(t('system.users.confirmBatchDelete', { count: checkedKeys.value.length }))) return
  error.value = ''
  try {
    await Promise.all(checkedKeys.value.map((id) => api.deleteUser(String(id))))
    checkedKeys.value = []
    await loadUsers()
  } catch (e: any) {
    error.value = e?.message || t('system.users.deleteError')
  }
}

function validatePhone(phone: string): boolean {
  if (!phone) return true
  return /^1[3-9]\d{9}$/.test(phone)
}

function openCreate() {
  editing.value = false
  phoneError.value = ''
  form.value = { id: '', username: '', display_name: '', email: '', phone: '', password: '', is_active: true }
  showForm.value = true
}

function openEdit(user: api.UserRecord) {
  editing.value = true
  form.value = { id: user.id, username: user.username, display_name: user.display_name || '', email: user.email || '', phone: user.phone || '', password: '', is_active: user.is_active }
  showForm.value = true
}

async function saveUser() {
  if (form.value.phone && !validatePhone(form.value.phone)) {
    phoneError.value = t('system.users.phoneInvalid')
    return
  }
  phoneError.value = ''
  error.value = ''
  try {
    if (editing.value) {
      const payload: api.UpdateUserPayload = {}
      if (form.value.display_name) payload.display_name = form.value.display_name
      if (form.value.email) payload.email = form.value.email
      payload.phone = form.value.phone
      payload.is_active = form.value.is_active
      await api.updateUser(form.value.id, payload)
    } else {
      await api.createUser({ username: form.value.username, password: form.value.password, display_name: form.value.display_name || undefined, email: form.value.email || undefined, phone: form.value.phone || undefined })
    }
    showForm.value = false
    await loadUsers()
  } catch (e: any) {
    error.value = e?.message || t('system.users.saveError')
  }
}

async function confirmDelete(user: api.UserRecord) {
  if (users.value.length <= 1) {
    error.value = t('system.users.cannotDeleteLast')
    return
  }
  if (!confirm(t('system.users.confirmDelete', { username: user.username }))) return
  error.value = ''
  try {
    await api.deleteUser(user.id)
    await loadUsers()
  } catch (e: any) {
    error.value = e?.message || t('system.users.deleteError')
  }
}

function openResetPassword(user: api.UserRecord) {
  passwordReset.value = { id: user.id, username: user.username, password: '', show: true }
}

async function saveResetPassword() {
  error.value = ''
  try {
    await api.resetPassword(passwordReset.value.id, passwordReset.value.password)
    passwordReset.value.show = false
  } catch (e: any) {
    error.value = e?.message || t('system.users.resetPasswordError')
  }
}

function openRoleBindings(user: api.UserRecord) {
  roleBindingUser.value = user
  roleBindingForm.value = { role_id: '', scope: 'global', scope_id: '' }
  showRoleBindings.value = true
}

async function addRoleBinding() {
  if (!roleBindingUser.value || !roleBindingForm.value.role_id) return
  error.value = ''
  try {
    await api.bindRole({
      user_id: roleBindingUser.value.id,
      role_id: roleBindingForm.value.role_id,
      scope: roleBindingForm.value.scope,
      scope_id: roleBindingForm.value.scope_id || undefined,
    })
    const res = await api.listRoleBindings()
    bindings.value = res.items
  } catch (e: any) {
    error.value = e?.message || t('system.users.bindError')
  }
}

async function removeRoleBinding(binding: api.RoleBindingRecord) {
  error.value = ''
  try {
    await api.unbindRole(binding.user_id, binding.scope, binding.scope_id || '')
    const res = await api.listRoleBindings()
    bindings.value = res.items
  } catch (e: any) {
    error.value = e?.message || t('system.users.unbindError')
  }
}

onMounted(async () => {
  await Promise.all([loadUsers(), loadRoles(), loadBindings()])
})
</script>

<template>
  <HNBPageShell :title="t('system.users.title')" :description="t('system.users.desc')">
    <template #actions>
      <HNBButton variant="primary" size="medium" @click="openCreate">{{ t('system.users.create') }}</HNBButton>
      <HNBButton v-if="checkedKeys.length" variant="danger" size="medium" @click="batchDelete">{{ t('system.users.batchDelete', { count: checkedKeys.length }) }}</HNBButton>
    </template>
    <HNBToolbar>
      <input v-model="search" :placeholder="t('system.users.searchPlaceholder')" class="toolbar-input" />
      <HNBSelectInput v-model="statusFilter" :options="[
        { label: t('system.users.statusAll'), value: 'all' },
        { label: t('system.users.statusActive'), value: 'active' },
        { label: t('system.users.statusDisabled'), value: 'disabled' },
      ]" />
    </HNBToolbar>
    <HNBTable :columns="columns" :data="filteredUsers" :loading="loading" :pagination="pagination" row-key="id" selectable :checked-row-keys="checkedKeys" :error="error" :empty-title="t('system.users.empty')" @update:checked-row-keys="(keys) => checkedKeys = keys" @update:page="onPage" @update:page-size="onPageSize" @error-retry="loadUsers" />
  </HNBPageShell>

  <div v-if="showForm" class="modal-mask" @click.self="showForm = false">
    <div class="modal-card">
      <div class="modal-header">
        <h2>{{ editing ? t('system.users.editUser') : t('system.users.createUser') }}</h2>
        <button class="icon-button" @click="showForm = false">×</button>
      </div>
      <div class="modal-body form-grid">
        <HNBFormField :label="t('system.users.username')" required>
          <input v-model="form.username" :readonly="editing" class="form-input" :placeholder="t('system.users.usernamePlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.users.displayName')">
          <input v-model="form.display_name" class="form-input" :placeholder="t('system.users.displayNamePlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.users.email')">
          <input v-model="form.email" type="email" class="form-input" :placeholder="t('system.users.emailPlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.users.phone')" :error="phoneError">
          <input v-model="form.phone" type="tel" class="form-input" :placeholder="t('system.users.phonePlaceholder')" />
        </HNBFormField>
        <HNBFormField v-if="!editing" :label="t('system.users.password')" required>
          <input v-model="form.password" type="password" class="form-input" :placeholder="t('system.users.passwordPlaceholder')" />
        </HNBFormField>
        <HNBFormField :label="t('system.users.status')">
          <label class="toggle-label">
            <input v-model="form.is_active" type="checkbox" />
            <span>{{ form.is_active ? t('system.users.active') : t('system.users.disabled') }}</span>
          </label>
        </HNBFormField>
      </div>
      <div class="modal-actions">
        <HNBButton @click="showForm = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :disabled="editing ? false : !form.username || !form.password" @click="saveUser">{{ t('system.common.confirm') }}</HNBButton>
      </div>
    </div>
  </div>

  <div v-if="passwordReset.show" class="modal-mask" @click.self="passwordReset.show = false">
    <div class="modal-card">
      <div class="modal-header">
        <h2>{{ t('system.users.resetPasswordTitle') }} - {{ passwordReset.username }}</h2>
        <button class="icon-button" @click="passwordReset.show = false">×</button>
      </div>
      <div class="modal-body form-grid">
        <HNBFormField :label="t('system.users.newPassword')" required>
          <input v-model="passwordReset.password" type="password" class="form-input" :placeholder="t('system.users.passwordPlaceholder')" />
        </HNBFormField>
      </div>
      <div class="modal-actions">
        <HNBButton @click="passwordReset.show = false">{{ t('system.common.cancel') }}</HNBButton>
        <HNBButton variant="primary" :disabled="!passwordReset.password" @click="saveResetPassword">{{ t('system.common.confirm') }}</HNBButton>
      </div>
    </div>
  </div>

  <div v-if="showRoleBindings && roleBindingUser" class="modal-mask" @click.self="showRoleBindings = false">
    <div class="modal-card wide">
      <div class="modal-header">
        <h2>{{ t('system.users.assignRoles') }} - {{ roleBindingUser.username }}</h2>
        <button class="icon-button" @click="showRoleBindings = false">×</button>
      </div>
      <div class="modal-body">
        <div class="binding-section">
          <h3>{{ t('system.users.currentBindings') }}</h3>
          <table class="binding-table">
            <tr v-for="b in getUserBindings(roleBindingUser.id)" :key="b.id">
              <td><strong>{{ getRoleName(b.role_id) }}</strong></td>
              <td>{{ b.scope }}{{ b.scope_id ? '/' + b.scope_id : '' }}</td>
              <td><HNBButton variant="danger" size="small" @click="removeRoleBinding(b)">{{ t('system.users.remove') }}</HNBButton></td>
            </tr>
            <tr v-if="!getUserBindings(roleBindingUser.id).length"><td colspan="3" class="text-muted">{{ t('system.users.noBindings') }}</td></tr>
          </table>
        </div>
        <div class="binding-section">
          <h3>{{ t('system.users.addBinding') }}</h3>
          <div class="form-row">
            <HNBSelectInput v-model="roleBindingForm.role_id" :options="roles.map((r) => ({ label: r.display_name || r.name, value: r.id }))" :placeholder="t('system.users.selectRole')" />
            <HNBSelectInput v-model="roleBindingForm.scope" :options="[
              { label: t('system.users.scopeGlobal'), value: 'global' },
              { label: t('system.users.scopeWorkspace'), value: 'workspace' },
              { label: t('system.users.scopeCluster'), value: 'cluster' },
              { label: t('system.users.scopeProject'), value: 'project' },
            ]" />
            <input v-model="roleBindingForm.scope_id" class="form-input" :placeholder="t('system.users.scopeIdPlaceholder')" style="width:180px" />
            <HNBButton variant="primary" :disabled="!roleBindingForm.role_id" @click="addRoleBinding">{{ t('system.users.bind') }}</HNBButton>
          </div>
        </div>
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
.toggle-label { display:flex;align-items:center;gap:8px;cursor:pointer;color:var(--hnb-color-text-primary); }
.binding-section { margin-bottom:20px; }
.binding-section h3 { margin:0 0 10px;font-size:15px;color:var(--hnb-color-text-primary); }
.binding-table { width:100%;border-collapse:collapse;color:var(--hnb-color-text-primary); }
.binding-table td { padding:8px 12px;border-bottom:1px solid var(--hnb-color-border); }
.form-row { display:flex;gap:8px;align-items:flex-end;flex-wrap:wrap; }
</style>
