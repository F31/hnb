<script setup lang="ts">
/**
 * PluginMarketPage — 资源 > 插件市场。
 * 展示平台可用插件（名称/版本/描述/分类/操作），支持安装与卸载（受 cluster:update 权限控制）。
 * 数据源：插件市场目录（开发 fixture；生产空态）。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBConfirmation, HNBButton } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { getPluginMarketCatalog, installPlugin, uninstallPlugin } from './api/p4Api'
import { getClusterContextStore, getClusterPermissionStore } from './api/clusterApi'
import { usePluginContext } from './composables/usePluginContext'
import { useClusterCapabilities } from './composables/useClusterCapabilities'
import type { CapabilityLevel, CniCapabilityMatrix, CniFeature } from './types/capability'
import type { MarketPlugin } from './types/p4'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()
const contextStore = getClusterContextStore()
const caps = useClusterCapabilities()

const plugins = ref<MarketPlugin[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

/** 当前目标集群（进入集群详情时 context 已带 clusterId；无则留空） */
const clusterId = computed(() => contextStore.current.clusterId ?? '')

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 卸载确认 ----
const confirmUninstall = ref(false)
const uninstallTarget = ref<MarketPlugin | null>(null)
const actionError = ref('')

function installedBadge(p: MarketPlugin): { label: string; semantic: `success` | `default` } {
  return p.installed
    ? { label: t('resource.clusterMgmt.pluginMarket.installed'), semantic: 'success' as const }
    : { label: t('resource.clusterMgmt.pluginMarket.notInstalled'), semantic: 'default' as const }
}

const columns = computed<HNBTableColumn<MarketPlugin>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.pluginMarket.colName'), render: (r) => r.name || '--' },
  { key: 'version', title: t('resource.clusterMgmt.pluginMarket.colVersion'), render: (r) => r.version || '--' },
  { key: 'description', title: t('resource.clusterMgmt.pluginMarket.colDesc'), render: (r) => r.description || '--' },
  { key: 'category', title: t('resource.clusterMgmt.pluginMarket.colCategory'), render: (r) => r.category || '--' },
  {
    key: 'installed',
    title: t('resource.clusterMgmt.pluginMarket.colStatus'),
    render: (r) => {
      const b = installedBadge(r)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (r) => {
      if (r.installed) {
        return h(
          'button',
          { type: 'button', class: 'text-action danger', disabled: !canUpdate.value, onClick: () => requestUninstall(r) },
          t('resource.clusterMgmt.pluginMarket.uninstall'),
        )
      }
      return h(
        'button',
        { type: 'button', class: 'text-action', disabled: !canUpdate.value, onClick: () => doInstall(r) },
        t('resource.clusterMgmt.pluginMarket.install'),
      )
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const all = await getPluginMarketCatalog(clusterId.value || undefined)
    const kw = keyword.value.trim().toLowerCase()
    plugins.value = kw
      ? all.filter((p) => p.name.toLowerCase().includes(kw) || p.category.toLowerCase().includes(kw))
      : all
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    plugins.value = []
  } finally {
    loading.value = false
  }
}

async function doInstall(p: MarketPlugin): Promise<void> {
  if (!canUpdate.value) return
  if (!clusterId.value) {
    actionError.value = t('resource.clusterMgmt.pluginMarket.requireCluster')
    return
  }
  actionError.value = ''
  try {
    await installPlugin(p.name, p.version, clusterId.value)
    pluginCtx.notify(t('resource.clusterMgmt.pluginMarket.installedMsg', { name: p.name }))
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

function requestUninstall(p: MarketPlugin): void {
  if (!canUpdate.value) return
  uninstallTarget.value = p
  actionError.value = ''
  confirmUninstall.value = true
}

async function onConfirmUninstall(): Promise<void> {
  const target = uninstallTarget.value
  if (!target) return
  if (!clusterId.value) {
    actionError.value = t('resource.clusterMgmt.pluginMarket.requireCluster')
    return
  }
  actionError.value = ''
  try {
    await uninstallPlugin(target.name, clusterId.value)
    confirmUninstall.value = false
    pluginCtx.notify(t('resource.clusterMgmt.pluginMarket.uninstalledMsg', { name: target.name }))
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(() => {
  load()
  caps.ensureLoaded()
})

/** CNI 能力矩阵行：特性 → 标签 key */
const capabilityRows: { feature: CniFeature; labelKey: string }[] = [
  { feature: 'networkPolicy', labelKey: 'resource.clusterMgmt.capability.feature.networkPolicy' },
  { feature: 'serviceLoadBalancing', labelKey: 'resource.clusterMgmt.capability.feature.serviceLb' },
  { feature: 'qosBandwidth', labelKey: 'resource.clusterMgmt.capability.feature.qos' },
  { feature: 'subnetIsolation', labelKey: 'resource.clusterMgmt.capability.feature.subnet' },
  { feature: 'rdma', labelKey: 'resource.clusterMgmt.capability.feature.rdma' },
  { feature: 'observability', labelKey: 'resource.clusterMgmt.capability.feature.observability' },
  { feature: 'networkAnomalyDetection', labelKey: 'resource.clusterMgmt.capability.feature.anomaly' },
  { feature: 'diagnosis', labelKey: 'resource.clusterMgmt.capability.feature.diagnosis' },
]

/** 能力级别 → 文案/语义 */
function levelBadge(level: CapabilityLevel): { label: string; cls: string } {
  const map: Record<CapabilityLevel, { label: string; cls: string }> = {
    strong: { label: t('resource.clusterMgmt.capability.level.strong'), cls: 'lv-strong' },
    medium: { label: t('resource.clusterMgmt.capability.level.medium'), cls: 'lv-medium' },
    weak: { label: t('resource.clusterMgmt.capability.level.weak'), cls: 'lv-weak' },
    none: { label: t('resource.clusterMgmt.capability.level.none'), cls: 'lv-none' },
  }
  return map[level]
}

function isInstalledCni(cni: CniCapabilityMatrix): boolean {
  return caps.overview.value?.installedCni === cni.cni
}
</script>

<template>
  <div class="plugin-market-page">
    <header class="page-header">
      <h2 class="page-title">{{ t('resource.clusterMgmt.pluginMarket.title') }}</h2>
      <div class="toolbar">
        <input
          v-model="keyword"
          class="keyword-input"
          type="text"
          :placeholder="t('resource.clusterMgmt.pluginMarket.keywordPlaceholder')"
          @keyup.enter="load"
        />
        <button class="secondary-button" type="button" @click="load">
          {{ t('resource.clusterMgmt.action.query') }}
        </button>
        <button class="secondary-button" type="button" @click="load">
          {{ t('resource.clusterMgmt.action.refresh') }}
        </button>
      </div>
    </header>

    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>

    <!-- CNI 插件能力矩阵 -->
    <section v-if="caps.overview.value?.cnis.length" class="capability-card" :aria-label="t('resource.clusterMgmt.capability.title')">
      <header class="capability-head">
        <h3 class="capability-title">{{ t('resource.clusterMgmt.capability.title') }}</h3>
        <span v-if="caps.overview.value?.installedCni" class="capability-installed">
          {{ t('resource.clusterMgmt.capability.installed', { cni: caps.overview.value.installedCni }) }}
        </span>
      </header>
      <div class="capability-scroll">
        <table class="capability-table">
          <thead>
            <tr>
              <th class="cap-feature-col">{{ t('resource.clusterMgmt.capability.colFeature') }}</th>
              <th v-for="cni in caps.overview.value.cnis" :key="cni.cni" :class="{ 'col-installed': isInstalledCni(cni) }">
                {{ cni.cni }}
                <span v-if="isInstalledCni(cni)" class="cni-tag">{{ t('resource.clusterMgmt.capability.current') }}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in capabilityRows" :key="row.feature">
              <td class="cap-feature-col">{{ t(row.labelKey) }}</td>
              <td v-for="cni in caps.overview.value.cnis" :key="`${cni.cni}-${row.feature}`" :class="{ 'col-installed': isInstalledCni(cni) }">
                <span class="cap-level" :class="levelBadge(cni.capabilities[row.feature]).cls">
                  {{ levelBadge(cni.capabilities[row.feature]).label }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <p v-else-if="!caps.loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.capability.empty') }}</p>

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="plugins"
      :empty-title="t('resource.clusterMgmt.pluginMarket.empty')"
      min-width="900px"
      :aria-label="t('resource.clusterMgmt.pluginMarket.title')"
    />

    <HNBConfirmation
      v-model="confirmUninstall"
      :title="t('resource.clusterMgmt.pluginMarket.uninstallTitle')"
      :description="t('resource.clusterMgmt.pluginMarket.uninstallMessage', { name: uninstallTarget?.name ?? '' })"
      :error="actionError"
      danger
      @confirm="onConfirmUninstall"
    />
  </div>
</template>

<style scoped>
.plugin-market-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 18px 20px;
}
.page-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.page-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.toolbar { display: flex; gap: 8px; flex-wrap: wrap; }
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
.capability-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 14px 16px;
  background: var(--hnb-color-bg-elevated, #f6f8fb);
}
.capability-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.capability-title { margin: 0; font-size: 15px; font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.capability-installed { font-size: 13px; color: var(--hnb-color-primary, #2f6fed); }
.capability-scroll { overflow-x: auto; max-width: 100%; scrollbar-width: thin; scrollbar-color: var(--hnb-color-text-tertiary, #8a94a3) transparent; }
.capability-table { border-collapse: collapse; width: 100%; min-width: 720px; }
.capability-table th, .capability-table td {
  padding: 8px 12px;
  font-size: 13px;
  border-bottom: 1px solid var(--hnb-color-divider, #e2e7ef);
  text-align: left;
  white-space: nowrap;
}
.capability-table th { font-weight: 600; color: var(--hnb-color-text-secondary, #5b6675); background: var(--hnb-color-bg-surface, #fff); }
.capability-table .cap-feature-col { color: var(--hnb-color-text-primary, #12172a); }
.capability-table .col-installed { background: color-mix(in srgb, var(--hnb-color-primary, #2f6fed) 8%, transparent); }
.cni-tag {
  margin-left: 6px;
  padding: 1px 8px;
  border-radius: 10px;
  background: var(--hnb-color-primary, #2f6fed);
  color: #fff;
  font-size: 11px;
}
.cap-level { display: inline-block; padding: 2px 10px; border-radius: 10px; font-size: 12px; }
.cap-level.lv-strong { background: color-mix(in srgb, var(--hnb-color-status-success, #12b76a) 18%, transparent); color: var(--hnb-color-status-success, #12b76a); }
.cap-level.lv-medium { background: color-mix(in srgb, var(--hnb-color-primary, #2f6fed) 15%, transparent); color: var(--hnb-color-primary, #2f6fed); }
.cap-level.lv-weak { background: color-mix(in srgb, var(--hnb-color-status-warning, #f79009) 16%, transparent); color: var(--hnb-color-status-warning, #f79009); }
.cap-level.lv-none { background: var(--hnb-color-bg-surface, #fff); color: var(--hnb-color-text-tertiary, #8a94a3); }
:deep(.text-action) {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 13px;
  padding: 2px 6px;
}
:deep(.text-action.danger) { color: var(--hnb-color-status-danger, #f04438); }
:deep(.text-action:disabled) { opacity: 0.5; cursor: not-allowed; }
</style>
