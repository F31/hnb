<script setup lang="ts">
/**
 * SubnetInfoTab — 网段信息（只读）。
 * 展示当前命名空间绑定的容器子网（来源资源层）；提供「申请新网段」入口。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, HNBButton } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { getNamespaceSubnets, requestSubnet, type NamespaceSubnetInfo } from '../../api/containerNetworkApi'
import { getContainerContextStore } from '../../api/containerApi'
import NetworkDrawer from './NetworkDrawer.vue'

const { t } = useI18n()
const contextStore = getContainerContextStore()

const items = ref<NamespaceSubnetInfo[]>([])
const loading = ref(true)
const error = ref('')

// ---- 申请对话框 ----
const dialogVisible = ref(false)
const dialogBusy = ref(false)
const dialogError = ref('')
const formCidr = ref('')
const requestSuccess = ref(false)

const currentNs = computed(() => String(contextStore.current.spaceId ?? 'default'))

const columns = computed<HNBTableColumn<NamespaceSubnetInfo>[]>(() => [
  { key: 'subnetName', title: t('container.network.subnet.colName'), render: (r) => r.subnetName || '--' },
  { key: 'cidr', title: t('container.network.subnet.colCidr'), render: (r) => r.cidr || '--' },
  { key: 'gateway', title: t('container.network.subnet.colGateway'), render: (r) => r.gateway || '--' },
  { key: 'cniType', title: t('container.network.subnet.colCni'), render: (r) => r.cniType || '--' },
  { key: 'mode', title: t('container.network.subnet.colMode'), render: (r) => r.mode || '--' },
  { key: 'usedIps', title: t('container.network.subnet.colUsed'), render: (r) => String(r.usedIps) },
  { key: 'totalIps', title: t('container.network.subnet.colTotal'), render: (r) => String(r.totalIps) },
  {
    key: 'utilization',
    title: t('container.network.subnet.colUtil'),
    render: (r) => `${totalIps(r) > 0 ? Math.round((r.usedIps / totalIps(r)) * 100) : 0}%`,
  },
])

function totalIps(r: NamespaceSubnetInfo): number {
  return Math.max(1, r.totalIps)
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getNamespaceSubnets('*')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

function openRequest(): void {
  formCidr.value = ''
  dialogError.value = ''
  requestSuccess.value = false
  dialogVisible.value = true
}

async function onDialogConfirm(): Promise<void> {
  if (!/^\d{1,3}(\.\d{1,3}){3}\/\d{1,2}$/.test(formCidr.value.trim())) {
    dialogError.value = t('container.network.subnet.request.cidrInvalid')
    return
  }
  dialogBusy.value = true
  dialogError.value = ''
  try {
    await requestSubnet({ namespace: currentNs.value, requestedCidr: formCidr.value.trim() })
    requestSuccess.value = true
    dialogBusy.value = false
  } catch (err) {
    dialogError.value = err instanceof Error ? err.message : String(err)
    dialogBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('container.network.tab.subnets')">
    <div class="panel-toolbar">
      <div class="toolbar-left">
        <HNBButton @click="openRequest">
          {{ t('container.network.subnet.request.create') }}
        </HNBButton>
        <span class="ns-hint">{{ t('container.network.subnet.currentNs', { ns: currentNs }) }}</span>
      </div>
      <button class="secondary-button" type="button" @click="load">
        {{ t('container.network.action.refresh') }}
      </button>
    </div>

    <p v-if="loading" class="panel-status" role="status">{{ t('container.network.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('container.network.subnet.empty')"
      min-width="900px"
      :aria-label="t('container.network.tab.subnets')"
    />
    <p class="readonly-hint">{{ t('container.network.subnet.readonlyHint') }}</p>

    <NetworkDrawer
      v-model="dialogVisible"
      :title="t('container.network.subnet.request.title')"
      :busy="dialogBusy"
      :error="dialogError"
      @confirm="onDialogConfirm"
    >
      <form class="network-form" @submit.prevent="onDialogConfirm">
        <p class="panel-status">{{ t('container.network.subnet.request.namespace', { ns: currentNs }) }}</p>
        <label class="form-field">
          <span>{{ t('container.network.subnet.request.cidr') }}</span>
          <input v-model="formCidr" type="text" placeholder="10.50.0.0/24" />
        </label>
        <p v-if="requestSuccess" class="request-success" role="status">{{ t('container.network.subnet.request.submitted') }}</p>
      </form>
    </NetworkDrawer>
  </section>
</template>

<style scoped>
.network-tab-panel { display: flex; flex-direction: column; gap: 10px; }
.panel-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.toolbar-left { display: flex; align-items: center; gap: 12px; }
.ns-hint { font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
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
.readonly-hint { margin: 0; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
.request-success { color: var(--hnb-color-status-success, #12b76a); font-size: 13px; margin: 0; }
.network-form { display: flex; flex-direction: column; gap: 12px; }
.form-field { display: flex; flex-direction: column; gap: 6px; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.form-field input {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
</style>
