/**
 * 集群监控 service adapter（restore-cluster-detail-console cluster-monitoring）。
 *
 * 生产环境通过平台 BFF 查询中心 Prometheus-compatible 后端；浏览器不接触
 * Prometheus 地址、凭据或任意 PromQL。fixture 仅用于显式开启的本地演示。
 */
import type {
  ClusterMonitoringSummary,
  MetricSeries,
  MonitoringMetricKey,
  MonitoringRange,
} from '../types/clusterMonitoring'
import { getClusterApiClient } from './clusterApi'

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

export const emptyMonitoringSummary: ClusterMonitoringSummary = {
  alerts: { critical: 0, major: 0, minor: 0, warning: 0, event: 0 },
  namespaceCount: 0,
  projectCount: 0,
  schedulableNodeCount: 0,
  cpu: { total: 0, usagePercent: 0, used: 0, allocationPercent: 0, allocated: 0, overcommitPercent: 0, overcommitted: 0 },
  memory: { total: 0, usagePercent: 0, used: 0, allocationPercent: 0, allocated: 0, overcommitPercent: 0, overcommitted: 0 },
}

/** 开发 fixture 摘要（来自 ui-fixtures.json） */
const summaryFixture: ClusterMonitoringSummary = {
  alerts: { critical: 0, major: 23, minor: 0, warning: 8, event: 0 },
  namespaceCount: 72,
  projectCount: 7,
  schedulableNodeCount: 5,
  cpu: { total: 600, usagePercent: 11.62, used: 64.94, allocationPercent: 41.35, allocated: 248.12, overcommitPercent: 204.14, overcommitted: 1224.85 },
  memory: { total: 1255.12, usagePercent: 45.33, used: 568.95, allocationPercent: 58.0, allocated: 728.0, overcommitPercent: 163.18, overcommitted: 2048.05 },
}

const basePercent: Record<MonitoringMetricKey, number> = {
  cpuUsage: 11.62,
  memoryUsage: 45.33,
  gpuUsage: 31.2,
  vramUsage: 26.8,
}

/** 生成时间序列：围绕基准百分比做确定性正弦波动 */
function generateSeries(key: MonitoringMetricKey, range: MonitoringRange): MetricSeries {
  const start = new Date(range.start).getTime()
  const end = new Date(range.end).getTime()
  const span = Math.max(1, end - start)
  const count = 48
  const base = basePercent[key]
  const points = Array.from({ length: count }, (_, i) => {
    const t = start + (span * i) / (count - 1)
    const phase = (i / 6) * Math.PI
    const value = Math.max(0, Math.min(100, base + Math.sin(phase) * base * 0.25 + Math.cos(phase * 0.5) * base * 0.08))
    return { timestamp: new Date(t).toISOString(), value: Math.round(value * 100) / 100 }
  })
  return { name: key, unit: '%', points }
}

/** 集群监控摘要（生产：空摘要 → 空态） */
export async function getClusterMonitoringSummary(clusterId: string): Promise<ClusterMonitoringSummary> {
  if (USE_FIXTURES) return summaryFixture
  return getClusterApiClient().get<ClusterMonitoringSummary>(
    `/api/v1/resources/clusters/${encodeURIComponent(clusterId)}/monitoring/summary`,
  )
}

/** 监控趋势序列（生产：空数组 → 图表空态） */
export async function getClusterMonitoringMetrics(
  clusterId: string,
  range: MonitoringRange,
): Promise<Record<MonitoringMetricKey, MetricSeries>> {
  if (USE_FIXTURES) {
    return {
      cpuUsage: generateSeries('cpuUsage', range),
      memoryUsage: generateSeries('memoryUsage', range),
      gpuUsage: generateSeries('gpuUsage', range),
      vramUsage: generateSeries('vramUsage', range),
    }
  }
  return getClusterApiClient().get<Record<MonitoringMetricKey, MetricSeries>>(
    `/api/v1/resources/clusters/${encodeURIComponent(clusterId)}/monitoring/metrics`,
    { params: range },
  )
}

/** 默认时间范围：最近 1 小时 */
export function defaultMonitoringRange(): MonitoringRange {
  const end = new Date()
  const start = new Date(end.getTime() - 60 * 60 * 1000)
  return { start: start.toISOString(), end: end.toISOString() }
}
