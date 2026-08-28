<script setup lang="ts">
/**
 * NodeMonitoringTab — 节点详情 > 节点监控（OpenSpec node-detail）。
 * CPU/内存资源摘要（MetricProgress）+ 9 张趋势图（CPU/内存使用率、网络收发多序列、
 * 磁盘读写速率、磁盘IO读写延时、磁盘分区使用率多序列）。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { defaultMonitoringRange } from '../api/clusterMonitoringApi'
import {
  getNodeMonitoringMetrics,
  getNodeMonitoringSummary,
} from '../api/nodeApi'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import { useNodeDetailId } from '../composables/useNodeDetailContext'
import SectionHeader from './SectionHeader.vue'
import MetricProgress from './MetricProgress.vue'
import MetricChart from './MetricChart.vue'
import type { NodeMonitoringMetricKey, NodeMonitoringSummary } from '../types/node'
import type { MetricSeries, MonitoringRange } from '../types/clusterMonitoring'

const { t } = useI18n()
const clusterId = useClusterDetailId()
const nodeId = useNodeDetailId()

const summary = ref<NodeMonitoringSummary | null>(null)
const metrics = ref<Record<NodeMonitoringMetricKey, MetricSeries[]>>({} as Record<NodeMonitoringMetricKey, MetricSeries[]>)
const loading = ref(true)
const error = ref('')

async function load(): Promise<void> {
  if (!clusterId || !nodeId) return
  loading.value = true
  error.value = ''
  const range: MonitoringRange = defaultMonitoringRange()
  try {
    const [sum, m] = await Promise.all([
      getNodeMonitoringSummary(clusterId, nodeId),
      getNodeMonitoringMetrics(clusterId, nodeId, range),
    ])
    summary.value = sum
    metrics.value = m
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(load)

const charts = computed(() => [
  { key: 'cpuUsage' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.cpuUsage'), unit: '%', series: metrics.value.cpuUsage ?? [] },
  { key: 'memoryUsage' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.memoryUsage'), unit: '%', series: metrics.value.memoryUsage ?? [] },
  { key: 'netRxPerNic' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.netRx'), unit: 'B/s', series: metrics.value.netRxPerNic ?? [] },
  { key: 'netTxPerNic' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.netTx'), unit: 'B/s', series: metrics.value.netTxPerNic ?? [] },
  { key: 'diskReadRate' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.diskRead'), unit: 'B/s', series: metrics.value.diskReadRate ?? [] },
  { key: 'diskWriteRate' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.diskWrite'), unit: 'B/s', series: metrics.value.diskWriteRate ?? [] },
  { key: 'diskReadLatency' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.diskReadLatency'), unit: 'ms', series: metrics.value.diskReadLatency ?? [] },
  { key: 'diskWriteLatency' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.diskWriteLatency'), unit: 'ms', series: metrics.value.diskWriteLatency ?? [] },
  { key: 'partitionUsage' as NodeMonitoringMetricKey, title: t('resource.clusterMgmt.nodeDetail.monitoring.chart.partitionUsage'), unit: '%', series: metrics.value.partitionUsage ?? [] },
])
</script>

<template>
  <div class="node-monitoring">
    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>

    <template v-else>
      <template v-if="summary">
        <SectionHeader :title="t('resource.clusterMgmt.nodeDetail.monitoring.resourceSummary')" />
        <div class="resource-overview">
          <MetricProgress :title="t('resource.clusterMgmt.monitoring.resource.cpu')" unit="核" :data="summary.cpu" />
          <MetricProgress :title="t('resource.clusterMgmt.monitoring.resource.memory')" unit="GiB" :data="summary.memory" />
        </div>
      </template>

      <SectionHeader :title="t('resource.clusterMgmt.nodeDetail.monitoring.trendTitle')" />
      <div class="chart-grid">
        <MetricChart
          v-for="chart in charts"
          :key="chart.key"
          :title="chart.title"
          :unit="chart.unit"
          :series="chart.series"
          :empty-text="t('resource.clusterMgmt.monitoring.chart.empty')"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.node-monitoring { display: flex; flex-direction: column; gap: 16px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.resource-overview { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; }
.chart-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px; }
</style>
