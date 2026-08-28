/**
 * 集群详情控制台 - 集群信息 > 集群详情 概览 PageSchema（UI 规范 V2.6 §7）。
 *
 * 结构：基本信息（三列）→ 插件/能力状态 → 节点资源摘要。
 * 注册组件通过注入的 DataSourceManager + clusterId 拉取数据；Schema 只负责
 * region 编排、条件求值与区块级错误隔离（V2.6 §7.2 per-region）。
 */
import type { PageSchema } from '@hnb/schema-engine'

export const clusterDetailOverviewSchema: PageSchema = {
  apiVersion: 'ui.hnb.io/v1',
  kind: 'PageSchema',
  metadata: {
    id: 'resource.cluster.overview',
    revision: 1,
    pluginId: 'resource',
    minShellVersion: '2.5.0',
  },
  spec: {
    template: 'detail',
    titleKey: 'resource.clusterMgmt.detail.title',
    descriptionKey: 'resource.clusterMgmt.detail.desc',
    contextRequirements: ['tenantId', 'clusterId'],
    layout: { type: 'grid', columns: 12, gap: 'md' },
    endpoints: [
      { id: 'resource.clusters.detail', path: '/api/v1/resources/clusters/{clusterId}', method: 'GET' },
      { id: 'resource.clusters.nodes', path: '/api/v1/resources/clusters/{clusterId}/nodes', method: 'GET' },
      { id: 'resource.clusters.description', path: '/api/v1/resources/clusters/{clusterId}/description', method: 'PATCH' },
      { id: 'runtime-intents.submit', path: '/api/v1/runtime-intents', method: 'POST' },
    ],
    dataSources: [
      {
        id: 'resource.cluster.detail',
        type: 'query',
        endpointId: 'resource.clusters.detail',
        contextBindings: ['clusterId'],
      },
      {
        id: 'resource.cluster.nodes',
        type: 'paginatedQuery',
        endpointId: 'resource.clusters.nodes',
        contextBindings: ['clusterId'],
        responseMapping: { items: 'items', total: 'total' },
      },
    ],
    regions: [
      {
        id: 'basicInfo',
        componentType: 'resource.ClusterOverviewPanel',
        span: 12,
        condition: { all: [{ permission: 'cluster:read' }] },
      },
      {
        id: 'pluginStatus',
        componentType: 'resource.ClusterPluginStatusPanel',
        span: 12,
        condition: { all: [{ permission: 'cluster:read' }] },
      },
      {
        id: 'nodeSummary',
        componentType: 'resource.ClusterNodeSummaryTable',
        span: 12,
        condition: { all: [{ permission: 'cluster:read' }] },
      },
    ],
  },
}
