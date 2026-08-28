/**
 * 资源-网络管控 service adapter。
 * 页面只依赖 typed 函数；开发 fixture（含子网/申请的内存变更），生产空态。
 */
import type { ContainerSubnet, IpUsageStat, SubnetRequest } from '../types/network'
import { ipUsageStatsFixture, subnetRequestsFixture, subnetsFixture } from './fixtures/network'
import { pluginT } from './pluginI18n'

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

// ---------------------------------------------------------------------------
// 容器子网
// ---------------------------------------------------------------------------

export async function getSubnets(params: { keyword?: string } = {}): Promise<ContainerSubnet[]> {
  if (!USE_FIXTURES) return []
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  if (!kw) return subnetsFixture
  return subnetsFixture.filter((s) => s.name.toLowerCase().includes(kw) || s.cidr.includes(kw))
}

export async function createSubnet(payload: ContainerSubnet): Promise<void> {
  if (USE_FIXTURES) {
    subnetsFixture.push(payload)
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.networkUnavailable'))
}

export async function updateSubnet(name: string, payload: ContainerSubnet): Promise<void> {
  if (USE_FIXTURES) {
    const idx = subnetsFixture.findIndex((s) => s.name === name)
    if (idx >= 0) subnetsFixture[idx] = payload
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.networkUnavailable'))
}

export async function deleteSubnet(name: string): Promise<void> {
  if (USE_FIXTURES) {
    const idx = subnetsFixture.findIndex((s) => s.name === name)
    if (idx >= 0) subnetsFixture.splice(idx, 1)
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.networkUnavailable'))
}

// ---------------------------------------------------------------------------
// IP 资源统计
// ---------------------------------------------------------------------------

export async function getIpUsageStats(): Promise<IpUsageStat[]> {
  if (!USE_FIXTURES) return []
  return ipUsageStatsFixture
}

// ---------------------------------------------------------------------------
// 网段申请审批
// ---------------------------------------------------------------------------

export async function getSubnetRequests(): Promise<SubnetRequest[]> {
  if (!USE_FIXTURES) return []
  return subnetRequestsFixture
}

/** 审批通过：自动创建子网并绑定到申请命名空间 */
export async function approveSubnetRequest(request: SubnetRequest): Promise<void> {
  if (USE_FIXTURES) {
    const idx = subnetRequestsFixture.findIndex((r) => r.id === request.id)
    if (idx >= 0) subnetRequestsFixture[idx] = { ...request, status: 'approved' }
    subnetsFixture.push({
      name: `subnet-${request.namespace.toLowerCase()}`,
      cidr: request.requestedCidr,
      gateway: `${request.requestedCidr.replace(/\/.*$/, '').replace(/\.\d+$/, '')}.1`,
      cniType: 'Kube-OVN',
      mode: 'overlay',
      namespaces: [request.namespace],
      status: 'available',
    })
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.networkUnavailable'))
}

/** 拒绝申请 */
export async function rejectSubnetRequest(request: SubnetRequest): Promise<void> {
  if (USE_FIXTURES) {
    const idx = subnetRequestsFixture.findIndex((r) => r.id === request.id)
    if (idx >= 0) subnetRequestsFixture[idx] = { ...request, status: 'rejected' }
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.networkUnavailable'))
}
