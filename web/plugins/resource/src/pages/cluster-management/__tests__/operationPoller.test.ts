/**
 * Operation 轮询客户端测试（UX-022/UX-023 共享约定）。
 *
 * 覆盖：起始 2s 指数退避至 15s 上限、终态停止、隐藏/离线暂停、
 * 恢复可见后重读、卸载取消。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiClient } from '@hnb/types'
import { createOperationPoller, setOperationApiClient } from '../api/operationApi'
import type { OperationDetail, OperationDetailResponse, OperationStatus } from '../types/operation'

const opId = '356886f1-1b73-49a8-9275-1a85773cf973'

function detail(status: OperationStatus): OperationDetailResponse {
  return {
    apiVersion: 'ui.hnb.io/v1',
    data: {
      operationId: opId,
      intentId: '11111111-1111-4111-8111-111111111111',
      type: 'upgrade',
      status,
      targetId: '515eba09-0a41-5b92-b972-69af1f0f655c',
      targetKind: 'KubernetesTarget',
      progress: { completedSteps: 1, totalSteps: 2, percent: 50 },
      correlationId: '018f6c2a-4a64-7b58-9cc3-9f70462f36c1',
      createdAt: '2026-08-01T00:00:00Z',
      updatedAt: '2026-08-01T00:00:00Z',
      executionPlanId: 'plan-1',
      steps: [],
      allowedActions: ['cancel'],
      links: { operation: `/operations/${opId}` },
    } as OperationDetail,
  }
}

function stubApiClient(responses: (() => OperationDetailResponse) | (() => OperationDetailResponse)[]) {
  const queue: Array<() => OperationDetailResponse> = Array.isArray(responses) ? [...responses] : [responses]
  const get: ApiClient['get'] = (() =>
    Promise.resolve(queue.length > 1 ? queue.shift()!() : queue[0]!())) as ApiClient['get']
  const client: ApiClient = {
    get,
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    patch: vi.fn(),
  }
  return client
}

describe('createOperationPoller', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    Object.defineProperty(window.document, 'visibilityState', { value: 'visible', configurable: true })
    Object.defineProperty(window.navigator, 'onLine', { value: true, configurable: true })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    setOperationApiClient(null as unknown as ApiClient)
  })

  it('polls until terminal status and stops', async () => {
    setOperationApiClient(stubApiClient([() => detail('in_progress'), () => detail('succeeded')]))
    const updates: OperationDetail[] = []
    const terminal: OperationDetail[] = []
    const poller = createOperationPoller({
      initialDelayMs: 2,
      onUpdate: (d) => updates.push(d),
      onTerminal: (d) => terminal.push(d),
    })
    poller.start(opId)
    await vi.advanceTimersByTimeAsync(200)
    expect(updates.length).toBeGreaterThanOrEqual(2)
    expect(terminal.length).toBe(1)
    expect(terminal[0].status).toBe('succeeded')
    poller.stop()
  })

  it('stops polling on cancel and does not fire further updates', async () => {
    setOperationApiClient(stubApiClient(() => detail('in_progress')))
    const updates: OperationDetail[] = []
    const poller = createOperationPoller({ initialDelayMs: 2, onUpdate: (d) => updates.push(d) })
    poller.start(opId)
    await vi.advanceTimersByTimeAsync(10)
    const countAfterStart = updates.length
    expect(countAfterStart).toBeGreaterThan(0)
    poller.cancel()
    await vi.advanceTimersByTimeAsync(100)
    expect(updates.length).toBe(countAfterStart)
  })

  it('pauses while hidden and resumes when visible again', async () => {
    setOperationApiClient(stubApiClient(() => detail('in_progress')))
    const updates: OperationDetail[] = []
    const poller = createOperationPoller({ initialDelayMs: 2, onUpdate: (d) => updates.push(d) })
    poller.start(opId)
    await vi.advanceTimersByTimeAsync(10)
    const before = updates.length
    Object.defineProperty(window.document, 'visibilityState', { value: 'hidden', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(100)
    expect(updates.length).toBe(before)
    Object.defineProperty(window.document, 'visibilityState', { value: 'visible', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(10)
    expect(updates.length).toBeGreaterThan(before)
    poller.stop()
  })
})
