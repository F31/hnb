<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBTable, HNBButton, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import * as api from '../systemApi'

const { t } = useI18n()

const extensions = ref<api.ExtensionRecord[]>([])
const loading = ref(false)
const error = ref('')

const columns: HNBTableColumn[] = [
  { key: 'name', title: t('system.extensions.colName'), render: (row) => h('strong', row.name) },
  { key: 'display_name', title: t('system.extensions.colDisplayName'), render: (row) => row.display_name || '-' },
  { key: 'version', title: t('system.extensions.colVersion') },
  { key: 'enabled', title: t('system.extensions.colStatus'), render: (row) => h(StatusBadge, { semantic: row.enabled ? 'success' : 'error', label: row.enabled ? t('system.extensions.enabled') : t('system.extensions.disabled') }) },
  { key: 'created_at', title: t('system.extensions.colCreatedAt'), render: (row) => row.created_at ? new Date(row.created_at).toLocaleDateString() : '-' },
]

async function loadExtensions() {
  loading.value = true
  error.value = ''
  try {
    extensions.value = await api.listExtensions()
  } catch (e: any) {
    error.value = e?.message || t('system.extensions.loadError')
  } finally {
    loading.value = false
  }
}

onMounted(loadExtensions)
</script>

<template>
  <HNBPageShell :title="t('system.extensions.title')" :description="t('system.extensions.desc')">
    <HNBTable :columns="columns" :data="extensions" :loading="loading" row-key="id" :error="error" :empty-title="t('system.extensions.empty')" @error-retry="loadExtensions" />
  </HNBPageShell>
</template>