/**
 * 集群详情控制台 service adapter（restore-cluster-detail-console）。
 *
 * 页面只依赖本模块的 typed 函数，不拼接后端 URL。开发期可通过
 * VITE_CLUSTER_DETAIL_USE_FIXTURES=true 使用 fixture 兜底，生产构建强制
 * 走真实 API，绝不自动回退假数据（OpenSpec design §14）。
 */
import type { ClusterDetail, ClusterNodeInfo, ClusterPluginStatus, NodeSummary } from '../types/cluster'
import { getClusterApiClient } from './clusterApi'
import {
  clusterDetailFixture,
  clusterNodeSummaryFixture,
  clusterPluginStatusesFixture,
} from './fixtures/clusterDetail'

const CLUSTERS_PATH = '/api/v1/resources/clusters'

/** 开发期 fixture 开关：仅显式设置构建环境变量开启，生产不自动回退 */
const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

function client() {
  return getClusterApiClient()
}

/** 把后端 ClusterSummary 投影映射为概览页 ClusterDetail（缺失字段补占位） */
export function mapSummaryToDetail(s: Record<string, unknown>): ClusterDetail {
  const status: ClusterDetail['status'] =
    s.status === 'RUNNING' ? 'running' : s.status === 'DEGRADED' || s.status === 'STALE' ? 'abnormal' : 'unknown'
  return {
    id: String(s.clusterId ?? ''),
    name: String(s.displayName ?? ''),
    kubernetesVersion: String(s.runtimeVersion ?? ''),
    createdAt: String(s.createdAt ?? ''),
    osVersion: '',
    cpuArchitecture: '',
    description: String(s.description ?? ''),
    status,
    stale: s.status === 'STALE'
      || (s.capabilitySnapshot as { freshness?: string } | undefined)?.freshness === 'stale',
    controlPlaneSchedulingEnabled: false,
    clusterType: String(s.kind ?? ''),
    source: s.source === 'created' || s.source === 'imported' ? s.source : undefined,
    managementVip: '',
    clusterVip: '',
    podCidr: '',
    serviceCidr: '',
    clusterDomain: '',
    kubeOvnJoinCidr: '',
  }
}

function parseCpu(value: string | undefined): number {
  const n = Number(String(value ?? '').replace(/[^0-9.]/g, ''))
  return Number.isFinite(n) ? n : 0
}

function parseMemGiB(value: string | undefined): number {
  const m = /([0-9.]+)\s*([KMGTPE]i?B)/i.exec(String(value ?? ''))
  if (!m) return parseCpu(value)
  const num = Number(m[1])
  const unit = m[2].toUpperCase()
  const exp = { KIB: 1, MB: 1, MIB: 1, GB: 2, GIB: 2, TB: 3, TIB: 3, PB: 4, PIB: 4 } as Record<string, number>
  const e = exp[unit] ?? 0
  return Math.round((num / Math.pow(1024, e - 1)) * 100) / 100
}

/** 把后端节点 Read Model 映射为概览页 NodeSummary */
export function mapNodeToSummary(n: ClusterNodeInfo): NodeSummary {
  return {
    id: n.nodeId,
    name: n.name,
    role: n.role,
    type: n.role,
    status: n.status,
    managementIp: n.ipAddress,
    cpuCores: parseCpu(n.cpuAllocatable),
    memoryGiB: parseMemGiB(n.memoryAllocatable),
    gpuResource: undefined,
    vramGiB: undefined,
    createdAt: n.lastHeartbeatAt,
  }
}

/** 集群基本信息（真实 Read Model 映射，缺失字段补 `--`） */
export async function getClusterDetail(clusterId: string): Promise<ClusterDetail> {
  if (USE_FIXTURES) return clusterDetailFixture
  const res = await client().get<Record<string, unknown>>(
    `${CLUSTERS_PATH}/${encodeURIComponent(clusterId)}`,
  )
  return mapSummaryToDetail(res)
}

/** 插件/平台能力状态列表（Read Model：最新能力快照派生，无数据时返回空数组 + 空态） */
export async function getClusterPluginStatuses(clusterId: string): Promise<ClusterPluginStatus[]> {
  if (USE_FIXTURES) return clusterPluginStatusesFixture
  const res = await client().get<{ items: ClusterPluginStatus[] }>(
    `${CLUSTERS_PATH}/${encodeURIComponent(clusterId)}/plugins`,
  )
  return res.items ?? []
}

/** 节点资源摘要（复用节点 Read Model，映射为 NodeSummary[]） */
export async function getClusterNodeSummary(clusterId: string): Promise<NodeSummary[]> {
  if (USE_FIXTURES) return clusterNodeSummaryFixture
  const res = await client().get<{ items: ClusterNodeInfo[]; total: number }>(
    `${CLUSTERS_PATH}/${encodeURIComponent(clusterId)}/nodes`,
    { params: { page: 1, pageSize: 200 } },
  )
  return (res.items ?? []).map(mapNodeToSummary)
}

/** 更新集群描述（后端 PATCH 暂缺 → 开发期直接成功，生产抛"未开放"） */
export async function updateClusterDescription(clusterId: string, description: string): Promise<void> {
  if (USE_FIXTURES) return
  await client().patch(`${CLUSTERS_PATH}/${encodeURIComponent(clusterId)}/description`, { description })
}

/** 下载 KubeConfig（敏感操作，必须走鉴权接口；后端暂缺 → 开发期 fixture） */
export async function downloadKubeConfig(clusterId: string): Promise<void> {
  if (USE_FIXTURES) return
  const res = await client().post<{ kubeconfig?: string; filename?: string }>(
    `${CLUSTERS_PATH}/${encodeURIComponent(clusterId)}/kubeconfig:download`,
    {},
    {},
  )
  if (!res?.kubeconfig) return
  triggerDownload(res.kubeconfig, res.filename || `${clusterId}.kubeconfig`)
}

function triggerDownload(content: string, filename: string): void {
  if (typeof window === 'undefined') return
  const blob = new Blob([content], { type: 'application/x-yaml' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

/** 格式化 CPU 核数（展示用） */
export function formatCpuCores(cores: number): string {
  return `${cores}`
}

/** 格式化内存/显存 GiB */
export function formatMemoryGiB(gib: number): string {
  return `${gib.toFixed(2)} GiB`
}

/** 占位符：缺失值统一 `--` */
export const PLACEHOLDER = '--'
