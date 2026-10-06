<script setup lang="ts">
/**
 * PluginMarketPage — 资源 > 插件市场。
 * 展示平台可用插件（名称/版本/描述/分类/操作），支持安装与卸载（受 cluster:update 权限控制）。
 * 数据源：插件市场目录（开发 fixture；生产空态）。
 */
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBConfirmation, HNBDialog } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { getPluginMarketCatalog, installPlugin, uninstallPlugin } from './api/p4Api'
import { getClusterContextStore, getClusterPermissionStore, listClusters } from './api/clusterApi'
import { usePluginContext } from './composables/usePluginContext'
import { useClusterCapabilities } from './composables/useClusterCapabilities'
import type { CapabilityLevel, CniCapabilityMatrix, CniFeature } from './types/capability'
import type { ClusterSummary } from './types/cluster'
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
const categoryFilter = ref('')
const targetClusters = ref<ClusterSummary[]>([])
const selectedClusterId = ref(contextStore.current.clusterId ?? '')
const installDialogOpen = ref(false)
const installTarget = ref<MarketPlugin | null>(null)
const installing = ref(false)
let refreshTimer: number | undefined

/** 当前目标集群：插件市场是平台目录，安装状态必须按选中的集群计算。 */
const selectedCluster = computed(() => targetClusters.value.find((c) => c.clusterId === selectedClusterId.value) ?? null)

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

// ---- 卸载确认 ----
const confirmUninstall = ref(false)
const uninstallTarget = ref<MarketPlugin | null>(null)
const actionError = ref('')
const capabilityTarget = ref<CniCapabilityMatrix | null>(null)
const capabilityDialogOpen = ref(false)

const cniComparisonOrder = [
  { key: 'kube-ovn', label: 'Kube-OVN' },
  { key: 'cilium', label: 'Cilium' },
  { key: 'calico', label: 'Calico' },
]

function installedBadge(p: MarketPlugin): { label: string; semantic: `success` | `default` } {
  if (p.installPhase === 'pending') return { label: t('resource.clusterMgmt.pluginMarket.pending'), semantic: 'default' as const }
  if (p.installPhase === 'installing') return { label: t('resource.clusterMgmt.pluginMarket.installing'), semantic: 'default' as const }
  if (p.installPhase === 'degraded') return { label: t('resource.clusterMgmt.pluginMarket.failed'), semantic: 'default' as const }
  return p.installed || p.installPhase === 'ready'
    ? { label: t('resource.clusterMgmt.pluginMarket.installed'), semantic: 'success' as const }
    : { label: isNetworkPlugin(p) ? t('resource.clusterMgmt.pluginMarket.networkPlan') : t('resource.clusterMgmt.pluginMarket.notInstalled'), semantic: 'default' as const }
}

function normalized(value: string | undefined): string {
  return (value ?? '').trim().toLowerCase()
}

function pluginTitle(p: MarketPlugin): string {
  return p.displayName?.trim() || p.name
}

function uniquePlugins(items: MarketPlugin[]): MarketPlugin[] {
  const seen = new Map<string, MarketPlugin>()
  for (const item of items) {
    const key = normalized(item.name)
    if (!key) continue
    const current = seen.get(key)
    if (!current || (!current.version && item.version)) {
      seen.set(key, { ...item, installed: current?.installed || item.installed })
      continue
    }
    if (item.installed && !current.installed) seen.set(key, { ...current, installed: true })
    if (phaseRank(item.installPhase) > phaseRank(current.installPhase)) {
      seen.set(key, { ...current, installPhase: item.installPhase, installed: item.installed, lastError: item.lastError })
    }
  }
  return Array.from(seen.values()).sort((a, b) => pluginTitle(a).localeCompare(pluginTitle(b)))
}

const categories = computed(() => Array.from(
  new Set(plugins.value.map((p) => p.category).filter(Boolean)),
).sort((a, b) => a.localeCompare(b)))

const filteredPlugins = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return plugins.value.filter((p) => {
    if (categoryFilter.value && p.category !== categoryFilter.value) return false
    if (!kw) return true
    return [p.name, p.displayName, p.category, p.description, p.version]
      .some((v) => normalized(v).includes(kw))
  })
})

const installedCount = computed(() => plugins.value.filter((p) => p.installed).length)
const activeInstallCount = computed(() => plugins.value.filter((p) => p.installPhase === 'pending' || p.installPhase === 'installing').length)
const selectedClusterName = computed(() => selectedCluster.value?.displayName || selectedCluster.value?.clusterId || '')

function phaseRank(phase: string | undefined): number {
  if (phase === 'ready') return 4
  if (phase === 'installing') return 3
  if (phase === 'pending') return 2
  if (phase === 'degraded') return 1
  return 0
}

function isNetworkPlugin(p: MarketPlugin): boolean {
  const name = normalized(p.name)
  const category = normalized(p.category)
  const kind = normalized(p.kind)
  const provider = normalized(p.provider)
  return ['calico', 'cilium', 'kube-ovn', 'kubeovn', 'multus-sriov'].includes(name)
    || category === '网络'
    || category === 'network'
    || kind.startsWith('cni')
    || provider === 'cni'
}

function findCniCapability(p: MarketPlugin): CniCapabilityMatrix | null {
  const key = normalized(`${p.name} ${p.displayName ?? ''}`)
  return caps.overview.value?.cnis.find((cni) => {
    const cniKey = normalized(cni.cni)
    return key.includes(cniKey) || (cniKey === 'kube-ovn' && key.includes('kubeovn'))
  }) ?? null
}

const cniComparison = computed(() => {
  const cnis = caps.overview.value?.cnis ?? []
  return cniComparisonOrder
    .map((item) => {
      const cni = cnis.find((entry) => normalized(entry.cni) === item.key)
      // 版本直接取矩阵自带的 version，不再硬编码（避免与 fixture/后端数据矛盾）。
      return cni ? { ...cni, displayName: item.label } : null
    })
    .filter((item): item is CniCapabilityMatrix & { displayName: string } => Boolean(item))
})

function openCapability(p: MarketPlugin): void {
  capabilityTarget.value = findCniCapability(p)
  capabilityDialogOpen.value = true
}

const columns = computed<HNBTableColumn<MarketPlugin>[]>(() => [
  {
    key: 'name',
    title: t('resource.clusterMgmt.pluginMarket.colName'),
    render: (r) => h('div', { class: 'plugin-name-cell' }, [
      h('span', { class: 'plugin-title' }, pluginTitle(r)),
      r.displayName && r.displayName !== r.name ? h('span', { class: 'plugin-key' }, r.name) : null,
    ]),
  },
  { key: 'version', title: t('resource.clusterMgmt.pluginMarket.colVersion'), render: (r) => r.version || '--' },
  { key: 'description', title: t('resource.clusterMgmt.pluginMarket.colDesc'), render: (r) => r.description || '--' },
  {
    key: 'category',
    title: t('resource.clusterMgmt.pluginMarket.colCategory'),
    render: (r) => {
      return h('div', { class: 'category-cell' }, [
        r.category ? h('span', { class: 'category-pill' }, r.category) : '--',
        isNetworkPlugin(r)
          ? h(
            'button',
            { type: 'button', class: 'capability-link', onClick: () => openCapability(r) },
            t('resource.clusterMgmt.pluginMarket.viewCapability'),
          )
          : null,
      ])
    },
  },
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
      if (!canUpdate.value) {
        return h('span', { class: 'install-hint' }, t('resource.clusterMgmt.pluginMarket.noPermission'))
      }
      if (isNetworkPlugin(r)) {
        return h(
          'span',
          { class: 'install-hint' },
          t('resource.clusterMgmt.pluginMarket.cniManagedByCluster'),
        )
      }
      if (r.installPhase === 'pending' || r.installPhase === 'installing') {
        return h('span', { class: 'install-hint' }, t('resource.clusterMgmt.pluginMarket.installing'))
      }
      if (r.installPhase === 'degraded') {
        return h('div', { class: 'action-stack' }, [
          h('button', { type: 'button', class: 'text-action', onClick: () => requestInstall(r) }, t('resource.clusterMgmt.pluginMarket.retry')),
          r.lastError ? h('span', { class: 'install-error', title: r.lastError }, t('resource.clusterMgmt.pluginMarket.executorNotReady')) : null,
        ])
      }
      if (r.installed || r.installPhase === 'ready') {
        return h(
          'button',
          { type: 'button', class: 'text-action danger', onClick: () => requestUninstall(r) },
          t('resource.clusterMgmt.pluginMarket.uninstall'),
        )
      }
      return h(
        'button',
        { type: 'button', class: 'text-action', onClick: () => requestInstall(r) },
        t('resource.clusterMgmt.pluginMarket.install'),
      )
    },
  },
])

async function loadClusters(): Promise<void> {
  try {
    const res = await listClusters({ page: 1, pageSize: 200 })
    targetClusters.value = res.items.filter((c) => c.kind === 'kubernetes' || c.kind === 'edge')
    if (!selectedClusterId.value && targetClusters.value.length === 1) {
      selectedClusterId.value = targetClusters.value[0].clusterId
    }
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
    targetClusters.value = []
  }
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const all = await getPluginMarketCatalog(selectedClusterId.value || undefined)
    plugins.value = uniquePlugins(Array.isArray(all) ? all : [])
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    plugins.value = []
  } finally {
    loading.value = false
  }
}

function scheduleRefresh(): void {
  if (refreshTimer) window.clearInterval(refreshTimer)
  refreshTimer = window.setInterval(() => {
    if (!selectedClusterId.value) return
    if (plugins.value.some((p) => p.installPhase === 'pending' || p.installPhase === 'installing')) {
      void load()
    }
  }, 5000)
}

function requestInstall(p: MarketPlugin): void {
  if (!canUpdate.value) return
  if (isNetworkPlugin(p)) {
    actionError.value = t('resource.clusterMgmt.pluginMarket.cniInstallBlocked')
    return
  }
  installTarget.value = p
  actionError.value = ''
  installDialogOpen.value = true
}

async function doInstall(): Promise<void> {
  const target = installTarget.value
  if (!target || !canUpdate.value) return
  if (!selectedClusterId.value) {
    actionError.value = t('resource.clusterMgmt.pluginMarket.requireCluster')
    return
  }
  installing.value = true
  actionError.value = ''
  try {
    const receipt = await installPlugin(target.name, target.version, selectedClusterId.value)
    installDialogOpen.value = false
    pluginCtx.notify(t('resource.clusterMgmt.pluginMarket.installSubmittedMsg', {
      name: pluginTitle(target),
      cluster: selectedClusterName.value,
      status: receipt.status || 'pending',
    }))
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  } finally {
    installing.value = false
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
  if (!selectedClusterId.value) {
    actionError.value = t('resource.clusterMgmt.pluginMarket.requireCluster')
    return
  }
  actionError.value = ''
  try {
    await uninstallPlugin(target.name, selectedClusterId.value)
    confirmUninstall.value = false
    pluginCtx.notify(t('resource.clusterMgmt.pluginMarket.uninstalledMsg', { name: target.name, cluster: selectedClusterName.value }))
    await load()
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(async () => {
  await loadClusters()
  await load()
  caps.ensureLoaded()
  scheduleRefresh()
})

onBeforeUnmount(() => {
  if (refreshTimer) window.clearInterval(refreshTimer)
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

const capabilityDialogTitle = computed(() => t('resource.clusterMgmt.capability.title'))
</script>

<template>
  <div class="plugin-market-page">
    <header class="page-header">
      <h2 class="page-title">{{ t('resource.clusterMgmt.pluginMarket.title') }}</h2>
      <div class="toolbar">
        <select v-model="selectedClusterId" class="cluster-select" :aria-label="t('resource.clusterMgmt.pluginMarket.targetCluster')" @change="load">
          <option value="">{{ t('resource.clusterMgmt.pluginMarket.selectCluster') }}</option>
          <option v-for="cluster in targetClusters" :key="cluster.clusterId" :value="cluster.clusterId">
            {{ cluster.displayName || cluster.clusterId }}
          </option>
        </select>
        <input
          v-model="keyword"
          class="keyword-input"
          type="text"
          :placeholder="t('resource.clusterMgmt.pluginMarket.keywordPlaceholder')"
        />
        <select v-model="categoryFilter" class="category-select" :aria-label="t('resource.clusterMgmt.pluginMarket.categoryFilter')">
          <option value="">{{ t('resource.clusterMgmt.pluginMarket.allCategories') }}</option>
          <option v-for="category in categories" :key="category" :value="category">{{ category }}</option>
        </select>
        <button class="secondary-button" type="button" @click="load">
          {{ t('resource.clusterMgmt.action.refresh') }}
        </button>
      </div>
    </header>

    <section class="market-summary" :aria-label="t('resource.clusterMgmt.pluginMarket.summary')">
      <div class="summary-card">
        <span class="summary-value">{{ plugins.length }}</span>
        <span class="summary-label">{{ t('resource.clusterMgmt.pluginMarket.total') }}</span>
      </div>
      <div class="summary-card">
        <span class="summary-value">{{ installedCount }}</span>
        <span class="summary-label">{{ t('resource.clusterMgmt.pluginMarket.installedTotal') }}</span>
      </div>
      <div class="summary-card">
        <span class="summary-value">{{ activeInstallCount }}</span>
        <span class="summary-label">{{ t('resource.clusterMgmt.pluginMarket.activeTotal') }}</span>
      </div>
      <div class="summary-card">
        <span class="summary-value">{{ categories.length }}</span>
        <span class="summary-label">{{ t('resource.clusterMgmt.pluginMarket.categoryTotal') }}</span>
      </div>
      <div class="summary-card summary-card--wide">
        <span class="summary-value summary-value--text">{{ selectedClusterName || t('resource.clusterMgmt.pluginMarket.noTargetCluster') }}</span>
        <span class="summary-label">{{ t('resource.clusterMgmt.pluginMarket.currentTarget') }}</span>
      </div>
    </section>

    <p v-if="actionError" class="panel-status error" role="alert">{{ actionError }}</p>

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="filteredPlugins"
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

    <HNBDialog
      v-model="installDialogOpen"
      :title="t('resource.clusterMgmt.pluginMarket.installTitle')"
      :description="t('resource.clusterMgmt.pluginMarket.installDesc')"
      :close-label="t('resource.clusterMgmt.common.close')"
      :busy="installing"
      :error="actionError"
    >
      <div v-if="installTarget" class="install-dialog-body">
        <div class="install-plugin-card">
          <span class="plugin-title">{{ pluginTitle(installTarget) }}</span>
          <span class="plugin-key">{{ installTarget.name }} {{ installTarget.version || '' }}</span>
        </div>
        <label class="install-field">
          <span>{{ t('resource.clusterMgmt.pluginMarket.targetCluster') }}</span>
          <select v-model="selectedClusterId" :disabled="installing">
            <option value="">{{ t('resource.clusterMgmt.pluginMarket.selectCluster') }}</option>
            <option v-for="cluster in targetClusters" :key="cluster.clusterId" :value="cluster.clusterId">
              {{ cluster.displayName || cluster.clusterId }}
            </option>
          </select>
        </label>
        <p class="install-note">{{ t('resource.clusterMgmt.pluginMarket.installNote') }}</p>
      </div>
      <template #footer>
        <button class="secondary-button" type="button" :disabled="installing" @click="installDialogOpen = false">
          {{ t('resource.clusterMgmt.common.cancel') }}
        </button>
        <button class="primary-button" type="button" :disabled="installing || !selectedClusterId" @click="doInstall">
          {{ installing ? t('resource.clusterMgmt.common.submitting') : t('resource.clusterMgmt.pluginMarket.confirmInstall') }}
        </button>
      </template>
    </HNBDialog>

    <HNBDialog
      v-model="capabilityDialogOpen"
      :title="capabilityDialogTitle"
      :description="t('resource.clusterMgmt.capability.pluginDesc')"
      :close-label="t('resource.clusterMgmt.common.close')"
    >
      <section v-if="cniComparison.length" class="capability-section">
        <h3 class="capability-section-title">{{ t('resource.clusterMgmt.capability.comparison') }}</h3>
        <div class="capability-scroll">
          <table class="capability-table">
            <thead>
              <tr>
                <th class="cap-feature-col">{{ t('resource.clusterMgmt.capability.colFeature') }}</th>
                <th
                  v-for="cni in cniComparison"
                  :key="cni.cni"
                  :class="{ 'col-installed': isInstalledCni(cni), 'col-selected': capabilityTarget?.cni === cni.cni }"
                >
                  {{ cni.displayName }} {{ cni.version }}
                  <span v-if="isInstalledCni(cni)" class="cni-tag">{{ t('resource.clusterMgmt.capability.current') }}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in capabilityRows" :key="row.feature">
                <td class="cap-feature-col">{{ t(row.labelKey) }}</td>
                <td
                  v-for="cni in cniComparison"
                  :key="`${cni.cni}-${row.feature}`"
                  :class="{ 'col-installed': isInstalledCni(cni), 'col-selected': capabilityTarget?.cni === cni.cni }"
                >
                  <span class="cap-level" :class="levelBadge(cni.capabilities[row.feature]).cls">
                    {{ levelBadge(cni.capabilities[row.feature]).label }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <p v-else-if="caps.loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
      <p v-else class="panel-status" role="status">{{ t('resource.clusterMgmt.capability.empty') }}</p>
    </HNBDialog>
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
.category-select {
  padding: 6px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  min-width: 140px;
}
.secondary-button {
  padding: 7px 14px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  cursor: pointer;
}
.market-summary { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 10px; }
.summary-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
}
.summary-value { font-size: 22px; font-weight: 700; color: var(--hnb-color-text-primary, #12172a); }
.summary-value--text { font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.summary-label { font-size: 12px; color: var(--hnb-color-text-secondary, #5b6675); }
.cluster-select,
.install-field select {
  padding: 6px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  min-width: 180px;
}
.install-dialog-body { display: flex; flex-direction: column; gap: 14px; }
.install-plugin-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
}
.install-field { display: flex; flex-direction: column; gap: 6px; color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.install-note { margin: 0; color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.primary-button {
  padding: 7px 14px;
  border: 1px solid var(--hnb-color-primary, #2f6fed);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-primary, #2f6fed);
  color: #fff;
  cursor: pointer;
}
.primary-button:disabled,
.secondary-button:disabled { opacity: 0.5; cursor: not-allowed; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.capability-section-title {
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 600;
  color: var(--hnb-color-text-primary, #12172a);
}
.capability-scroll { overflow-x: auto; max-width: 100%; scrollbar-width: thin; scrollbar-color: var(--hnb-color-text-tertiary, #8a94a3) transparent; }
.capability-table {
  width: 100%;
  min-width: 720px;
  border-collapse: separate;
  border-spacing: 0;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  overflow: hidden;
}
.capability-table th,
.capability-table td {
  padding: 10px 12px;
  border-right: 1px solid var(--hnb-color-divider, #e2e7ef);
  border-bottom: 1px solid var(--hnb-color-divider, #e2e7ef);
  text-align: left;
  vertical-align: middle;
}
.capability-table th:last-child,
.capability-table td:last-child { border-right: 0; }
.capability-table tbody tr:last-child td { border-bottom: 0; }
.capability-table th {
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  font-weight: 600;
  white-space: nowrap;
}
.capability-table td { color: var(--hnb-color-text-secondary, #5b6675); }
.capability-table .cap-feature-col {
  width: 190px;
  color: var(--hnb-color-text-primary, #12172a);
  font-weight: 600;
}
.capability-table .col-installed { background: color-mix(in srgb, var(--hnb-color-primary, #2f6fed) 6%, transparent); }
.capability-table .col-selected { box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--hnb-color-primary, #2f6fed) 32%, transparent); }
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
:deep(.install-hint) { color: var(--hnb-color-text-tertiary, #8a94a3); font-size: 12px; }
:deep(.action-stack) { display: flex; flex-direction: column; align-items: flex-start; gap: 3px; }
:deep(.install-error) { max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--hnb-color-status-warning, #f79009); font-size: 12px; }
:deep(.plugin-name-cell) { display: flex; flex-direction: column; gap: 2px; }
:deep(.plugin-title) { font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
:deep(.plugin-key) { font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
:deep(.category-cell) { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
:deep(.category-pill) {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--hnb-color-primary, #2f6fed) 10%, transparent);
  color: var(--hnb-color-primary, #2f6fed);
  font-size: 12px;
}
:deep(.capability-link) {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 12px;
  padding: 0;
}
@media (max-width: 720px) {
  .market-summary { grid-template-columns: 1fr; }
}
</style>
