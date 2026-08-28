/**
 * 集群列表列定义与筛选选项（V2.5 §10）。
 *
 * 单一真源：ClusterList.vue 的表格列、筛选、Action 均取自本模块。
 */
import type { ClusterKind, ClusterStatus } from '../types/cluster'

export interface ClusterListColumn {
  key: string
  titleKey: string
  width?: string
}

export const clusterListColumns: ClusterListColumn[] = [
  { key: 'displayName', titleKey: 'resource.clusterMgmt.col.name' },
  { key: 'kind', titleKey: 'resource.clusterMgmt.col.kind' },
  { key: 'source', titleKey: 'resource.clusterMgmt.col.source' },
  { key: 'status', titleKey: 'resource.clusterMgmt.col.status' },
  { key: 'runtimeVersion', titleKey: 'resource.clusterMgmt.col.version' },
  { key: 'nodeCount', titleKey: 'resource.clusterMgmt.col.nodes' },
  { key: 'updatedAt', titleKey: 'resource.clusterMgmt.col.updatedAt' },
  { key: 'actions', titleKey: 'resource.clusterMgmt.col.actions', width: '160px' },
]

export const CLUSTER_KIND_OPTIONS: Array<{ value: ClusterKind | ''; labelKey: string }> = [
  { value: '', labelKey: 'resource.clusterMgmt.filter.allKinds' },
  { value: 'kubernetes', labelKey: 'resource.clusterMgmt.kind.kubernetes' },
  { value: 'edge', labelKey: 'resource.clusterMgmt.kind.edge' },
  { value: 'container-engine', labelKey: 'resource.clusterMgmt.kind.container_engine' },
]

export const CLUSTER_STATUS_OPTIONS: Array<{ value: ClusterStatus | ''; labelKey: string }> = [
  { value: '', labelKey: 'resource.clusterMgmt.filter.allStatus' },
  { value: 'RUNNING', labelKey: 'resource.clusterMgmt.status.RUNNING' },
  { value: 'DEGRADED', labelKey: 'resource.clusterMgmt.status.DEGRADED' },
  { value: 'STALE', labelKey: 'resource.clusterMgmt.status.STALE' },
  { value: 'REGISTERING', labelKey: 'resource.clusterMgmt.status.REGISTERING' },
  { value: 'PROVISIONING', labelKey: 'resource.clusterMgmt.status.PROVISIONING' },
  { value: 'UPGRADING', labelKey: 'resource.clusterMgmt.status.UPGRADING' },
  { value: 'FAILED', labelKey: 'resource.clusterMgmt.status.FAILED' },
]
