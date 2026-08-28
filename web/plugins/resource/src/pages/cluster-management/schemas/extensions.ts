/**
 * 集群详情声明式扩展点（UI 规范 V2.6 §7.4）：`resource.cluster.detail.tabs`。
 *
 * 受控扩展点定义（命名空间 / componentType / 权限 / 最小 Shell 版本），由
 * Schema Engine 消费：只渲染已注册、调用方有权限、版本兼容的扩展。
 */
import type { ExtensionPointDefinition } from '@hnb/schema-engine'

/** `resource.cluster.detail.tabs` 扩展点贡献（按 order 升序渲染） */
export const clusterDetailTabsExtensions: ExtensionPointDefinition[] = [
  {
    namespace: 'resource.cluster.detail.tabs.overview',
    componentType: 'ClusterOverviewPanel',
    order: 1,
    permission: 'cluster:read',
    minShellVersion: '2.5.0',
  },
  {
    namespace: 'resource.cluster.detail.tabs.nodes',
    componentType: 'ClusterNodeSummaryTable',
    order: 2,
    permission: 'cluster:read',
    minShellVersion: '2.5.0',
  },
]
