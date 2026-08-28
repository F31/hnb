import type { ApiClient } from '@hnb/types'

export interface ConfigMapItem {
  name: string
  namespace: string
  data: Record<string, string>
  createdAt: string
}

export interface SecretItem {
  name: string
  namespace: string
  type: string
  dataKeys: string[]
  createdAt: string
  protected: boolean
}

export interface ConfigMapInput {
  name: string
  namespace: string
  data: Record<string, string>
}

export interface SecretInput {
  name: string
  namespace: string
  type: string
  stringData: Record<string, string>
}

const PROTECTED_SECRET_NAMES = new Set(['argocd-helm', 'argocd-secret'])
const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'
let apiClient: ApiClient | null = null

export function setContainerConfigClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('container config api client is not initialized')
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

export function isProtectedSecret(name: string): boolean {
  return PROTECTED_SECRET_NAMES.has(name)
}

export function mapConfigMap(raw: unknown): ConfigMapItem {
  const item = record(raw)
  const metadata = record(item.metadata)
  return {
    name: String(metadata.name ?? ''),
    namespace: String(metadata.namespace ?? ''),
    data: stringMap(item.data),
    createdAt: String(metadata.creationTimestamp ?? ''),
  }
}

export function mapSecret(raw: unknown): SecretItem {
  const item = record(raw)
  const metadata = record(item.metadata)
  const name = String(metadata.name ?? '')
  return {
    name,
    namespace: String(metadata.namespace ?? ''),
    type: String(item.type ?? 'Opaque'),
    dataKeys: Object.keys(record(item.data)),
    createdAt: String(metadata.creationTimestamp ?? ''),
    protected: isProtectedSecret(name),
  }
}

export function configMapResource(input: ConfigMapInput): Record<string, unknown> {
  return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: input.name, namespace: input.namespace }, data: input.data }
}

export function secretResource(input: SecretInput): Record<string, unknown> {
  return { apiVersion: 'v1', kind: 'Secret', metadata: { name: input.name, namespace: input.namespace }, type: input.type, stringData: input.stringData }
}

function cloneConfigMap(item: ConfigMapItem): ConfigMapItem {
  return { ...item, data: { ...item.data } }
}

function cloneSecret(item: SecretItem): SecretItem {
  return { ...item, dataKeys: [...item.dataKeys] }
}

export async function listConfigMaps(clusterId: string, namespace: string): Promise<ConfigMapItem[]> {
  if (USE_FIXTURES) {
    const { configMapsFixture } = await import('./fixtures/config')
    return configMapsFixture.filter((item) => item.namespace === namespace).map(cloneConfigMap)
  }
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/configmaps`))
  return (data.items ?? []).map(mapConfigMap)
}

export async function listSecrets(clusterId: string, namespace: string): Promise<SecretItem[]> {
  if (USE_FIXTURES) {
    const { secretsFixture } = await import('./fixtures/config')
    return secretsFixture.filter((item) => item.namespace === namespace).map(cloneSecret)
  }
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/secrets`))
  return (data.items ?? []).map(mapSecret)
}

export async function saveConfigMap(clusterId: string, input: ConfigMapInput, originalName?: string, previousData: Record<string, string> = {}): Promise<void> {
  if (USE_FIXTURES) {
    const { configMapsFixture } = await import('./fixtures/config')
    const item: ConfigMapItem = { ...input, data: { ...input.data }, createdAt: new Date().toISOString() }
    const index = configMapsFixture.findIndex((candidate) => candidate.namespace === input.namespace && candidate.name === (originalName || input.name))
    if (index >= 0) configMapsFixture.splice(index, 1, item)
    else configMapsFixture.unshift(item)
    return
  }
  const collection = `api/v1/namespaces/${encodeURIComponent(input.namespace)}/configmaps`
  if (originalName) {
    const data: Record<string, string | null> = { ...input.data }
    for (const key of Object.keys(previousData)) if (!(key in input.data)) data[key] = null
    await client().patch(proxyUrl(clusterId, `${collection}/${encodeURIComponent(originalName)}`), { ...configMapResource({ ...input, name: originalName }), data }, { headers: { 'Content-Type': 'application/merge-patch+json' } })
  } else await client().post(proxyUrl(clusterId, collection), configMapResource(input))
}

export async function createSecret(clusterId: string, input: SecretInput): Promise<void> {
  if (isProtectedSecret(input.name)) throw new Error(`Secret ${input.name} is protected`)
  if (USE_FIXTURES) {
    const { secretsFixture } = await import('./fixtures/config')
    secretsFixture.unshift({ name: input.name, namespace: input.namespace, type: input.type, dataKeys: Object.keys(input.stringData), createdAt: new Date().toISOString(), protected: false })
    return
  }
  await client().post(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(input.namespace)}/secrets`), secretResource(input))
}

export async function deleteConfigMap(clusterId: string, namespace: string, name: string): Promise<void> {
  if (USE_FIXTURES) {
    const { configMapsFixture } = await import('./fixtures/config')
    const index = configMapsFixture.findIndex((item) => item.namespace === namespace && item.name === name)
    if (index >= 0) configMapsFixture.splice(index, 1)
    return
  }
  await client().delete(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/configmaps/${encodeURIComponent(name)}`))
}

export async function deleteSecret(clusterId: string, namespace: string, name: string): Promise<void> {
  if (isProtectedSecret(name)) throw new Error(`Secret ${name} is protected`)
  if (USE_FIXTURES) {
    const { secretsFixture } = await import('./fixtures/config')
    const index = secretsFixture.findIndex((item) => item.namespace === namespace && item.name === name)
    if (index >= 0) secretsFixture.splice(index, 1)
    return
  }
  await client().delete(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/secrets/${encodeURIComponent(name)}`))
}
