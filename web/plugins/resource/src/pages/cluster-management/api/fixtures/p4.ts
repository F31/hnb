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

/** 插件市场目录（GPU / 网络 / 边缘计算 / 监控 / 存储 / 多集群 / 弹性伸缩 / 安全） */
export const pluginMarketCatalogFixture: MarketPlugin[] = [
  { name: 'hami', version: 'v2.10.0', description: 'HAMi GPU 虚拟化与显存调度（多厂商 vGPU / MIG / NPU）', category: 'GPU', installed: false },
  { name: 'gpu-operator', version: 'v26.7.0', description: 'NVIDIA GPU Operator（驱动 / Device Plugin / DCGM）', category: 'GPU', installed: false },
  { name: 'calico', version: 'v3.32.1', description: 'Calico CNI：网络策略、BGP 路由、可选 eBPF 数据面', category: '网络', installed: false },
  { name: 'cilium', version: 'v1.20.1', description: 'Cilium eBPF CNI：L7 策略、Hubble、带宽管理', category: '网络', installed: false },
  { name: 'kube-ovn', version: 'v1.16.2', description: 'Kube-OVN：子网、固定 IP、安全组、多网卡与 QoS', category: '网络', installed: true },
  { name: 'multus-sriov', version: '', description: 'Multus + SR-IOV / RDMA / DPDK 高性能多网卡', category: '网络', installed: false },
  { name: 'kubeedge', version: 'v1.23.1', description: 'KubeEdge 云边协同：CloudCore / EdgeCore、离线自治', category: '边缘计算', installed: false },
  { name: 'prometheus-operator', version: 'v0.93.1', description: 'Prometheus 监控与告警（kube-prometheus-stack）', category: '监控', installed: true },
  { name: 'rook-ceph', version: 'v1.20.6', description: 'Rook Ceph 分布式存储（块 / 文件 / 对象 + CSI）', category: '存储', installed: false },
  { name: 'longhorn', version: 'v1.12.1', description: 'Longhorn 块存储（快照 / 克隆 / 备份）', category: '存储', installed: false },
  { name: 'karmada', version: 'v1.18.2', description: 'Karmada 联邦多集群编排', category: '多集群', installed: false },
  { name: 'keda', version: 'v2.20.2', description: 'KEDA 事件驱动弹性伸缩', category: '弹性伸缩', installed: false },
  { name: 'falco', version: '0.44.1', description: 'Falco 运行时安全异常检测', category: '安全', installed: false },
]
