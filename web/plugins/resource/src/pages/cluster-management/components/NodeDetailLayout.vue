<script setup lang="ts">
/**
 * NodeDetailLayout — 节点详情页签壳层（OpenSpec node-detail）。
 * 面包屑「节点列表 / 节点名称」+ 六个页签（基础配置/节点监控/磁盘/网卡/容器组/虚拟机），
 * 激活页签紫色文字+下划线；内容以默认 slot 包裹。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getNodeDetail } from '../api/nodeApi'
import { usePluginContext } from '../composables/usePluginContext'
import { provideNodeDetailName, useNodeDetailId } from '../composables/useNodeDetailContext'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const pluginCtx = usePluginContext()
const nodeId = useNodeDetailId()

const nodeName = ref('')
const loadingName = ref(true)

async function loadNodeName(): Promise<void> {
  if (!nodeId) return
  loadingName.value = true
  try {
    const detail = await getNodeDetail(String(route.params.clusterId ?? ''), nodeId)
    nodeName.value = detail?.name ?? nodeId
  } catch {
    nodeName.value = nodeId
  } finally {
    loadingName.value = false
  }
}

provideNodeDetailName(nodeName.value)

watch(
  () => [nodeId, route.params.clusterId] as const,
  () => loadNodeName(),
  { immediate: true },
)

const tabs = computed(() => [
  { key: 'basic', label: t('resource.clusterMgmt.nodeDetail.tab.basic'), suffix: 'basic' },
  { key: 'monitoring', label: t('resource.clusterMgmt.nodeDetail.tab.monitoring'), suffix: 'monitoring' },
  { key: 'disks', label: t('resource.clusterMgmt.nodeDetail.tab.disks'), suffix: 'disks' },
  { key: 'nics', label: t('resource.clusterMgmt.nodeDetail.tab.nics'), suffix: 'nics' },
  { key: 'pods', label: t('resource.clusterMgmt.nodeDetail.tab.pods'), suffix: 'pods' },
  { key: 'virtual-machines', label: t('resource.clusterMgmt.nodeDetail.tab.virtualMachines'), suffix: 'virtual-machines' },
])

function isActive(suffix: string): boolean {
  return route.path.endsWith(`/${suffix}`)
}

function navigateTo(suffix: string): void {
  if (isActive(suffix)) return
  router.push(`/resource/clusters/${encodeURIComponent(String(route.params.clusterId ?? ''))}/nodes/${encodeURIComponent(nodeId)}/${suffix}`)
}

function goBackToNodes(): void {
  pluginCtx.navigate(`/resource/clusters/${encodeURIComponent(String(route.params.clusterId ?? ''))}/nodes`)
}
</script>

<template>
  <div class="node-detail-layout">
    <nav class="node-breadcrumb" :aria-label="t('resource.clusterMgmt.aria.breadcrumb')">
      <button class="back-link" type="button" @click="goBackToNodes">
        {{ t('resource.clusterMgmt.nodeDetail.breadcrumb.nodes') }}
      </button>
      <span class="crumb-sep">/</span>
      <span class="crumb-current">{{ loadingName ? '…' : nodeName }}</span>
    </nav>

    <nav class="node-tabs" :aria-label="t('resource.clusterMgmt.aria.nodeTabs')">
      <a
        v-for="tab in tabs"
        :key="tab.key"
        class="node-tab"
        :class="{ active: isActive(tab.suffix) }"
        :href="`/resource/clusters/${encodeURIComponent(String(route.params.clusterId ?? ''))}/nodes/${encodeURIComponent(nodeId)}/${tab.suffix}`"
        @click.prevent="navigateTo(tab.suffix)"
      >
        {{ tab.label }}
      </a>
    </nav>

    <slot />
  </div>
</template>

<style scoped>
.node-detail-layout { display: flex; flex-direction: column; gap: 12px; }
.node-breadcrumb {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--hnb-color-text-secondary, #5b6675);
}
.back-link { border: 0; background: transparent; color: var(--hnb-color-primary, #2f6fed); cursor: pointer; font-size: 13px; padding: 2px 4px; }
.crumb-sep { color: var(--hnb-color-text-tertiary, #8a94a3); }
.crumb-current { color: var(--hnb-color-text-primary, #12172a); font-weight: 600; }
.node-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--hnb-color-border, #e2e7ef);
  flex-wrap: wrap;
}
.node-tab {
  position: relative;
  padding: 8px 16px;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 14px;
  text-decoration: none;
  cursor: pointer;
}
.node-tab:hover { color: var(--hnb-color-primary, #2f6fed); }
.node-tab.active { color: var(--hnb-color-primary, #2f6fed); font-weight: 600; }
.node-tab.active::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: -1px;
  height: 2px;
  background: var(--hnb-color-primary, #2f6fed);
}
</style>
