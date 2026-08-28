/**
 * 资源-网络管控领域类型（容器子网 / IP 资源统计 / 网段申请审批）。
 * 纯管控原则：网段创建/分配权收在资源层；后端端点暂缺，adapter + fixture 支撑。
 */

/** 容器子网 */
export interface ContainerSubnet {
  name: string
  cidr: string
  gateway: string
  cniType: string
  mode: 'overlay' | 'underlay'
  namespaces: string[]
  status: 'available' | 'exhausted' | 'unknown'
}

/** IP 资源统计（单条子网） */
export interface IpUsageStat {
  subnetName: string
  cidr: string
  allocatedNamespaces: number
  allocationRate: number
  usedIps: number
  totalIps: number
  utilization: number
  critical: boolean
}

/** 网段申请（容器层发起，运维审批） */
export interface SubnetRequest {
  id: string
  namespace: string
  requestedCidr: string
  status: 'pending' | 'approved' | 'rejected'
  requestedAt: string
}
