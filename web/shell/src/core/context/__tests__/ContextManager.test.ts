import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useContextStore } from '@/stores/contextStore'
import { usePermissionStore } from '@/stores/permissionStore'
import { switchTenantAtomic } from '@/core/context'

vi.mock('@/core/plugin/PluginManager', () => ({
  getPluginManager: () => ({ reset: vi.fn() }),
}))

vi.mock('@/core/router/RouterManager', () => ({
  getRouterManager: () => ({ unregisterDynamicRoutes: vi.fn() }),
}))

vi.mock('@/core/navigation/NavigationManager', () => ({
  getNavigationManager: () => ({ clear: vi.fn() }),
}))

const bootstrapResponse = {
  subject: { id: 'u1', type: 'user', displayName: 'admin' },
  selectedTenantId: 't-2',
  memberships: [{ membershipId: 'm1', tenantId: 't-2', tenantName: 't-2' }],
  capabilities: [],
  permissions: [
    { tenantId: 't-2', resourceKind: '*', resourceId: '', action: 'read' },
    { tenantId: 't-2', resourceKind: '*', resourceId: '', action: 'create' },
  ],
  policyVersion: 'v1',
  permissionVersion: 'v1',
}

vi.mock('@/core/api/session', () => ({
  fetchSessionBootstrap: vi.fn(async () => bootstrapResponse),
  scopedPermissionsToCodes: (permissions: Array<{ resourceKind?: string; action?: string }>) => {
    const result = new Set<string>()
    for (const permission of permissions) {
      if (!permission.resourceKind || !permission.action) continue
      result.add(`${permission.resourceKind}:${permission.action}`)
      if (permission.action === 'read') result.add(`${permission.resourceKind}:view`)
      if (permission.resourceKind === '*') result.add('*')
    }
    return Array.from(result)
  },
}))

describe('switchTenantAtomic', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('repopulates permissions after clearing the store', async () => {
    const contextStore = useContextStore()
    const permissionStore = usePermissionStore()

    permissionStore.setPermissions(['old:perm'])
    permissionStore.setVersion('old:1')
    contextStore.setFullContext({ tenantId: 't-1', spaceId: 'ws-1' })

    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: [] }), { status: 200 }),
    )

    const gen = await switchTenantAtomic('t-2')

    expect(contextStore.tenantId).toBe('t-2')
    expect(permissionStore.hasPermission('*')).toBe(true)
    expect(permissionStore.hasPermission('cluster:create')).toBe(true)
    expect(permissionStore.version).toBe('v1')
    expect(gen).toBe(1)
  })
})
