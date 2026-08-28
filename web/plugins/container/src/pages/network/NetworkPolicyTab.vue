<script setup lang="ts">
/**
 * NetworkPolicyTab — 网络策略（仅作用于当前命名空间）。
 * 网络安全策略的唯一实操入口；按 CNI 能力探测决定可用性（不支持则置灰+提示）。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, HNBButton, HNBConfirmation, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import {
  containerCniFeatureAvailable,
  createNetworkPolicy,
  deleteNetworkPolicy,
  listNetworkPolicies,
  type ContainerNetworkPolicy,
} from '../../api/containerNetworkApi'
import { getContainerContextStore } from '../../api/containerApi'
import NetworkDrawer from './NetworkDrawer.vue'

const { t } = useI18n()
const contextStore = getContainerContextStore()
const CLUSTER_ID = '79eb7403-2e06-4502-901a-420e3c40cd55'

const available = containerCniFeatureAvailable('networkPolicy')
const items = ref<ContainerNetworkPolicy[]>([])
const loading = ref(true)
const error = ref('')

// ---- 创建对话框 ----
const dialogVisible = ref(false)
const dialogBusy = ref(false)
const dialogError = ref('')
const formName = ref('')
const formPodSelector = ref('')
const formPolicyTypes = ref<'ingress' | 'egress' | 'both'>('both')
const formIngressFrom = ref('')
const formEgressTo = ref('')

// ---- 删除确认 ----
const confirmDelete = ref(false)
const deleteTarget = ref('')
const actionError = ref('')

const columns = computed<HNBTableColumn<ContainerNetworkPolicy>[]>(() => [
  { key: 'name', title: t('container.network.np.colName'), render: (r) => r.name || '--' },
  { key: 'namespace', title: t('container.network.service.colNamespace'), render: (r) => r.namespace || '--' },
  { key: 'podSelector', title: t('container.network.np.colSelector'), render: (r) => r.podSelector || '--' },
  { key: 'policyTypes', title: t('container.network.np.colTypes'), render: (r) => r.policyTypes || '--' },
  { key: 'ingressFrom', title: t('container.network.np.colIngress'), render: (r) => r.ingressFrom || '--' },
  { key: 'egressTo', title: t('container.network.np.colEgress'), render: (r) => r.egressTo || '--' },
  { key: 'createdAt', title: t('container.network.service.colCreated'), render: (r) => r.createdAt || '--' },
  {
    key: 'actions',
    title: t('container.network.colActions'),
    render: (r) => {
      const actions: HNBTableAction[] = [{ label: t('container.network.action.delete'), key: 'delete', variant: 'danger' }]
      return h(HNBTableActions, { actions, disabled: !available, onAction: () => requestDelete(r) })
    },
  },
])

async function load(): Promise<void> {
  if (!available) return
  loading.value = true
  error.value = ''
  try {
    items.value = await listNetworkPolicies(CLUSTER_ID, '*')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  formName.value = ''
  formPodSelector.value = ''
  formPolicyTypes.value = 'both'
  formIngressFrom.value = ''
  formEgressTo.value = ''
  dialogError.value = ''
  dialogVisible.value = true
}

async function onDialogConfirm(): Promise<void> {
  if (!formName.value.trim() || !formPodSelector.value.trim()) {
    dialogError.value = t('container.network.np.form.required')
    return
  }
  dialogBusy.value = true
  dialogError.value = ''
  try {
    await createNetworkPolicy(CLUSTER_ID, {
      name: formName.value.trim(),
      namespace: String(contextStore.current.spaceId ?? 'default'),
      podSelector: formPodSelector.value.trim(),
      policyTypes: formPolicyTypes.value,
      ingressFrom: formIngressFrom.value.trim(),
      egressTo: formEgressTo.value.trim(),
    })
    dialogVisible.value = false
    await load()
  } catch (err) {
    dialogError.value = err instanceof Error ? err.message : String(err)
  } finally {
    dialogBusy.value = false
  }
}

function requestDelete(p: ContainerNetworkPolicy): void {
  deleteTarget.value = p.name
  actionError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  actionError.value = ''
  try {
    await deleteNetworkPolicy(CLUSTER_ID, deleteTarget.value, String(contextStore.current.spaceId ?? 'default'))
    confirmDelete.value = false
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('container.network.tab.networkPolicies')">
    <div class="panel-toolbar">
      <HNBButton :disabled="!available" @click="openCreate">
        {{ t('container.network.np.action.create') }}
      </HNBButton>
      <button class="secondary-button" type="button" @click="load">
        {{ t('container.network.action.refresh') }}
      </button>
    </div>

    <p v-if="!available" class="panel-status" role="status">{{ t('container.network.np.notSupported') }}</p>
    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>
    <p v-if="loading" class="panel-status" role="status">{{ t('container.network.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('container.network.empty')"
      min-width="1080px"
      :aria-label="t('container.network.tab.networkPolicies')"
    />

    <NetworkDrawer
      v-model="dialogVisible"
      :title="t('container.network.np.dialog.title')"
      :busy="dialogBusy"
      :error="dialogError"
      @confirm="onDialogConfirm"
    >
      <form class="network-form" @submit.prevent="onDialogConfirm">
        <label class="form-field">
          <span>{{ t('container.network.np.form.name') }}</span>
          <input v-model="formName" type="text" :placeholder="t('container.network.np.form.namePlaceholder')" />
        </label>
        <div class="form-row">
          <label class="form-field">
            <span>{{ t('container.network.np.form.selector') }}</span>
            <input v-model="formPodSelector" type="text" placeholder="app=web" />
          </label>
          <label class="form-field">
            <span>{{ t('container.network.np.form.types') }}</span>
            <select v-model="formPolicyTypes">
              <option value="ingress">Ingress</option>
              <option value="egress">Egress</option>
              <option value="both">Ingress + Egress</option>
            </select>
          </label>
        </div>
        <div class="form-row">
          <label class="form-field">
            <span>{{ t('container.network.np.form.ingressFrom') }}</span>
            <input v-model="formIngressFrom" type="text" placeholder="api" />
          </label>
          <label class="form-field">
            <span>{{ t('container.network.np.form.egressTo') }}</span>
            <input v-model="formEgressTo" type="text" placeholder="db" />
          </label>
        </div>
      </form>
    </NetworkDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('container.network.action.deleteTitle')"
      :description="t('container.network.np.deleteMessage', { name: deleteTarget })"
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
