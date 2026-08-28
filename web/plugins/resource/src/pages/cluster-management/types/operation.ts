/**
 * Operation Center 领域类型。
 *
 * 数据来源：apiserver Operation BFF（`/api/v1/operations`），该 BFF 仅转发
 * platform-api 版本化 API，携带受信 delegation 保留 actor/correlation。
 * 字段命名与 `contracts/openapi/console/v1/openapi.yaml` 的 Operation 契约一致。
 */

export type OperationStatus =
  | 'pending'
  | 'pending_approval'
  | 'queued'
  | 'queued_offline'
  | 'in_progress'
  | 'paused'
  | 'compensating'
  | 'succeeded'
  | 'failed'
  | 'cancelled'

export type OperationAction = 'approve' | 'reject' | 'cancel'

export type OperationStepStatus =
  | 'pending'
  | 'queued'
  | 'in_progress'
  | 'paused'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'compensating'

export interface OperationProgress {
  completedSteps: number
  totalSteps: number
  percent: number
}

export interface SafeFailure {
  code: string
  message: string
  retryable: boolean
}

export interface OperationSummary {
  operationId: string
  intentId: string
  type: string
  status: OperationStatus
  targetId: string
  targetKind: 'KubernetesTarget' | 'EdgeRuntimeTarget'
  progress: OperationProgress
  failure?: SafeFailure
  correlationId: string
  createdAt: string
  updatedAt: string
  completedAt?: string
}

export interface OperationStep {
  stepId: string
  name: string
  status: OperationStepStatus
  attempt: number
  startedAt?: string
  completedAt?: string
  failure?: SafeFailure
}

export interface OperationLinks {
  operation: string
  intent?: string
  target?: string
}

export interface OperationDetail extends OperationSummary {
  executionPlanId: string
  steps: OperationStep[]
  allowedActions: OperationAction[]
  links: OperationLinks
}

export interface OperationListResponse {
  apiVersion: string
  items: OperationSummary[]
  pagination: {
    page: number
    pageSize: number
    total: number
    pageCount: number
    exactTotal: boolean
  }
}

export interface OperationDetailResponse {
  apiVersion: string
  data: OperationDetail
}

export interface OperationListParams {
  page?: number
  pageSize?: number
  status?: OperationStatus | ''
  type?: string
  targetId?: string
}

/** 已提交（非终态）状态，轮询应继续 */
const RUNNING_STATUSES: ReadonlySet<OperationStatus> = new Set([
  'pending',
  'pending_approval',
  'queued',
  'queued_offline',
  'in_progress',
  'paused',
  'compensating',
])

export function isTerminalStatus(status: OperationStatus): boolean {
  return !RUNNING_STATUSES.has(status)
}
