import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { RouterManager } from '../RouterManager'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'
import { usePluginStore } from '@/stores/pluginStore'

vi.mock('@/core/plugin-loader/PluginRegistry', () => ({
  getPluginRegistry: () => ({
    resolveComponent: vi.fn(async () => ({ render: () => {} })),
  }),
}))

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('RouterManager', () => {
  it('creates a router instance', () => {
    const rm = new RouterManager()
    expect(rm.getRouter()).toBeDefined()
    expect(rm.getRoutes().length).toBeGreaterThan(0)
  })

  it('has base routes (Login, Dashboard, NotFound, etc.)', () => {
    const rm = new RouterManager()
    const routeNames = rm.getRoutes().map((r) => r.name)
    expect(routeNames).toContain('Login')
    expect(routeNames).toContain('Dashboard')
    expect(routeNames).toContain('NotFound')
    expect(routeNames).toContain('TenantSelect')
  })

  it('registerDynamicRoutes adds routes once', () => {
    const rm = new RouterManager()
    const dynamicRoutes = [
      { name: 'Projects', path: '/projects', pluginId: 'app', componentKey: 'ProjectList', permission: 'read:project' },
    ]
    rm.registerDynamicRoutes(dynamicRoutes as any)
    const routeNames = rm.getRoutes().map((r) => r.name)
    expect(routeNames).toContain('Projects')
    // Second call should be skipped
    rm.registerDynamicRoutes(dynamicRoutes as any)
    expect(rm.getRoutes().filter((r) => r.name === 'Projects')).toHaveLength(1)
  })

  it('registers an apiserver-owned compatibility redirect that preserves query context', () => {
    const rm = new RouterManager()
    rm.registerDynamicRoutes([{
      name: 'container.storage.legacy',
      path: '/container/instances/storage',
      pluginId: 'container',
      componentKey: 'Storage',
      permission: 'cluster:read',
      redirect: '/container/storage',
    }] as any)

    const route = rm.getRoutes().find((item) => item.name === 'container.storage.legacy')
    expect(route?.meta?.permissionCode).toBe('cluster:read')
    expect(typeof route?.redirect).toBe('function')
    expect((route?.redirect as Function)({
      query: { cluster: 'c1', target: 't1', namespace: 'ns1', offering: 'gold', storageClass: 'fast' },
      hash: '#pvc',
    })).toEqual({
      path: '/container/storage',
      query: { cluster: 'c1', target: 't1', namespace: 'ns1', offering: 'gold', storageClass: 'fast' },
      hash: '#pvc',
    })
  })

  it('re-resolves an initial deep link after dynamic routes register', async () => {
    const rm = new RouterManager()
    useAuthStore().token = 'access-token'
    useContextStore().setFullContext({ tenantId: 'tenant-a', spaceId: 'space-a' })
    usePluginStore().activate('resource')

    await rm.getRouter().push('/late-page?storageClass=fast')
    expect(rm.getLocation().name).toBe('NotFound')
    const replaceSpy = vi.spyOn(rm.getRouter(), 'replace')

    rm.registerDynamicRoutes([{
      name: 'LatePage',
      path: '/late-page',
      pluginId: 'resource',
      componentKey: 'Storage',
    }] as any)
    expect(rm.getRouter().resolve('/late-page?storageClass=fast').name).toBe('LatePage')
    expect(replaceSpy).toHaveBeenCalled()
    await replaceSpy.mock.results[0].value
    expect(rm.getLocation().name).toBe('LatePage')
    expect(rm.getLocation().query.storageClass).toBe('fast')
  })

  it('navigate calls router.push', () => {
    const rm = new RouterManager()
    const pushSpy = vi.spyOn(rm.getRouter(), 'push')
    rm.navigate('Login')
    expect(pushSpy).toHaveBeenCalledWith({ name: 'Login', params: undefined })
  })

  it('getLocation returns current route', () => {
    const rm = new RouterManager()
    expect(rm.getLocation()).toBeDefined()
    expect(rm.getLocation().path).toBeDefined()
  })

  it('unregisterDynamicRoutes removes routes from the router', () => {
    const rm = new RouterManager()
    const dynamicRoutes = [
      { name: 'Projects', path: '/projects', pluginId: 'app', componentKey: 'ProjectList' },
    ]
    rm.registerDynamicRoutes(dynamicRoutes as any)
    expect(rm.getRoutes().filter((r) => r.name === 'Projects')).toHaveLength(1)

    rm.unregisterDynamicRoutes()
    expect(rm.getRoutes().filter((r) => r.name === 'Projects')).toHaveLength(0)

    // Should allow re-registration
    rm.registerDynamicRoutes(dynamicRoutes as any)
    expect(rm.getRoutes().filter((r) => r.name === 'Projects')).toHaveLength(1)
  })

  it('reconcile replaces old routes with the new set', () => {
    const rm = new RouterManager()
    rm.registerDynamicRoutes([
      { name: 'Old', path: '/old', pluginId: 'app', componentKey: 'OldPage' },
      { name: 'Kept', path: '/kept', pluginId: 'app', componentKey: 'KeptPage' },
    ] as any)

    rm.reconcile([
      { name: 'Kept', path: '/kept', pluginId: 'app', componentKey: 'KeptPage' },
      { name: 'New', path: '/new', pluginId: 'app', componentKey: 'NewPage' },
    ] as any)

    const names = rm.getRoutes().map((r) => r.name)
    expect(names).not.toContain('Old')
    expect(names).toContain('New')
    // No duplicates after reconcile
    expect(rm.getRoutes().filter((r) => r.name === 'Kept')).toHaveLength(1)
  })

  it('reconcile tolerates duplicate names across old and new sets', () => {
    const rm = new RouterManager()
    rm.registerDynamicRoutes([
      { name: 'Projects', path: '/projects', pluginId: 'app', componentKey: 'ProjectList' },
    ] as any)
    rm.reconcile([
      { name: 'Projects', path: '/projects-v2', pluginId: 'app', componentKey: 'ProjectList' },
    ] as any)
    expect(rm.getRoutes().filter((r) => r.name === 'Projects')).toHaveLength(1)
    expect(rm.getRoutes().find((r) => r.name === 'Projects')?.path).toBe('/projects-v2')
  })
})
