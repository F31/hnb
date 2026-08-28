/**
 * CNI 能力矩阵 fixture（对照 OpenSpec CNI 能力速查表）。
 * Kube-OVN 为默认安装（与插件市场目录一致）；Cilium / Calico 供参考展示。
 */
import type { CniCapabilityMatrix } from '../../types/capability'

export const cniCapabilityMatricesFixture: CniCapabilityMatrix[] = [
  {
    cni: 'Kube-OVN',
    version: 'v1.12.0',
    capabilities: {
      networkPolicy: 'medium',
      serviceLoadBalancing: 'medium',
      qosBandwidth: 'medium',
      subnetIsolation: 'strong',
      rdma: 'strong',
      observability: 'medium',
      networkAnomalyDetection: 'none',
      diagnosis: 'weak',
    },
  },
  {
    cni: 'Cilium',
    version: 'v1.15.0',
    capabilities: {
      networkPolicy: 'strong',
      serviceLoadBalancing: 'strong',
      qosBandwidth: 'strong',
      subnetIsolation: 'weak',
      rdma: 'none',
      observability: 'strong',
      networkAnomalyDetection: 'strong',
      diagnosis: 'strong',
    },
  },
  {
    cni: 'Calico',
    version: 'v3.27.0',
    capabilities: {
      networkPolicy: 'strong',
      serviceLoadBalancing: 'medium',
      qosBandwidth: 'weak',
      subnetIsolation: 'medium',
      rdma: 'none',
      observability: 'none',
      networkAnomalyDetection: 'none',
      diagnosis: 'weak',
    },
  },
]
