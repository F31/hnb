import type { AccessIngress, AccessNetworkPolicy, AccessService, MetalLBPool } from '../accessApi'

export const accessServicesFixture: AccessService[] = Array.from({ length: 45 }, (_, index) => {
  const number = index + 1
  const nodePort = number % 3 === 0
  return {
    name: number === 1 ? 'os-elasticsearch' : `service-${String(number).padStart(2, '0')}`,
    namespace: 'default',
    type: nodePort ? 'NodePort' : 'ClusterIP',
    clusterIp: number % 11 === 0 ? 'None' : `10.96.${Math.floor(number / 255)}.${20 + number}`,
    ipv6: false,
    ports: [
      { name: 'http-main', port: 28000 + number, targetPort: 28000 + number, protocol: 'TCP', nodePort: nodePort ? 30000 + number : undefined },
      ...(number % 5 === 0 ? [{ name: 'http-admin', port: 29000 + number, targetPort: 29000 + number, protocol: 'TCP' as const }] : []),
    ],
    selector: { app: `application-${number}` },
    appCategory: number % 2 ? 'stateful' : 'stateless',
    appName: `application-${number}`,
    labels: { app: `application-${number}` },
    createdAt: new Date(Date.UTC(2026, 7, 1 + (index % 8), 9, index % 60)).toISOString(),
  }
})

accessServicesFixture.push(
  {
    name: 'argocd-server', namespace: 'argocd', type: 'ClusterIP', clusterIp: '10.96.10.10', ipv6: false,
    ports: [{ name: 'https', port: 443, targetPort: 8080, protocol: 'TCP' }], selector: { app: 'argocd-server' },
    appCategory: 'stateless', appName: 'argocd-server', labels: { app: 'argocd-server' }, createdAt: '2026-08-01T08:00:00Z',
  },
  {
    name: 'grafana', namespace: 'argocd', type: 'ClusterIP', clusterIp: '10.96.10.11', ipv6: false,
    ports: [{ name: 'http', port: 3000, targetPort: 3000, protocol: 'TCP' }], selector: { app: 'grafana' },
    appCategory: 'stateless', appName: 'grafana', labels: { app: 'grafana' }, createdAt: '2026-08-02T08:00:00Z',
  },
)

export const accessIngressesFixture: AccessIngress[] = [
  {
    name: 'argocd-server', namespace: 'argocd', tls: false,
    rules: [{ host: 'argocd.hnb.local', path: '/argocd', serviceName: 'argocd-server', servicePortName: 'https', servicePort: 443 }],
    labels: { app: 'argocd' }, createdAt: '2026-08-01T08:00:00Z',
  },
  {
    name: 'grafana', namespace: 'argocd', tls: true, certificate: 'grafana-tls',
    rules: [{ host: 'grafana.hnb.local', path: '/', serviceName: 'grafana', servicePortName: 'http', servicePort: 3000 }],
    labels: { app: 'grafana' }, createdAt: '2026-08-02T09:00:00Z',
  },
  {
    name: 'api-route', namespace: 'argocd', tls: false,
    rules: [
      { host: 'api.hnb.local', path: '/v1', serviceName: 'api-gateway', servicePortName: 'http', servicePort: 8080 },
      { host: 'api.hnb.local', path: '/health', serviceName: 'api-gateway', servicePortName: 'admin', servicePort: 8081 },
    ],
    labels: { app: 'api' }, createdAt: '2026-08-03T10:00:00Z',
  },
]

export const metalLbPoolsFixture: MetalLBPool[] = [
  { name: 'production-pool', description: '生产环境负载均衡地址池', startIp: '10.20.30.100', endIp: '10.20.30.149', availableIps: 50, usedIps: 12, createdAt: '2026-08-01T08:00:00Z' },
  { name: 'development-pool', description: '开发测试地址池', startIp: '10.20.40.10', endIp: '10.20.40.29', availableIps: 20, usedIps: 5, createdAt: '2026-08-02T08:00:00Z' },
  { name: 'dmz-pool', description: '', startIp: '172.20.10.50', endIp: '172.20.10.59', availableIps: 10, usedIps: 2, createdAt: '2026-08-03T08:00:00Z' },
]

export const accessNetworkPoliciesFixture: AccessNetworkPolicy[] = [
  {
    name: 'argocd-application-controller-network-policy', namespace: 'argocd', policyTypes: ['Ingress'], description: '限制应用控制器入站访问',
    matchLabels: { 'app.kubernetes.io/name': 'argocd-application-controller' },
    ingress: [{ namespace: 'argocd', port: 8082, protocol: 'TCP' }], egress: [], labels: { managedBy: 'hnb' }, createdAt: '2026-08-01T10:00:00Z',
  },
  {
    name: 'argocd-server-network-policy', namespace: 'argocd', policyTypes: ['Ingress', 'Egress'], description: 'ArgoCD Server 网络访问控制',
    matchLabels: { 'app.kubernetes.io/name': 'argocd-server' },
    ingress: [{ namespace: 'ingress-nginx', port: 443, protocol: 'TCP' }], egress: [{ namespace: 'argocd', port: 6379, protocol: 'TCP' }], labels: { managedBy: 'hnb' }, createdAt: '2026-08-02T10:00:00Z',
  },
  {
    name: 'argocd-repo-server-ingress', namespace: 'argocd', policyTypes: ['Ingress'], description: '',
    matchLabels: { 'app.kubernetes.io/name': 'argocd-repo-server' }, ingress: [], egress: [], labels: {}, createdAt: '2026-08-03T10:00:00Z',
  },
]
