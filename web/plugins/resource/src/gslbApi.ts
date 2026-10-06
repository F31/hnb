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

/** 健康检查配置（创建请求与 Read Model 共用同一结构） */
export interface GslbHealthCheck {
  protocol: 'http' | 'https'
  path: string
  intervalSeconds: number
  timeoutSeconds: number
  failureThreshold: number
}

export interface GslbReadModel {
  serviceId: string
  tenantId: string
  domain: string
  activePoolId?: string
  lifecycleState: string
  healthyPools: string[]
  currentDnsTargets: string[]
  healthCheck?: GslbHealthCheck
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

/** 地址池成员（GSLB-007 投影） */
export interface GslbPoolMember {
  clusterId: string
  weight: number
  enabled?: boolean
  healthy?: boolean
}

/** 地址池（按 priority 升序，priority 0 为主池） */
export interface GslbPool {
  id?: string
  serviceId?: string
  poolId: string
  name: string
  priority: number
  isActive: boolean
  healthy?: boolean
  members: GslbPoolMember[]
}

/** 切换请求状态（以应用层 service.go 状态机为准） */
export type GslbSwitchRequestStatus =
  | 'PendingApproval'
  | 'Approved'
  | 'Dispatched'
  | 'Succeeded'
  | 'Failed'
  | 'Rejected'
  | 'DrillCompleted'

/** 终态集合：到达后停止轮询 */
export const GSLB_TERMINAL_STATUSES: ReadonlySet<string> = new Set([
  'Succeeded',
  'Failed',
  'Rejected',
  'DrillCompleted',
])

export interface GslbIntentPayload {
  apiVersion: 'gslb.hnb.io/v1'
  kind: GslbIntentKind
  serviceId: string
  tenantId: string
  targetPoolId?: string
  reason?: string
  /** gslb.weight-update 专用：成员 clusterId → 权重（0-100，总和 ≤100，后端权威校验） */
  weights?: Record<string, number>
  metadata: {
    idempotencyKey: string
    correlationId: string
  }
}

/** 服务主档（创建/编辑响应：GslbServiceDetail = Service + pools，含 updatedAt 可作 If-Match） */
export interface GslbServiceDetail {
  id: string
  tenantId: string
  name: string
  domain: string
  routingMode?: string
  activePoolId?: string
  lifecycleState: string
  requireApproval: boolean
  healthCheck?: GslbHealthCheck
  createdAt?: string
  updatedAt?: string
  pools?: GslbPool[]
}

/** 合并式服务编辑输入（PATCH /services/{id}：仅提交需要变更的字段；domain 不可改） */
export interface UpdateGslbServicePayload {
  name?: string
  requireApproval?: boolean
  healthCheck?: GslbHealthCheck
}

/** 地址池整组替换输入（PUT /services/{id}/pools：池以 name 为身份） */
export interface ReplaceGslbPoolsPayload {
  pools: Array<{
    name: string
    priority: number
    members: Array<{ clusterId: string; weight: number }>
  }>
}

/** 创建 GSLB 服务请求体（POST /api/v1/gslb/services；同租户同 domain 幂等返回已有） */
export interface CreateGslbServicePayload {
  name: string
  domain: string
  requireApproval: boolean
  pools: Array<{
    name: string
    priority: number
    members: Array<{ clusterId: string; weight: number }>
  }>
  healthCheck: GslbHealthCheck
}

/** 只读投影列表（GSLB-007：请求路径零探测） */
export async function listGslbServices(): Promise<{ items: GslbReadModel[]; total: number }> {
  const res = await client().get<{ items: GslbReadModel[]; total: number }>(SERVICES_PATH)
  return res
}

export async function getGslbService(id: string): Promise<GslbReadModel> {
  return client().get<GslbReadModel>(`${SERVICES_PATH}/${id}`)
}

/** 地址池拓扑（只读投影） */
export async function listGslbPools(serviceId: string): Promise<{ items: GslbPool[]; total: number }> {
  const res = await client().get<GslbPool[] | { items: GslbPool[]; total?: number }>(`${SERVICES_PATH}/${serviceId}/pools`)
  const items = (Array.isArray(res) ? res : res.items ?? []).map((pool) => ({
    ...pool,
    poolId: pool.poolId || pool.id || '',
  }))
  return { items, total: Array.isArray(res) ? items.length : res.total ?? items.length }
}

/** 切换请求列表（只读；可按状态过滤） */
export async function listGslbSwitchRequests(
  serviceId: string,
  status?: string,
): Promise<{ items: GslbSwitchRequest[]; total: number }> {
  const params = new URLSearchParams({ service_id: serviceId })
  if (status) params.set('status', status)
  return client().get<{ items: GslbSwitchRequest[]; total: number }>(
    `/api/v1/gslb/switch-requests?${params.toString()}`,
  )
}

export async function getGslbSwitchRequest(id: string): Promise<GslbSwitchRequest> {
  return client().get<GslbSwitchRequest>(`/api/v1/gslb/switch-requests/${id}`)
}

/** 受控流量变更：提交类型化 RuntimeIntent（GSLB-005） */
export async function submitGslbIntent(
  serviceId: string,
  kind: GslbIntentKind,
  targetPoolId: string | undefined,
  reason: string,
  weights?: Record<string, number>,
): Promise<GslbSwitchRequest> {
  const idempotencyKey = `gslb-${kind}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  const payload: GslbIntentPayload = {
    apiVersion: 'gslb.hnb.io/v1',
    kind,
    serviceId,
    tenantId: tenantId(),
    targetPoolId,
    reason,
    weights,
    metadata: {
      idempotencyKey,
      correlationId: crypto.randomUUID(),
    },
  }
  return client().post<GslbSwitchRequest>(`${SERVICES_PATH}/${serviceId}/intents`, payload)
}

/** 创建 GSLB 服务（同步返回创建后的服务详情，含 pools） */
export async function createGslbService(payload: CreateGslbServicePayload): Promise<GslbReadModel> {
  return client().post<GslbReadModel>(SERVICES_PATH, payload)
}

/**
 * 受控写路径统一头（PATCH/PUT/DELETE 共用）：
 * If-Match 乐观并发（服务 updatedAt 的 RFC3339Nano，或 * 跳过版本比对——
 * 只读投影不含 updatedAt，控制台默认传 *）+ 幂等键 + 关联 ID。
 */
function gslbWriteHeaders(op: string, ifMatch: string): Record<string, string> {
  return {
    'If-Match': ifMatch,
    'X-Correlation-ID': crypto.randomUUID(),
    'Idempotency-Key': `gslb-${op}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
  }
}

/** 合并式编辑服务可变字段（name/requireApproval/healthCheck；409 = If-Match 过期或 FailingOver） */
export async function updateGslbService(
  id: string,
  patch: UpdateGslbServicePayload,
  ifMatch = '*',
): Promise<GslbServiceDetail> {
  return client().patch<GslbServiceDetail>(`${SERVICES_PATH}/${id}`, patch, {
    headers: gslbWriteHeaders('update', ifMatch),
  })
}

/** 地址池整组替换（409 = 当前 active 池名不在新配置中或 FailingOver） */
export async function replaceGslbPools(
  id: string,
  pools: ReplaceGslbPoolsPayload['pools'],
  ifMatch = '*',
): Promise<{ items: GslbPool[]; total: number }> {
  return client().put<{ items: GslbPool[]; total: number }>(
    `${SERVICES_PATH}/${id}/pools`,
    { pools } satisfies ReplaceGslbPoolsPayload,
    { headers: gslbWriteHeaders('pools-replace', ifMatch) },
  )
}

/** 删除服务（204；非 Inactive/Paused 需 force=true；有在途切换请求时 409） */
export async function deleteGslbService(id: string, force: boolean): Promise<void> {
  return client().delete<void>(`${SERVICES_PATH}/${id}?force=${force ? 'true' : 'false'}`, {
    headers: gslbWriteHeaders('delete', '*'),
  })
}

/** 审批通过（POST 空 body，后端从可信上下文取审批人） */
export async function approveSwitchRequest(id: string): Promise<GslbSwitchRequest> {
  return client().post<GslbSwitchRequest>(`/api/v1/gslb/switch-requests/${id}/approve`, {})
}

/** 审批拒绝（POST 空 body） */
export async function rejectSwitchRequest(id: string): Promise<GslbSwitchRequest> {
  return client().post<GslbSwitchRequest>(`/api/v1/gslb/switch-requests/${id}/reject`, {})
}

export async function approveGslbSwitchRequest(id: string): Promise<GslbSwitchRequest> {
  return approveSwitchRequest(id)
}

export async function rejectGslbSwitchRequest(id: string): Promise<GslbSwitchRequest> {
  return rejectSwitchRequest(id)
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

export function gslbSwitchStatusLabel(status: string): string {
  switch (status) {
    case 'PendingApproval':
      return '待审批'
    case 'Approved':
      return '已批准'
    case 'Dispatched':
      return '执行中'
    case 'Succeeded':
      return '成功'
    case 'Failed':
      return '失败'
    case 'Rejected':
      return '已拒绝'
    case 'DrillCompleted':
      return '演练完成'
    default:
      return status
  }
}
