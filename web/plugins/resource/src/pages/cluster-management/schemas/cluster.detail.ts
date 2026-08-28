/**
 * 集群详情字段定义（映射自 ClusterSummary）。
 *
 * 保留：供旧详情展示与未来详情扩展复用；概览页由 ClusterOverviewPanel 承担。
 */

/** 详情概览字段定义（映射自 ClusterSummary） */
export interface ClusterDetailField {
  key: string
  labelKey: string
}

export const CLUSTER_DETAIL_FIELDS: ClusterDetailField[] = [
  { key: 'clusterId', labelKey: 'resource.clusterMgmt.detail.id' },
  { key: 'kind', labelKey: 'resource.clusterMgmt.col.kind' },
  { key: 'source', labelKey: 'resource.clusterMgmt.col.source' },
  { key: 'runtimeVersion', labelKey: 'resource.clusterMgmt.col.version' },
  { key: 'nodeCount', labelKey: 'resource.clusterMgmt.col.nodes' },
  { key: 'cpuTotal', labelKey: 'resource.clusterMgmt.detail.cpu' },
  { key: 'memoryTotal', labelKey: 'resource.clusterMgmt.detail.memory' },
  { key: 'environmentId', labelKey: 'resource.clusterMgmt.detail.environment' },
  { key: 'createdAt', labelKey: 'resource.clusterMgmt.detail.createdAt' },
  { key: 'updatedAt', labelKey: 'resource.clusterMgmt.detail.updatedAt' },
]

/** 配置 Tab 字段（能力快照，RT-003） */
export const CLUSTER_CONFIG_FIELDS: ClusterDetailField[] = [
  { key: 'snapshotVersion', labelKey: 'resource.clusterMgmt.config.snapshotVersion' },
  { key: 'observedAt', labelKey: 'resource.clusterMgmt.config.observedAt' },
  { key: 'freshness', labelKey: 'resource.clusterMgmt.config.freshness' },
  { key: 'tenantId', labelKey: 'resource.clusterMgmt.config.tenant' },
]
