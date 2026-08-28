/**
 * GSLB 数据访问层（Resource 插件，GSLB-005/007）。
 *
 * 与 cluster-management/api 约定一致：插件 create(ctx) 注入 apiClient；
 * 查询只读 Read Model；写动作提交类型化 RuntimeIntent（审批门控），
 * 绝不直接携带执行细节（providerId/command 等被后端 fail-closed）。
 */

import type { ApiClient, ContextStore } from '@hnb/types'

let apiClient: ApiClient | null = null
let contextStore: ContextStore | null = null

export function setGslbApiClient(client: ApiClient): void {
  apiClient = client
}

export function setGslbContextStore(store: ContextStore): void {
  contextStore = store
}

function client(): ApiClient {
  if (!apiClient) throw new Error('gslb api client is not initialized')
  return apiClient
}

function tenantId(): string {
  // 后端 fail-closed 校验 tenantId 与可信上下文一致；缺失时提交会被拒绝。
  return contextStore?.current.tenantId ?? ''
}

const SERVICES_PATH = '/api/v1/gslb/services'

export interface GslbReadModel {
  serviceId: string
  tenantId: string
  domain: string
  activePoolId?: string
  lifecycleState: string
  healthyPools: string[]
  currentDnsTargets: string[]
  lastSwitchAt?: string
  lastDrillReportId?: string
  lastDrillVerdict?: GslbDrillVerdict
  lastDrillAt?: string
  observedAt: string
}

export interface GslbSwitchRequest {
  id: string
  tenantId: string
  serviceId: string
  intentKind: string
  intentDigest: string
  idempotencyKey: string
  correlationId: string
  requireApproval: boolean
  status: string
  actorId?: string
  approvedBy?: string
  approvedAt?: string
  reason?: string
  error?: string
  createdAt: string
  updatedAt: string
}

export type GslbIntentKind = 'gslb.failover' | 'gslb.switchback' | 'gslb.weight-update' | 'gslb.drill'

export type GslbDrillVerdict = 'Ready' | 'Degraded' | 'Blocked'

export interface GslbDrillCheck {
  name: string
  passed: boolean
  detail?: string
}

export interface GslbDrillReportPayload {
  serviceId: string
  domain: string
  activePoolId?: string
  targetPoolId?: string
  currentTargets: string[]
  projectedTargets: string[]
  projectedWeights?: Record<string, number>
  healthyPools?: string[]
  checks: GslbDrillCheck[]
  verdict: GslbDrillVerdict
  generatedAt: string
}

/** 结构化演练报告（GSLB-010） */
export interface GslbDrillReport {
  id: string
  tenantId: string
  serviceId: string
  requestId: string
  verdict: GslbDrillVerdict
  report: GslbDrillReportPayload
  createdAt: string
}

export interface GslbIntentPayload {
  apiVersion: 'gslb.hnb.io/v1'
  kind: GslbIntentKind
  serviceId: string
  tenantId: string
  targetPoolId?: string
  reason?: string
  metadata: {
    idempotencyKey: string
    correlationId: string
  }
}

/** 只读投影列表（GSLB-007：请求路径零探测） */
export async function listGslbServices(): Promise<{ items: GslbReadModel[]; total: number }> {
  const res = await client().get<{ items: GslbReadModel[]; total: number }>(SERVICES_PATH)
  return res
}

export async function getGslbService(id: string): Promise<GslbReadModel> {
  return client().get<GslbReadModel>(`${SERVICES_PATH}/${id}`)
}

/** 受控流量变更：提交类型化 RuntimeIntent（GSLB-005） */
export async function submitGslbIntent(
  serviceId: string,
  kind: GslbIntentKind,
  targetPoolId: string | undefined,
  reason: string,
): Promise<GslbSwitchRequest> {
  const idempotencyKey = `gslb-${kind}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  const payload: GslbIntentPayload = {
    apiVersion: 'gslb.hnb.io/v1',
    kind,
    serviceId,
    tenantId: tenantId(),
    targetPoolId,
    reason,
    metadata: {
      idempotencyKey,
      correlationId: crypto.randomUUID(),
    },
  }
  return client().post<GslbSwitchRequest>(`${SERVICES_PATH}/${serviceId}/intents`, payload)
}

/** 结构化演练报告列表（GSLB-010，只读） */
export async function listGslbDrillReports(
  serviceId: string,
): Promise<{ items: GslbDrillReport[]; total: number }> {
  return client().get<{ items: GslbDrillReport[]; total: number }>(`${SERVICES_PATH}/${serviceId}/drills`)
}

export function gslbKindLabel(kind: string): string {
  switch (kind) {
    case 'gslb.failover':
      return '故障转移'
    case 'gslb.switchback':
      return '回切'
    case 'gslb.weight-update':
      return '调权'
    case 'gslb.drill':
      return '演练'
    default:
      return kind
  }
}

export function gslbStateLabel(state: string): string {
  switch (state) {
    case 'Active':
      return '运行中'
    case 'Degraded':
      return '降级'
    case 'FailingOver':
      return '切换中'
    case 'Paused':
      return '已冻结'
    case 'Inactive':
      return '未激活'
    default:
      return state
  }
}
