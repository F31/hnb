import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiClient } from '@hnb/types'
import { setClusterApiClient } from '../../cluster-management/api/clusterApi'
import {
  getStorageOverview,
  createStorageBackend,
  listStorageBackends,
  listStorageDriverInstallations,
  listStorageOfferingBindings,
  listStorageOfferings,
  listStorageProviderSchemas,
} from '../storageApi'
import { formatCapacity } from '../storagePresentation'

describe('storage read adapter', () => {
  const get = vi.fn<ApiClient['get']>()
  const post = vi.fn<ApiClient['post']>()

  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({})
    post.mockReset()
    post.mockResolvedValue({})
    setClusterApiClient({ get, post } as unknown as ApiClient)
  })

  it('uses only the dedicated storage overview and collection endpoints', async () => {
    await Promise.all([
      getStorageOverview(),
      listStorageBackends(),
      listStorageProviderSchemas(),
      listStorageOfferings(),
      listStorageDriverInstallations(),
      listStorageOfferingBindings('offering/a'),
    ])

    expect(get.mock.calls.map(([path]) => path)).toEqual([
      '/api/v1/storage/overview',
      '/api/v1/storage/backends',
      '/api/v1/storage/provider-schemas',
      '/api/v1/storage/offerings',
      '/api/v1/storage/driver-installations',
      '/api/v1/storage/offerings/offering%2Fa/bindings',
    ])
  })

  it('submits only typed configuration and SecretReference metadata', async () => {
    await createStorageBackend({
      providerType: 'nfs',
      providerSchemaVersion: '1.0.0',
      backendId: 'primary',
      displayName: 'Primary NFS',
      secretReference: { provider: 'platform-secrets', scope: 'tenant:tenant-a', name: 'nfs-primary' },
      attributes: { server: 'nfs.internal', exportPath: '/workloads', readOnly: false },
    })

    expect(post).toHaveBeenCalledOnce()
    const [path, payload, config] = post.mock.calls[0]!
    expect(path).toBe('/api/v1/storage/backends')
    expect(config.headers['Idempotency-Key']).toBeTruthy()
    expect(JSON.stringify(payload)).not.toMatch(/secretValue|password|token/)
  })
})

describe('capacity presentation', () => {
  it('formats known byte values without changing their meaning', () => {
    expect(formatCapacity('Known', 2 * 1024 ** 4)).toBe('2 TiB')
    expect(formatCapacity('Known', 0)).toBe('0 B')
  })

  it.each(['Elastic', 'Unknown', 'NotReported'] as const)(
    'preserves the %s state instead of displaying zero',
    (status) => {
      expect(formatCapacity(status, 0)).toBe(status)
    },
  )

  it('does not invent a value for an incomplete Known report', () => {
    expect(formatCapacity('Known')).toBe('Known')
  })
})
