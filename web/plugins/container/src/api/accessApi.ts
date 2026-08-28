import type { ApiClient } from '@hnb/types'

export type ServiceAccessType = 'ClusterIP' | 'NodePort' | 'LoadBalancer'
export type NetworkProtocol = 'TCP' | 'UDP' | 'SCTP'

export interface AccessServicePort {
  name: string
  port: number
  targetPort: number | string
  protocol: NetworkProtocol
  nodePort?: number
}

export interface AccessService {
  name: string
  namespace: string
  type: ServiceAccessType
  clusterIp: string
  ipv6: boolean
  ports: AccessServicePort[]
  selector: Record<string, string>
  appCategory: string
  appName: string
  labels: Record<string, string>
  createdAt: string
}

export interface AccessIngressRule {
  host: string
  path: string
  serviceName: string
  servicePortName: string
  servicePort: number
}

export interface AccessIngress {
  name: string
  namespace: string
  tls: boolean
  certificate?: string
  rules: AccessIngressRule[]
  labels: Record<string, string>
  createdAt: string
}

export interface MetalLBPool {
  name: string
  description: string
  startIp: string
  endIp: string
  availableIps: number
  usedIps: number
  createdAt: string
}

export interface AccessPolicyRule {
  namespace: string
  port?: number
  protocol: NetworkProtocol
}

export interface AccessNetworkPolicy {
  name: string
  namespace: string
  policyTypes: Array<'Ingress' | 'Egress'>
  description: string
  matchLabels: Record<string, string>
  ingress: AccessPolicyRule[]
  egress: AccessPolicyRule[]
  labels: Record<string, string>
  createdAt: string
}

let apiClient: ApiClient | null = null
const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

export function setContainerAccessClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('container access api client is not initialized')
  return apiClient
}

function proxyUrl(clusterId: string, path: string): string {
  return `/api/v1/proxy/${encodeURIComponent(clusterId)}/${path.replace(/^\//, '')}`
}

function record(value: unknown): Record<string, any> {
  return value && typeof value === 'object' ? value as Record<string, any> : {}
}

function stringMap(value: unknown): Record<string, string> {
  return Object.fromEntries(Object.entries(record(value)).map(([key, item]) => [key, String(item)]))
}

function replaceFixture<T extends { name: string }>(items: T[], item: T, originalName?: string): void {
  const index = items.findIndex((candidate) => candidate.name === (originalName || item.name))
  if (index >= 0) items.splice(index, 1, item)
  else items.unshift(item)
}

function removeFixture<T extends { name: string }>(items: T[], name: string): void {
  const index = items.findIndex((item) => item.name === name)
  if (index >= 0) items.splice(index, 1)
}

export function mapAccessService(raw: unknown): AccessService {
  const item = record(raw)
  const metadata = record(item.metadata)
  const spec = record(item.spec)
  const annotations = record(metadata.annotations)
  return {
    name: String(metadata.name ?? ''), namespace: String(metadata.namespace ?? ''), type: spec.type ?? 'ClusterIP',
    clusterIp: String(spec.clusterIP ?? 'None'), ipv6: Array.isArray(spec.ipFamilies) && spec.ipFamilies.includes('IPv6'),
    ports: Array.isArray(spec.ports) ? spec.ports.map((port: any) => ({
      name: String(port.name ?? ''), port: Number(port.port ?? 0), targetPort: port.targetPort ?? port.port ?? 0,
      protocol: port.protocol ?? 'TCP', nodePort: port.nodePort ? Number(port.nodePort) : undefined,
    })) : [],
    selector: stringMap(spec.selector), appCategory: String(annotations['hnb.io/app-category'] ?? ''), appName: String(annotations['hnb.io/app-name'] ?? ''),
    labels: stringMap(metadata.labels), createdAt: String(metadata.creationTimestamp ?? ''),
  }
}

export function mapAccessIngress(raw: unknown): AccessIngress {
  const item = record(raw)
  const metadata = record(item.metadata)
  const spec = record(item.spec)
  const rules: AccessIngressRule[] = []
  for (const rule of Array.isArray(spec.rules) ? spec.rules : []) {
    for (const path of Array.isArray(rule.http?.paths) ? rule.http.paths : []) {
      rules.push({
        host: String(rule.host ?? ''), path: String(path.path ?? '/'), serviceName: String(path.backend?.service?.name ?? ''),
        servicePortName: String(path.backend?.service?.port?.name ?? ''), servicePort: Number(path.backend?.service?.port?.number ?? 0),
      })
    }
  }
  const tls = Array.isArray(spec.tls) && spec.tls.length > 0
  return {
    name: String(metadata.name ?? ''), namespace: String(metadata.namespace ?? ''), tls,
    certificate: tls ? String(spec.tls[0]?.secretName ?? '') : '', rules, labels: stringMap(metadata.labels), createdAt: String(metadata.creationTimestamp ?? ''),
  }
}

function ipToNumber(ip: string): number {
  const parts = ip.split('.').map(Number)
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) return -1
  return parts.reduce((total, part) => total * 256 + part, 0)
}

export function validIpRange(startIp: string, endIp: string): boolean {
  const start = ipToNumber(startIp)
  const end = ipToNumber(endIp)
  return start >= 0 && end >= start
}

function addressCount(startIp: string, endIp: string): number {
  const start = ipToNumber(startIp)
  const end = ipToNumber(endIp)
  return start >= 0 && end >= start ? end - start + 1 : 0
}

export function mapMetalLBPool(raw: unknown): MetalLBPool {
  const item = record(raw)
  const metadata = record(item.metadata)
  const spec = record(item.spec)
  const annotations = record(metadata.annotations)
  const range = String(Array.isArray(spec.addresses) ? spec.addresses[0] ?? '' : '').split('-')
  const startIp = range[0] ?? ''
  const endIp = range[1] ?? range[0] ?? ''
  return {
    name: String(metadata.name ?? ''), description: String(annotations['hnb.io/description'] ?? ''), startIp, endIp,
    availableIps: Number(annotations['hnb.io/available-ips'] ?? addressCount(startIp, endIp)), usedIps: Number(annotations['hnb.io/used-ips'] ?? 0),
    createdAt: String(metadata.creationTimestamp ?? ''),
  }
}

function mapPolicyRules(rules: unknown): AccessPolicyRule[] {
  if (!Array.isArray(rules)) return []
  return rules.map((rule: any) => {
    const peer = rule.from?.[0] ?? rule.to?.[0] ?? {}
    const port = rule.ports?.[0]
    return {
      namespace: String(peer.namespaceSelector?.matchLabels?.['kubernetes.io/metadata.name'] ?? ''),
      port: port?.port ? Number(port.port) : undefined, protocol: port?.protocol ?? 'TCP',
    }
  })
}

export function mapAccessNetworkPolicy(raw: unknown): AccessNetworkPolicy {
  const item = record(raw)
  const metadata = record(item.metadata)
  const spec = record(item.spec)
  return {
    name: String(metadata.name ?? ''), namespace: String(metadata.namespace ?? ''), policyTypes: Array.isArray(spec.policyTypes) ? spec.policyTypes : [],
    description: String(record(metadata.annotations)['hnb.io/description'] ?? ''), matchLabels: stringMap(record(spec.podSelector).matchLabels),
    ingress: mapPolicyRules(spec.ingress), egress: mapPolicyRules(spec.egress), labels: stringMap(metadata.labels), createdAt: String(metadata.creationTimestamp ?? ''),
  }
}

export function serviceResource(item: AccessService): Record<string, unknown> {
  return {
    apiVersion: 'v1', kind: 'Service',
    metadata: { name: item.name, namespace: item.namespace, labels: item.labels, annotations: { 'hnb.io/app-category': item.appCategory, 'hnb.io/app-name': item.appName } },
    spec: { type: item.type, ipFamilies: item.ipv6 ? ['IPv6'] : ['IPv4'], selector: item.selector, ports: item.ports },
  }
}

export function ingressResource(item: AccessIngress): Record<string, unknown> {
  const grouped = new Map<string, AccessIngressRule[]>()
  for (const rule of item.rules) grouped.set(rule.host, [...(grouped.get(rule.host) ?? []), rule])
  return {
    apiVersion: 'networking.k8s.io/v1', kind: 'Ingress', metadata: { name: item.name, namespace: item.namespace, labels: item.labels },
    spec: {
      tls: item.tls ? [{ hosts: [...grouped.keys()], secretName: item.certificate || undefined }] : undefined,
      rules: [...grouped.entries()].map(([host, rules]) => ({ host, http: { paths: rules.map((rule) => ({
        path: rule.path, pathType: 'Prefix', backend: { service: { name: rule.serviceName, port: rule.servicePortName ? { name: rule.servicePortName } : { number: rule.servicePort } } },
      })) } })),
    },
  }
}

export function metalLBResource(item: MetalLBPool): Record<string, unknown> {
  return {
    apiVersion: 'metallb.io/v1beta1', kind: 'IPAddressPool',
    metadata: { name: item.name, namespace: 'metallb-system', annotations: { 'hnb.io/description': item.description, 'hnb.io/available-ips': String(item.availableIps), 'hnb.io/used-ips': String(item.usedIps) } },
    spec: { addresses: [`${item.startIp}-${item.endIp}`], autoAssign: true },
  }
}

function policyRuleResource(rule: AccessPolicyRule, direction: 'from' | 'to'): Record<string, unknown> {
  return {
    [direction]: rule.namespace ? [{ namespaceSelector: { matchLabels: { 'kubernetes.io/metadata.name': rule.namespace } } }] : undefined,
    ports: rule.port ? [{ port: rule.port, protocol: rule.protocol }] : undefined,
  }
}

export function networkPolicyResource(item: AccessNetworkPolicy): Record<string, unknown> {
  return {
    apiVersion: 'networking.k8s.io/v1', kind: 'NetworkPolicy',
    metadata: { name: item.name, namespace: item.namespace, labels: item.labels, annotations: { 'hnb.io/description': item.description } },
    spec: {
      podSelector: { matchLabels: item.matchLabels }, policyTypes: item.policyTypes,
      ingress: item.policyTypes.includes('Ingress') ? item.ingress.map((rule) => policyRuleResource(rule, 'from')) : undefined,
      egress: item.policyTypes.includes('Egress') ? item.egress.map((rule) => policyRuleResource(rule, 'to')) : undefined,
    },
  }
}

async function listResource<T>(clusterId: string, path: string, mapper: (raw: unknown) => T): Promise<T[]> {
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, path))
  return (data.items ?? []).map(mapper)
}

export async function listAccessServices(clusterId: string, namespace: string): Promise<AccessService[]> {
  if (USE_FIXTURES) return (await import('./fixtures/access')).accessServicesFixture.filter((item) => item.namespace === namespace).map((item) => ({ ...item, ports: item.ports.map((port) => ({ ...port })), labels: { ...item.labels } }))
  return listResource(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/services`, mapAccessService)
}

export async function listAccessIngresses(clusterId: string, namespace: string): Promise<AccessIngress[]> {
  if (USE_FIXTURES) return (await import('./fixtures/access')).accessIngressesFixture.filter((item) => item.namespace === namespace).map((item) => ({ ...item, rules: item.rules.map((rule) => ({ ...rule })) }))
  return listResource(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(namespace)}/ingresses`, mapAccessIngress)
}

export async function listMetalLBPools(clusterId: string): Promise<MetalLBPool[]> {
  if (USE_FIXTURES) return (await import('./fixtures/access')).metalLbPoolsFixture.map((item) => ({ ...item }))
  return listResource(clusterId, 'apis/metallb.io/v1beta1/namespaces/metallb-system/ipaddresspools', mapMetalLBPool)
}

export async function listAccessNetworkPolicies(clusterId: string, namespace: string): Promise<AccessNetworkPolicy[]> {
  if (USE_FIXTURES) return (await import('./fixtures/access')).accessNetworkPoliciesFixture.filter((item) => item.namespace === namespace).map((item) => ({ ...item, matchLabels: { ...item.matchLabels } }))
  return listResource(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(namespace)}/networkpolicies`, mapAccessNetworkPolicy)
}

async function saveResource(clusterId: string, collectionPath: string, resource: Record<string, unknown>, originalName?: string): Promise<void> {
  if (originalName) {
    await client().patch(proxyUrl(clusterId, `${collectionPath}/${encodeURIComponent(originalName)}`), resource, { headers: { 'Content-Type': 'application/merge-patch+json' } })
  } else await client().post(proxyUrl(clusterId, collectionPath), resource)
}

export async function saveAccessService(clusterId: string, item: AccessService, originalName?: string): Promise<void> {
  if (USE_FIXTURES) { const fixtures = await import('./fixtures/access'); replaceFixture(fixtures.accessServicesFixture, { ...item, createdAt: item.createdAt || new Date().toISOString() }, originalName); return }
  await saveResource(clusterId, `api/v1/namespaces/${encodeURIComponent(item.namespace)}/services`, serviceResource(item), originalName)
}

export async function saveAccessIngress(clusterId: string, item: AccessIngress, originalName?: string): Promise<void> {
  if (USE_FIXTURES) { const fixtures = await import('./fixtures/access'); replaceFixture(fixtures.accessIngressesFixture, { ...item, createdAt: item.createdAt || new Date().toISOString() }, originalName); return }
  await saveResource(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(item.namespace)}/ingresses`, ingressResource(item), originalName)
}

export async function saveMetalLBPool(clusterId: string, item: MetalLBPool, originalName?: string): Promise<void> {
  const normalized = { ...item, availableIps: addressCount(item.startIp, item.endIp), createdAt: item.createdAt || new Date().toISOString() }
  if (USE_FIXTURES) { const fixtures = await import('./fixtures/access'); replaceFixture(fixtures.metalLbPoolsFixture, normalized, originalName); return }
  await saveResource(clusterId, 'apis/metallb.io/v1beta1/namespaces/metallb-system/ipaddresspools', metalLBResource(normalized), originalName)
}

export async function saveAccessNetworkPolicy(clusterId: string, item: AccessNetworkPolicy, originalName?: string): Promise<void> {
  if (USE_FIXTURES) { const fixtures = await import('./fixtures/access'); replaceFixture(fixtures.accessNetworkPoliciesFixture, { ...item, createdAt: item.createdAt || new Date().toISOString() }, originalName); return }
  await saveResource(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(item.namespace)}/networkpolicies`, networkPolicyResource(item), originalName)
}

async function deleteResource(clusterId: string, path: string): Promise<void> { await client().delete(proxyUrl(clusterId, path)) }

export async function deleteAccessService(clusterId: string, namespace: string, name: string): Promise<void> {
  if (USE_FIXTURES) { removeFixture((await import('./fixtures/access')).accessServicesFixture, name); return }
  await deleteResource(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/services/${encodeURIComponent(name)}`)
}
export async function deleteAccessIngress(clusterId: string, namespace: string, name: string): Promise<void> {
  if (USE_FIXTURES) { removeFixture((await import('./fixtures/access')).accessIngressesFixture, name); return }
  await deleteResource(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(namespace)}/ingresses/${encodeURIComponent(name)}`)
}
export async function deleteMetalLBPool(clusterId: string, name: string): Promise<void> {
  if (USE_FIXTURES) { removeFixture((await import('./fixtures/access')).metalLbPoolsFixture, name); return }
  await deleteResource(clusterId, `apis/metallb.io/v1beta1/namespaces/metallb-system/ipaddresspools/${encodeURIComponent(name)}`)
}
export async function deleteAccessNetworkPolicy(clusterId: string, namespace: string, name: string): Promise<void> {
  if (USE_FIXTURES) { removeFixture((await import('./fixtures/access')).accessNetworkPoliciesFixture, name); return }
  await deleteResource(clusterId, `apis/networking.k8s.io/v1/namespaces/${encodeURIComponent(namespace)}/networkpolicies/${encodeURIComponent(name)}`)
}
