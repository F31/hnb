/**
 * 节点详情 service adapter（restore-cluster-detail-console node-detail）。
 * 页面只依赖 typed 函数；开发 fixture，生产返回空态/空序列。
 */
import type {
  NodeDetail,
  NodeDisk,
  NodeMonitoringMetricKey,
  NodeMonitoringSummary,
  NodeNic,
  NodePod,
  NodePodListResponse,
} from '../types/node'
import type { NodeSummary } from '../types/cluster'
import type { MetricSeries, MonitoringRange } from '../types/clusterMonitoring'
import { listClusterNodes } from './clusterApi'
import { mapNodeToSummary } from './clusterDetailApi'
import { clusterNodeSummaryFixture } from './fixtures/clusterDetail'
import { pluginT } from './pluginI18n'
import {
  nodeDetailFixture,
  nodeDisksFixture,
  nodeMonitoringSummaryFixture,
  nodeNicsFixture,
  nodePodsFixture,
} from './fixtures/node'

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

/** 节点列表（分页 + 关键词；开发 fixture，生产映射节点 Read Model） */
export async function getNodeList(
  clusterId: string,
  params: { page?: number; pageSize?: number; keyword?: string } = {},
): Promise<{ items: NodeSummary[]; total: number }> {
  const page = params.page ?? 1
  const pageSize = params.pageSize ?? 10
  if (USE_FIXTURES) {
    const kw = params.keyword?.trim().toLowerCase() ?? ''
    const filtered = kw
      ? clusterNodeSummaryFixture.filter((n) => n.name.toLowerCase().includes(kw))
      : clusterNodeSummaryFixture
    const start = (page - 1) * pageSize
    return { items: filtered.slice(start, start + pageSize), total: filtered.length }
  }
  const res = await listClusterNodes(clusterId, { page, pageSize })
  return { items: (res.items ?? []).map(mapNodeToSummary), total: res.total }
}

/** 生产路径占位节点（后端无详情接口时页面以空字段渲染，不臆造数据） */
export function PLACEHOLDER_NODE(nodeId: string, name: string): NodeDetail {
  return {
    id: nodeId,
    name: name || nodeId,
    status: 'unknown',
    createdAt: '',
    managementIp: '',
    clusterIp: '',
    os: '',
    kernel: '',
    architecture: '',
    cpuCores: 0,
    memoryGiB: 0,
    gpuResource: undefined,
    vramGiB: undefined,
  }
}

/** 节点详情（开发 fixture；生产空态由页面处理） */
export async function getNodeDetail(_clusterId: string, _nodeId: string): Promise<NodeDetail | null> {
  if (USE_FIXTURES) return nodeDetailFixture
  return null
}

/** 节点磁盘列表 */
export async function getNodeDisks(_clusterId: string, _nodeId: string): Promise<NodeDisk[]> {
  if (USE_FIXTURES) return nodeDisksFixture
  return []
}

/** 节点网卡列表 */
export async function getNodeNics(_clusterId: string, _nodeId: string): Promise<NodeNic[]> {
  if (USE_FIXTURES) return nodeNicsFixture
  return []
}

/** 节点容器组列表（分页 + 关键词） */
export async function getNodePods(
  _clusterId: string,
  _nodeId: string,
  params: { page?: number; pageSize?: number; keyword?: string } = {},
): Promise<NodePodListResponse> {
  if (!USE_FIXTURES) return { items: [], total: 0 }
  const keyword = params.keyword?.trim().toLowerCase() ?? ''
  const filtered = keyword ? nodePodsFixture.filter((p) => p.name.toLowerCase().includes(keyword)) : nodePodsFixture
  const page = params.page ?? 1
  const pageSize = params.pageSize ?? 10
  const start = (page - 1) * pageSize
  return { items: filtered.slice(start, start + pageSize), total: filtered.length }
}

/** 节点监控资源摘要（CPU / 内存） */
export async function getNodeMonitoringSummary(
  _clusterId: string,
  _nodeId: string,
): Promise<NodeMonitoringSummary | null> {
  if (USE_FIXTURES) return nodeMonitoringSummaryFixture
  return null
}

// ---------------------------------------------------------------------------
// 节点监控 9 指标序列
// ---------------------------------------------------------------------------

type SeriesSpec = {
  name: string
  unit: string
  base: number
  amplitude: number
}

const networkNicNames = ['ens2f0np0', 'ens2f1np1', 'ens3f0', 'ens3f1']
const diskMountPoints = ['/dev/sda', '/dev/sdb']

/** 生成一组时序：多个命名字序列，围绕 base 做确定性波动 */
function generateGroup(
  key: NodeMonitoringMetricKey,
  range: MonitoringRange,
  specs: SeriesSpec[],
): MetricSeries[] {
  const start = new Date(range.start).getTime()
  const end = new Date(range.end).getTime()
  const span = Math.max(1, end - start)
  const count = 48
  return specs.map((spec) => ({
    name: spec.name,
    unit: spec.unit,
    points: Array.from({ length: count }, (_, i) => {
      const t = start + (span * i) / (count - 1)
      const phase = (i / 5) * Math.PI + (spec.name.length % 5)
      let value = spec.base + Math.sin(phase) * spec.base * spec.amplitude
      if (key === 'netRxPerNic' || key === 'netTxPerNic' || key === 'diskReadRate' || key === 'diskWriteRate') {
        value = Math.max(0, value)
      } else {
        value = Math.max(0, Math.min(100, value))
      }
      return { timestamp: new Date(t).toISOString(), value: Math.round(value * 100) / 100 }
    }),
  }))
}

export async function getNodeMonitoringMetrics(
  _clusterId: string,
  _nodeId: string,
  range: MonitoringRange,
): Promise<Record<NodeMonitoringMetricKey, MetricSeries[]>> {
  if (!USE_FIXTURES) {
    const empty: MetricSeries[] = []
    return {
      cpuUsage: empty, memoryUsage: empty, netRxPerNic: empty, netTxPerNic: empty,
      diskReadRate: empty, diskWriteRate: empty, diskReadLatency: empty,
      diskWriteLatency: empty, partitionUsage: empty,
    }
  }
  return {
    cpuUsage: generateGroup('cpuUsage', range, [{ name: 'CPU', unit: '%', base: 18.4, amplitude: 0.3 }]),
    memoryUsage: generateGroup('memoryUsage', range, [{ name: pluginT('resource.clusterMgmt.nodeDetail.monitoring.chart.memoryUsage'), unit: '%', base: 52.1, amplitude: 0.12 }]),
    netRxPerNic: generateGroup('netRxPerNic', range, networkNicNames.map((n, i) => ({ name: n, unit: 'B/s', base: 2e6 * (i + 1), amplitude: 0.35 }))),
    netTxPerNic: generateGroup('netTxPerNic', range, networkNicNames.map((n, i) => ({ name: n, unit: 'B/s', base: 1.2e6 * (i + 1), amplitude: 0.3 }))),
    diskReadRate: generateGroup('diskReadRate', range, [{ name: pluginT('resource.clusterMgmt.nodeDetail.monitoring.chart.diskRead'), unit: 'B/s', base: 1.5e7, amplitude: 0.4 }]),
    diskWriteRate: generateGroup('diskWriteRate', range, [{ name: pluginT('resource.clusterMgmt.nodeDetail.monitoring.chart.diskWrite'), unit: 'B/s', base: 9e6, amplitude: 0.45 }]),
    diskReadLatency: generateGroup('diskReadLatency', range, [{ name: pluginT('resource.clusterMgmt.nodeDetail.monitoring.chart.diskReadLatency'), unit: 'ms', base: 4.2, amplitude: 0.5 }]),
    diskWriteLatency: generateGroup('diskWriteLatency', range, [{ name: pluginT('resource.clusterMgmt.nodeDetail.monitoring.chart.diskWriteLatency'), unit: 'ms', base: 6.8, amplitude: 0.4 }]),
    partitionUsage: generateGroup('partitionUsage', range, diskMountPoints.map((m, i) => ({ name: m, unit: '%', base: i === 0 ? 62.4 : 41.7, amplitude: 0.1 }))),
  }
}
