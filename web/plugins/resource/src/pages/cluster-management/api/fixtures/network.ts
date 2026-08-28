/**
 * 资源-网络管控 fixture（容器子网 / IP 统计 / 申请审批）。
 * 仅构建时显式设置 VITE_CLUSTER_DETAIL_USE_FIXTURES=true 时启用。
 */
import type { ContainerSubnet, IpUsageStat, SubnetRequest } from '../../types/network'

export const subnetsFixture: ContainerSubnet[] = [
  { name: 'subnet-prod-a', cidr: '10.20.0.0/24', gateway: '10.20.0.1', cniType: 'Kube-OVN', mode: 'overlay', namespaces: ['default', 'rd'], status: 'available' },
  { name: 'subnet-vm', cidr: '10.30.0.0/22', gateway: '10.30.0.1', cniType: 'Kube-OVN', mode: 'overlay', namespaces: ['vm-973fd1ef'], status: 'available' },
  { name: 'subnet-underlay-gpu', cidr: '192.168.40.0/24', gateway: '192.168.40.1', cniType: 'Kube-OVN', mode: 'underlay', namespaces: ['ai'], status: 'exhausted' },
]

export const ipUsageStatsFixture: IpUsageStat[] = [
  { subnetName: 'subnet-prod-a', cidr: '10.20.0.0/24', allocatedNamespaces: 2, allocationRate: 66, usedIps: 180, totalIps: 250, utilization: 72, critical: false },
  { subnetName: 'subnet-vm', cidr: '10.30.0.0/22', allocatedNamespaces: 1, allocationRate: 33, usedIps: 520, totalIps: 1022, utilization: 51, critical: false },
  { subnetName: 'subnet-underlay-gpu', cidr: '192.168.40.0/24', allocatedNamespaces: 1, allocationRate: 33, usedIps: 240, totalIps: 250, utilization: 96, critical: true },
]

export const subnetRequestsFixture: SubnetRequest[] = [
  { id: 'REQ-20260807-001', namespace: 'rd', requestedCidr: '10.40.0.0/24', status: 'pending', requestedAt: '2026-08-07 09:30:00' },
  { id: 'REQ-20260807-002', namespace: 'ai', requestedCidr: '10.41.0.0/22', status: 'pending', requestedAt: '2026-08-07 10:12:00' },
  { id: 'REQ-20260806-003', namespace: 'dr', requestedCidr: '10.42.0.0/24', status: 'approved', requestedAt: '2026-08-06 16:00:00' },
  { id: 'REQ-20260805-004', namespace: 'test', requestedCidr: '10.43.0.0/24', status: 'rejected', requestedAt: '2026-08-05 14:00:00' },
]
