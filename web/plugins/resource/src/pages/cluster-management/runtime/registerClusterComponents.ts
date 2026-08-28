/**
 * 注册集群受信组件到 ComponentRegistry（UI 规范 V2.6 §8）。
 *
 * 这些 componentType 仅供 cluster-management 插件使用，被 PageRenderer
 * 通过 PageSchema.spec.regions[].componentType 引用（组件类型使用命名空间
 * `resource.` 前缀，符合 §8.3）。Schema 内引用未注册的 componentType 会触发
 * PageRenderer 的 fail-closed 区块错误隔离。
 */
import type { ComponentRegistry } from '@hnb/schema-engine'
import ClusterOverviewPanel from '../components/ClusterOverviewPanel.vue'
import ClusterPluginStatusPanel from '../components/ClusterPluginStatusPanel.vue'
import ClusterNodeSummaryTable from '../components/ClusterNodeSummaryTable.vue'

export function registerClusterComponents(registry: ComponentRegistry): void {
  registry.register({
    type: 'resource.ClusterOverviewPanel',
    component: ClusterOverviewPanel,
    propsSchema: {
      type: 'object',
      properties: { data: { type: 'object' }, loading: { type: 'boolean' } },
    },
  })
  registry.register({
    type: 'resource.ClusterPluginStatusPanel',
    component: ClusterPluginStatusPanel,
    propsSchema: {
      type: 'object',
      properties: { items: { type: 'array' }, loading: { type: 'boolean' } },
    },
  })
  registry.register({
    type: 'resource.ClusterNodeSummaryTable',
    component: ClusterNodeSummaryTable,
    propsSchema: {
      type: 'object',
      properties: { items: { type: 'array' }, loading: { type: 'boolean' } },
    },
  })
}
