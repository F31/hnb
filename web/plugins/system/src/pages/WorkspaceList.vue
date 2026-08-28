<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBToolbar, HNBTable, HNBButton, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import * as api from '../systemApi'

const { t } = useI18n()

const workspaces = ref<api.WorkspaceRecord[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')

const filteredWorkspaces = computed(() => {
  let list = workspaces.value
  if (search.value) {
    const q = search.value.toLowerCase()
    list = list.filter((w) => w.name.toLowerCase().includes(q) || (w.display_name || '').toLowerCase().includes(q))
  }
  return list
})

const columns: HNBTableColumn[] = [
  { key: 'name', title: t('system.tenants.wsName'), render: (row) => h('strong', row.name) },
  { key: 'display_name', title: t('system.tenants.wsDisplayName'), render: (row) => row.display_name || '-' },
  { key: 'tenant_id', title: '租户', render: (row) => row.tenant_id },
  {
    key: 'status', title: t('system.tenants.wsStatus'),
    render: (row) => h(StatusBadge, { semantic: row.is_active ? 'success' : 'error', label: row.is_active ? t('system.tenants.active') : t('system.tenants.disabled') }),
  },
  { key: 'created_at', title: t('system.tenants.wsCreatedAt'), render: (row) => new Date(row.created_at).toLocaleDateString() },
]

async function loadWorkspaces() {
  loading.value = true
  error.value = ''
  try {
    workspaces.value = await api.listWorkspaces()
  } catch (e: any) {
    error.value = e?.message || '加载工作空间失败'
  } finally {
    loading.value = false
  }
}

onMounted(loadWorkspaces)
</script>

<template>
  <HNBPageShell :title="t('system.tenants.workspaces')" :description="'管理所有工作空间'">
    <HNBToolbar>
      <input v-model="search" :placeholder="t('system.tenants.wsNamePlaceholder')" class="toolbar-input" />
    </HNBToolbar>
    <HNBTable :columns="columns" :data="filteredWorkspaces" :loading="loading" row-key="id" :error="error" :empty-title="t('system.tenants.noWorkspaces')" @error-retry="loadWorkspaces" />
  </HNBPageShell>
</template>

<style scoped>
.toolbar-input { height:34px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface);color:var(--hnb-color-text-primary);min-width:220px;box-sizing:border-box; }
</style>