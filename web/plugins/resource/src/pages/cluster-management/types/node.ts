/**
 * 节点详情领域类型（restore-cluster-detail-console node-detail）。
 * 后端节点详情/磁盘/网卡/容器组监控端点暂缺，由 service adapter + fixture 提供。
 */
import type { ResourceUsageSummary } from './clusterMonitoring'

/** 节点详情（基础配置 + 规格） */
export interface NodeDetail {
  id: string
  name: string
  status: 'running' | 'abnormal' | 'unknown'
  createdAt: string
  managementIp: string
  clusterIp: string
  os: string
  kernel: string
  architecture: string
  cpuCores: number
  memoryGiB: number
  gpuResource?: string
  vramGiB?: number
}

/** 节点磁盘（系统盘/数据盘等） */
export interface NodeDisk {
  name: string
  type: string
  model: string
  capacity: string
  mountPoint: string
}

/** 节点网卡 */
export interface NodeNic {
  name: string
  mac: string
  status: 'available' | 'unavailable'
  type: string
  ip?: string | null
  speed?: string | null
}

/** 节点容器组 */
export interface NodePod {
  name: string
  status: string
  namespace: string
  podIp: string
  nodeIp: string
  createdAt: string
  /** 只读 YAML（查看 YAML 弹窗） */
  yaml?: string
}

export interface NodePodListResponse {
  items: NodePod[]
  total: number
}

/** 节点监控资源摘要（CPU / 内存） */
export interface NodeMonitoringSummary {
  cpu: ResourceUsageSummary
  memory: ResourceUsageSummary
}

/** 节点监控指标 key（基础 2 + 网络 2 + 磁盘 IO 4 + 分区 1） */
export type NodeMonitoringMetricKey =
  | 'cpuUsage'
  | 'memoryUsage'
  | 'netRxPerNic'
  | 'netTxPerNic'
  | 'diskReadRate'
  | 'diskWriteRate'
  | 'diskReadLatency'
  | 'diskWriteLatency'
  | 'partitionUsage'
