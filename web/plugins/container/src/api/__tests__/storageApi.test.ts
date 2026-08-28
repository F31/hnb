import { describe, expect, it, vi } from 'vitest'
import * as storageApi from '../storageApi'

const {
  deletePersistentVolume,
  listPersistentVolumes,
  mapPersistentVolume,
  mapPersistentVolumeClaim,
  mapStorageClass,
  setContainerStorageClient,
} = storageApi

function mockClient(overrides: Record<string, unknown> = {}) {
  return {
    get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn(), ...overrides,
  } as any
}

describe('storageApi Kubernetes mappers', () => {
  it('maps PersistentVolume status and claim association', () => {
    expect(mapPersistentVolume({
      metadata: { name: 'pv-a', labels: { tier: 'fast' }, annotations: { 'hnb.io/service': 'mysql' }, creationTimestamp: '2026-08-01T00:00:00Z' },
      spec: { capacity: { storage: '20Gi' }, accessModes: ['ReadWriteOnce'], persistentVolumeReclaimPolicy: 'Retain', claimRef: { name: 'data', namespace: 'default' } },
      status: { phase: 'Bound', capacity: { storage: '20Gi' } },
    })).toMatchObject({ name: 'pv-a', capacity: '20Gi', status: 'Bound', claimName: 'data', claimNamespace: 'default', service: 'mysql' })
  })

  it('reports a retained Released volume as still released and associated with its prior claim', () => {
    expect(mapPersistentVolume({
      metadata: { name: 'pv-retained', annotations: { 'hnb.io/service': 'customer-data' } },
      spec: {
        capacity: { storage: '20Gi' },
        persistentVolumeReclaimPolicy: 'Retain',
        claimRef: { name: 'database', namespace: 'production' },
      },
      status: { phase: 'Released' },
    })).toMatchObject({
      name: 'pv-retained',
      status: 'Released',
      reclaimPolicy: 'Retain',
      claimName: 'database',
      claimNamespace: 'production',
      service: 'customer-data',
    })
  })

  it('does not expose a generic PersistentVolume recycle action', () => {
    expect(storageApi).not.toHaveProperty('recyclePersistentVolume')
  })

  it('maps PersistentVolumeClaim storage bindings', () => {
    expect(mapPersistentVolumeClaim({
      metadata: { name: 'data', namespace: 'default' },
      spec: { accessModes: ['ReadWriteMany'], storageClassName: 'sc-nfs', volumeName: 'pv-a', resources: { requests: { storage: '50Gi' } } },
      status: { phase: 'Pending' },
    })).toMatchObject({ name: 'data', namespace: 'default', capacity: '50Gi', storageClassName: 'sc-nfs', volumeName: 'pv-a' })
  })

  it('maps StorageClass expansion and parameters', () => {
    expect(mapStorageClass({
      metadata: { name: 'sc-nfs' }, provisioner: 'nfs.csi.k8s.io', reclaimPolicy: 'Retain', allowVolumeExpansion: true,
      parameters: { poolPolicy: 'ec-4+2' },
    })).toMatchObject({ name: 'sc-nfs', provisioner: 'nfs.csi.k8s.io', allowVolumeExpansion: true, poolPolicy: 'ec-4+2' })
  })

  it('preserves PersistentVolume list and delete proxy behavior', async () => {
    const get = vi.fn().mockResolvedValue({ items: [{ metadata: { name: 'pv-a' }, status: { phase: 'Available' } }] })
    const remove = vi.fn().mockResolvedValue(undefined)
    setContainerStorageClient(mockClient({ get, delete: remove }))

    await expect(listPersistentVolumes('cluster-a')).resolves.toMatchObject([{ name: 'pv-a', status: 'Available' }])
    expect(get).toHaveBeenCalledWith('/api/v1/proxy/cluster-a/api/v1/persistentvolumes')

    await deletePersistentVolume('cluster-a', 'pv-a')
    expect(remove).toHaveBeenCalledWith('/api/v1/proxy/cluster-a/api/v1/persistentvolumes/pv-a')
  })
})
