<script setup lang="ts">
/**
 * NodeNicsTab — 节点详情 > 网卡（OpenSpec node-detail）。
 * 列：网卡名称与 MAC、状态（绿点可用/红点不可用）、类型、IP、当前速率。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { getNodeNics } from '../api/nodeApi'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import { useNodeDetailId } from '../composables/useNodeDetailContext'
import SectionHeader from './SectionHeader.vue'
import type { NodeNic } from '../types/node'

const { t } = useI18n()
const clusterId = useClusterDetailId()
const nodeId = useNodeDetailId()

const nics = ref<NodeNic[]>([])
const loading = ref(true)
const error = ref('')

function statusBadge(nic: NodeNic): any {
  return nic.status === 'available'
    ? { label: t('resource.clusterMgmt.nodeDetail.nic.available'), semantic: 'success' as const }
    : { label: t('resource.clusterMgmt.nodeDetail.nic.unavailable'), semantic: 'error' as const }
}

const columns = computed<HNBTableColumn<NodeNic>[]>(() => [
  {
    key: 'name',
    title: t('resource.clusterMgmt.nodeDetail.nic.name'),
    render: (row) => `${row.name} (${row.mac})`,
  },
  {
    key: 'status',
    title: t('resource.clusterMgmt.nodeDetail.nic.status'),
    render: (row) => {
      const b = statusBadge(row)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'type', title: t('resource.clusterMgmt.nodeDetail.nic.type'), render: (row) => row.type || '--' },
  { key: 'ip', title: t('resource.clusterMgmt.nodeDetail.nic.ip'), render: (row) => row.ip || '--' },
  { key: 'speed', title: t('resource.clusterMgmt.nodeDetail.nic.speed'), render: (row) => row.speed || '--' },
])

async function load(): Promise<void> {
  if (!clusterId || !nodeId) return
  loading.value = true
  error.value = ''
  try {
    nics.value = await getNodeNics(clusterId, nodeId)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    nics.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="node-nics" :aria-label="t('resource.clusterMgmt.aria.nodeNics')">
    <SectionHeader :title="t('resource.clusterMgmt.nodeDetail.nic.title')" />
    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="nics"
      :empty-title="t('resource.clusterMgmt.nodeDetail.nic.empty')"
      min-width="760px"
      :aria-label="t('resource.clusterMgmt.nodeDetail.nic.title')"
    />
  </section>
</template>

<style scoped>
.node-nics { display: flex; flex-direction: column; gap: 8px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
</style>
