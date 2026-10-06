import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiClient } from '@hnb/types'
import {
  approveSwitchRequest,
  createGslbService,
  deleteGslbService,
  gslbSwitchStatusLabel,
  GSLB_TERMINAL_STATUSES,
  listGslbPools,
  listGslbSwitchRequests,
  rejectSwitchRequest,
  replaceGslbPools,
  setGslbApiClient,
  submitGslbIntent,
  updateGslbService,
  type GslbIntentPayload,
} from '../gslbApi'

describe('gslb read adapter', () => {
  const get = vi.fn<ApiClient['get']>()
  const post = vi.fn<ApiClient['post']>()

  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ items: [], total: 0 })
    post.mockReset()
    post.mockResolvedValue({})
    setGslbApiClient({ get, post } as unknown as ApiClient)
  })

  it('queries pools via the dedicated service pools endpoint', async () => {
    await listGslbPools('svc-1')
    expect(get).toHaveBeenCalledWith('/api/v1/gslb/services/svc-1/pools')
  })

  it('lists switch requests with service filter and optional status', async () => {
    await listGslbSwitchRequests('svc-1')
    expect(get).toHaveBeenCalledWith('/api/v1/gslb/switch-requests?service_id=svc-1')

    await listGslbSwitchRequests('svc-1', 'PendingApproval')
    expect(get).toHaveBeenCalledWith('/api/v1/gslb/switch-requests?service_id=svc-1&status=PendingApproval')
  })

  it('posts empty bodies to the approve / reject endpoints', async () => {
    await approveSwitchRequest('req-1')
    expect(post).toHaveBeenCalledWith('/api/v1/gslb/switch-requests/req-1/approve', {})

    await rejectSwitchRequest('req-2')
    expect(post).toHaveBeenCalledWith('/api/v1/gslb/switch-requests/req-2/reject', {})
  })

  it('creates a service via POST /api/v1/gslb/services', async () => {
    const payload = {
      name: 'api-global',
      domain: 'api.example.com',
      requireApproval: true,
      pools: [{ name: 'primary', priority: 0, members: [{ clusterId: 'cluster-a', weight: 100 }] }],
      healthCheck: { protocol: 'http' as const, path: '/healthz', intervalSeconds: 10, timeoutSeconds: 3, failureThreshold: 3 },
    }
    await createGslbService(payload)
    expect(post).toHaveBeenCalledWith('/api/v1/gslb/services', payload)
  })

  it('keeps submitGslbIntent backwards compatible without weights', async () => {
    await submitGslbIntent('svc-1', 'gslb.failover', 'pool-1', 'manual failover')
    const payload = post.mock.calls[0][1] as GslbIntentPayload
    expect(payload.kind).toBe('gslb.failover')
    expect(payload.targetPoolId).toBe('pool-1')
    expect(payload.weights).toBeUndefined()
  })

  it('attaches weights to gslb.weight-update intents', async () => {
    const weights = { 'cluster-a': 60, 'cluster-b': 40 }
    await submitGslbIntent('svc-1', 'gslb.weight-update', undefined, '调权', weights)
    const payload = post.mock.calls[0][1] as GslbIntentPayload
    expect(post).toHaveBeenCalledWith('/api/v1/gslb/services/svc-1/intents', payload)
    expect(payload.kind).toBe('gslb.weight-update')
    expect(payload.targetPoolId).toBeUndefined()
    expect(payload.weights).toEqual(weights)
  })
})

describe('gslb write adapter（编辑/池替换/删除）', () => {
  const patch = vi.fn<ApiClient['patch']>()
  const put = vi.fn<ApiClient['put']>()
  const del = vi.fn<ApiClient['delete']>()

  beforeEach(() => {
    patch.mockReset()
    patch.mockResolvedValue({})
    put.mockReset()
    put.mockResolvedValue({ items: [], total: 0 })
    del.mockReset()
    del.mockResolvedValue(undefined)
    setGslbApiClient({ patch, put, delete: del } as unknown as ApiClient)
  })

  it('PATCHes merge-style updates with If-Match / correlation / idempotency headers', async () => {
    const body = {
      name: 'api-v2',
      healthCheck: { protocol: 'https' as const, path: '/healthz', intervalSeconds: 10, timeoutSeconds: 3, failureThreshold: 3 },
    }
    await updateGslbService('svc-1', body, '2026-09-01T00:00:00.000000001Z')
    expect(patch).toHaveBeenCalledTimes(1)
    const [url, payload, config] = patch.mock.calls[0]
    expect(url).toBe('/api/v1/gslb/services/svc-1')
    expect(payload).toEqual(body)
    expect(config?.headers?.['If-Match']).toBe('2026-09-01T00:00:00.000000001Z')
    expect(config?.headers?.['X-Correlation-ID']).toMatch(/^[0-9a-f-]{36}$/)
    expect(config?.headers?.['Idempotency-Key']).toMatch(/^gslb-update-/)
  })

  it('defaults If-Match to * when the read model has no updatedAt', async () => {
    await updateGslbService('svc-1', { requireApproval: false })
    expect(patch.mock.calls[0][2]?.headers?.['If-Match']).toBe('*')
  })

  it('PUTs the whole pool topology wrapped in { pools }', async () => {
    const pools = [{ name: 'primary', priority: 0, members: [{ clusterId: 'cluster-a', weight: 100 }] }]
    await replaceGslbPools('svc-1', pools)
    const [url, payload, config] = put.mock.calls[0]
    expect(url).toBe('/api/v1/gslb/services/svc-1/pools')
    expect(payload).toEqual({ pools })
    expect(config?.headers?.['If-Match']).toBe('*')
    expect(config?.headers?.['Idempotency-Key']).toMatch(/^gslb-pools-replace-/)
  })

  it('DELETEs with an explicit force query flag and no body', async () => {
    await deleteGslbService('svc-1', true)
    const [url, config] = del.mock.calls[0]
    expect(url).toBe('/api/v1/gslb/services/svc-1?force=true')
    expect(config?.headers?.['Idempotency-Key']).toMatch(/^gslb-delete-/)

    await deleteGslbService('svc-2', false)
    expect(del.mock.calls[1][0]).toBe('/api/v1/gslb/services/svc-2?force=false')
  })
})

describe('gslb switch status presentation', () => {
  it.each([
    ['PendingApproval', '待审批'],
    ['Approved', '已批准'],
    ['Dispatched', '执行中'],
    ['Succeeded', '成功'],
    ['Failed', '失败'],
    ['Rejected', '已拒绝'],
    ['DrillCompleted', '演练完成'],
  ])('labels %s as %s', (status, label) => {
    expect(gslbSwitchStatusLabel(status)).toBe(label)
  })

  it('falls back to the raw status for unknown values', () => {
    expect(gslbSwitchStatusLabel('SomethingElse')).toBe('SomethingElse')
  })

  it('treats only finished states as terminal', () => {
    expect(GSLB_TERMINAL_STATUSES.has('Succeeded')).toBe(true)
    expect(GSLB_TERMINAL_STATUSES.has('DrillCompleted')).toBe(true)
    expect(GSLB_TERMINAL_STATUSES.has('PendingApproval')).toBe(false)
    expect(GSLB_TERMINAL_STATUSES.has('Dispatched')).toBe(false)
  })
})
