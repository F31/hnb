import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useContextStore } from '../contextStore'

beforeEach(() => {
  setActivePinia(createPinia())
})

const mockWorkspaces = [
  { id: 'ws-1', tenantId: 't-1', name: 'Workspace 1' },
  { id: 'ws-2', tenantId: 't-2', name: 'Workspace 2' },
]

describe('contextStore', () => {
  it('starts with empty context', () => {
    const store = useContextStore()
    expect(store.tenantId).toBeUndefined()
    expect(store.spaceId).toBeUndefined()
    expect(store.workspaces).toEqual([])
  })

  it('setSpace sets tenantId from workspace', async () => {
    const store = useContextStore()
    store.workspaces = mockWorkspaces as any
    await store.setSpace('ws-1', store.switchGeneration)
    expect(store.spaceId).toBe('ws-1')
    expect(store.tenantId).toBe('t-1')
  })

  it('setFullContext replaces context', () => {
    const store = useContextStore()
    store.setFullContext({ tenantId: 't-1', spaceId: 'ws-1' })
    expect(store.tenantId).toBe('t-1')
    expect(store.spaceId).toBe('ws-1')
  })

  it('reset clears all state', () => {
    const store = useContextStore()
    store.setFullContext({ tenantId: 't-1', spaceId: 'ws-1' })
    store.workspaces = mockWorkspaces as any
    store.reset()
    expect(store.tenantId).toBeUndefined()
    expect(store.workspaces).toEqual([])
  })

  it('matches returns true when context matches', () => {
    const store = useContextStore()
    store.setFullContext({ tenantId: 't-1', spaceId: 'ws-1' })
    expect(store.matches({ tenantId: 't-1', spaceId: 'ws-1' })).toBe(true)
    expect(store.matches({ tenantId: 't-2', spaceId: 'ws-1' })).toBe(false)
  })

  it('switchTenant increments generation and clears context', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ data: [] }), { status: 200 }),
    )
    const store = useContextStore()
    store.setFullContext({ tenantId: 't-1', spaceId: 'ws-1' })
    const gen = await store.switchTenant('t-2')
    expect(store.tenantId).toBe('t-2')
    expect(store.spaceId).toBeUndefined()
    expect(gen).toBe(1)
  })

  it('normalizes snake_case workspace fields from the API', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ data: [{ id: 'ws-1', tenant_id: 'tenant-1', display_name: 'Default', is_active: true }] }), { status: 200 }),
    )
    const store = useContextStore()
    const list = await store.loadWorkspaces(store.switchGeneration)
    expect(list[0].tenantId).toBe('tenant-1')
    expect(list[0].displayName).toBe('Default')
    expect(list[0].isActive).toBe(true)
  })
})
