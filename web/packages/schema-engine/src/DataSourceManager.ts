/**
 * DataSourceManager — 统一数据源执行（V2.5 §13）。
 *
 * 安全约束（V2.5 §12.3 / §16.1）：
 *  - Schema 只能引用已注册且通过 allowlist 的 endpointId，禁止任意 URL；
 *  - 查询参数仅来自 queryBindings 白名单，禁止客户端拼接任意查询表达式；
 *  - tenant/context 切换会推进 generation：在途（迟到）响应被丢弃，
 *    缓存键包含 contextKey 与稳定序列化参数，避免跨租户/跨上下文串数据。
 */

import type { ApiClient } from '@hnb/types'
import type {
  DataSourceDefinition,
  EndpointDefinition,
  PaginatedResult,
} from './types'
import { stableJSON } from './stable'

export interface DataQuery {
  page?: number
  pageSize?: number
  params?: Record<string, unknown>
  /** 调用方提供的 AbortSignal；中止后 in-flight 请求不再应用其结果 */
  signal?: AbortSignal
  /** 租户/上下文标识，参与缓存键隔离（tenantId 或 context signature） */
  contextKey?: string
}

const MAX_URL_LENGTH = 512

/** 响应缓存条目上限：超出后按插入顺序淘汰最旧条目（近似 LRU） */
const MAX_RESPONSE_CACHE = 500

/**
 * 有界写入：先删旧键再写入以刷新插入顺序，超出上限时淘汰最旧条目，
 * 防止 dataSource × 参数组合无限增长内存（V2.6 §13.6）。
 */
function setBounded<K, V>(map: Map<K, V>, key: K, value: V, max: number): void {
  if (map.has(key)) map.delete(key)
  map.set(key, value)
  while (map.size > max) {
    const oldest = map.keys().next().value
    if (oldest === undefined) break
    map.delete(oldest)
  }
}

function getPath(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((acc, key) => {
    if (acc && typeof acc === 'object') return (acc as Record<string, unknown>)[key]
    return undefined
  }, obj)
}

/**
 * 将 `{param}` 路径占位符替换为 query.params 中的值，并从 params 中剔除
 * 已消费的路径参数（避免它们重复出现在 query string）。未提供的占位符保留原样。
 */
function interpolatePath(path: string, params: Record<string, unknown>): string {
  if (!path.includes('{')) return path
  return path.replace(/\{([^}]+)\}/g, (match, key: string) => {
    const value = params[key]
    if (value === undefined || value === null) return match
    delete params[key]
    return encodeURIComponent(String(value))
  })
}

export class DataSourceManager {
  private endpoints = new Map<string, EndpointDefinition>()
  private dataSources = new Map<string, DataSourceDefinition>()
  private allowedPrefixes: string[] = []
  private generation = 0
  private responseCache = new Map<string, { data: unknown; gen: number; expireAt: number }>()
  private inflightMap = new Map<string, Promise<unknown>>()

  /** V2.6 §21.1：缓存命中率探针（仅统计开启缓存的数据源） */
  private stats = { cacheHits: 0, cacheMisses: 0, dedupReuses: 0 }

  constructor(private apiClient: ApiClient) {}

  /** V2.6 §21.1：返回缓存命中统计，供性能预算与仪表盘观测。 */
  getCacheStats(): { cacheHits: number; cacheMisses: number; dedupReuses: number; hitRate: number } {
    const total = this.stats.cacheHits + this.stats.cacheMisses
    return {
      cacheHits: this.stats.cacheHits,
      cacheMisses: this.stats.cacheMisses,
      dedupReuses: this.stats.dedupReuses,
      hitRate: total === 0 ? 0 : this.stats.cacheHits / total,
    }
  }

  /**
   * 允许的 endpoint 路径前缀（服务端下发的受信端点白名单）。
   * 未调用 allowEndpoint 时使用默认拒绝（仅注册显式调用 allow）。
   */
  allowEndpoint(pathPrefix: string): void {
    if (!pathPrefix) throw new Error('endpoint allowlist prefix is required')
    this.allowedPrefixes.push(pathPrefix)
  }

  registerEndpoint(def: EndpointDefinition): void {
    if (!def?.id || !def.path) throw new Error('EndpointDefinition requires id and path')
    if (!isTrustedPath(def.path, this.allowedPrefixes)) {
      throw new Error(`Endpoint "${def.id}" path "${def.path}" is not allowlisted`)
    }
    this.endpoints.set(def.id, def)
  }

  registerDataSource(def: DataSourceDefinition): void {
    if (!def?.id || !def.endpointId) {
      throw new Error('DataSourceDefinition requires id and endpointId')
    }
    if (!this.endpoints.has(def.endpointId)) {
      throw new Error(`Unknown endpointId "${def.endpointId}" for dataSource "${def.id}"`)
    }
    this.dataSources.set(def.id, def)
  }

  has(id: string): boolean {
    return this.dataSources.has(id)
  }

  /** Action 执行时经白名单解析受信端点（V2.5 §12.3） */
  resolveEndpoint(id: string): EndpointDefinition | undefined {
    return this.endpoints.get(id)
  }

  /**
   * context/tenant 切换时调用：递增 generation，使在途请求结果不再被应用，
   * 并清空缓存（缓存键已含 contextKey，切换后自然 miss）。
   */
  invalidateContext(): void {
    this.generation++
    this.responseCache.clear()
    this.inflightMap.clear()
  }

  currentGeneration(): number {
    return this.generation
  }

  /** 普通查询 / 字典 / 聚合：返回响应 data 字段（或整体） */
  async fetch<T = unknown>(dataSourceId: string, query: DataQuery = {}): Promise<T> {
    const def = this.dataSources.get(dataSourceId)
    if (!def) throw new Error(`Unknown dataSource "${dataSourceId}"`)
    const endpoint = this.endpoints.get(def.endpointId)!
    const params = this.buildParams(def, query)
    const path = interpolatePath(endpoint.path, params)
    const gen = this.generation
    if (query.signal?.aborted) throw new Error('aborted')

    const key = this.cacheKey(dataSourceId, query)
    const cacheDef = def.cache

    // 开启缓存时检查缓存命中
    if (cacheDef) {
      const cached = this.responseCache.get(key)
      if (cached && cached.gen === gen && Date.now() < cached.expireAt) {
        this.stats.cacheHits++
        return cached.data as T
      }
    }

    // in-flight 去重（不依赖缓存配置，同一请求并发时共享 Promise）
    const inflight = this.inflightMap.get(key)
    if (inflight) {
      this.stats.dedupReuses++
      return inflight as Promise<T>
    }
    if (cacheDef) this.stats.cacheMisses++

    const promise = this.apiClient.get<T>(path, {
      headers: {},
      ...(Object.keys(params).length > 0 ? { params } : {}),
      signal: query.signal,
    } as any).then((data) => {
      // V2.6 §13.6.4 / §16.4：上下文切换后的迟到响应必须丢弃，
      // 不能只跳过缓存仍把旧上下文数据交给调用方。
      if (gen !== this.generation) throw new Error('discarded: stale context response')
      if (cacheDef) {
        const ttl = (cacheDef.ttl ?? (cacheDef.mode === 'realtime' ? 5 : 30)) * 1000
        setBounded(this.responseCache, key, { data, gen, expireAt: Date.now() + ttl }, MAX_RESPONSE_CACHE)
      }
      if (this.inflightMap.get(key) === promise) this.inflightMap.delete(key)
      return data
    }).catch((err) => {
      // 仅当 map 中仍是本请求的 Promise 时才删除，避免误删
      // generation 切换后同 key 新请求的 in-flight 条目。
      if (this.inflightMap.get(key) === promise) this.inflightMap.delete(key)
      throw err
    })

    this.inflightMap.set(key, promise)
    return promise
  }

  /** 服务端分页查询：按 responseMapping 提取 items/total */
  async fetchPaginated<T = unknown>(
    dataSourceId: string,
    query: DataQuery = {},
  ): Promise<PaginatedResult<T>> {
    const def = this.dataSources.get(dataSourceId)
    if (!def) throw new Error(`Unknown dataSource "${dataSourceId}"`)
    const endpoint = this.endpoints.get(def.endpointId)!
    const params = this.buildParams(def, query)
    const path = interpolatePath(endpoint.path, params)
    const gen = this.generation
    if (query.signal?.aborted) throw new Error('aborted')

    const key = this.cacheKey(dataSourceId, query)
    const cacheDef = def.cache

    if (cacheDef) {
      const cached = this.responseCache.get(key)
      if (cached && cached.gen === gen && Date.now() < cached.expireAt) {
        this.stats.cacheHits++
        return cached.data as PaginatedResult<T>
      }
    }

    const inflight = this.inflightMap.get(key)
    if (inflight) {
      this.stats.dedupReuses++
      return inflight as Promise<PaginatedResult<T>>
    }
    if (cacheDef) this.stats.cacheMisses++

    const promise = this.apiClient.get<unknown>(path, {
      params,
      signal: query.signal,
    } as any).then((raw) => {
      if (gen !== this.generation) throw new Error('discarded: stale context response')
      const itemsPath = def.responseMapping?.items ?? 'data.items'
      const totalPath = def.responseMapping?.total ?? 'data.total'
      const items = getPath(raw, itemsPath)
      const total = getPath(raw, totalPath)
      const result: PaginatedResult<T> = {
        items: Array.isArray(items) ? (items as T[]) : [],
        total: typeof total === 'number' ? total : Array.isArray(items) ? items.length : 0,
      }
      if (cacheDef) {
        const ttl = (cacheDef.ttl ?? (cacheDef.mode === 'realtime' ? 5 : 30)) * 1000
        setBounded(this.responseCache, key, { data: result, gen, expireAt: Date.now() + ttl }, MAX_RESPONSE_CACHE)
      }
      if (this.inflightMap.get(key) === promise) this.inflightMap.delete(key)
      return result
    }).catch((err) => {
      if (this.inflightMap.get(key) === promise) this.inflightMap.delete(key)
      throw err
    })

    this.inflightMap.set(key, promise)
    return promise
  }

  /** 缓存键：dataSourceId + contextKey + 稳定序列化参数（隔离跨租户/上下文） */
  cacheKey(dataSourceId: string, query: DataQuery = {}): string {
    const params = this.buildParams(this.dataSources.get(dataSourceId)!, query)
    return `${dataSourceId}::${query.contextKey ?? ''}::${stableJSON(params)}`
  }

  /**
   * 仅放行 queryBindings 声明的参数 + 标准分页参数 + 路径占位符参数，
   * 其余参数丢弃（V2.5 §10.4）。
   */
  private buildParams(
    def: DataSourceDefinition,
    query: DataQuery,
  ): Record<string, unknown> {
    const allowed = new Set(def.queryBindings ?? [])
    const endpoint = this.endpoints.get(def.endpointId)
    if (endpoint) {
      for (const match of endpoint.path.matchAll(/\{([^}]+)\}/g)) {
        allowed.add(match[1])
      }
    }
    for (const key of def.contextBindings ?? []) allowed.add(key)
    const params: Record<string, unknown> = {}
    for (const [key, value] of Object.entries(query.params ?? {})) {
      if (allowed.has(key) && value !== undefined && value !== null && value !== '') {
        params[key] = value
      }
    }
    if (def.type === 'paginatedQuery') {
      if (query.page) params.page = query.page
      if (query.pageSize) params.pageSize = query.pageSize
    }
    return params
  }

  clear(): void {
    this.endpoints.clear()
    this.dataSources.clear()
    this.generation++
    this.responseCache.clear()
    this.inflightMap.clear()
  }
}

/**
 * 校验端点路径是否为可信 URL 且命中 allowlist：
 *  - 必须是相对路径（无 scheme / host / userinfo / fragment / 协议相对）；
 *  - 命中至少一个允许前缀。
 */
export function isTrustedPath(path: string, allowedPrefixes: string[]): boolean {
  if (!path || path.length > MAX_URL_LENGTH) return false
  if (/^(https?:|wss?:|javascript:|data:|file:|\/\/)/i.test(path)) return false
  if (/[?#]/.test(path)) return false
  if (allowedPrefixes.length === 0) return false
  return allowedPrefixes.some((prefix) => path === prefix || path.startsWith(prefix + '/'))
}

export function createDataSourceManager(apiClient: ApiClient): DataSourceManager {
  return new DataSourceManager(apiClient)
}
