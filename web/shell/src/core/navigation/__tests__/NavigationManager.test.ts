import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { NavigationManager } from '../NavigationManager'
import { useNavigationStore } from '@/stores/navigationStore'
import { getEventBus } from '@/core/event-bus'

function navResponse(tenantId: string, etag = `etag-${tenantId}`) {
  return {
    apiVersion: 'navigation.hnb.io/v1',
    etag,
    generatedAt: new Date().toISOString(),
    context: { tenantId },
    versions: { permission: 'p1', pluginCatalog: 'pc1', navigation: 'n1' },
    plugins: [],
    menus: [
      { group: 'main', items: [{ title: `Menu-${tenantId}`, path: `/${tenantId}` }] },
    ],
    routes: [],
  }
}

function okResponse(body: any, etag?: string) {
  return {
    status: 200,
    ok: true,
    json: () => Promise.resolve(body),
    headers: { get: (name: string) => (name.toLowerCase() === 'etag' ? (etag ?? null) : null) },
  } as unknown as Response
}

const notModified = { status: 304, ok: false } as unknown as Response

describe('NavigationManager', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let manager: NavigationManager

  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    manager = new NavigationManager()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetches menus from the backend and stores etag/versions', async () => {
    fetchMock.mockResolvedValue(okResponse(navResponse('t1')))
    const menus = await manager.loadMenus({ tenantId: 't1' })
    expect(menus).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(useNavigationStore().getEtag).toBe('etag-t1')
    expect(manager.getVersion()).toBe('n1')
  })

  it('hits the L1 cache within TTL for the same tenant', async () => {
    fetchMock.mockResolvedValue(okResponse(navResponse('t1')))
    await manager.loadMenus({ tenantId: 't1' })
    await manager.loadMenus({ tenantId: 't1' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('does not hit the L1 cache across tenants', async () => {
    fetchMock
      .mockResolvedValueOnce(okResponse(navResponse('t1')))
      .mockResolvedValueOnce(okResponse(navResponse('t2')))
    await manager.loadMenus({ tenantId: 't1' })
    const menus = await manager.loadMenus({ tenantId: 't2' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(menus[0].items[0].title).toBe('Menu-t2')
  })

  it('sends If-None-Match and reuses the cached payload on 304', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(navResponse('t1')))
    await manager.loadMenus({ tenantId: 't1' })

    manager.invalidateCache()
    fetchMock.mockResolvedValueOnce(notModified)
    const menus = await manager.loadMenus({ tenantId: 't1' })

    const headers = fetchMock.mock.calls[1][1].headers
    expect(headers['If-None-Match']).toBe('etag-t1')
    expect(menus[0].items[0].title).toBe('Menu-t1')
  })

  it('serves last known good menus on API failure for the same tenant', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(navResponse('t1')))
    await manager.loadMenus({ tenantId: 't1' })

    manager.invalidateCache()
    fetchMock.mockRejectedValueOnce(new Error('network down'))
    const menus = await manager.loadMenus({ tenantId: 't1' })
    expect(menus[0].items[0].title).toBe('Menu-t1')
  })

  it('throws on API failure when the cached menus belong to another tenant', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(navResponse('t1')))
    await manager.loadMenus({ tenantId: 't1' })

    fetchMock.mockRejectedValueOnce(new Error('network down'))
    await expect(manager.loadMenus({ tenantId: 't2' })).rejects.toThrow('network down')
  })

  it('clear resets store, etag and L1 cache', async () => {
    fetchMock.mockResolvedValue(okResponse(navResponse('t1')))
    await manager.loadMenus({ tenantId: 't1' })
    manager.clear()
    expect(useNavigationStore().current).toBeNull()
    expect(useNavigationStore().getEtag).toBe('')

    await manager.loadMenus({ tenantId: 't1' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('仅在 etag 变化时发布 navigation:updated 事件', async () => {
    const events: any[] = []
    getEventBus().on('navigation:updated', (e: any) => events.push(e))

    fetchMock.mockResolvedValue(okResponse(navResponse('t1')))
    await manager.loadMenus({ tenantId: 't1' })
    manager.invalidateCache()
    await manager.loadMenus({ tenantId: 't1' })

    expect(events).toHaveLength(1)
    expect(events[0].version).toBe('n1')
  })

  it('clear 后丢弃进行中的迟到响应', async () => {
    let resolveFetch!: (v: Response) => void
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => { resolveFetch = resolve }),
    )
    const pending = manager.loadMenus({ tenantId: 't1' })
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    manager.clear()
    resolveFetch(okResponse(navResponse('t1')))
    await pending

    expect(useNavigationStore().current).toBeNull()
  })

  it('版本轮询按间隔发起请求，clear 后停止', async () => {
    vi.useFakeTimers()
    try {
      fetchMock.mockResolvedValue(okResponse(navResponse('t1')))
      manager.startVersionWatcher({ tenantId: 't1' }, 1000)

      await vi.advanceTimersByTimeAsync(1000)
      expect(fetchMock).toHaveBeenCalledTimes(1)

      manager.clear()
      await vi.advanceTimersByTimeAsync(3000)
      expect(fetchMock).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('API 失败时回放本地持久化的 LKG 快照', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(navResponse('t1')))
    await manager.loadMenus({ tenantId: 't1' })
    // 清空内存态（保留 localStorage 中的 LKG 快照）
    manager.clear()

    fetchMock.mockRejectedValueOnce(new Error('down'))
    const menus = await manager.loadMenus({ tenantId: 't1' })
    expect(menus[0].items[0].title).toBe('Menu-t1')
  })

  it('持久化快照上下文不匹配时拒绝回放', async () => {
    const withSpace = {
      ...navResponse('t1'),
      context: { tenantId: 't1', spaceId: 's1' },
    }
    fetchMock.mockResolvedValueOnce(okResponse(withSpace))
    await manager.loadMenus({ tenantId: 't1', spaceId: 's1' })
    manager.clear()

    fetchMock.mockRejectedValueOnce(new Error('down'))
    await expect(manager.loadMenus({ tenantId: 't1', spaceId: 's2' })).rejects.toThrow('down')
  })
})
