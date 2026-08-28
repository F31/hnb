/**
 * 容器-网络使用数据访问层（Container 插件）。
 * 服务 / 应用路由走 K8s proxy；网段信息与申请对接资源层（fixture 支撑演示）。
 * 与 containerApi 一致：统一走 @hnb/api-client；开发 fixture 由
 * VITE_CLUSTER_DETAIL_USE_FIXTURES=true 显式开启。
 */
import type { ApiClient, ContextStore } from '@hnb/types'

export type ServiceType = 'ClusterIP' | 'NodePort' | 'LoadBalancer'

export interface ContainerService {
  name: string
  type: ServiceType
  clusterIp: string
  ports: string
  selector: string
  namespace: string
  createdAt: string
}

export interface ContainerIngress {
  name: string
  domain: string
  path: string
  backendService: string
  backendPort: number
  tls: boolean
  namespace: string
  createdAt: string
}

export interface NamespaceSubnetInfo {
  subnetName: string
  cidr: string
  gateway: string
  cniType: string
  mode: string
  usedIps: number
  totalIps: number
}

export interface SubnetRequestPayload {
  namespace: string
  requestedCidr: string
}

let apiClient: ApiClient | null = null
let contextStore: ContextStore | null = null

export function setContainerNetworkClient(client: ApiClient): void {
  apiClient = client
}

export function setContainerNetworkContext(store: ContextStore): void {
  contextStore = store
}

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

function client(): ApiClient {
  if (!apiClient) throw new Error('container network api client is not initialized')
  return apiClient
}

function currentNamespace(): string {
  return contextStore?.current?.spaceId ?? 'default'
}

function proxyUrl(clusterId: string, path: string): string {
  return `/api/v1/proxy/${encodeURIComponent(clusterId)}/${path.replace(/^\//, '')}`
}

// ---------------------------------------------------------------------------
// 服务管理（K8s Service）
// ---------------------------------------------------------------------------

export async function listServices(clusterId: string, namespace?: string): Promise<ContainerService[]> {
  if (USE_FIXTURES) {
    const ns = namespace || currentNamespace()
    const { servicesFixture } = await import('./fixtures/containerNetwork')
    return servicesFixture.filter((s) => s.namespace === ns || ns === '*')
  }
  const ns = namespace || currentNamespace()
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(ns)}/services`))
  return (data?.items ?? []).map((raw) => mapK8sService(raw as Record<string, unknown>))
}

function mapK8sService(raw: Record<string, unknown>): ContainerService {
  const meta = (raw.metadata ?? {}) as Record<string, unknown>
  const spec = (raw.spec ?? {}) as Record<string, unknown>
  const ports = Array.isArray(spec.ports) ? (spec.ports as Array<Record<string, unknown>>) : []
  return {
    name: String(meta.name ?? ''),
    type: (spec.type as ServiceType) ?? 'ClusterIP',
    clusterIp: String(spec.clusterIP ?? '--'),
    ports: ports.map((p) => `${String(p.port ?? '')}:${String(p.targetPort ?? '')}/${String(p.protocol ?? '')}`).join(', ') || '--',
    selector: Object.entries((spec.selector ?? {}) as Record<string, string>).map(([k, v]) => `${k}=${v}`).join(', ') || '--',
    namespace: String(meta.namespace ?? ''),
    createdAt: String(meta.creationTimestamp ?? ''),
  }
}

export interface CreateServicePayload {
  name: string
  type: ServiceType
  port: number
  targetPort: number
  namespace: string
}

export async function createService(clusterId: string, payload: CreateServicePayload): Promise<void> {
  if (USE_FIXTURES) return
  await client().post(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(payload.namespace)}/services`), {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: { name: payload.name, namespace: payload.namespace },
    spec: { type: payload.type, selector: { app: payload.name }, ports: [{ port: payload.port, targetPort: payload.targetPort }] },
  })
}

export async function deleteService(clusterId: string, name: string, namespace: string): Promise<void> {
  if (USE_FIXTURES) return
  await client().delete(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/services/${encodeURIComponent(name)}`))
}

// ---------------------------------------------------------------------------
// 应用路由（Ingress）
// ---------------------------------------------------------------------------

export async function listIngresses(clusterId: string, namespace?: string): Promise<ContainerIngress[]> {
  if (USE_FIXTURES) {
    const ns = namespace || currentNamespace()
    const { ingressesFixture } = await import('./fixtures/containerNetwork')
    return ingressesFixture.filter((i) => i.namespace === ns || ns === '*')
  }
  const ns = namespace || currentNamespace()
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(ns)}/ingresses`))
  return (data?.items ?? []).map((raw) => mapK8sIngress(raw as Record<string, unknown>))
}

function mapK8sIngress(raw: Record<string, unknown>): ContainerIngress {
  const meta = (raw.metadata ?? {}) as Record<string, unknown>
  const spec = (raw.spec ?? {}) as Record<string, unknown>
  const tls = Array.isArray(spec.tls) && (spec.tls as unknown[]).length > 0
  const rules = Array.isArray(spec.rules) ? (spec.rules as Array<Record<string, unknown>>) : []
  const rule = rules[0] ?? {}
  const http = (rule.http ?? {}) as Record<string, unknown>
  const paths = Array.isArray(http.paths) ? (http.paths as Array<Record<string, unknown>>) : []
  const pathObj = paths[0] ?? {}
  const backend = (pathObj.backend ?? {}) as Record<string, unknown>
  const service = (backend.service ?? {}) as Record<string, unknown>
  return {
    name: String(meta.name ?? ''),
    domain: String(rule.host ?? ''),
    path: String(pathObj.path ?? '/'),
    backendService: String(service.name ?? ''),
    backendPort: Number(((service.port ?? {}) as Record<string, unknown>).number ?? 80),
    tls,
    namespace: String(meta.namespace ?? ''),
    createdAt: String(meta.creationTimestamp ?? ''),
  }
}

export interface CreateIngressPayload {
  name: string
  domain: string
  path: string
  backendService: string
  backendPort: number
  tls: boolean
  namespace: string
}

export async function createIngress(clusterId: string, payload: CreateIngressPayload): Promise<void> {
  if (USE_FIXTURES) return
  await client().post(proxyUrl(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(payload.namespace)}/ingresses`), {
    apiVersion: 'networking.k8s.io/v1',
    kind: 'Ingress',
    metadata: { name: payload.name, namespace: payload.namespace },
    spec: {
      tls: payload.tls ? [{ hosts: [payload.domain] }] : undefined,
      rules: [{ host: payload.domain, http: { paths: [{ path: payload.path, pathType: 'Prefix', backend: { service: { name: payload.backendService, port: { number: payload.backendPort } } } }] } }],
    },
  })
}

export async function deleteIngress(clusterId: string, name: string, namespace: string): Promise<void> {
  if (USE_FIXTURES) return
  await client().delete(proxyUrl(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(namespace)}/ingresses/${encodeURIComponent(name)}`))
}

// ---------------------------------------------------------------------------
// 网段信息（只读，来源资源层） + 申请新网段
// ---------------------------------------------------------------------------

export async function getNamespaceSubnets(namespace?: string): Promise<NamespaceSubnetInfo[]> {
  if (!USE_FIXTURES) return []
  const ns = namespace || currentNamespace()
  const { subnetInfoFixture } = await import('./fixtures/containerNetwork')
  return subnetInfoFixture.filter((s) => ns === '*' || ns === 'default' || ns === 'rd' || ns === 'ai' || ns === 'dr')
}

export async function requestSubnet(payload: SubnetRequestPayload): Promise<void> {
  if (USE_FIXTURES) {
    const { subnetRequestRecordsFixture } = await import('./fixtures/containerNetwork')
    subnetRequestRecordsFixture.push({
      id: `REQ-${Date.now()}`,
      namespace: payload.namespace,
      requestedCidr: payload.requestedCidr,
      status: 'pending',
      requestedAt: new Date().toISOString().slice(0, 19).replace('T', ' '),
    })
    return
  }
  await client().post('/api/v1/networks/subnet-requests', payload)
}

// ---------------------------------------------------------------------------
// 网络策略（NetworkPolicy）
// ---------------------------------------------------------------------------

export interface ContainerNetworkPolicy {
  name: string
  namespace: string
  podSelector: string
  policyTypes: string
  ingressFrom: string
  egressTo: string
  createdAt: string
}

export interface CreateNetworkPolicyPayload {
  name: string
  namespace: string
  podSelector: string
  policyTypes: 'ingress' | 'egress' | 'both'
  ingressFrom: string
  egressTo: string
}

export async function listNetworkPolicies(clusterId: string, namespace?: string): Promise<ContainerNetworkPolicy[]> {
  if (USE_FIXTURES) {
    const { networkPoliciesFixture } = await import('./fixtures/containerNetwork')
    const ns = namespace || currentNamespace()
    return networkPoliciesFixture.filter((p) => p.namespace === ns || ns === '*')
  }
  const ns = namespace || currentNamespace()
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(ns)}/networkpolicies`))
  return (data?.items ?? []).map((raw) => mapK8sNetworkPolicy(raw as Record<string, unknown>))
}

function mapK8sNetworkPolicy(raw: Record<string, unknown>): ContainerNetworkPolicy {
  const meta = (raw.metadata ?? {}) as Record<string, unknown>
  const spec = (raw.spec ?? {}) as Record<string, unknown>
  const podSelector = (spec.podSelector ?? {}) as Record<string, unknown>
  const match = Object.entries((podSelector.matchLabels ?? {}) as Record<string, string>).map(([k, v]) => `${k}=${v}`).join(', ')
  const types = Array.isArray(spec.policyTypes) ? (spec.policyTypes as string[]).join(', ') : ''
  return {
    name: String(meta.name ?? ''),
    namespace: String(meta.namespace ?? ''),
    podSelector: match || '*',
    policyTypes: types || '--',
    ingressFrom: '--',
    egressTo: '--',
    createdAt: String(meta.creationTimestamp ?? ''),
  }
}

export async function createNetworkPolicy(clusterId: string, payload: CreateNetworkPolicyPayload): Promise<void> {
  if (USE_FIXTURES) return
  const policyTypes = payload.policyTypes === 'both' ? ['Ingress', 'Egress'] : [payload.policyTypes === 'ingress' ? 'Ingress' : 'Egress']
  await client().post(proxyUrl(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(payload.namespace)}/networkpolicies`), {
    apiVersion: 'networking.k8s.io/v1',
    kind: 'NetworkPolicy',
    metadata: { name: payload.name, namespace: payload.namespace },
    spec: {
      podSelector: { matchLabels: { app: payload.podSelector } },
      policyTypes,
      ingress: payload.ingressFrom ? [{ from: [{ namespaceSelector: { matchLabels: { name: payload.ingressFrom } } }] }] : [],
      egress: payload.egressTo ? [{ to: [{ namespaceSelector: { matchLabels: { name: payload.egressTo } } }] }] : [],
    },
  })
}

export async function deleteNetworkPolicy(clusterId: string, name: string, namespace: string): Promise<void> {
  if (USE_FIXTURES) return
  await client().delete(proxyUrl(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(namespace)}/networkpolicies/${encodeURIComponent(name)}`))
}

// ---------------------------------------------------------------------------
// QoS / 带宽策略
// ---------------------------------------------------------------------------

export interface QosBandwidthPolicy {
  name: string
  workload: string
  namespace: string
  ingressBandwidth: string
  egressBandwidth: string
  createdAt: string
}

export interface CreateQosPolicyPayload {
  name: string
  workload: string
  namespace: string
  ingressBandwidth: string
  egressBandwidth: string
}

export async function listQosPolicies(namespace?: string): Promise<QosBandwidthPolicy[]> {
  if (!USE_FIXTURES) return []
  const { qosPoliciesFixture } = await import('./fixtures/containerNetwork')
  const ns = namespace || currentNamespace()
  return qosPoliciesFixture.filter((p) => p.namespace === ns || ns === '*')
}

export async function createQosPolicy(payload: CreateQosPolicyPayload): Promise<void> {
  if (USE_FIXTURES) {
    const { qosPoliciesFixture } = await import('./fixtures/containerNetwork')
    qosPoliciesFixture.push({ ...payload, createdAt: new Date().toISOString().slice(0, 19).replace('T', ' ') })
    return
  }
  throw new Error('QoS 带宽策略接口未开放')
}

export async function deleteQosPolicy(name: string): Promise<void> {
  if (USE_FIXTURES) {
    const { qosPoliciesFixture } = await import('./fixtures/containerNetwork')
    const idx = qosPoliciesFixture.findIndex((p) => p.name === name)
    if (idx >= 0) qosPoliciesFixture.splice(idx, 1)
    return
  }
  throw new Error('QoS 带宽策略接口未开放')
}

// ---------------------------------------------------------------------------
// 高性能网络（RDMA 资源池）
// ---------------------------------------------------------------------------

export interface RdmaPoolInfo {
  poolName: string
  availableNodes: number
  rdmaEnabled: boolean
}

export async function getRdmaPoolInfo(): Promise<RdmaPoolInfo | null> {
  if (!USE_FIXTURES) return null
  const { rdmaPoolFixture } = await import('./fixtures/containerNetwork')
  return rdmaPoolFixture
}

// ---------------------------------------------------------------------------
// 网络诊断
// ---------------------------------------------------------------------------

export type DiagnosisType = 'connectivity' | 'networkPolicy' | 'dns'

export interface DiagnosisResult {
  id: string
  type: DiagnosisType
  source: string
  target: string
  result: 'success' | 'fail'
  detail: string
}

export async function runDiagnosis(type: DiagnosisType, source: string, target: string): Promise<DiagnosisResult> {
  if (USE_FIXTURES) {
    const ok = type === 'connectivity' || type === 'dns'
    const names = { connectivity: ['连通性测试', '可达'], networkPolicy: ['策略命中测试', '被默认拒绝策略拦截'], dns: ['DNS 解析测试', '解析成功'] }
    return {
      id: `DG-${Date.now()}`,
      type,
      source,
      target,
      result: ok ? 'success' : 'fail',
      detail: `${names[type][0]}：${source} → ${target}，${names[type][1]}`,
    }
  }
  throw new Error('网络诊断接口未开放')
}

// ---------------------------------------------------------------------------
// CNI 能力判定（网络策略 / QoS / RDMA / 诊断 的显隐与置灰）
// ---------------------------------------------------------------------------

export type ContainerCniFeature = 'networkPolicy' | 'qosBandwidth' | 'rdma' | 'diagnosis'

/** 当前 CNI（fixture 默认为 Kube-OVN）是否支持某项能力 */
export function containerCniFeatureAvailable(feature: ContainerCniFeature): boolean {
  if (USE_FIXTURES) return true
  // 生产：由资源层能力探测决定；这里按 K8s 标准能力保守放行
  return feature === 'networkPolicy' || feature === 'diagnosis'
}
