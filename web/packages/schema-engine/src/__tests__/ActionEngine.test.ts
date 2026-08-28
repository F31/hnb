import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ActionEngine } from '../ActionEngine'
import { DataSourceManager } from '../DataSourceManager'
import type { ActionSchema } from '../types'

function makeEventBus() {
  const handlers = new Map<string, ((...args: any[]) => void)[]>()
  return {
    on: (e: string, h: (...args: any[]) => void) => {
      handlers.set(e, [...(handlers.get(e) ?? []), h])
    },
    off: () => {},
    emit: (e: string, ...args: any[]) => handlers.get(e)?.forEach((h) => h(...args)),
  }
}

describe('ActionEngine', () => {
  let apiClient: any
  let eventBus: ReturnType<typeof makeEventBus>
  let dataSources: DataSourceManager
  let engine: ActionEngine

  beforeEach(() => {
    apiClient = { get: vi.fn(async () => ({})), post: vi.fn(async () => ({})), put: vi.fn(), patch: vi.fn(), delete: vi.fn() }
    eventBus = makeEventBus()
    dataSources = new DataSourceManager(apiClient)
    dataSources.allowEndpoint('/api/v1/workloads')
    dataSources.registerEndpoint({ id: 'workload.restart', path: '/api/v1/workloads/:uid/restart' })
    engine = new ActionEngine({
      apiClient,
      eventBus,
      dataSources,
      hasPermission: (p) => p === 'workload:restart',
      navigate: vi.fn(),
      notify: vi.fn(),
    })
  })

  it('api 动作经 endpoint 白名单执行并替换 pathParams', async () => {
    const action: ActionSchema = {
      id: 'workload.restart',
      type: 'api',
      request: { method: 'POST', endpointId: 'workload.restart', pathParams: ['uid'] },
    }
    await engine.execute(action, { record: { uid: 'abc-1' } })
    expect(apiClient.post).toHaveBeenCalledWith('/api/v1/workloads/abc-1/restart', expect.anything(), undefined)
  })

  it('未注册 endpointId 拒绝执行', async () => {
    const action: ActionSchema = {
      id: 'evil',
      type: 'api',
      request: { method: 'POST', endpointId: 'https://evil.com/x' },
    }
    await expect(engine.execute(action)).rejects.toThrow(/Unknown endpointId/)
    expect(apiClient.post).not.toHaveBeenCalled()
  })

  it('权限不足时拒绝执行', async () => {
    const action: ActionSchema = {
      id: 'workload.restart',
      type: 'api',
      permission: 'workload:delete',
      request: { method: 'POST', endpointId: 'workload.restart' },
    }
    await expect(engine.execute(action)).rejects.toThrow(/Permission denied/)
    expect(apiClient.post).not.toHaveBeenCalled()
  })

  it('confirm 取消时不执行', async () => {
    engine = new ActionEngine({
      apiClient, eventBus, dataSources,
      confirm: async () => false,
    })
    const action: ActionSchema = {
      id: 'workload.restart',
      type: 'api',
      confirm: { messageKey: '确认重启？' },
      request: { method: 'POST', endpointId: 'workload.restart' },
    }
    await engine.execute(action, { record: { uid: '1' } })
    expect(apiClient.post).not.toHaveBeenCalled()
  })

  it('navigate 动作调用注入的导航器', async () => {
    const navigate = vi.fn()
    engine = new ActionEngine({ apiClient, eventBus, dataSources, navigate })
    await engine.execute({
      id: 'go.detail',
      type: 'navigate',
      route: { name: 'container.workloads.detail', params: { uid: '1' } },
    })
    expect(navigate).toHaveBeenCalledWith('container.workloads.detail', { uid: '1' })
  })

  it('emitEvent 只携带 payloadKeys 声明的字段', async () => {
    const received: any[] = []
    eventBus.on('workload:changed', (p) => received.push(p))
    await engine.execute({
      id: 'notify',
      type: 'emitEvent',
      event: { name: 'workload:changed', payloadKeys: ['uid'] },
    }, { record: { uid: '1', secret: 'hidden' } })
    expect(received).toEqual([{ uid: '1' }])
  })
})
