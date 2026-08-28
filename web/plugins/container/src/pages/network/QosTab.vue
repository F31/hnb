<script setup lang="ts">
/**
 * QosTab — QoS / 带宽策略。
 * 针对具体工作负载配置带宽限速（依赖 CNI 能力，能力不足则置灰提示）。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, HNBButton, HNBConfirmation, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import {
  containerCniFeatureAvailable,
  createQosPolicy,
  deleteQosPolicy,
  listQosPolicies,
  type QosBandwidthPolicy,
} from '../../api/containerNetworkApi'
import { getContainerContextStore } from '../../api/containerApi'
import NetworkDrawer from './NetworkDrawer.vue'

const { t } = useI18n()
const contextStore = getContainerContextStore()

const available = containerCniFeatureAvailable('qosBandwidth')
const items = ref<QosBandwidthPolicy[]>([])
const loading = ref(true)
const error = ref('')

// ---- 创建对话框 ----
const dialogVisible = ref(false)
const dialogBusy = ref(false)
const dialogError = ref('')
const formName = ref('')
const formWorkload = ref('')
const formIngress = ref('100M')
const formEgress = ref('50M')

// ---- 删除确认 ----
const confirmDelete = ref(false)
const deleteTarget = ref('')
const actionError = ref('')

const columns = computed<HNBTableColumn<QosBandwidthPolicy>[]>(() => [
  { key: 'name', title: t('container.network.qos.colName'), render: (r) => r.name || '--' },
  { key: 'workload', title: t('container.network.qos.colWorkload'), render: (r) => r.workload || '--' },
  { key: 'namespace', title: t('container.network.service.colNamespace'), render: (r) => r.namespace || '--' },
  { key: 'ingressBandwidth', title: t('container.network.qos.colIngress'), render: (r) => r.ingressBandwidth || '--' },
  { key: 'egressBandwidth', title: t('container.network.qos.colEgress'), render: (r) => r.egressBandwidth || '--' },
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
    items.value = await listQosPolicies('*')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  formName.value = ''
  formWorkload.value = ''
  formIngress.value = '100M'
  formEgress.value = '50M'
  dialogError.value = ''
  dialogVisible.value = true
}

async function onDialogConfirm(): Promise<void> {
  if (!formName.value.trim() || !formWorkload.value.trim()) {
    dialogError.value = t('container.network.qos.form.required')
    return
  }
  dialogBusy.value = true
  dialogError.value = ''
  try {
    await createQosPolicy({
      name: formName.value.trim(),
      workload: formWorkload.value.trim(),
      namespace: String(contextStore.current.spaceId ?? 'default'),
      ingressBandwidth: formIngress.value.trim() || '--',
      egressBandwidth: formEgress.value.trim() || '--',
    })
    dialogVisible.value = false
    await load()
  } catch (err) {
    dialogError.value = err instanceof Error ? err.message : String(err)
  } finally {
    dialogBusy.value = false
  }
}

function requestDelete(p: QosBandwidthPolicy): void {
  deleteTarget.value = p.name
  actionError.value = ''
  confirmDelete.value = true
}

async function onConfirmDelete(): Promise<void> {
  actionError.value = ''
  try {
    await deleteQosPolicy(deleteTarget.value)
    confirmDelete.value = false
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('container.network.tab.qos')">
    <div class="panel-toolbar">
      <HNBButton :disabled="!available" @click="openCreate">
        {{ t('container.network.qos.action.create') }}
      </HNBButton>
      <button class="secondary-button" type="button" @click="load">
        {{ t('container.network.action.refresh') }}
      </button>
    </div>

    <p v-if="!available" class="panel-status" role="status">{{ t('container.network.qos.notSupported') }}</p>
    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>
    <p v-if="loading" class="panel-status" role="status">{{ t('container.network.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('container.network.empty')"
      min-width="900px"
      :aria-label="t('container.network.tab.qos')"
    />

    <NetworkDrawer
      v-model="dialogVisible"
      :title="t('container.network.qos.dialog.title')"
      :busy="dialogBusy"
      :error="dialogError"
      @confirm="onDialogConfirm"
    >
      <form class="network-form" @submit.prevent="onDialogConfirm">
        <label class="form-field">
          <span>{{ t('container.network.qos.form.name') }}</span>
          <input v-model="formName" type="text" :placeholder="t('container.network.qos.form.namePlaceholder')" />
        </label>
        <label class="form-field">
          <span>{{ t('container.network.qos.form.workload') }}</span>
          <input v-model="formWorkload" type="text" :placeholder="t('container.network.qos.form.workloadPlaceholder')" />
        </label>
        <div class="form-row">
          <label class="form-field">
            <span>{{ t('container.network.qos.form.ingress') }}</span>
            <input v-model="formIngress" type="text" placeholder="100M" />
          </label>
          <label class="form-field">
            <span>{{ t('container.network.qos.form.egress') }}</span>
            <input v-model="formEgress" type="text" placeholder="50M" />
          </label>
        </div>
      </form>
    </NetworkDrawer>

    <HNBConfirmation
      v-model="confirmDelete"
      :title="t('container.network.action.deleteTitle')"
      :description="t('container.network.qos.deleteMessage', { name: deleteTarget })"
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
.form-field input {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
</style>
