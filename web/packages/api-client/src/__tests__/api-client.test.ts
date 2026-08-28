import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createApiClient, ApiError } from '../index'

function jsonResponse(status: number, body: any, headers: Record<string, string> = {}) {
  const text = JSON.stringify(body)
  return {
    status,
    ok: status >= 200 && status < 300,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(text),
    headers: { get: (n: string) => headers[n.toLowerCase()] ?? null },
  } as unknown as Response
}

describe('api-client', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('注入 Token、上下文头与 traceId', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { ok: true }))
    const client = createApiClient({
      getToken: () => 'token-1',
      getContext: () => ({ tenantId: 't1', spaceId: 's1', environmentId: 'e1', clusterId: 'c1' }),
    })
    await client.get('/api/v1/x')

    const headers = fetchMock.mock.calls[0][1].headers
    expect(headers.Authorization).toBe('Bearer token-1')
    expect(headers['X-Tenant-ID']).toBe('t1')
    expect(headers['X-Space-ID']).toBe('s1')
    expect(headers['X-Environment-ID']).toBe('e1')
    expect(headers['X-Cluster-ID']).toBe('c1')
    expect(headers['X-Trace-Id']).toBeTruthy()
  })

  it('错误响应标准化为 ApiError（code/message/traceId）', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(403, { code: 'FORBIDDEN', message: '无权限' }, { 'x-trace-id': 'tr-1' }),
    )
    const client = createApiClient({ getToken: () => null })

    const err: any = await client.get('/api/v1/x').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(403)
    expect(err.code).toBe('FORBIDDEN')
    expect(err.message).toBe('无权限')
    expect(err.traceId).toBe('tr-1')
  })

  it('401 时刷新 Token 并重试一次', async () => {
    let token = 'expired'
    fetchMock
      .mockResolvedValueOnce(jsonResponse(401, { message: 'expired' }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }))
    const refresh = vi.fn(async () => {
      token = 'fresh'
    })
    const client = createApiClient({ getToken: () => token, refreshToken: refresh })

    const result = await client.get<{ ok: boolean }>('/api/v1/x')
    expect(result.ok).toBe(true)
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[1][1].headers.Authorization).toBe('Bearer fresh')
  })

  it('并发 401 只刷新一次（单飞）', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(401, {}))
      .mockResolvedValueOnce(jsonResponse(401, {}))
      .mockResolvedValue(jsonResponse(200, { ok: true }))
    const refresh = vi.fn(async () => {})
    const client = createApiClient({ getToken: () => 't', refreshToken: refresh })

    await Promise.all([client.get('/a'), client.get('/b')])
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('无刷新能力时 401 直接抛错', async () => {
    fetchMock.mockResolvedValue(jsonResponse(401, { message: 'expired' }))
    const client = createApiClient({ getToken: () => 't' })
    await expect(client.get('/x')).rejects.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('默认 abortSignal 中止后请求信号随之取消', async () => {
    const controller = new AbortController()
    controller.abort()
    fetchMock.mockImplementation((_url: string, init: any) => {
      expect(init.signal.aborted).toBe(true)
      return Promise.reject(new DOMException('Aborted', 'AbortError'))
    })
    const client = createApiClient({ getToken: () => null, signal: controller.signal })
    await expect(client.get('/x')).rejects.toThrow()
  })
})
