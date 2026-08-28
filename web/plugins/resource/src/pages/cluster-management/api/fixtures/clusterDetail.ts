/**
 * 集群详情控制台开发期 fixture（restore-cluster-detail-console）。
 *
 * 仅在构建时显式设置 `VITE_CLUSTER_DETAIL_USE_FIXTURES=true` 时启用，
 * 生产构建默认不包含 fixture 回退（service adapter 强制走真实 API）。
 * 数据来自 OpenSpec references/fixtures/ui-fixtures.json（截图开发基准）。
 */
import type { ClusterDetail, ClusterPluginStatus, NodeSummary } from '../../types/cluster'

export const clusterDetailFixture: ClusterDetail = {
  id: 'a1a4a7f4',
  name: 'default',
  kubernetesVersion: 'v1.31.1',
  createdAt: '2026-03-31 16:44:37',
  osVersion: 'UniOS V1 (1.0.2503)',
  cpuArchitecture: 'x86_64',
  description: 'default cluster.',
  status: 'running',
  controlPlaneSchedulingEnabled: true,
  clusterType: '自建集群',
  managementVip: '10.230.100.59',
  clusterVip: '192.168.0.59',
  podCidr: '11.0.0.0/10',
  serviceCidr: '11.128.0.0/12',
  clusterDomain: 'cloudos',
  kubeOvnJoinCidr: '100.64.0.0/16',
}

/** 插件状态清单（截图基准：运行中 / 未安装 / 已安装 等） */
export const clusterPluginStatusesFixture: ClusterPluginStatus[] = [
  ['Kubernetes调度扩展插件', 'not-installed'],
  ['虚拟GPU插件', 'not-installed'],
  ['监控前置插件', 'running'],
  ['rdma网络插件', 'running'],
  ['虚拟化vmware迁移agent服务', 'running'],
  ['天数GPU插件', 'not-installed'],
  ['metallb', 'running'],
  ['ovn网络插件', 'running'],
  ['物理GPU插件', 'running'],
  ['监控', 'running'],
  ['虚拟化设备管理插件', 'not-installed'],
  ['边缘云', 'not-installed'],
  ['备份恢复', 'running'],
  ['虚拟化vmware迁移编排服务', 'running'],
  ['虚拟化插件', 'running'],
  ['日志审计', 'running'],
  ['内部存储', 'not-installed'],
  ['内部存储基础资源', 'not-installed'],
].map(([displayName, status], index) => ({
  key: `plugin-${index + 1}`,
  displayName,
  status: status as ClusterPluginStatus['status'],
}))

/** 节点资源摘要（截图基准：controller + worker + GPU worker 异构） */
export const clusterNodeSummaryFixture: NodeSummary[] = [
  {
    id: '29ff76b2',
    name: 'dilab-worknode01',
    role: 'worker',
    type: 'cloud',
    status: 'running',
    managementIp: '10.230.100.63',
    clusterIp: '192.168.0.63',
    cpuCores: 96,
    memoryGiB: 251.03,
    gpuResource: 'Tesla V100-PCIE-16GB*1',
    vramGiB: 16.0,
    createdAt: '2026-04-02 16:06:29',
  },
  {
    id: '2d1862f0',
    name: 'dilab-node02',
    role: 'controller',
    type: 'cloud',
    status: 'running',
    managementIp: '10.230.100.61',
    clusterIp: '192.168.0.61',
    cpuCores: 104,
    memoryGiB: 251.02,
    createdAt: '2026-03-31 16:40:00',
  },
  {
    id: 'b06343f2',
    name: 'dilab-node01',
    role: 'controller',
    type: 'cloud',
    status: 'running',
    managementIp: '10.230.100.60',
    clusterIp: '192.168.0.60',
    cpuCores: 104,
    memoryGiB: 251.02,
    createdAt: '2026-03-31 16:39:00',
  },
  {
    id: '80c04e7b',
    name: 'dilab-node03',
    role: 'worker',
    type: 'cloud',
    status: 'running',
    managementIp: '10.230.100.62',
    clusterIp: '192.168.0.62',
    cpuCores: 104,
    memoryGiB: 251.02,
    createdAt: '2026-03-31 16:40:00',
  },
  {
    id: '1c1e1c0f',
    name: 'dilab-worknode02',
    role: 'worker',
    type: 'cloud',
    status: 'running',
    managementIp: '10.230.100.64',
    clusterIp: '192.168.0.64',
    cpuCores: 96,
    memoryGiB: 251.03,
    createdAt: '2026-04-02 16:06:29',
  },
]
