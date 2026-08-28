<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBToolbar, HNBTable, HNBButton, HNBSelectInput, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import * as api from '../systemApi'

const { t } = useI18n()

const logs = ref<api.AuditLogRecord[]>([])
const loading = ref(false)
const error = ref('')
const actionFilter = ref('all')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

const pagination = computed(() => ({ page: page.value, pageSize: pageSize.value, total: total.value }))

const filteredLogs = computed(() => {
  let list = logs.value
  if (actionFilter.value !== 'all') {
    list = list.filter((l) => l.action === actionFilter.value)
  }
  return list
})

const columns: HNBTableColumn[] = [
  { key: 'timestamp', title: t('system.audit.colTime'), render: (row) => new Date(row.timestamp).toLocaleString() },
  { key: 'user_id', title: t('system.audit.colUser'), render: (row) => row.user_id.substring(0, 8) + '...' },
  { key: 'action', title: t('system.audit.colAction'), render: (row) => h(StatusBadge, { semantic: row.action === 'create' ? 'success' : row.action === 'delete' ? 'error' : 'info', label: row.action }) },
  { key: 'resource_type', title: t('system.audit.colResourceType') },
  { key: 'path', title: t('system.audit.colPath'), render: (row) => row.path },
  { key: 'status_code', title: t('system.audit.colStatusCode'), render: (row) => h('span', { style: { color: row.status_code >= 400 ? 'var(--hnb-color-status-danger)' : 'var(--hnb-color-status-success)' } }, String(row.status_code)) },
]

async function loadLogs() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.listAuditLogs(page.value, pageSize.value)
    logs.value = res
    total.value = res.length
  } catch (e: any) {
    error.value = e?.message || t('system.audit.loadError')
  } finally {
    loading.value = false
  }
}

function onPage(p: number) { page.value = p; loadLogs() }
function onPageSize(ps: number) { pageSize.value = ps; page.value = 1; loadLogs() }

onMounted(loadLogs)
</script>

<template>
  <HNBPageShell :title="t('system.audit.title')" :description="t('system.audit.desc')">
    <HNBToolbar>
      <HNBSelectInput v-model="actionFilter" :options="[
        { label: t('system.audit.filterAll'), value: 'all' },
        { label: t('system.audit.filterCreate'), value: 'create' },
        { label: t('system.audit.filterRead'), value: 'read' },
        { label: t('system.audit.filterUpdate'), value: 'update' },
        { label: t('system.audit.filterDelete'), value: 'delete' },
      ]" />
    </HNBToolbar>
    <HNBTable :columns="columns" :data="filteredLogs" :loading="loading" :pagination="pagination" row-key="id" :error="error" :empty-title="t('system.audit.empty')" @update:page="onPage" @update:page-size="onPageSize" @error-retry="loadLogs" />
  </HNBPageShell>
</template>