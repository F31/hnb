<script setup lang="ts">
/**
 * ServicesTab — 服务管理（K8s Service）。
 * 四层服务发现与负载均衡；列表 + 创建（ClusterIP/NodePort/LoadBalancer）+ 删除。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, HNBButton, HNBConfirmation, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { createService, deleteService, listServices, type ContainerService, type ServiceType } from '../../api/containerNetworkApi'
import { getContainerContextStore } from '../../api/containerApi'
import NetworkDrawer from './NetworkDrawer.vue'

const { t } = useI18n()
const contextStore = getContainerContextStore()
const CLUSTER_ID = '79eb7403-2e06-4502-901a-420e3c40cd55'

const items = ref<ContainerService[]>([])
const loading = ref(true)
const error = ref('')

// ---- 创建对话框 ----
const dialogVisible = ref(false)
const dialogBusy = ref(false)
const dialogError = ref('')
const formName = ref('')
const formType = ref<ServiceType>('ClusterIP')
const formPort = ref(80)
const formTargetPort = ref(8080)

const typeOptions: ServiceType[] = ['ClusterIP', 'NodePort', 'LoadBalancer']

// ---- 删除确认 ----
const confirmDelete = ref(false)
const deleteTarget = ref('')
const actionError = ref('')

const columns = computed<HNBTableColumn<ContainerService>[]>(() => [
  { key: 'name', title: t('container.network.service.colName'), render: (r) => r.name || '--' },
  { key: 'type', title: t('container.network.service.colType'), render: (r) => r.type || '--' },
  { key: 'clusterIp', title: t('container.network.service.colClusterIp'), render: (r) => r.clusterIp || '--' },
  { key: 'ports', title: t('container.network.service.colPorts'), render: (r) => r.ports || '--' },
  { key: 'selector', title: t('container.network.service.colSelector'), render: (r) => r.selector || '--' },
  { key: 'namespace', title: t('container.network.service.colNamespace'), render: (r) => r.namespace || '--' },
  { key: 'createdAt', title: t('container.network.service.colCreated'), render: (r) => r.createdAt || '--' },
  {
    key: 'actions',
    title: t('container.network.colActions'),
    render: (r) => {
      const actions: HNBTableAction[] = [{ label: t('container.network.action.delete'), key: 'delete', variant: 'danger' }]
      return h(HNBTableActions, { actions, onAction: () => requestDelete(r) })
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await listServices(CLUSTER_ID, '*')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  formName.value = ''
  formType.value = 'ClusterIP'
  formPort.value = 80
  formTargetPort.value = 8080
  dialogError.value = ''
  dialogVisible.value = true
}

async function onDialogConfirm(): Promise<void> {
  if (!formName.value.trim()) {
    dialogError.value = t('container.network.service.form.nameRequired')
    return
  }
  dialogBusy.value = true
  dialogError.value = ''
  try {
    await createService(CLUSTER_ID, {
      name: formName.value.trim(),
      type: formType.value,
      port: formPort.value,
      targetPort: formTargetPort.value,
      namespace: String(contextStore.current.spaceId ?? 'default'),
    })
    dialogVisible.value = false
    await load()
  } catch (err) {
    dialogError.value = err instanceof Error ? err.message : String(err)
  } finally {
    dialogBusy.value = false
  }
}

function requestDelete(s: ContainerService): void {
  deleteTarget.value = s.name
  actionError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  actionError.value = ''
  try {
    await deleteService(CLUSTER_ID, deleteTarget.value, String(contextStore.current.spaceId ?? 'default'))
    confirmDelete.value = false
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('container.network.tab.services')">
    <div class="panel-toolbar">
      <HNBButton @click="openCreate">
        {{ t('container.network.service.action.create') }}
      </HNBButton>
      <button class="secondary-button" type="button" @click="load">
        {{ t('container.network.action.refresh') }}
      </button>
    </div>

    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>
    <p v-if="loading" class="panel-status" role="status">{{ t('container.network.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('container.network.empty')"
      min-width="1080px"
      :aria-label="t('container.network.tab.services')"
    />

    <NetworkDrawer
      v-model="dialogVisible"
      :title="t('container.network.service.dialog.title')"
      :busy="dialogBusy"
      :error="dialogError"
      @confirm="onDialogConfirm"
    >
      <form class="network-form" @submit.prevent="onDialogConfirm">
        <label class="form-field">
          <span>{{ t('container.network.service.form.name') }}</span>
          <input v-model="formName" type="text" :placeholder="t('container.network.service.form.namePlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('container.network.service.form.type') }}</span>
          <select v-model="formType">
            <option v-for="tp in typeOptions" :key="tp" :value="tp">{{ tp }}</option>
          </select>
        </label>
        <div class="form-row">
          <label class="form-field">
            <span>{{ t('container.network.service.form.port') }}</span>
            <input v-model.number="formPort" type="number" min="1" max="65535" />
          </label>
          <label class="form-field">
            <span>{{ t('container.network.service.form.targetPort') }}</span>
            <input v-model.number="formTargetPort" type="number" min="1" max="65535" />
          </label>
        </div>
      </form>
    </NetworkDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('container.network.action.deleteTitle')"
      :description="t('container.network.service.deleteMessage', { name: deleteTarget })"
      :error="actionError"
      danger
      @confirm="onConfirmDelete"
    />
  </section>
</template>

<style scoped>
.network-tab-panel { display: flex; flex-direction: column; gap: 10px; }
.panel-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
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
.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
</style>
