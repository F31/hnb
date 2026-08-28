import type { ApiClient } from '@hnb/types'

export type StorageAccessMode = 'ReadWriteOnce' | 'ReadWriteMany' | 'ReadOnlyMany'
export type VolumeStatus = 'Available' | 'Bound' | 'Released' | 'Pending' | 'Failed' | string

export interface PersistentVolume {
  name: string
  capacity: string
  status: VolumeStatus
  accessModes: StorageAccessMode[]
  reclaimPolicy: string
  claimName: string
  claimNamespace: string
  service: string
  storageClassName: string
  createdAt: string
  labels: Record<string, string>
}

export interface PersistentVolumeClaim {
  name: string
  namespace: string
  status: VolumeStatus
  capacity: string
  accessModes: StorageAccessMode[]
  storageClassName: string
  volumeName: string
  service: string
  createdAt: string
  labels: Record<string, string>
}

export interface StorageClassInfo {
  name: string
  provisioner: string
  reclaimPolicy: string
  allowVolumeExpansion: boolean
  poolPolicy: string
  createdAt: string
  parameters: Record<string, string>
  labels: Record<string, string>
}

interface K8sListEnvelope {
  items?: unknown[]
}

let apiClient: ApiClient | null = null
const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

export function setContainerStorageClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('container storage api client is not initialized')
  return apiClient
}

function proxyUrl(clusterId: string, path: string): string {
  return `/api/v1/proxy/${encodeURIComponent(clusterId)}/${path.replace(/^\//, '')}`
}

function record(value: unknown): Record<string, any> {
  return value && typeof value === 'object' ? value as Record<string, any> : {}
}

function labels(value: unknown): Record<string, string> {
  const source = record(value)
  return Object.fromEntries(Object.entries(source).map(([key, item]) => [key, String(item)]))
}

function accessModes(value: unknown): StorageAccessMode[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is StorageAccessMode => ['ReadWriteOnce', 'ReadWriteMany', 'ReadOnlyMany'].includes(String(item)))
}

export function mapPersistentVolume(raw: unknown): PersistentVolume {
  const item = record(raw)
  const metadata = record(item.metadata)
  const spec = record(item.spec)
  const status = record(item.status)
  const claimRef = record(spec.claimRef)
  const annotations = record(metadata.annotations)
  return {
    name: String(metadata.name ?? ''),
    capacity: String(record(status.capacity).storage ?? record(spec.capacity).storage ?? '--'),
    status: String(status.phase ?? 'Available'),
    accessModes: accessModes(spec.accessModes),
    reclaimPolicy: String(spec.persistentVolumeReclaimPolicy ?? 'Retain'),
    claimName: String(claimRef.name ?? ''),
    claimNamespace: String(claimRef.namespace ?? ''),
    service: String(annotations['hnb.io/service'] ?? ''),
    storageClassName: String(spec.storageClassName ?? ''),
    createdAt: String(metadata.creationTimestamp ?? ''),
    labels: labels(metadata.labels),
  }
}

export function mapPersistentVolumeClaim(raw: unknown): PersistentVolumeClaim {
  const item = record(raw)
  const metadata = record(item.metadata)
  const spec = record(item.spec)
  const status = record(item.status)
  const annotations = record(metadata.annotations)
  return {
    name: String(metadata.name ?? ''),
    namespace: String(metadata.namespace ?? ''),
    status: String(status.phase ?? 'Pending'),
    capacity: String(record(status.capacity).storage ?? record(spec.resources).requests?.storage ?? '--'),
    accessModes: accessModes(spec.accessModes),
    storageClassName: String(spec.storageClassName ?? ''),
    volumeName: String(spec.volumeName ?? ''),
    service: String(annotations['hnb.io/service'] ?? ''),
    createdAt: String(metadata.creationTimestamp ?? ''),
    labels: labels(metadata.labels),
  }
}

export function mapStorageClass(raw: unknown): StorageClassInfo {
  const item = record(raw)
  const metadata = record(item.metadata)
  const parameters = labels(item.parameters)
  return {
    name: String(metadata.name ?? ''),
    provisioner: String(item.provisioner ?? ''),
    reclaimPolicy: String(item.reclaimPolicy ?? 'Delete'),
    allowVolumeExpansion: Boolean(item.allowVolumeExpansion),
    poolPolicy: String(parameters.poolPolicy ?? parameters.pool ?? ''),
    createdAt: String(metadata.creationTimestamp ?? ''),
    parameters,
    labels: labels(metadata.labels),
  }
}

export async function listPersistentVolumes(clusterId: string): Promise<PersistentVolume[]> {
  if (USE_FIXTURES) {
    const { persistentVolumesFixture } = await import('./fixtures/storage')
    return persistentVolumesFixture.map((item) => ({ ...item, labels: { ...item.labels } }))
  }
  const data = await client().get<K8sListEnvelope>(proxyUrl(clusterId, 'api/v1/persistentvolumes'))
  return (data.items ?? []).map(mapPersistentVolume)
}

export async function listPersistentVolumeClaims(clusterId: string): Promise<PersistentVolumeClaim[]> {
  if (USE_FIXTURES) {
    const { persistentVolumeClaimsFixture } = await import('./fixtures/storage')
    return persistentVolumeClaimsFixture.map((item) => ({ ...item, labels: { ...item.labels } }))
  }
  const data = await client().get<K8sListEnvelope>(proxyUrl(clusterId, 'api/v1/persistentvolumeclaims'))
  return (data.items ?? []).map(mapPersistentVolumeClaim)
}

export async function listStorageClasses(clusterId: string): Promise<StorageClassInfo[]> {
  if (USE_FIXTURES) {
    const { storageClassesFixture } = await import('./fixtures/storage')
    return storageClassesFixture.map((item) => ({ ...item, labels: { ...item.labels }, parameters: { ...item.parameters } }))
  }
  const data = await client().get<K8sListEnvelope>(proxyUrl(clusterId, 'apis/storage.k8s.io/v1/storageclasses'))
  return (data.items ?? []).map(mapStorageClass)
}

export async function createPersistentVolume(clusterId: string, resource: unknown): Promise<void> {
  if (USE_FIXTURES) {
    const { persistentVolumesFixture } = await import('./fixtures/storage')
    persistentVolumesFixture.unshift({ ...mapPersistentVolume(resource), status: 'Available', createdAt: new Date().toISOString() })
    return
  }
  await client().post(proxyUrl(clusterId, 'api/v1/persistentvolumes'), resource)
}

export async function createPersistentVolumeClaim(clusterId: string, namespace: string, resource: unknown): Promise<void> {
  if (USE_FIXTURES) {
    const { persistentVolumeClaimsFixture } = await import('./fixtures/storage')
    const item = mapPersistentVolumeClaim(resource)
    persistentVolumeClaimsFixture.unshift({ ...item, namespace, status: 'Pending', createdAt: new Date().toISOString() })
    return
  }
  await client().post(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/persistentvolumeclaims`), resource)
}

export async function createStorageClass(clusterId: string, resource: unknown): Promise<void> {
  if (USE_FIXTURES) {
    const { storageClassesFixture } = await import('./fixtures/storage')
    storageClassesFixture.unshift({ ...mapStorageClass(resource), createdAt: new Date().toISOString() })
    return
  }
  await client().post(proxyUrl(clusterId, 'apis/storage.k8s.io/v1/storageclasses'), resource)
}

export async function deletePersistentVolume(clusterId: string, name: string): Promise<void> {
  if (USE_FIXTURES) {
    const { persistentVolumesFixture } = await import('./fixtures/storage')
    const index = persistentVolumesFixture.findIndex((item) => item.name === name)
    if (index >= 0) persistentVolumesFixture.splice(index, 1)
    return
  }
  await client().delete(proxyUrl(clusterId, `api/v1/persistentvolumes/${encodeURIComponent(name)}`))
}

export async function deletePersistentVolumeClaim(clusterId: string, namespace: string, name: string): Promise<void> {
  if (USE_FIXTURES) {
    const { persistentVolumeClaimsFixture } = await import('./fixtures/storage')
    const index = persistentVolumeClaimsFixture.findIndex((item) => item.namespace === namespace && item.name === name)
    if (index >= 0) persistentVolumeClaimsFixture.splice(index, 1)
    return
  }
  await client().delete(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/persistentvolumeclaims/${encodeURIComponent(name)}`))
}

export async function deleteStorageClass(clusterId: string, name: string): Promise<void> {
  if (USE_FIXTURES) {
    const { storageClassesFixture } = await import('./fixtures/storage')
    const index = storageClassesFixture.findIndex((item) => item.name === name)
    if (index >= 0) storageClassesFixture.splice(index, 1)
    return
  }
  await client().delete(proxyUrl(clusterId, `apis/storage.k8s.io/v1/storageclasses/${encodeURIComponent(name)}`))
}

export type StorageResourceKind = 'pv' | 'pvc' | 'storageClass'

export async function updateStorageLabels(
  clusterId: string,
  kind: StorageResourceKind,
  name: string,
  resourceLabels: Record<string, string>,
  namespace = '',
): Promise<void> {
  if (USE_FIXTURES) {
    const fixtures = await import('./fixtures/storage')
    const items: Array<PersistentVolume | PersistentVolumeClaim | StorageClassInfo> = kind === 'pv'
      ? fixtures.persistentVolumesFixture
      : kind === 'pvc'
        ? fixtures.persistentVolumeClaimsFixture
        : fixtures.storageClassesFixture
    const item = items.find((candidate) => candidate.name === name && (kind !== 'pvc' || ('namespace' in candidate && candidate.namespace === namespace)))
    if (item) item.labels = { ...resourceLabels }
    return
  }
  const path = kind === 'pv'
    ? `api/v1/persistentvolumes/${encodeURIComponent(name)}`
    : kind === 'pvc'
      ? `api/v1/namespaces/${encodeURIComponent(namespace)}/persistentvolumeclaims/${encodeURIComponent(name)}`
      : `apis/storage.k8s.io/v1/storageclasses/${encodeURIComponent(name)}`
  await client().patch(
    proxyUrl(clusterId, path),
    { metadata: { labels: resourceLabels } },
    { headers: { 'Content-Type': 'application/merge-patch+json' } },
  )
}
