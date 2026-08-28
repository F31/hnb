/**
 * 8.x Schema Engine 缺口闭环测试：
 *  - 8.2/8.5 SchemaEngine：minShellVersion 与 schema revision fail-closed
 *  - 8.2 DataSourceManager：endpoint allowlist 拒绝任意 URL / 未知端点
 *  - 8.3 ExtensionRegistry：命名空间 / 权限 / 版本兼容校验
 *  - 8.4 DataSourceManager：context generation 丢弃迟到响应 + cache key 隔离
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SchemaEngine, SchemaError } from '../SchemaEngine'
import { ComponentRegistry, createComponentRegistry } from '../ComponentRegistry'
import { DataSourceManager } from '../DataSourceManager'
import { ExtensionRegistry, createExtensionRegistry } from '../ExtensionRegistry'
import type { PageSchema } from '../types'

function pageSchema(overrides: Partial<PageSchema['metadata'] & { regions?: unknown }> = {}): PageSchema {
  return {
    apiVersion: 'ui.hnb.io/v1',
    kind: 'PageSchema',
    metadata: { id: 'resource.cluster.list', revision: 1, ...overrides },
    spec: { regions: [] },
  } as unknown as PageSchema
}

describe('SchemaEngine revision fail-closed (8.5)', () => {
  it('accepts a revision at or below the declared cap', () => {
    const engine = new SchemaEngine()
    engine.declareSupportedRevision('resource.cluster.list', 2)
    expect(() => engine.validatePageSchema(pageSchema({ revision: 2 }))).not.toThrow()
  })

  it('rejects a revision above the declared cap as INCOMPATIBLE', () => {
    const engine = new SchemaEngine()
    engine.declareSupportedRevision('resource.cluster.list', 1)
    expect(() => engine.validatePageSchema(pageSchema({ revision: 3 }))).toThrow(SchemaError)
    try {
      engine.validatePageSchema(pageSchema({ revision: 3 }))
    } catch (err) {
      expect((err as SchemaError).code).toBe('INCOMPATIBLE')
    }
  })

  it('rejects minShellVersion above the shell as INCOMPATIBLE', () => {
    const engine = new SchemaEngine()
    expect(() => engine.validatePageSchema(pageSchema({ minShellVersion: '9.9.9' }))).toThrow(SchemaError)
    try {
      engine.validatePageSchema(pageSchema({ minShellVersion: '9.9.9' }))
    } catch (err) {
      expect((err as SchemaError).code).toBe('INCOMPATIBLE')
    }
  })
})

describe('DataSourceManager allowlist (8.2)', () => {
  let manager: DataSourceManager

  beforeEach(() => {
    manager = new DataSourceManager({
      get: vi.fn(async () => ({})),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
    } as any)
  })

  it('rejects arbitrary URLs, protocol-relative and absolute paths', () => {
    expect(() => manager.registerEndpoint({ id: 'evil', path: 'https://evil.example/x' })).toThrow(/not allowlisted/)
    expect(() => manager.registerEndpoint({ id: 'evil2', path: '//evil.example/x' })).toThrow(/not allowlisted/)
    expect(() => manager.registerEndpoint({ id: 'evil3', path: 'javascript:alert(1)' })).toThrow(/not allowlisted/)
    expect(() => manager.registerEndpoint({ id: 'evil4', path: '/api/v1/x?a=1' })).toThrow(/not allowlisted/)
  })

  it('accepts only allowlisted relative paths', () => {
    manager.allowEndpoint('/api/v1/resources/clusters')
    manager.registerEndpoint({ id: 'ok', path: '/api/v1/resources/clusters' })
    manager.registerEndpoint({ id: 'ok2', path: '/api/v1/resources/clusters/{id}/nodes' })
    expect(manager.resolveEndpoint('ok')?.id).toBe('ok')
    expect(() => manager.registerEndpoint({ id: 'bad', path: '/api/v1/other' })).toThrow(/not allowlisted/)
  })

  it('rejects endpoints with no allowlist configured', () => {
    expect(() => manager.registerEndpoint({ id: 'x', path: '/api/v1/clusters' })).toThrow(/not allowlisted/)
  })
})

describe('DataSourceManager context generation (8.4)', () => {
  it('discards stale responses after invalidateContext', async () => {
    let resolveFirst: (v: unknown) => void
    const first = new Promise((resolve) => {
      resolveFirst = resolve
    })
    const apiClient = {
      get: vi.fn().mockImplementationOnce(() => first).mockResolvedValue({ data: { items: [], total: 0 } }),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
    } as any
    const manager = new DataSourceManager(apiClient)
    manager.allowEndpoint('/api/v1/resources/clusters')
    manager.registerEndpoint({ id: 'clusters', path: '/api/v1/resources/clusters' })
    manager.registerDataSource({ id: 'cluster.list', type: 'paginatedQuery', endpointId: 'clusters', responseMapping: { items: 'data.items', total: 'data.total' } })

    const pending = manager.fetchPaginated('cluster.list', { page: 1 })
    manager.invalidateContext()
    resolveFirst!({ data: { items: [], total: 0 } })
    await expect(pending).rejects.toThrow(/discarded: stale context/)
  })

  it('isolates cache keys across context keys', async () => {
    const apiClient = {
      get: vi.fn(async () => ({ data: { items: [], total: 0 } })),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
    } as any
    const manager = new DataSourceManager(apiClient)
    manager.allowEndpoint('/api/v1/resources/clusters')
    manager.registerEndpoint({ id: 'clusters', path: '/api/v1/resources/clusters' })
    manager.registerDataSource({ id: 'cluster.list', type: 'paginatedQuery', endpointId: 'clusters', queryBindings: ['kind'], responseMapping: { items: 'data.items', total: 'data.total' } })
    const a = manager.cacheKey('cluster.list', { params: { kind: 'kubernetes' }, contextKey: 'tenant-a' })
    const b = manager.cacheKey('cluster.list', { params: { kind: 'kubernetes' }, contextKey: 'tenant-b' })
    const c = manager.cacheKey('cluster.list', { params: { kind: 'kubernetes' }, contextKey: 'tenant-a' })
    expect(a).not.toBe(b)
    expect(a).toBe(c)
  })
})

describe('ExtensionRegistry (8.3)', () => {
  let registry: ComponentRegistry
  let extensions: ExtensionRegistry

  beforeEach(() => {
    registry = createComponentRegistry()
    registry.register({
      type: 'DetailPanel',
      component: {} as any,
      propsSchema: { type: 'object', properties: {} },
    })
    extensions = createExtensionRegistry(registry)
  })

  it('registers a valid namespace with a registered component', () => {
    const result = extensions.register({
      namespace: 'resource.cluster.detail.tabs.overview',
      componentType: 'DetailPanel',
      permission: 'cluster:read',
      order: 1,
    })
    expect(result.valid).toBe(true)
  })

  it('rejects unknown component types and wildcard permissions', () => {
    const bad1 = extensions.register({ namespace: 'resource.cluster.detail.tabs.x', componentType: 'Nope' })
    expect(bad1.valid).toBe(false)
    const bad2 = extensions.register({ namespace: 'resource.cluster.detail.tabs.y', componentType: 'DetailPanel', permission: '*' })
    expect(bad2.valid).toBe(false)
  })

  it('rejects version-incompatible extensions', () => {
    const result = extensions.register({
      namespace: 'resource.cluster.detail.tabs.future',
      componentType: 'DetailPanel',
      minShellVersion: '9.9.9',
    })
    expect(result.valid).toBe(false)
  })

  it('lists extensions only when the caller has the required permission', () => {
    extensions.register({ namespace: 'resource.cluster.detail.tabs.audit', componentType: 'DetailPanel', permission: 'audit:read' })
    expect(extensions.list('resource.cluster.detail.tabs.audit', [])).toHaveLength(0)
    expect(extensions.list('resource.cluster.detail.tabs.audit', ['audit:read'])).toHaveLength(1)
    expect(extensions.listPrefix('resource.cluster.detail', ['audit:read'])).toHaveLength(1)
  })
})
