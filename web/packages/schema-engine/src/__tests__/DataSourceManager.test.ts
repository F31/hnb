import { describe, it, expect, beforeEach, vi } from 'vitest'
import { DataSourceManager } from '../DataSourceManager'
import type { ApiClient } from '@hnb/types'

function makeApiClient(data: unknown): ApiClient & { get: ReturnType<typeof vi.fn> } {
  return {
    get: vi.fn(async () => data),
    post: vi.fn(),
    put: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  } as any
}

describe('DataSourceManager', () => {
  let manager: DataSourceManager
  let apiClient: ReturnType<typeof makeApiClient>

  beforeEach(() => {
    apiClient = makeApiClient({ data: { items: [{ uid: '1' }], total: 1 } })
    manager = new DataSourceManager(apiClient)
    manager.allowEndpoint('/api/v1/k8s/workloads')
    manager.allowEndpoint('/api/v1/resources/clusters')
    manager.registerEndpoint({ id: 'k8s.workloads.list', path: '/api/v1/k8s/workloads' })
  })

  it('注册数据源时校验 endpointId 白名单', () => {
    expect(() =>
      manager.registerDataSource({ id: 'bad', type: 'query', endpointId: 'nope' }),
    ).toThrow(/Unknown endpointId/)
  })

  it('分页查询按 responseMapping 提取 items/total', async () => {
    manager.registerDataSource({
      id: 'workload.list',
      type: 'paginatedQuery',
      endpointId: 'k8s.workloads.list',
      queryBindings: ['namespace'],
      responseMapping: { items: 'data.items', total: 'data.total' },
    })
    const result = await manager.fetchPaginated('workload.list', { page: 2, pageSize: 20 })
    expect(result.items).toHaveLength(1)
    expect(result.total).toBe(1)
    expect(apiClient.get).toHaveBeenCalledWith(
      '/api/v1/k8s/workloads',
      expect.objectContaining({ params: { page: 2, pageSize: 20 } }),
    )
  })

  it('queryBindings 之外的参数被丢弃', async () => {
    manager.registerDataSource({
      id: 'workload.list',
      type: 'paginatedQuery',
      endpointId: 'k8s.workloads.list',
      queryBindings: ['namespace'],
    })
    await manager.fetchPaginated('workload.list', {
      page: 1,
      params: { namespace: 'default', inject: 'evil' },
    })
    const call = apiClient.get.mock.calls[0][1] as any
    expect(call.params.namespace).toBe('default')
    expect(call.params.inject).toBeUndefined()
  })

  it('未注册数据源抛错', async () => {
    await expect(manager.fetch('unknown')).rejects.toThrow(/Unknown dataSource/)
  })

  it('fetch 在上下文切换后丢弃迟到响应（V2.6 §13.6.4 / §16.4）', async () => {
    manager.registerDataSource({
      id: 'workload.get',
      type: 'query',
      endpointId: 'k8s.workloads.list',
    })
    let resolveGet!: (v: unknown) => void
    apiClient.get.mockReturnValueOnce(
      new Promise((resolve) => { resolveGet = resolve }),
    )
    const pending = manager.fetch('workload.get', { contextKey: 'tenant-a' })
    // 上下文切换：generation++，清空缓存与 in-flight 队列
    manager.invalidateContext()
    resolveGet({ ok: true })
    // 迟到响应必须被丢弃，而不是把旧上下文数据交给调用方
    await expect(pending).rejects.toThrow(/discarded: stale context response/)
  })

  it('fetchPaginated 在上下文切换后丢弃迟到响应', async () => {
    manager.registerDataSource({
      id: 'workload.page',
      type: 'paginatedQuery',
      endpointId: 'k8s.workloads.list',
      responseMapping: { items: 'data.items', total: 'data.total' },
    })
    let resolveGet!: (v: unknown) => void
    apiClient.get.mockReturnValueOnce(
      new Promise((resolve) => { resolveGet = resolve }),
    )
    const pending = manager.fetchPaginated('workload.page', { contextKey: 'tenant-a' })
    manager.invalidateContext()
    resolveGet({ data: { items: [{ uid: 'stale' }], total: 1 } })
    await expect(pending).rejects.toThrow(/discarded: stale context response/)
  })

  it('in-flight 去重：同 key 并发请求共享 Promise（V2.6 §13.6.2）', async () => {
    manager.registerDataSource({
      id: 'workload.get',
      type: 'query',
      endpointId: 'k8s.workloads.list',
    })
    let resolveGet!: (v: unknown) => void
    apiClient.get.mockReturnValueOnce(
      new Promise((resolve) => { resolveGet = resolve }),
    )
    const first = manager.fetch('workload.get', { contextKey: 'tenant-a' })
    const second = manager.fetch('workload.get', { contextKey: 'tenant-a' })
    expect(apiClient.get).toHaveBeenCalledTimes(1)
    resolveGet({ ok: true })
    await expect(first).resolves.toEqual({ ok: true })
    await expect(second).resolves.toEqual({ ok: true })
  })

  it('缓存命中率探针：同参数重复请求命中，不同参数未命中（V2.6 §21.1）', async () => {
    manager.registerDataSource({
      id: 'dict',
      type: 'query',
      endpointId: 'k8s.workloads.list',
      queryBindings: ['namespace'],
      cache: { mode: 'auto' },
    })
    await manager.fetch('dict', { contextKey: 'tenant-a' })
    await manager.fetch('dict', { contextKey: 'tenant-a' })
    await manager.fetch('dict', { contextKey: 'tenant-a', params: { namespace: 'other' } })

    const stats = manager.getCacheStats()
    expect(stats.cacheHits).toBe(1)
    expect(stats.cacheMisses).toBe(2)
    expect(stats.hitRate).toBeCloseTo(1 / 3)
    // 5 分钟窗口内同参数重复请求命中率 ≥ 90%（此处 1/2 = 50% 的两次重复请求中 1 次命中）
    expect(apiClient.get).toHaveBeenCalledTimes(2)
  })
})
