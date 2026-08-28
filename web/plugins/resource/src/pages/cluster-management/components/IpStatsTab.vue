<script setup lang="ts">
/**
 * IpStatsTab — IP 资源统计。
 * 列表：子网名称/CIDR/已分配命名空间数/分配率/已用IP/总IP/利用率/临界预警；
 * 临界子网提供「联动提醒」入口。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { getIpUsageStats } from '../api/networkApi'
import { usePluginContext } from '../composables/usePluginContext'
import type { IpUsageStat } from '../types/network'

const { t } = useI18n()
const pluginCtx = usePluginContext()

const items = ref<IpUsageStat[]>([])
const loading = ref(true)
const error = ref('')

const columns = computed<HNBTableColumn<IpUsageStat>[]>(() => [
  { key: 'subnetName', title: t('resource.clusterMgmt.network.ipStats.colName'), render: (r) => r.subnetName || '--' },
  { key: 'cidr', title: t('resource.clusterMgmt.network.ipStats.colCidr'), render: (r) => r.cidr || '--' },
  { key: 'allocatedNamespaces', title: t('resource.clusterMgmt.network.ipStats.colNamespaces'), render: (r) => String(r.allocatedNamespaces) },
  { key: 'allocationRate', title: t('resource.clusterMgmt.network.ipStats.colAllocRate'), render: (r) => `${r.allocationRate}%` },
  { key: 'usedIps', title: t('resource.clusterMgmt.network.ipStats.colUsed'), render: (r) => String(r.usedIps) },
  { key: 'totalIps', title: t('resource.clusterMgmt.network.ipStats.colTotal'), render: (r) => String(r.totalIps) },
  {
    key: 'utilization',
    title: t('resource.clusterMgmt.network.ipStats.colUtil'),
    render: (r) => {
      const util = Math.min(100, Math.max(0, r.utilization))
      return h('div', { class: 'util-cell' }, [
        h('div', { class: 'util-bar', style: { width: `${util}%` } }),
        h('span', { class: 'util-text' }, `${r.utilization}%`),
      ])
    },
  },
  {
    key: 'critical',
    title: t('resource.clusterMgmt.network.ipStats.colCritical'),
    render: (r) => h(StatusBadge, {
      label: r.critical ? t('resource.clusterMgmt.network.ipStats.critical') : t('resource.clusterMgmt.network.ipStats.normal'),
      semantic: r.critical ? ('error' as const) : ('success' as const),
    }),
  },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (r) => {
      const actions: HNBTableAction[] = [{ label: t('resource.clusterMgmt.network.ipStats.action.notify'), key: 'notify', disabled: !r.critical }]
      return h(HNBTableActions, { actions, onAction: () => pluginCtx.notify(t('resource.clusterMgmt.network.ipStats.notified', { name: r.subnetName })) })
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    items.value = await getIpUsageStats()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('resource.clusterMgmt.network.tab.ipStats')">
    <div class="panel-toolbar">
      <button class="secondary-button" type="button" @click="load">
        {{ t('resource.clusterMgmt.action.refresh') }}
      </button>
    </div>

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="items"
      :empty-title="t('resource.clusterMgmt.network.empty')"
      min-width="1180px"
      :aria-label="t('resource.clusterMgmt.network.tab.ipStats')"
    />
  </section>
</template>

<style scoped>
.network-tab-panel { display: flex; flex-direction: column; gap: 10px; }
.panel-toolbar { display: flex; justify-content: flex-end; }
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
:deep(.util-cell) { display: flex; align-items: center; gap: 8px; min-width: 140px; }
:deep(.util-bar) { height: 6px; border-radius: 3px; background: var(--hnb-color-primary, #2f6fed); }
:deep(.util-text) { font-size: 12px; color: var(--hnb-color-text-primary, #12172a); white-space: nowrap; }
</style>
