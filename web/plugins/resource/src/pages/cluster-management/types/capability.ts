/**
 * CNI 能力探测领域类型（UI 规范 / restore-cluster-detail-console 能力矩阵）。
 * 用于容器层功能显隐与置灰判断：按安装的 CNI 插件决定 NetworkPolicy / QoS /
 * RDMA / 可观测等能力是否可用（CNI 无关原则的前提）。
 */

export type CniName = 'kube-ovn' | 'cilium' | 'calico'

/** 能力级别 */
export type CapabilityLevel = 'strong' | 'medium' | 'weak' | 'none'

/** CNI 能力特性（供 hasCapability 判断） */
export type CniFeature =
  | 'networkPolicy'
  | 'serviceLoadBalancing'
  | 'qosBandwidth'
  | 'subnetIsolation'
  | 'rdma'
  | 'observability'
  | 'networkAnomalyDetection'
  | 'diagnosis'

/** 单个 CNI 的能力矩阵 */
export interface CniCapabilityMatrix {
  cni: CniName
  version: string
  capabilities: Record<CniFeature, CapabilityLevel>
}

/** 能力总览：全部 CNI 矩阵 + 当前已安装的 CNI */
export interface CniCapabilityOverview {
  cnis: CniCapabilityMatrix[]
  installedCni: CniName | null
}

/** 判定特性是否可用（strong/medium/weak 均视为可用，none 视为不可用） */
export function isCniCapabilityAvailable(level: CapabilityLevel | undefined): boolean {
  return level === 'strong' || level === 'medium' || level === 'weak'
}
