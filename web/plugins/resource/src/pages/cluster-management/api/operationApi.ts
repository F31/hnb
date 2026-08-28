/**
 * Operation 数据访问层（Resource 插件 · cluster-management）。
 *
 * 与 `clusterApi.ts` 同约定：在插件 create(ctx) 中注入 ApiClient，组件只消费
 * 本模块封装函数，不直接 fetch。Operation 列表/详情为只读 BFF 投影；动作
 * 转发保留 actor/idempotency，由服务端 re-authorize。
 */
import type { ApiClient } from '@hnb/types'
import type {
  OperationAction,
  OperationDetailResponse,
  OperationListParams,
  OperationListResponse,
  OperationStatus,
} from '../types/operation'

const OPERATIONS_PATH = '/api/v1/operations'

let apiClient: ApiClient | null = null

export function setOperationApiClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('operation api client is not initialized')
  return apiClient
}

export async function listOperations(params: OperationListParams = {}): Promise<OperationListResponse> {
  return client().get<OperationListResponse>(OPERATIONS_PATH, {
    params: {
      page: params.page ?? 1,
      pageSize: params.pageSize ?? 20,
      status: params.status || undefined,
      type: params.type || undefined,
      targetId: params.targetId || undefined,
    },
  })
}

export async function getOperation(operationId: string): Promise<OperationDetailResponse> {
  return client().get<OperationDetailResponse>(`${OPERATIONS_PATH}/${encodeURIComponent(operationId)}`)
}

export async function operationAction(
  operationId: string,
  action: OperationAction,
  reason?: string,
): Promise<OperationDetailResponse> {
  const body = reason ? { reason } : {}
  return client().post<OperationDetailResponse>(
    `${OPERATIONS_PATH}/${encodeURIComponent(operationId)}/actions/${action}`,
    body,
  )
}

// ---------------------------------------------------------------------------
// Operation 轮询客户端（UX-022/UX-023 共享约定）
//
//  - 起始 2s，指数退避至 15s 上限，加入 ±20% jitter；
//  - 页面隐藏或浏览器离线时暂停；恢复可见后立即重读一次；
//  - 组件卸载 / beforeunload 时取消在途轮询；
//  - 到达终态后停止轮询。
// ---------------------------------------------------------------------------

export interface OperationPollingOptions {
  onUpdate?: (detail: OperationDetailResponse['data']) => void
  onTerminal?: (detail: OperationDetailResponse['data']) => void
  onError?: (err: unknown) => void
  initialDelayMs?: number
  maxDelayMs?: number
}

interface OperationPoller {
  start(operationId: string): void
  stop(): void
  cancel(): void
}

export function createOperationPoller(options: OperationPollingOptions): OperationPoller {
  const initialDelayMs = options.initialDelayMs ?? 2000
  const maxDelayMs = options.maxDelayMs ?? 15000

  let timer: ReturnType<typeof setTimeout> | null = null
  let delayMs = initialDelayMs
  let running = false
  let stopped = false
  let operationId = ''
  let active = true

  function jitter(base: number): number {
    const span = base * 0.2
    return Math.round(base + (Math.random() * 2 - 1) * span)
  }

  async function poll(): Promise<void> {
    if (stopped || !active) return
    let terminal = false
    try {
      const res = await getOperation(operationId)
      const detail = res.data
      options.onUpdate?.(detail)
      terminal = detail.status === 'succeeded' || detail.status === 'failed' || detail.status === 'cancelled'
      if (terminal) {
        stopped = true
        options.onTerminal?.(detail)
      }
    } catch (err) {
      options.onError?.(err)
    }
    if (stopped) return
    delayMs = Math.min(maxDelayMs, delayMs * 2)
    schedule()
  }

  function schedule(): void {
    if (stopped || !active) return
    timer = setTimeout(poll, jitter(delayMs))
  }

  function handleVisibility(): void {
    if (document.visibilityState === 'hidden' || navigator.onLine === false) {
      active = false
      if (timer) {
        clearTimeout(timer)
        timer = null
      }
      return
    }
    if (!active && !stopped) {
      active = true
      delayMs = initialDelayMs
      schedule()
    }
  }

  return {
    start(id: string): void {
      if (running) return
      running = true
      stopped = false
      operationId = id
      active = true
      delayMs = initialDelayMs
      document.addEventListener('visibilitychange', handleVisibility)
      window.addEventListener('offline', handleVisibility)
      window.addEventListener('online', handleVisibility)
      window.addEventListener('beforeunload', () => this.cancel())
      schedule()
    },
    stop(): void {
      stopped = true
      if (timer) {
        clearTimeout(timer)
        timer = null
      }
      document.removeEventListener('visibilitychange', handleVisibility)
      window.removeEventListener('offline', handleVisibility)
      window.removeEventListener('online', handleVisibility)
    },
    cancel(): void {
      this.stop()
      running = false
    },
  }
}

