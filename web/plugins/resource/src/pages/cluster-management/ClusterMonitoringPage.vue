<script setup lang="ts">
/**
 * ClusterMonitoringPage — 集群信息 > 集群监控（OpenSpec cluster-monitoring）。
 *
 * 告警摘要 + 规模统计 + CPU/内存可调度资源概览 + 开始/结束时间范围（URL query
 * 持久化）+ 4 张趋势图（CPU/内存/GPU/显存使用率）。切换时间范围时旧请求被
 * generation 丢弃；无数据展示标准空态，不绘制零值曲线。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  defaultMonitoringRange,
  getClusterMonitoringMetrics,
  getClusterMonitoringSummary,
  emptyMonitoringSummary,
} from './api/clusterMonitoringApi'
import ClusterDetailLayout from './components/ClusterDetailLayout.vue'
import ClusterInfoTabs from './components/ClusterInfoTabs.vue'
import SectionHeader from './components/SectionHeader.vue'
import AlertSeverityCards from './components/AlertSeverityCards.vue'
import ClusterScaleCards from './components/ClusterScaleCards.vue'
import MetricProgress from './components/MetricProgress.vue'
import MetricChart from './components/MetricChart.vue'
import { isoToLocalInput, localInputToIso } from './utils/datetime'
import type {
  ClusterMonitoringSummary,
  MetricSeries,
  MonitoringMetricKey,
  MonitoringRange,
} from './types/clusterMonitoring'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const clusterId = String(route.params.clusterId ?? '')

const summary = ref<ClusterMonitoringSummary>(emptyMonitoringSummary)
const summaryLoading = ref(true)
const summaryError = ref('')

// 时间范围：canonical 存 ISO（用于查询与 URL），input 绑定本地 yyyy-MM-ddTHH:mm
const start = ref('')
const end = ref('')

const startInput = computed({
  get: () => isoToLocalInput(start.value),
  set: (v: string) => {
    start.value = localInputToIso(v)
  },
})
const endInput = computed({
  get: () => isoToLocalInput(end.value),
  set: (v: string) => {
    end.value = localInputToIso(v)
  },
})

const metrics = ref<Record<MonitoringMetricKey, MetricSeries>>({
  cpuUsage: { name: 'cpuUsage', unit: '%', points: [] },
  memoryUsage: { name: 'memoryUsage', unit: '%', points: [] },
  gpuUsage: { name: 'gpuUsage', unit: '%', points: [] },
  vramUsage: { name: 'vramUsage', unit: '%', points: [] },
})
const metricsLoading = ref(false)
const metricsError = ref('')
let metricsGen = 0

function initRangeFromQuery(): void {
  const qStart = typeof route.query.start === 'string' ? route.query.start : ''
  const qEnd = typeof route.query.end === 'string' ? route.query.end : ''
  if (qStart && qEnd && !Number.isNaN(new Date(qStart).getTime()) && !Number.isNaN(new Date(qEnd).getTime())) {
    start.value = qStart
    end.value = qEnd
    return
  }
  const d = defaultMonitoringRange()
  start.value = d.start
  end.value = d.end
}

async function loadSummary(): Promise<void> {
  summaryLoading.value = true
  summaryError.value = ''
  try {
    summary.value = await getClusterMonitoringSummary(clusterId)
  } catch (err) {
    summaryError.value = err instanceof Error ? err.message : String(err)
    summary.value = emptyMonitoringSummary
  } finally {
    summaryLoading.value = false
  }
}

async function loadMetrics(): Promise<void> {
  if (!clusterId || !start.value || !end.value) return
  const gen = ++metricsGen
  metricsLoading.value = true
  metricsError.value = ''
  try {
    const res = await getClusterMonitoringMetrics(clusterId, { start: start.value, end: end.value })
    if (gen !== metricsGen) return
    metrics.value = res
  } catch (err) {
    if (gen !== metricsGen) return
    metricsError.value = err instanceof Error ? err.message : String(err)
    metrics.value = {
      cpuUsage: { name: 'cpuUsage', unit: '%', points: [] },
      memoryUsage: { name: 'memoryUsage', unit: '%', points: [] },
      gpuUsage: { name: 'gpuUsage', unit: '%', points: [] },
      vramUsage: { name: 'vramUsage', unit: '%', points: [] },
    }
  } finally {
    if (gen === metricsGen) metricsLoading.value = false
  }
}

function persistRange(range: MonitoringRange): void {
  router.replace({ query: { ...route.query, start: range.start, end: range.end } })
}

function applyRange(): void {
  const range: MonitoringRange = { start: start.value, end: end.value }
  persistRange(range)
  loadMetrics()
}

function onRefresh(): void {
  loadSummary()
  loadMetrics()
}

const chartItems = computed(() => [
  { key: 'cpuUsage' as MonitoringMetricKey, title: t('resource.clusterMgmt.monitoring.chart.cpuUsage'), series: metrics.value.cpuUsage },
  { key: 'memoryUsage' as MonitoringMetricKey, title: t('resource.clusterMgmt.monitoring.chart.memoryUsage'), series: metrics.value.memoryUsage },
  { key: 'gpuUsage' as MonitoringMetricKey, title: t('resource.clusterMgmt.monitoring.chart.gpuUsage'), series: metrics.value.gpuUsage },
  { key: 'vramUsage' as MonitoringMetricKey, title: t('resource.clusterMgmt.monitoring.chart.vramUsage'), series: metrics.value.vramUsage },
])

watch(
  () => [start.value, end.value] as const,
  ([s, e]) => {
    if (s && e) applyRange()
  },
)

onMounted(() => {
  initRangeFromQuery()
  loadSummary()
  loadMetrics()
})

onBeforeUnmount(() => {
  metricsGen++
})
</script>

<template>
  <ClusterDetailLayout>
    <ClusterInfoTabs />

    <p v-if="summaryLoading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="summaryError" class="panel-status error" role="alert">
      <span>{{ summaryError }}</span>
      <button class="retry-button" type="button" @click="loadSummary">{{ t('resource.clusterMgmt.action.retry') }}</button>
    </div>

    <template v-else>
      <SectionHeader :title="t('resource.clusterMgmt.monitoring.alerts.title')" />
      <AlertSeverityCards :alerts="summary.alerts" />

      <SectionHeader :title="t('resource.clusterMgmt.monitoring.scale.title')" />
      <ClusterScaleCards :summary="summary" />

      <SectionHeader :title="t('resource.clusterMgmt.monitoring.resource.title')" />
      <div class="resource-overview">
        <MetricProgress :title="t('resource.clusterMgmt.monitoring.resource.cpu')" :unit="t('resource.clusterMgmt.monitoring.resource.cpuUnit')" :data="summary.cpu" />
        <MetricProgress :title="t('resource.clusterMgmt.monitoring.resource.memory')" unit="GiB" :data="summary.memory" />
      </div>

      <SectionHeader :title="t('resource.clusterMgmt.monitoring.trend.title')" />
      <div class="time-toolbar">
        <label class="time-field">
          <span>{{ t('resource.clusterMgmt.monitoring.time.start') }}</span>
          <input v-model="startInput" type="datetime-local" :aria-label="t('resource.clusterMgmt.monitoring.time.start')" />
        </label>
        <label class="time-field">
          <span>{{ t('resource.clusterMgmt.monitoring.time.end') }}</span>
          <input v-model="endInput" type="datetime-local" :aria-label="t('resource.clusterMgmt.monitoring.time.end')" />
        </label>
        <button class="secondary-button" type="button" :disabled="metricsLoading" @click="onRefresh">
          {{ t('resource.clusterMgmt.action.refresh') }}
        </button>
      </div>

      <p v-if="metricsError" class="panel-status error" role="alert">{{ metricsError }}</p>
      <p v-if="metricsLoading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>

      <div class="chart-grid">
        <MetricChart
          v-for="item in chartItems"
          :key="item.key"
          :title="item.title"
          unit="%"
          :series="[item.series]"
          :empty-text="t('resource.clusterMgmt.monitoring.chart.empty')"
        />
      </div>
    </template>
  </ClusterDetailLayout>
</template>

<style scoped>
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
.resource-overview {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}
.time-toolbar {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.time-field { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--hnb-color-text-secondary, #5b6675); }
.time-field input {
  padding: 6px 8px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
}
.secondary-button {
  padding: 7px 16px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  cursor: pointer;
}
.secondary-button:disabled { opacity: 0.6; cursor: not-allowed; }
.chart-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}
</style>
