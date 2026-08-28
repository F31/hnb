<script setup lang="ts">
/**
 * ClusterNodeSummaryTable — 集群信息 > 节点资源摘要表（OpenSpec cluster-overview）。
 * 列：节点名称、角色、节点 ID、CPU、内存、GPU 资源、显存资源。
 * 无 GPU 的节点显示 `/`；数据经注入 DataSourceManager 走节点 Read Model。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable } from '@hnb/ui-kit'
import { useDataSourceManager } from '@hnb/schema-engine'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { mapNodeToSummary, formatCpuCores, formatMemoryGiB } from '../api/clusterDetailApi'
import { usePluginContext } from '../composables/usePluginContext'
import { deriveContextKey } from '../composables/usePluginContext'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import SectionHeader from './SectionHeader.vue'
import type { ClusterNodeInfo, NodeSummary } from '../types/cluster'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const clusterId = useClusterDetailId()
const dataSources = useDataSourceManager()

const nodes = ref<NodeSummary[]>([])
const loading = ref(true)
const error = ref('')

function roleLabel(role: string): string {
  const map: Record<string, string> = {
    worker: t('resource.clusterMgmt.nodeSummary.roleWorker'),
    controller: t('resource.clusterMgmt.nodeSummary.roleController'),
    'control-plane': t('resource.clusterMgmt.nodeSummary.roleController'),
    edge: t('resource.clusterMgmt.nodeSummary.roleEdge'),
  }
  return map[role] ?? role
}

const columns = computed<HNBTableColumn<NodeSummary>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.col.name'), render: (r) => r.name || '--' },
  { key: 'role', title: t('resource.clusterMgmt.nodeSummary.role'), render: (r) => roleLabel(r.role) },
  { key: 'id', title: t('resource.clusterMgmt.nodeSummary.nodeId'), render: (r) => r.id || '--' },
  { key: 'cpuCores', title: t('resource.clusterMgmt.nodeSummary.cpu'), render: (r) => formatCpuCores(r.cpuCores) },
  { key: 'memoryGiB', title: t('resource.clusterMgmt.nodeSummary.memory'), render: (r) => formatMemoryGiB(r.memoryGiB) },
  { key: 'gpuResource', title: t('resource.clusterMgmt.nodeSummary.gpu'), render: (r) => r.gpuResource || '/' },
  { key: 'vramGiB', title: t('resource.clusterMgmt.nodeSummary.vram'), render: (r) => (r.vramGiB != null ? formatMemoryGiB(r.vramGiB) : '/') },
])

async function load(): Promise<void> {
  if (!clusterId || !dataSources) return
  loading.value = true
  error.value = ''
  try {
    const res = await dataSources.fetchPaginated<ClusterNodeInfo>('resource.cluster.nodes', {
      params: { clusterId, page: 1, pageSize: 200 },
      contextKey: deriveContextKey(pluginCtx.contextStore.current),
    })
    nodes.value = (res?.items ?? []).map(mapNodeToSummary)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    nodes.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="node-summary-panel" aria-label="节点资源摘要">
    <SectionHeader :title="t('resource.clusterMgmt.nodeSummary.title')" />

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">
      <span>{{ error }}</span>
      <button class="retry-button" type="button" @click="load">{{ t('resource.clusterMgmt.action.retry') }}</button>
    </div>
    <HNBTable
      v-else
      :columns="columns"
      :data="nodes"
      :empty-title="t('resource.clusterMgmt.nodes.empty')"
      min-width="720px"
      :aria-label="t('resource.clusterMgmt.nodeSummary.title')"
    />
  </section>
</template>

<style scoped>
.node-summary-panel { display: flex; flex-direction: column; gap: 8px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.retry-button {
  margin-left: 8px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  padding: 2px 10px;
}
</style>
