<script setup lang="ts">
/**
 * SubnetsTab — 容器子网管理。
 * 列表：名称/CIDR/网关/CNI 类型/模式/绑定命名空间/状态/操作；
 * 顶部「创建子网」；操作：编辑/删除/分配命名空间。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBButton, HNBConfirmation, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { createSubnet, deleteSubnet, getSubnets, updateSubnet } from '../api/networkApi'
import { getClusterPermissionStore } from '../api/clusterApi'
import { usePluginContext } from '../composables/usePluginContext'
import ClusterDrawer from './ClusterDrawer.vue'
import type { ContainerSubnet } from '../types/network'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()

const items = ref<ContainerSubnet[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 抽屉（创建/编辑） ----
const drawerVisible = ref(false)
const drawerBusy = ref(false)
const drawerError = ref('')
const editingName = ref('')
const formName = ref('')
const formCidr = ref('')
const formGateway = ref('')
const formCniType = ref('Kube-OVN')
const formMode = ref<'overlay' | 'underlay'>('overlay')
const formNamespaces = ref<string[]>([])

const cniTypeOptions = ['Kube-OVN', 'Cilium', 'Calico']
const namespaceOptions = ['default', 'rd', 'ai', 'dr', 'test', 'vm-973fd1ef']

// ---- 删除确认 ----
const confirmDelete = ref(false)
const deleteTarget = ref('')
const actionError = ref('')

function statusBadge(s: ContainerSubnet): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = { available: 'success', exhausted: 'error', unknown: 'default' }
  return { label: t(`resource.clusterMgmt.network.subnet.status.${s.status}`), semantic: map[s.status] ?? 'default' }
}

const columns = computed<HNBTableColumn<ContainerSubnet>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.network.subnet.colName'), render: (r) => r.name || '--' },
  { key: 'cidr', title: t('resource.clusterMgmt.network.subnet.colCidr'), render: (r) => r.cidr || '--' },
  { key: 'gateway', title: t('resource.clusterMgmt.network.subnet.colGateway'), render: (r) => r.gateway || '--' },
  { key: 'cniType', title: t('resource.clusterMgmt.network.subnet.colCni'), render: (r) => r.cniType || '--' },
  { key: 'mode', title: t('resource.clusterMgmt.network.subnet.colMode'), render: (r) => t(`resource.clusterMgmt.network.subnet.mode.${r.mode}`) },
  { key: 'namespaces', title: t('resource.clusterMgmt.network.subnet.colNamespaces'), render: (r) => r.namespaces.join(', ') || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.network.subnet.colStatus'),
    render: (r) => {
      const b = statusBadge(r)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (r) => {
      const actions: HNBTableAction[] = [
        { label: t('resource.clusterMgmt.network.subnet.action.assign'), key: 'assign' },
        { label: t('resource.clusterMgmt.network.subnet.action.edit'), key: 'edit' },
        { label: t('resource.clusterMgmt.network.subnet.action.delete'), key: 'delete', variant: 'danger' },
      ]
      return h(HNBTableActions, { actions, disabled: !canUpdate.value, onAction: (key: string) => {
        if (key === 'assign') openEdit(r, true)
        else if (key === 'edit') openEdit(r, false)
        else if (key === 'delete') requestDelete(r)
      }})
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getSubnets({ keyword: keyword.value })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openEdit(s: ContainerSubnet, assignOnly: boolean): void {
  if (!canUpdate.value) return
  editingName.value = s.name
  formName.value = assignOnly ? s.name : s.name
  formCidr.value = s.cidr
  formGateway.value = s.gateway
  formCniType.value = s.cniType
  formMode.value = s.mode
  formNamespaces.value = [...s.namespaces]
  drawerError.value = ''
  drawerVisible.value = true
}

function openCreate(): void {
  if (!canUpdate.value) return
  editingName.value = ''
  formName.value = ''
  formCidr.value = ''
  formGateway.value = ''
  formCniType.value = 'Kube-OVN'
  formMode.value = 'overlay'
  formNamespaces.value = []
  drawerError.value = ''
  drawerVisible.value = true
}

async function onDrawerConfirm(): Promise<void> {
  if (!formName.value.trim() || !formCidr.value.trim()) {
    drawerError.value = t('resource.clusterMgmt.network.subnet.form.required')
    return
  }
  drawerBusy.value = true
  drawerError.value = ''
  const payload: ContainerSubnet = {
    name: formName.value.trim(),
    cidr: formCidr.value.trim(),
    gateway: formGateway.value.trim() || '--',
    cniType: formCniType.value,
    mode: formMode.value,
    namespaces: formNamespaces.value,
    status: 'available',
  }
  try {
    if (editingName.value) await updateSubnet(editingName.value, payload)
    else await createSubnet(payload)
    drawerVisible.value = false
    pluginCtx.notify(t('resource.clusterMgmt.network.subnet.saved'))
    await load()
  } catch (err) {
    drawerError.value = err instanceof Error ? err.message : String(err)
  } finally {
    drawerBusy.value = false
  }
}

function toggleNamespace(ns: string): void {
  const idx = formNamespaces.value.indexOf(ns)
  if (idx >= 0) formNamespaces.value.splice(idx, 1)
  else formNamespaces.value.push(ns)
}

function requestDelete(s: ContainerSubnet): void {
  if (!canUpdate.value) return
  deleteTarget.value = s.name
  actionError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  actionError.value = ''
  try {
    await deleteSubnet(deleteTarget.value)
    confirmDelete.value = false
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('resource.clusterMgmt.network.tab.subnets')">
    <div class="panel-toolbar">
      <HNBButton v-if="canUpdate" @click="openCreate">
        {{ t('resource.clusterMgmt.network.subnet.action.create') }}
      </HNBButton>
      <div class="toolbar-right">
        <input
          v-model="keyword"
          class="keyword-input"
          type="text"
          :placeholder="t('resource.clusterMgmt.network.keywordPlaceholder')"
          @keyup.enter="load"
        />
        <button class="secondary-button" type="button" @click="load">
          {{ t('resource.clusterMgmt.action.query') }}
        </button>
        <button class="secondary-button" type="button" @click="load">
          {{ t('resource.clusterMgmt.action.refresh') }}
        </button>
      </div>
    </div>

    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>
    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('resource.clusterMgmt.network.empty')"
      min-width="1180px"
      :aria-label="t('resource.clusterMgmt.network.tab.subnets')"
    />

    <ClusterDrawer
      v-model="drawerVisible"
      :title="editingName ? t('resource.clusterMgmt.network.subnet.drawer.editTitle') : t('resource.clusterMgmt.network.subnet.drawer.createTitle')"
      :busy="drawerBusy"
      :error="drawerError"
      @confirm="onDrawerConfirm"
    >
      <form class="network-form" @submit.prevent="onDrawerConfirm">
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.network.subnet.form.name') }}</span>
          <input v-model="formName" type="text" :disabled="!!editingName" :placeholder="t('resource.clusterMgmt.network.subnet.form.namePlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.network.subnet.form.cidr') }}</span>
          <input v-model="formCidr" type="text" placeholder="10.20.0.0/24" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.network.subnet.form.gateway') }}</span>
          <input v-model="formGateway" type="text" placeholder="10.20.0.1" />
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.network.subnet.form.cni') }}</span>
          <select v-model="formCniType">
            <option v-for="c in cniTypeOptions" :key="c" :value="c">{{ c }}</option>
          </select>
        </label>
        <label class="form-field">
          <span>{{ t('resource.clusterMgmt.network.subnet.form.mode') }}</span>
          <select v-model="formMode">
            <option value="overlay">{{ t('resource.clusterMgmt.network.subnet.mode.overlay') }}</option>
            <option value="underlay">{{ t('resource.clusterMgmt.network.subnet.mode.underlay') }}</option>
          </select>
        </label>
        <div class="form-field">
          <span class="field-title">{{ t('resource.clusterMgmt.network.subnet.form.namespaces') }}</span>
          <div class="checkbox-group">
            <label v-for="ns in namespaceOptions" :key="ns">
              <input type="checkbox" :checked="formNamespaces.includes(ns)" @change="toggleNamespace(ns)" />
              <span>{{ ns }}</span>
            </label>
          </div>
        </div>
      </form>
    </ClusterDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('resource.clusterMgmt.network.subnet.deleteTitle')"
      :description="t('resource.clusterMgmt.network.subnet.deleteMessage', { name: deleteTarget })"
      :error="actionError"
      danger
      @confirm="onConfirmDelete"
    />
  </section>
</template>

<style scoped>
.network-tab-panel { display: flex; flex-direction: column; gap: 10px; }
.panel-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
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
.network-form { display: flex; flex-direction: column; gap: 14px; }
.form-field { display: flex; flex-direction: column; gap: 6px; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.form-field input, .form-field select {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
.form-field input:disabled { opacity: 0.6; }
.field-title { color: var(--hnb-color-text-secondary, #5b6675); }
.checkbox-group { display: flex; flex-wrap: wrap; gap: 8px 16px; }
.checkbox-group label { display: flex; align-items: center; gap: 6px; cursor: pointer; }
.checkbox-group input { width: 14px; height: 14px; accent-color: var(--hnb-color-primary, #2f6fed); }
</style>
