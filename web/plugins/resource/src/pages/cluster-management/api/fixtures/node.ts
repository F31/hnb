/**
 * 节点详情 fixture（restore-cluster-detail-console node-detail）。
 * 仅构建时显式设置 VITE_CLUSTER_DETAIL_USE_FIXTURES=true 时启用。
 * 数据来自 OpenSpec references/fixtures/ui-fixtures.json。
 */
import type {
  NodeDetail,
  NodeDisk,
  NodeMonitoringSummary,
  NodeNic,
  NodePod,
} from '../../types/node'

export const nodeDetailFixture: NodeDetail = {
  id: '29ff76b2',
  name: 'dilab-worknode01',
  status: 'running',
  createdAt: '2026-04-02 16:06:29',
  managementIp: '10.230.100.63',
  clusterIp: '192.168.0.63',
  os: 'UniOS V1 (1.0.2503)',
  kernel: '5.10.0-136.12.0.86.4.nos1.x86_64',
  architecture: 'AMD64',
  cpuCores: 96,
  memoryGiB: 251.03,
  gpuResource: 'Tesla V100-PCIE-16GB*1',
  vramGiB: 16.0,
}

export const nodeDisksFixture: NodeDisk[] = [
  { name: 'sda', type: '系统盘', model: 'HDD', capacity: '893.8G', mountPoint: '/dev/sda' },
  { name: 'sdb', type: '数据盘', model: 'SSD', capacity: '1.7T', mountPoint: '/dev/sdb' },
]

export const nodeNicsFixture: NodeNic[] = [
  { name: 'ens2f0np0', mac: '4c:e9:e4:cd:89:16', status: 'available', type: '物理网卡(支持RoCE)', ip: null, speed: '10Gbit/s' },
  { name: 'ens2f1np1', mac: '4c:e9:e4:cd:89:17', status: 'available', type: '物理网卡(支持RoCE)', ip: null, speed: '10Gbit/s' },
  { name: 'ens3f0', mac: 'a4:fa:76:07:9b:f8', status: 'available', type: '物理网卡', ip: null, speed: '10Gbit/s' },
  { name: 'ens3f1', mac: 'a4:fa:76:07:9b:fa', status: 'available', type: '物理网卡', ip: '192.166.0.63', speed: '10Gbit/s' },
  { name: 'enp6s1f0', mac: '90:f7:b2:4a:be:7d', status: 'unavailable', type: '物理网卡', ip: null, speed: null },
]

const podNames = [
  'hp-volume-9btf8', 'hp-volume-cckhf', 'csi-snapshot-controller-7d9c', 'kube-flannel-ds-2k5f',
  'node-exporter-9x8m', 'multus-7p2l', 'ovn-kube-5j7k', 'metrics-server-7f8d',
  'hpa-controller-3b2a', 'kube-proxy-j5d9',
]

export const nodePodsFixture: NodePod[] = podNames.map((name, index) => {
  const status = index % 4 === 0 ? 'running' : 'running'
  return {
    name,
    status,
    namespace: index % 3 === 0 ? 'kube-system' : index % 3 === 1 ? 'vm-973fd1ef' : 'default',
    podIp: `11.0.${Math.floor(index / 2)}.${60 + (index % 60)}`,
    nodeIp: '192.168.0.63',
    createdAt: `2026-08-0${(index % 6) + 1} 1${index % 9}:2${index % 10}:0${index % 9}`,
    yaml: `apiVersion: v1\nkind: Pod\nmetadata:\n  name: ${name}\n  namespace: ${index % 3 === 0 ? 'kube-system' : index % 3 === 1 ? 'vm-973fd1ef' : 'default'}\nspec:\n  nodeName: dilab-worknode01\n  containers:\n    - name: main\n      image: example.io/main:v1\nstatus:\n  phase: Running\n  podIP: 11.0.${Math.floor(index / 2)}.${60 + (index % 60)}\n`,
  }
})

export const nodeMonitoringSummaryFixture: NodeMonitoringSummary = {
  cpu: { total: 96, usagePercent: 18.4, used: 17.7, allocationPercent: 60.5, allocated: 58.1, overcommitPercent: 142.3, overcommitted: 136.6 },
  memory: { total: 251.03, usagePercent: 52.1, used: 130.8, allocationPercent: 61.2, allocated: 153.6, overcommitPercent: 121.7, overcommitted: 305.5 },
}
