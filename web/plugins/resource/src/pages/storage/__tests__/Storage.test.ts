import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { ApiClient } from '@hnb/types'
import Storage from '../../Storage.vue'
import { setClusterApiClient } from '../../cluster-management/api/clusterApi'
import { createTestI18n } from '../../cluster-management/__tests__/testUtils'

function mountStorage(get: ApiClient['get'], locale: 'zh-CN' | 'en-US' = 'en-US') {
  setClusterApiClient({ get } as unknown as ApiClient)
  return mount(Storage, {
    global: { plugins: [createTestI18n(locale)] },
  })
}

describe('Resource Storage page', () => {
  it('renders supply navigation and explicit empty collection states', async () => {
    const get = vi.fn<ApiClient['get']>().mockImplementation(async (path) => {
      if (path === '/api/v1/storage/overview') {
        return {
          schemaVersion: '1.0.0',
          source: 'runtime_target_storage_inventory',
          observedAt: '2026-08-10T01:00:00Z',
          freshness: 'Stale',
          counts: { backends: 0, offerings: 0, driverInstallations: 0, targets: 1, bindings: 0 },
          capacityStates: { Known: 0, Elastic: 0, Unknown: 0, NotReported: 0 },
        }
      }
      return { schemaVersion: '1.0.0', items: [], total: 0 }
    }) as ApiClient['get']

    const wrapper = mountStorage(get)
    await flushPromises()

    expect(wrapper.text()).toContain('Storage Systems')
    expect(wrapper.text()).toContain('Drivers & Connectors')
    expect(wrapper.text()).toContain('Stale')
    expect(wrapper.text()).toContain('Not reported')
    expect(wrapper.text()).toContain('Cluster agent storage observation')
    expect(wrapper.text()).not.toContain('runtime_target_storage_inventory')
    expect(wrapper.find('[title="runtime_target_storage_inventory"]').exists()).toBe(true)

    await wrapper.get('[role="tab"][aria-controls$="panel-systems"]').trigger('click')
    expect(wrapper.text()).toContain('No storage systems')

    await wrapper.get('[role="tab"][aria-controls$="panel-alerts"]').trigger('click')
    expect(wrapper.text()).toContain('No storage alert data')
    expect(wrapper.text()).toContain('no simulated alerts are shown')
  })

  it('renders an error state when the overview endpoint fails', async () => {
    const get = vi.fn<ApiClient['get']>().mockRejectedValue(new Error('projection unavailable')) as ApiClient['get']
    const wrapper = mountStorage(get)
    await flushPromises()

    expect(wrapper.text()).toContain('Failed to load storage data')
    expect(wrapper.text()).toContain('projection unavailable')
    expect(wrapper.text()).toContain('Retry')
  })

  it('localizes known storage system labels using the active locale', async () => {
    const get = vi.fn<ApiClient['get']>().mockImplementation(async (path) => {
      if (path === '/api/v1/storage/backends') {
        return { schemaVersion: '1.0.0', total: 1, items: [{
          schemaVersion: '1.0.0', id: 'backend-1', tenantId: 'tenant-1', providerType: 'generic-csi',
          backendId: 'primary', displayName: 'Primary storage', healthState: 'Healthy',
          source: 'runtime_target_storage_inventory', observedAt: '2026-08-10T01:00:00Z', freshness: 'Stale',
          conditions: [], version: 1, createdAt: '2026-08-01T00:00:00Z', updatedAt: '2026-08-10T01:00:00Z',
        }] }
      }
      if (path === '/api/v1/storage/overview') throw new Error('not projected')
      return { schemaVersion: '1.0.0', items: [], total: 0 }
    }) as ApiClient['get']

    const wrapper = mountStorage(get, 'zh-CN')
    await flushPromises()
    await wrapper.get('[role="tab"][aria-controls$="panel-systems"]').trigger('click')

    expect(wrapper.text()).toContain('提供方类型')
    expect(wrapper.text()).toContain('通用 CSI')
    expect(wrapper.text()).toContain('健康')
    expect(wrapper.text()).toContain('集群 Agent 存储观测')
    expect(wrapper.text()).toContain('已过期')
    expect(wrapper.text()).not.toContain('generic-csi')
    expect(wrapper.text()).not.toContain('Healthy')
  })

  it('links authorized offering bindings to Container Storage filters', async () => {
    const get = vi.fn<ApiClient['get']>().mockImplementation(async (path) => {
      if (path === '/api/v1/storage/overview') {
        return {
          schemaVersion: '1.0.0', source: 'storage-projection', observedAt: '2026-08-10T01:00:00Z', freshness: 'Fresh',
          counts: { backends: 0, offerings: 1, driverInstallations: 0, targets: 1, bindings: 1 },
          capacityStates: { Known: 0, Elastic: 0, Unknown: 0, NotReported: 0 },
        }
      }
      if (path === '/api/v1/storage/offerings') {
        return { schemaVersion: '1.0.0', total: 1, items: [{
          schemaVersion: '1.0.0', id: 'fast/block', scope: 'Tenant', name: 'Fast block', serviceMode: 'Block',
          accessModes: ['ReadWriteOnce'], volumeExpansion: 'Supported', snapshots: 'Supported', clones: 'Unknown',
          protectionClass: 'gold', conditions: [], version: 1, createdAt: '2026-08-01T00:00:00Z', updatedAt: '2026-08-01T00:00:00Z',
        }] }
      }
      if (path === '/api/v1/storage/offerings/fast%2Fblock/bindings') {
        return { schemaVersion: '1.0.0', total: 1, items: [{
          schemaVersion: '1.0.0', id: 'binding-1', tenantId: 'tenant-1', offeringId: 'fast/block', offeringVersion: 1,
          targetId: 'cluster-a', storageClassName: 'sc fast', storageClassUid: 'uid-1', storageClassResourceVersion: '1',
          syncState: 'Active', isDefault: false, source: 'storage-projection', freshness: 'Fresh', conditions: [], version: 1,
          observedAt: '2026-08-10T01:00:00Z', createdAt: '2026-08-01T00:00:00Z', updatedAt: '2026-08-10T01:00:00Z',
        }] }
      }
      return { schemaVersion: '1.0.0', items: [], total: 0 }
    }) as ApiClient['get']

    const wrapper = mountStorage(get)
    await flushPromises()
    await wrapper.get('[role="tab"][aria-controls$="panel-services"]').trigger('click')

    const link = wrapper.get('a[href^="/container/storage?"]')
    const url = new URL(link.attributes('href')!, 'https://hnb.test')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      target: 'cluster-a', cluster: 'cluster-a', offering: 'fast/block', storageClass: 'sc fast',
    })
  })

  it('does not render Container links when offering bindings are unavailable', async () => {
    const get = vi.fn<ApiClient['get']>().mockImplementation(async (path) => {
      if (path === '/api/v1/storage/offerings') {
        return { schemaVersion: '1.0.0', total: 1, items: [{
          schemaVersion: '1.0.0', id: 'empty', scope: 'Global', name: 'Empty offering', serviceMode: 'File', accessModes: [],
          volumeExpansion: 'Unknown', snapshots: 'Unknown', clones: 'Unknown', protectionClass: 'none', conditions: [], version: 1,
          createdAt: '', updatedAt: '',
        }] }
      }
      if (path.includes('/bindings')) throw new Error('not projected')
      if (path === '/api/v1/storage/overview') throw new Error('not projected')
      return { schemaVersion: '1.0.0', items: [], total: 0 }
    }) as ApiClient['get']

    const wrapper = mountStorage(get)
    await flushPromises()
    await wrapper.get('[role="tab"][aria-controls$="panel-services"]').trigger('click')

    expect(wrapper.find('a[href^="/container/storage?"]').exists()).toBe(false)
  })
})
