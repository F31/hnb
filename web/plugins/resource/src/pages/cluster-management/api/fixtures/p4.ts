/**
 * P4 fixture（边缘节点组 / 租户分配 / 插件实例 / 漏洞库）。
 * 仅构建时显式设置 VITE_CLUSTER_DETAIL_USE_FIXTURES=true 时启用。
 * 数据来自 OpenSpec references/fixtures/ui-fixtures.json。
 */
import type {
  EdgeNodeGroup,
  MarketPlugin,
  PluginInstance,
  PluginVersionCatalog,
  TenantAllocation,
} from '../../types/p4'

export const edgeNodeGroupsFixture: EdgeNodeGroup[] = [
  { name: 'edge-group-1', status: 'running', nodeCount: 12, description: '生产边缘计算节点组' },
  { name: 'edge-group-2', status: 'running', nodeCount: 6, description: '测试环境边缘节点组' },
]

function allocation(cpu: number, mem: number, storage: number): Omit<TenantAllocation, 'tenantName'> {
  return {
    cpu: { limit: cpu, used: Math.round(cpu * 0.4), percent: 40 },
    memory: { limit: mem, used: Math.round(mem * 0.5), percent: 50 },
    storage: { limit: storage, used: Math.round(storage * 0.3), percent: 30 },
    virtualGpu: { limit: 0, used: 0, percent: 0 },
    virtualVram: { limit: 0, used: 0, percent: 0 },
    physicalGpu: { limit: 0, used: 0, percent: 0 },
  }
}

export const tenantAllocationsFixture: TenantAllocation[] = [
  {
    tenantName: 'RD',
    cpu: { limit: 1000, used: 223, percent: 22 },
    memory: { limit: 1000, used: 224, percent: 22 },
    storage: { limit: 2500, used: 550, percent: 22 },
    virtualGpu: { limit: 200, used: 0, percent: 0 },
    virtualVram: { limit: 90000, used: 0, percent: 0 },
    physicalGpu: { limit: 999, used: 0, percent: 0 },
  },
  {
    tenantName: 'AI',
    ...allocation(800, 512, 1200),
  },
]

const pluginYaml = (app: string, plugin: string, version: string): string =>
  `apiVersion: hnb.io/v1\nkind: PluginInstance\nmetadata:\n  name: ${app}\nspec:\n  plugin: ${plugin}\n  version: ${version}\n  values:\n    replicas: 1\n    imagePullPolicy: IfNotPresent\n`

export const pluginInstancesFixture: PluginInstance[] = [
  { applicationName: 'vmoto', description: '虚拟化vmware迁移agent', pluginName: 'os-virt-migrate-agent', pluginVersion: '1.0.4', status: 'running', createdAt: '2026-05-08 10:25:40', valuesYaml: pluginYaml('vmoto', 'os-virt-migrate-agent', '1.0.4') },
  { applicationName: 'elk-collector', description: '日志审计', pluginName: 'log-collector', pluginVersion: '1.0.6', status: 'running', createdAt: '2026-03-31 18:13:25', valuesYaml: pluginYaml('elk-collector', 'log-collector', '1.0.6') },
  { applicationName: 'rdma-operator', description: 'rdma 网络插件', pluginName: 'rdma-network', pluginVersion: '2.1.0', status: 'running', createdAt: '2026-04-15 09:02:11', valuesYaml: pluginYaml('rdma-operator', 'rdma-network', '2.1.0') },
  { applicationName: 'backup-agent', description: '备份恢复', pluginName: 'backup-restore', pluginVersion: '3.0.1', status: 'abnormal', createdAt: '2026-06-20 14:30:00', valuesYaml: pluginYaml('backup-agent', 'backup-restore', '3.0.1') },
]

export const pluginVersionCatalogFixture: PluginVersionCatalog[] = [
  { pluginName: 'os-virt-migrate-agent', versions: ['1.0.3', '1.0.4', '1.1.0'] },
  { pluginName: 'log-collector', versions: ['1.0.5', '1.0.6', '1.1.0'] },
  { pluginName: 'rdma-network', versions: ['2.0.0', '2.1.0'] },
  { pluginName: 'backup-restore', versions: ['3.0.0', '3.0.1', '3.1.0'] },
]

export const vulnerabilityDbStatusFixture = {
  label: 'trivy-db-2026-08-01',
  updatedAt: '2026-08-01 00:00:00',
}

/** 插件市场目录（GPU / 边缘计算 / 网络 / 存储 / 监控） */
export const pluginMarketCatalogFixture: MarketPlugin[] = [
  { name: 'hami', version: 'v1.0.2', description: 'HAMI GPU 虚拟化与显存调度', category: 'GPU', installed: false },
  { name: 'gpu-operator', version: 'v24.9.0', description: 'NVIDIA GPU Operator', category: 'GPU', installed: false },
  { name: 'kubeEdge', version: 'v1.18.0', description: 'KubeEdge 边缘计算框架', category: '边缘计算', installed: false },
  { name: 'Kube-OVN', version: 'v1.12.0', description: 'Kube-OVN 容器网络（含安全组/多网卡）', category: '网络', installed: true },
  { name: 'Cilium', version: 'v1.15.0', description: 'Cilium eBPF 容器网络', category: '网络', installed: false },
  { name: 'Rook-Ceph', version: 'v1.14.0', description: 'Rook Ceph 分布式存储', category: '存储', installed: false },
  { name: 'Longhorn', version: 'v1.6.0', description: 'Longhorn 块存储', category: '存储', installed: false },
  { name: 'Prometheus Operator', version: 'v0.73.0', description: 'Prometheus 监控与告警', category: '监控', installed: true },
]
