import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useNavigationStore } from '../navigationStore'

const mockEntry = {
  cacheKeyHash: 'hash1',
  userIdHash: 'uid1',
  context: { tenantId: 't-1', spaceId: 'ws-1' },
  payload: {
    apiVersion: 'v1',
    etag: 'etag1',
    generatedAt: new Date().toISOString(),
    context: { tenantId: 't-1', spaceId: 'ws-1' },
    versions: { permission: '1', pluginCatalog: '1', navigation: '1' },
    plugins: [],
    menus: [{ group: 'main', items: [{ title: 'Dashboard', path: '/dashboard' }] }],
    routes: [],
  },
  versions: { app: '1.0' },
  etag: '',
  generatedAt: new Date().toISOString(),
  expiresAt: new Date(Date.now() + 3600000).toISOString(),
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('navigationStore', () => {
  it('starts with empty state', () => {
    const store = useNavigationStore()
    expect(store.current).toBeNull()
    expect(store.items).toEqual([])
    expect(store.tenantId).toBeUndefined()
  })

  it('replace sets navigation data', () => {
    const store = useNavigationStore()
    store.replace({ current: mockEntry as any, etag: 'etag1', versions: { app: '1.0' } })
    expect(store.tenantId).toBe('t-1')
    expect(store.spaceId).toBe('ws-1')
    expect(store.items).toHaveLength(1)
    expect(store.items[0].group).toBe('main')
    expect(store.getEtag).toBe('etag1')
  })

  it('replace with minimal data', () => {
    const store = useNavigationStore()
    store.replace({ current: mockEntry as any })
    expect(store.tenantId).toBe('t-1')
    expect(store.getEtag).toBe('')
  })

  it('clear resets state', () => {
    const store = useNavigationStore()
    store.replace({ current: mockEntry as any })
    store.clear()
    expect(store.current).toBeNull()
    expect(store.items).toEqual([])
    expect(store.getEtag).toBe('')
  })
})