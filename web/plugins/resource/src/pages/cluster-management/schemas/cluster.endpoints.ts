/**
 * 集群管理受信 endpoint / dataSource 注册表（UI 规范 V2.6 §13.2/§13.3）。
 *
 * 单一来源：DataSourceManager 仅放行这些 endpointId/path。PageRenderer
 * 渲染期间不会执行 Schema 中未列出的 endpoint，也不允许执行任意 URL。
 *
 * 端点白名单前缀由 useClusterRuntime 在构造时调用 allowEndpoint() 注入；
 * 本文件只列受信 endpoint/dataSource 自身。
 */
import type { DataSourceDefinition, EndpointDefinition } from '@hnb/schema-engine'

/** 集群列表/详情/节点/字典/intent endpoint 注册表 */
export const CLUSTER_LIST_ENDPOINTS: EndpointDefinition[] = [
  {
    id: 'resource.clusters.list',
    path: '/api/v1/resources/clusters',
    method: 'GET',
  },
  {
    id: 'resource.clusters.detail',
    path: '/api/v1/resources/clusters/{clusterId}',
    method: 'GET',
  },
  {
    id: 'resource.clusters.nodes',
    path: '/api/v1/resources/clusters/{clusterId}/nodes',
    method: 'GET',
  },
  {
    id: 'resource.clusters.description',
    path: '/api/v1/resources/clusters/{clusterId}/description',
    method: 'PATCH',
  },
  {
    id: 'resource.clusters.kubeconfig',
    path: '/api/v1/resources/clusters/{clusterId}/kubeconfig:download',
    method: 'POST',
  },
  {
    id: 'resource.clusters.dictionaries',
    path: '/api/v1/dictionaries/cluster.status',
    method: 'GET',
  },
  {
    id: 'runtime-intents.submit',
    path: '/api/v1/runtime-intents',
    method: 'POST',
  },
  {
    id: 'operations.list',
    path: '/api/v1/operations',
    method: 'GET',
  },
  {
    id: 'operations.detail',
    path: '/api/v1/operations/{operationId}',
    method: 'GET',
  },
]

/** 集群详情控制台 dataSource 注册表 */
export const CLUSTER_LIST_DATASOURCES: DataSourceDefinition[] = [
  {
    id: 'resource.cluster.list',
    type: 'paginatedQuery',
    endpointId: 'resource.clusters.list',
    queryBindings: ['keyword', 'kind', 'status'],
    responseMapping: { items: 'items', total: 'total' },
  },
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
  {
    id: 'resource.cluster.dictionary',
    type: 'dictionary',
    endpointId: 'resource.clusters.dictionaries',
  },
]

/**
 * 由 runtime 调用：把受信 endpoint / dataSource 喂给已初始化的
 * DataSourceManager。幂等，覆盖式更新。
 */
export function registerClusterEndpointsAndDataSources(target: {
  registerEndpoint: (e: EndpointDefinition) => void
  registerDataSource: (d: DataSourceDefinition) => void
}): void {
  for (const endpoint of CLUSTER_LIST_ENDPOINTS) target.registerEndpoint(endpoint)
  for (const dataSource of CLUSTER_LIST_DATASOURCES) target.registerDataSource(dataSource)
}
