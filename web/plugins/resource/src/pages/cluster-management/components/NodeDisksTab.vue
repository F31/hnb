<script setup lang="ts">
/**
 * NodeDisksTab — 节点详情 > 磁盘（OpenSpec node-detail）。
 * 列：磁盘（图标+名称）、类型、型号、容量、挂载点。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { getNodeDisks } from '../api/nodeApi'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import { useNodeDetailId } from '../composables/useNodeDetailContext'
import SectionHeader from './SectionHeader.vue'
import type { NodeDisk } from '../types/node'

const { t } = useI18n()
const clusterId = useClusterDetailId()
const nodeId = useNodeDetailId()

const disks = ref<NodeDisk[]>([])
const loading = ref(true)
const error = ref('')

const columns = computed<HNBTableColumn<NodeDisk>[]>(() => [
  {
    key: 'name',
    title: t('resource.clusterMgmt.nodeDetail.disk.name'),
    render: (row) => `💾 ${row.name}`,
  },
  { key: 'type', title: t('resource.clusterMgmt.nodeDetail.disk.type') },
  { key: 'model', title: t('resource.clusterMgmt.nodeDetail.disk.model') },
  { key: 'capacity', title: t('resource.clusterMgmt.nodeDetail.disk.capacity') },
  { key: 'mountPoint', title: t('resource.clusterMgmt.nodeDetail.disk.mountPoint') },
])

async function load(): Promise<void> {
  if (!clusterId || !nodeId) return
  loading.value = true
  error.value = ''
  try {
    disks.value = await getNodeDisks(clusterId, nodeId)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    disks.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="node-disks" :aria-label="t('resource.clusterMgmt.aria.nodeDisks')">
    <SectionHeader :title="t('resource.clusterMgmt.nodeDetail.disk.title')" />
    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="disks"
      :empty-title="t('resource.clusterMgmt.nodeDetail.disk.empty')"
      min-width="720px"
      :aria-label="t('resource.clusterMgmt.nodeDetail.disk.title')"
    />
  </section>
</template>

<style scoped>
.node-disks { display: flex; flex-direction: column; gap: 8px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
</style>
