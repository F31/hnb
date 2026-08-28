<script setup lang="ts">
/**
 * NodeBasicInfoTab — 节点详情 > 基础配置（OpenSpec node-detail）。
 * 基本参数（ID/运行状态/创建时间/管理IP/集群IP/OS/内核/架构）+ 节点规格
 * （CPU核数/内存/GPU资源/显存资源）。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { StatusBadge } from '@hnb/ui-kit'
import { getNodeDetail, PLACEHOLDER_NODE } from '../api/nodeApi'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import { useNodeDetailId, useNodeDetailName } from '../composables/useNodeDetailContext'
import SectionHeader from './SectionHeader.vue'
import type { NodeDetail } from '../types/node'

const { t } = useI18n()
const clusterId = useClusterDetailId()
const nodeId = useNodeDetailId()
const nodeName = useNodeDetailName()

const detail = ref<NodeDetail | null>(null)
const loading = ref(true)
const error = ref('')

const statusSemantic = computed<`success` | `error` | `default`>(() =>
  detail.value?.status === 'running' ? 'success' : detail.value?.status === 'abnormal' ? 'error' : 'default',
)
const statusLabel = computed(() =>
  detail.value?.status === 'running'
    ? t('resource.clusterMgmt.nodeDetail.status.running')
    : detail.value?.status === 'abnormal'
      ? t('resource.clusterMgmt.nodeDetail.status.abnormal')
      : t('resource.clusterMgmt.nodeDetail.status.unknown'),
)

const basicFields = computed(() => {
  const d = detail.value
  if (!d) return []
  return [
    { label: t('resource.clusterMgmt.nodeDetail.field.nodeId'), value: d.id || '--' },
    { label: t('resource.clusterMgmt.nodeDetail.field.createdAt'), value: d.createdAt || '--' },
    { label: t('resource.clusterMgmt.nodeDetail.field.managementIp'), value: d.managementIp || '--' },
    { label: t('resource.clusterMgmt.nodeDetail.field.clusterIp'), value: d.clusterIp || '--' },
    { label: t('resource.clusterMgmt.nodeDetail.field.os'), value: d.os || '--' },
    { label: t('resource.clusterMgmt.nodeDetail.field.kernel'), value: d.kernel || '--' },
    { label: t('resource.clusterMgmt.nodeDetail.field.architecture'), value: d.architecture || '--' },
  ]
})

const specFields = computed(() => {
  const d = detail.value
  if (!d) return []
  return [
    { label: t('resource.clusterMgmt.nodeDetail.spec.cpuCores'), value: `${d.cpuCores}` },
    { label: t('resource.clusterMgmt.nodeDetail.spec.memory'), value: `${d.memoryGiB} GiB` },
    { label: t('resource.clusterMgmt.nodeDetail.spec.gpu'), value: d.gpuResource || '--' },
    { label: t('resource.clusterMgmt.nodeDetail.spec.vram'), value: d.vramGiB != null ? `${d.vramGiB} GiB` : '--' },
  ]
})

async function load(): Promise<void> {
  if (!clusterId || !nodeId) return
  loading.value = true
  error.value = ''
  try {
    const d = await getNodeDetail(clusterId, nodeId)
    detail.value = d ?? PLACEHOLDER_NODE(nodeId, nodeName)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    detail.value = null
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="node-basic-info">
    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>

    <template v-else-if="detail">
      <section>
        <div class="head-row">
          <SectionHeader :title="t('resource.clusterMgmt.nodeDetail.basicParams')" />
          <StatusBadge :label="statusLabel" :semantic="statusSemantic" />
        </div>
        <dl class="field-grid">
          <div v-for="field in basicFields" :key="field.label" class="field-item">
            <dt>{{ field.label }}</dt>
            <dd :title="String(field.value)">{{ field.value }}</dd>
          </div>
        </dl>
      </section>

      <section>
        <SectionHeader :title="t('resource.clusterMgmt.nodeDetail.specTitle')" />
        <dl class="field-grid">
          <div v-for="field in specFields" :key="field.label" class="field-item">
            <dt>{{ field.label }}</dt>
            <dd :title="String(field.value)">{{ field.value }}</dd>
          </div>
        </dl>
      </section>
    </template>
  </div>
</template>

<style scoped>
.node-basic-info { display: flex; flex-direction: column; gap: 20px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.head-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.field-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px 20px;
  margin: 0;
}
.field-item { min-width: 0; border-bottom: 1px dashed var(--hnb-color-border, #e2e7ef); padding-bottom: 6px; }
.field-item dt { font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); margin-bottom: 2px; }
.field-item dd { margin: 0; font-size: 14px; color: var(--hnb-color-text-primary, #12172a); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
