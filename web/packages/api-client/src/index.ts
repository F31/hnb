/**
 * @hnb/api-client — 统一 API Client（UI 规范 V2.5 §13.3）
 *
 * Shell、插件与 Schema Renderer 只能通过本 Client 访问后端，统一处理：
 *  - Token 注入与 401 单飞刷新重试
 *  - tenant/space/environment/cluster 上下文头
 *  - 每次请求生成 traceId
 *  - 超时与取消（默认信号 + 调用方信号 + 超时信号三路合并）
 *  - 错误码标准化为 ApiError
 *
 * 安全约束：不得在本 Client 之外绕过它直接 fetch 业务 API；
 * 错误信息只提取服务端 message/code，不回显请求体，避免敏感数据进入日志。
 */

import type { ApiClient } from '@hnb/types'

export interface ApiContext {
  tenantId?: string
  spaceId?: string
  environmentId?: string
  clusterId?: string
}

export interface ApiClientOptions {
  /** 返回当前访问令牌（不持有 Refresh Token） */
  getToken: () => string | null
  /** 401 时调用的刷新逻辑；内部单飞，并发 401 只刷新一次 */
  refreshToken?: () => Promise<void>
  /** 每次请求前调用，用于主动刷新即将过期的 token（滑动会话）；
   *  返回后保证 token 可用，避免 401 被动触发 */
  beforeRequest?: () => Promise<void>
  /** 全局 401 回调（如 refresh 失败后通知 UI 显示会话过期提示） */
  onUnauthorized?: (error: ApiError) => void
  /** 返回当前多租户上下文，用于上下文头注入 */
  getContext?: () => ApiContext
  /** 默认超时毫秒数，默认 30s */
  timeoutMs?: number
  /** URL 前缀，默认 ''（同源相对路径） */
  baseUrl?: string
  /** 默认中止信号（如插件生命周期 abortSignal），所有请求受其约束 */
  signal?: AbortSignal
}

export interface RequestConfig {
  headers?: Record<string, string>
  params?: Record<string, string | number | boolean | undefined | null>
  signal?: AbortSignal
  /** 覆盖默认超时 */
  timeoutMs?: number
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly traceId?: string
  /** 服务端 Problem Details 扩展字段（STALE challenge 等），仅白名单复制，不含敏感数据 */
  readonly problem?: Record<string, unknown>

  constructor(
    status: number,
    code: string,
    message: string,
    traceId?: string,
    problem?: Record<string, unknown>,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.traceId = traceId
    if (problem) {
      const safe: Record<string, unknown> = {}
      for (const key of [
        'confirmation',
        'policyOutcome',
        'lastKnownStateAt',
        'lifecycleState',
        'healthState',
        'connectivityState',
        'targetId',
        'action',
        'retryable',
      ]) {
        if (key in problem) safe[key] = problem[key]
      }
      this.problem = safe
    }
  }
}

export interface HNBApiClient extends ApiClient {
  request<T>(method: string, url: string, data?: unknown, config?: RequestConfig): Promise<T>
  requestRaw(method: string, url: string, data?: unknown, config?: RequestConfig): Promise<Response>
}

function buildUrl(baseUrl: string, url: string, params?: RequestConfig['params']): string {
  const fullUrl = `${baseUrl}${url}`
  if (!params) return fullUrl
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null) continue
    search.set(key, String(value))
  }
  const query = search.toString()
  if (!query) return fullUrl
  return `${fullUrl}${fullUrl.includes('?') ? '&' : '?'}${query}`
}

function generateTraceId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `trace-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}

function isRawRequestBody(data: unknown): data is BodyInit {
  return (typeof Blob !== 'undefined' && data instanceof Blob)
    || (typeof ArrayBuffer !== 'undefined' && data instanceof ArrayBuffer)
    || (typeof FormData !== 'undefined' && data instanceof FormData)
    || (typeof URLSearchParams !== 'undefined' && data instanceof URLSearchParams)
}

/** 合并多个中止信号：任一触发即中止 */
function combineSignals(signals: Array<AbortSignal | undefined | null>): AbortSignal {
  const active = signals.filter((s): s is AbortSignal => !!s)
  const controller = new AbortController()
  if (active.some((s) => s.aborted)) {
    controller.abort()
    return controller.signal
  }
  const onAbort = () => controller.abort()
  for (const s of active) {
    s.addEventListener('abort', onAbort, { once: true })
  }
  return controller.signal
}

function timeoutSignal(ms: number): AbortSignal | undefined {
  if (ms <= 0) return undefined
  if (typeof AbortSignal !== 'undefined' && typeof AbortSignal.timeout === 'function') {
    return AbortSignal.timeout(ms)
  }
  const controller = new AbortController()
  setTimeout(() => controller.abort(), ms)
  return controller.signal
}

export function createApiClient(options: ApiClientOptions): HNBApiClient {
  const timeoutMs = options.timeoutMs ?? 30_000
  const baseUrl = options.baseUrl ?? ''
  let refreshInFlight: Promise<void> | null = null

  async function refreshOnce(): Promise<void> {
    if (!options.refreshToken) throw new ApiError(401, 'UNAUTHENTICATED', '未认证或会话已过期')
    if (!refreshInFlight) {
      refreshInFlight = options.refreshToken().finally(() => {
        refreshInFlight = null
      })
    }
    return refreshInFlight
  }

  function buildHeaders(config?: RequestConfig): Record<string, string> {
    const headers: Record<string, string> = {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      'X-Trace-Id': generateTraceId(),
    }
    const token = options.getToken()
    if (token) headers.Authorization = `Bearer ${token}`

    const ctx = options.getContext?.()
    if (ctx?.tenantId) headers['X-Tenant-ID'] = ctx.tenantId
    if (ctx?.spaceId) headers['X-Space-ID'] = ctx.spaceId
    if (ctx?.environmentId) headers['X-Environment-ID'] = ctx.environmentId
    if (ctx?.clusterId) headers['X-Cluster-ID'] = ctx.clusterId

    return { ...headers, ...(config?.headers ?? {}) }
  }

  function buildBody(data: unknown): BodyInit | undefined {
    if (data === undefined) return undefined
    if (isRawRequestBody(data)) return data
    return JSON.stringify(data)
  }

  async function doRawFetch(method: string, url: string, data?: unknown, config?: RequestConfig): Promise<Response> {
    const signal = combineSignals([
      options.signal,
      config?.signal,
      timeoutSignal(config?.timeoutMs ?? timeoutMs),
    ])
    return fetch(buildUrl(baseUrl, url, config?.params), {
      method,
      headers: buildHeaders(config),
      body: buildBody(data),
      signal,
    })
  }

  async function requestRaw(method: string, url: string, data?: unknown, config?: RequestConfig): Promise<Response> {
    await options.beforeRequest?.()
    const res = await doRawFetch(method, url, data, config)
    if (res.status === 401 && options.refreshToken) {
      await refreshOnce()
      return doRawFetch(method, url, data, config)
    }
    return res
  }

  async function doFetch<T>(method: string, url: string, data: unknown, config?: RequestConfig): Promise<T> {
    const res = await requestRaw(method, url, data, config)

    if (!res.ok) {
      const text = await res.text().catch(() => '')
      let body: any = {}
      if (text) {
        try {
          body = JSON.parse(text)
        } catch {
          body = { message: text.trim() }
        }
      }
      const traceId = res.headers.get('x-trace-id') ?? undefined
      const error = new ApiError(
        res.status,
        typeof body?.code === 'string' ? body.code : `HTTP_${res.status}`,
        typeof body?.message === 'string' ? body.message : `请求失败: ${res.status}`,
        traceId,
        typeof body === 'object' && body !== null ? body : undefined,
      )
      if (res.status === 401) options.onUnauthorized?.(error)
      throw error
    }
    if (res.status === 204) return undefined as T
    return (await res.json()) as T
  }

  async function request<T>(method: string, url: string, data?: unknown, config?: RequestConfig): Promise<T> {
    // 401 单飞刷新 + 重试统一在 requestRaw 层处理，避免双重 refresh 导致
    // logout 被重复触发或 refresh token 被并发消费。
    return doFetch<T>(method, url, data, config)
  }

  return {
    requestRaw,
    request,
    get: <T>(url: string, config?: RequestConfig) => request<T>('GET', url, undefined, config),
    post: <T>(url: string, data?: unknown, config?: RequestConfig) => request<T>('POST', url, data, config),
    put: <T>(url: string, data?: unknown, config?: RequestConfig) => request<T>('PUT', url, data, config),
    patch: <T>(url: string, data?: unknown, config?: RequestConfig) => request<T>('PATCH', url, data, config),
    delete: <T>(url: string, config?: RequestConfig) => request<T>('DELETE', url, undefined, config),
  }
}

export default createApiClient
