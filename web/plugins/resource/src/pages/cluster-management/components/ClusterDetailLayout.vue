<script setup lang="ts">
/**
 * ClusterDetailLayout — 集群详情统一壳层（OpenSpec cluster-detail-shell）。
 *
 * 面包屑（返回/集群名）+ 左侧 5 项菜单（激活项浅紫背景 + 右侧紫色竖条）+
 * 顶部快捷操作（配置DNS/NTP/下载KubeConfig/帮助，权限控制）。
 * 子页面以默认 slot 包裹本组件渲染内容；clusterId 经 provide 注入子内容。
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getClusterDetail, downloadKubeConfig } from '../api/clusterDetailApi'
import { clusterPermissions, getClusterPermissionStore } from '../api/clusterApi'
import { usePluginContext } from '../composables/usePluginContext'
import { provideClusterDetailId } from '../composables/useClusterDetailContext'
import type { ClusterDetail } from '../types/cluster'
import AgentOnboardingGuide from './AgentOnboardingGuide.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()

const clusterId = computed(() => String(route.params.clusterId ?? ''))
provideClusterDetailId(clusterId.value)

const clusterName = ref('')
const clusterStale = ref(false)
const loadingName = ref(true)
const nameError = ref('')
const clusterDetail = ref<ClusterDetail | null>(null)

// 闭环恢复路径：仅「导入纳管」的 Kubernetes 集群在概览页展示 cluster-agent
// 接入指引（平台创建集群由控制面自动装机；边缘运行时走 CloudCore）。
const showAgentOnboarding = computed(() => {
  const d = clusterDetail.value
  if (!d) return false
	if (!permissionStore.hasPermission(clusterPermissions.execute) && !permissionStore.hasPermission('*')) return false
  if (d.clusterType !== 'kubernetes') return false
  if (d.source && d.source !== 'imported') return false
  return /\/overview$/.test(route.path)
})

// KubeConfig issuance can expose cluster credentials; a read-only viewer must
// not receive it merely because that viewer can inspect the resource model.
const canDownload = computed(() => permissionStore.hasPermission(clusterPermissions.execute) || permissionStore.hasPermission('*'))
const downloading = ref(false)
const downloadError = ref('')

async function loadClusterName(): Promise<void> {
  if (!clusterId.value) return
  loadingName.value = true
  nameError.value = ''
  try {
    const detail: ClusterDetail = await getClusterDetail(clusterId.value)
    clusterDetail.value = detail
    clusterName.value = detail?.name || ''
    clusterStale.value = detail?.stale === true
  } catch (err) {
    nameError.value = err instanceof Error ? err.message : String(err)
    clusterDetail.value = null
  } finally {
    loadingName.value = false
  }
}

watch(clusterId, loadClusterName, { immediate: true })
onBeforeUnmount(() => {
  /* runtime cleanup handled by child pages */
})

const navItems = computed(() => [
  { key: 'overview', label: t('resource.clusterMgmt.sideNav.clusterInfo'), base: 'overview' },
  { key: 'nodes', label: t('resource.clusterMgmt.sideNav.nodeList'), base: 'nodes' },
  ...(import.meta.env.VITE_FEATURE_RESOURCE_CLUSTER_ADVANCED === 'true'
    ? [
        { key: 'edge-node-groups', label: t('resource.clusterMgmt.sideNav.edgeNodeGroups'), base: 'edge-node-groups' },
        { key: 'tenant-allocations', label: t('resource.clusterMgmt.sideNav.tenantAllocation'), base: 'tenant-allocations' },
        { key: 'plugin-instances', label: t('resource.clusterMgmt.sideNav.pluginManagement'), base: 'plugin-instances' },
      ]
    : []),
])

function isActiveNav(base: string): boolean {
  const path = route.path
  if (base === 'overview') return /\/overview$|\/monitoring$/.test(path)
  return path.endsWith(`/${base}`)
}

function goBack(): void {
  pluginCtx.navigate('/resource/clusters')
}

function navigateTo(base: string): void {
  if (route.path.endsWith(`/${base}`)) return
  router.push(`/resource/clusters/${encodeURIComponent(clusterId.value)}/${base}`)
}

async function onDownloadKubeConfig(): Promise<void> {
  if (!canDownload.value || downloading.value) return
  downloading.value = true
  downloadError.value = ''
  try {
    await downloadKubeConfig(clusterId.value)
    pluginCtx.notify(t('resource.clusterMgmt.quick.downloadKubeConfigSuccess'))
  } catch (err) {
    downloadError.value = err instanceof Error ? err.message : String(err)
  } finally {
    downloading.value = false
  }
}

function onHelp(): void {
  if (typeof window !== 'undefined') {
    window.open('https://docs.hnb.example.io', '_blank', 'noopener,noreferrer')
  }
}

</script>

<template>
  <div class="cluster-detail-layout">
    <!-- 面包屑 -->
    <nav class="breadcrumb" :aria-label="t('resource.clusterMgmt.aria.breadcrumb')">
      <button class="back-link" type="button" @click="goBack">
        ← {{ t('resource.clusterMgmt.detail.back') }}
      </button>
      <span class="crumb-sep">/</span>
      <span class="crumb-current">
        {{ loadingName ? '…' : (clusterName || route.params.clusterId) }}
      </span>
      <span v-if="nameError" class="name-error" role="alert">{{ nameError }}</span>
    </nav>

    <div class="layout-body">
      <!-- 左侧菜单 -->
      <aside class="side-nav" :aria-label="t('resource.clusterMgmt.aria.clusterMenu')">
        <a
          v-for="item in navItems"
          :key="item.key"
          class="side-nav-item"
          :class="{ active: isActiveNav(item.base) }"
          :href="`/resource/clusters/${encodeURIComponent(clusterId)}/${item.base}`"
          @click.prevent="navigateTo(item.base)"
        >
          {{ item.label }}
        </a>
      </aside>

      <!-- 内容区 -->
      <main class="content-area">
        <div class="content-header">
          <h2 class="page-title">
            {{ t('resource.clusterMgmt.detail.title') }}
          </h2>
          <div class="quick-actions" :aria-label="t('resource.clusterMgmt.aria.quickActions')">
            <button
              v-if="canDownload"
              type="button"
              class="quick-btn"
              :disabled="downloading"
              @click="onDownloadKubeConfig"
            >
              {{ downloading ? t('resource.clusterMgmt.quick.downloading') : t('resource.clusterMgmt.quick.downloadKubeConfig') }}
            </button>
            <button type="button" class="quick-btn" @click="onHelp">
              {{ t('resource.clusterMgmt.quick.help') }}
            </button>
          </div>
        </div>
        <p v-if="downloadError" class="quick-error" role="alert">{{ downloadError }}</p>
        <p v-if="clusterStale" class="stale-banner" role="alert">
          {{ t('resource.clusterMgmt.nodes.staleHint') }}
        </p>

        <!-- 闭环恢复路径：导入纳管的 Kubernetes 集群可在此（重新）生成 cluster-agent 接入指引 -->
        <div v-if="showAgentOnboarding" class="agent-onboarding-detail-block">
          <AgentOnboardingGuide :cluster-id="clusterId" :cluster-name="clusterName" />
        </div>

        <slot />
      </main>
    </div>
  </div>
</template>

<style scoped>
.cluster-detail-layout {
  display: flex;
  flex-direction: column;
  gap: 12px;
  color: var(--hnb-color-text-primary, #12172a);
}
.breadcrumb {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--hnb-color-text-secondary, #5b6675);
}
.back-link {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 13px;
  padding: 2px 4px;
}
.crumb-sep { color: var(--hnb-color-text-tertiary, #8a94a3); }
.crumb-current { color: var(--hnb-color-text-primary, #12172a); font-weight: 600; }
.name-error { color: var(--hnb-color-status-danger, #f04438); }

.layout-body {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.side-nav {
  width: 216px;
  flex-shrink: 0;
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.side-nav-item {
  position: relative;
  display: block;
  padding: 10px 14px;
  border-radius: var(--hnb-radius-sm, 4px);
  color: var(--hnb-color-text-secondary, #5b6675);
  text-decoration: none;
  font-size: 14px;
}
.side-nav-item:hover { background: var(--hnb-color-bg-elevated, #f6f8fb); color: var(--hnb-color-text-primary, #12172a); }
.side-nav-item.active {
  background: var(--hnb-color-primary-soft, #eef4ff);
  color: var(--hnb-color-primary, #2f6fed);
  font-weight: 600;
}
.side-nav-item.active::after {
  content: '';
  position: absolute;
  right: 0;
  top: 8px;
  bottom: 8px;
  width: 4px;
  border-radius: 2px;
  background: var(--hnb-color-primary, #2f6fed);
}
.content-area {
  flex: 1;
  min-width: 0;
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 16px 20px;
}
.content-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}
.page-title { margin: 0; font-size: 18px; font-weight: 600; }
.quick-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.quick-btn {
  padding: 5px 12px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-surface, #fff);
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 13px;
  cursor: pointer;
}
.quick-btn:hover:not(:disabled) { color: var(--hnb-color-primary, #2f6fed); border-color: var(--hnb-color-primary, #2f6fed); }
.quick-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.quick-error { color: var(--hnb-color-status-danger, #f04438); font-size: 13px; margin: 0 0 8px; }
.stale-banner { margin: 0 0 12px; padding: 10px 12px; border: 1px solid var(--hnb-color-status-warning, #f79009); border-radius: var(--hnb-radius-sm, 4px); color: var(--hnb-color-status-warning, #f79009); font-size: 13px; }
.agent-onboarding-detail-block { margin-bottom: 16px; }
</style>
