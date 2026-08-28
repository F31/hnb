/**
 * 容器-网络使用 fixture（服务 / 应用路由 / 网段信息 / 网段申请）。
 * 仅构建时显式设置 VITE_CLUSTER_DETAIL_USE_FIXTURES=true 时启用。
 */
import type {
  ContainerIngress,
  ContainerNetworkPolicy,
  ContainerService,
  NamespaceSubnetInfo,
  QosBandwidthPolicy,
  RdmaPoolInfo,
} from '../containerNetworkApi'

export const servicesFixture: ContainerService[] = [
  { name: 'web-frontend', type: 'ClusterIP', clusterIp: '10.20.1.10', ports: '80:8080/TCP', selector: 'app=web', namespace: 'default', createdAt: '2026-08-01 10:00:00' },
  { name: 'api-gateway', type: 'NodePort', clusterIp: '10.20.1.11', ports: '80:8080/TCP, 443:8443/TCP', selector: 'app=api', namespace: 'default', createdAt: '2026-08-02 11:30:00' },
  { name: 'redis', type: 'ClusterIP', clusterIp: '10.20.1.12', ports: '6379:6379/TCP', selector: 'app=redis', namespace: 'rd', createdAt: '2026-07-20 09:00:00' },
]

export const ingressesFixture: ContainerIngress[] = [
  { name: 'web-ingress', domain: 'web.hnb.local', path: '/', backendService: 'web-frontend', backendPort: 80, tls: true, namespace: 'default', createdAt: '2026-08-01 10:05:00' },
  { name: 'api-ingress', domain: 'api.hnb.local', path: '/v1', backendService: 'api-gateway', backendPort: 80, tls: false, namespace: 'default', createdAt: '2026-08-02 11:35:00' },
]

export const subnetInfoFixture: NamespaceSubnetInfo[] = [
  { subnetName: 'subnet-prod-a', cidr: '10.20.0.0/24', gateway: '10.20.0.1', cniType: 'Kube-OVN', mode: 'overlay', usedIps: 180, totalIps: 250 },
]

/** 容器层发起的网段申请（资源层审批） */
export interface SubnetRequestRecord {
  id: string
  namespace: string
  requestedCidr: string
  status: 'pending' | 'approved' | 'rejected'
  requestedAt: string
}

export const subnetRequestRecordsFixture: SubnetRequestRecord[] = []

/** 网络策略 */
export const networkPoliciesFixture: ContainerNetworkPolicy[] = [
  { name: 'default-deny-all', namespace: 'default', podSelector: '*', policyTypes: 'Ingress', ingressFrom: '', egressTo: '', createdAt: '2026-08-01 10:00:00' },
  { name: 'allow-web-from-api', namespace: 'default', podSelector: 'app=web', policyTypes: 'Ingress', ingressFrom: 'api', egressTo: '', createdAt: '2026-08-02 11:00:00' },
]

/** QoS / 带宽策略 */
export const qosPoliciesFixture: QosBandwidthPolicy[] = [
  { name: 'qos-web-limit', workload: 'web-frontend', namespace: 'default', ingressBandwidth: '100M', egressBandwidth: '50M', createdAt: '2026-08-03 09:00:00' },
]

/** RDMA 资源池 */
export const rdmaPoolFixture: RdmaPoolInfo = { poolName: 'rdma-pool-gpu', availableNodes: 3, rdmaEnabled: true }

