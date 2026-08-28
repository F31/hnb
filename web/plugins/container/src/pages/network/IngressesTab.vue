<script setup lang="ts">
/**
 * IngressesTab — 应用路由（K8s Ingress）。
 * 七层路由规则（域名/路径/TLS）；列表 + 创建 + 删除。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, HNBButton, HNBConfirmation, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { createIngress, deleteIngress, listIngresses, type ContainerIngress } from '../../api/containerNetworkApi'
import { getContainerContextStore } from '../../api/containerApi'
import NetworkDrawer from './NetworkDrawer.vue'

const { t } = useI18n()
const contextStore = getContainerContextStore()
const CLUSTER_ID = '79eb7403-2e06-4502-901a-420e3c40cd55'

const items = ref<ContainerIngress[]>([])
const loading = ref(true)
const error = ref('')

// ---- 创建对话框 ----
const dialogVisible = ref(false)
const dialogBusy = ref(false)
const dialogError = ref('')
const formName = ref('')
const formDomain = ref('')
const formPath = ref('/')
const formBackendService = ref('')
const formBackendPort = ref(80)
const formTls = ref(true)

// ---- 删除确认 ----
const confirmDelete = ref(false)
const deleteTarget = ref('')
const actionError = ref('')

const columns = computed<HNBTableColumn<ContainerIngress>[]>(() => [
  { key: 'name', title: t('container.network.ingress.colName'), render: (r) => r.name || '--' },
  { key: 'domain', title: t('container.network.ingress.colDomain'), render: (r) => r.domain || '--' },
  { key: 'path', title: t('container.network.ingress.colPath'), render: (r) => r.path || '--' },
  { key: 'backendService', title: t('container.network.ingress.colBackend'), render: (r) => `${r.backendService}:${r.backendPort}` || '--' },
  {
    key: 'tls',
    title: t('container.network.ingress.colTls'),
    render: (r) => h('span', { class: r.tls ? 'tag-tls' : 'tag-plain' }, r.tls ? t('container.network.ingress.tlsEnabled') : t('container.network.ingress.tlsDisabled')),
  },
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
    items.value = await listIngresses(CLUSTER_ID, '*')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  formName.value = ''
  formDomain.value = ''
  formPath.value = '/'
  formBackendService.value = ''
  formBackendPort.value = 80
  formTls.value = true
  dialogError.value = ''
  dialogVisible.value = true
}

async function onDialogConfirm(): Promise<void> {
  if (!formName.value.trim() || !formDomain.value.trim() || !formBackendService.value.trim()) {
    dialogError.value = t('container.network.ingress.form.required')
    return
  }
  dialogBusy.value = true
  dialogError.value = ''
  try {
    await createIngress(CLUSTER_ID, {
      name: formName.value.trim(),
      domain: formDomain.value.trim(),
      path: formPath.value.trim() || '/',
      backendService: formBackendService.value.trim(),
      backendPort: formBackendPort.value,
      tls: formTls.value,
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

function requestDelete(i: ContainerIngress): void {
  deleteTarget.value = i.name
  actionError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  actionError.value = ''
  try {
    await deleteIngress(CLUSTER_ID, deleteTarget.value, String(contextStore.current.spaceId ?? 'default'))
    confirmDelete.value = false
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('container.network.tab.ingresses')">
    <div class="panel-toolbar">
      <HNBButton @click="openCreate">
        {{ t('container.network.ingress.action.create') }}
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
      :aria-label="t('container.network.tab.ingresses')"
    />

    <NetworkDrawer
      v-model="dialogVisible"
      :title="t('container.network.ingress.dialog.title')"
      :busy="dialogBusy"
      :error="dialogError"
      @confirm="onDialogConfirm"
    >
      <form class="network-form" @submit.prevent="onDialogConfirm">
        <label class="form-field">
          <span>{{ t('container.network.ingress.form.name') }}</span>
          <input v-model="formName" type="text" :placeholder="t('container.network.ingress.form.namePlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('container.network.ingress.form.domain') }}</span>
          <input v-model="formDomain" type="text" placeholder="app.hnb.local" />
        </label>
        <div class="form-row">
          <label class="form-field">
            <span>{{ t('container.network.ingress.form.path') }}</span>
            <input v-model="formPath" type="text" placeholder="/" />
          </label>
          <label class="form-field">
            <span>{{ t('container.network.ingress.form.backend') }}</span>
            <input v-model="formBackendService" type="text" :placeholder="t('container.network.ingress.form.backendPlaceholder')" />
          </label>
        </div>
        <div class="form-row">
          <label class="form-field">
            <span>{{ t('container.network.ingress.form.backendPort') }}</span>
            <input v-model.number="formBackendPort" type="number" min="1" max="65535" />
          </label>
          <label class="form-field switch-field">
            <input v-model="formTls" type="checkbox" />
            <span>{{ t('container.network.ingress.form.tls') }}</span>
          </label>
        </div>
      </form>
    </NetworkDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('container.network.action.deleteTitle')"
      :description="t('container.network.ingress.deleteMessage', { name: deleteTarget })"
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
.switch-field { flex-direction: row; align-items: center; gap: 8px; }
.switch-field input { width: 16px; height: 16px; accent-color: var(--hnb-color-primary, #2f6fed); }
:deep(.tag-tls) { color: var(--hnb-color-primary, #2f6fed); }
:deep(.tag-plain) { color: var(--hnb-color-text-tertiary, #8a94a3); }
</style>
