/**
 * 集群管理数据访问层（Resource 插件）。
 *
 * 约定与 `application/marketApi.ts` 一致：在插件 create(ctx) 中通过
 * setClusterApiClient / setClusterPermissionStore 注入实例，组件只消费
 * 本模块的封装函数，不直接 fetch。
 *
 * 安全约束（V2.5 §13.3 / §16.1）：
 *  - 统一走 @hnb/api-client，携带上下文头 / traceId / 401 单飞刷新；
 *  - 查询只读 Read Model；写动作统一提交 RuntimeIntent 并跟踪 Operation；
 *  - 菜单/按钮隐藏不构成安全边界，服务端仍独立校验权限与租户。
 */

import type { ApiClient, CapabilityManager, ContextStore, EventBus, PermissionStore } from '@hnb/types'
import type {
  ClusterListParams,
  ClusterListResponse,
  ClusterNodeListResponse,
  ClusterSummary,
  RegisterClusterSecretRequest,
  RiskConfirmation,
  RuntimeIntentEnvelope,
  RuntimeIntentSpec,
  RuntimeIntentTargetKind,
  RuntimeIntentRecord,
  SecretReference,
  SecretRegistrationResponse,
  StaleChallenge,
} from '../types/cluster'
import { CLUSTER_PERMISSIONS } from '../types/cluster'

const CLUSTERS_PATH = '/api/v1/resources/clusters'
const INTENTS_PATH = '/api/v1/runtime-intents'
const INTENT_BATCHES_PATH = '/api/v1/runtime-intent-batches'
const SECRETS_REGISTER_PATH = '/api/v1/secrets:register'

let apiClient: ApiClient | null = null
let permissionStore: PermissionStore | null = null
let contextStore: ContextStore | null = null
let eventBus: EventBus | null = null
let capabilityManager: CapabilityManager | null = null
let navigate: ((path: string) => void) | null = null

export function setClusterApiClient(client: ApiClient): void {
  apiClient = client
}

/** 取回已注入的 ApiClient 单例（供集群管理组件使用） */
export function getClusterApiClient(): ApiClient {
  if (!apiClient) throw new Error('cluster api client is not initialized')
  return apiClient
}

export function setClusterPermissionStore(store: PermissionStore): void {
  permissionStore = store
}

export function getClusterPermissionStore(): PermissionStore {
  if (!permissionStore) throw new Error('cluster permission store is not initialized')
  return permissionStore
}

export function setClusterContextStore(store: ContextStore): void {
  contextStore = store
}

export function getClusterContextStore(): ContextStore {
  if (!contextStore) throw new Error('cluster context store is not initialized')
  return contextStore
}

/** EventBus 单例由 plugin create(ctx) 注入；runtime 用它构造 ActionEngine */
export function setClusterEventBus(bus: EventBus): void {
  eventBus = bus
}

export function getClusterEventBus(): EventBus {
  if (!eventBus) throw new Error('cluster event bus is not initialized')
  return eventBus
}

/** 能力门禁单例由 plugin create(ctx) 注入（服务端 gate 的部署覆盖） */
export function setClusterCapabilityManager(manager: CapabilityManager): void {
  capabilityManager = manager
}

export function getClusterCapabilityManager(): CapabilityManager {
  if (!capabilityManager) throw new Error('cluster capability manager is not initialized')
  return capabilityManager
}

/** 路由导航函数由 plugin create(ctx) 注入；插件 composable 用它实现 SPA 内跳转 */
export function setClusterNavigate(fn: (path: string) => void): void {
  navigate = fn
}

export function getClusterNavigate(): (path: string) => void {
  if (!navigate) throw new Error('cluster navigate is not initialized')
  return navigate
}

/** 生成 RuntimeIntent spec.scopeRef（租户/空间作用域，供服务端鉴权与隔离） */
export function currentClusterScope(): string {
  const ctx = contextStore?.current
  if (!ctx?.tenantId) return 'tenant:default'
  return ctx.spaceId
    ? `tenant:${ctx.tenantId}/space:${ctx.spaceId}`
    : `tenant:${ctx.tenantId}`
}

function client(): ApiClient {
  if (!apiClient) throw new Error('cluster api client is not initialized')
  return apiClient
}

/** 前端权限提示（防泄露校验；服务端始终为权威） */
export function hasClusterPermission(permission: string): boolean {
  if (!permissionStore) return false
  return permissionStore.hasPermission(permission) || permissionStore.hasPermission('*')
}

export const clusterPermissions = CLUSTER_PERMISSIONS

// ---------------------------------------------------------------------------
// 只读 Read Model 查询
// ---------------------------------------------------------------------------

export async function listClusters(params: ClusterListParams = {}): Promise<ClusterListResponse> {
  return client().get<ClusterListResponse>(CLUSTERS_PATH, {
    params: {
      page: params.page ?? 1,
      pageSize: params.pageSize ?? 20,
      keyword: params.keyword?.trim() || undefined,
      kind: params.kind || undefined,
      status: params.status || undefined,
    },
  })
}

export async function getCluster(clusterId: string): Promise<ClusterSummary> {
  return client().get<ClusterSummary>(`${CLUSTERS_PATH}/${encodeURIComponent(clusterId)}`)
}

export async function listClusterNodes(
  clusterId: string,
  params: { page?: number; pageSize?: number } = {},
): Promise<ClusterNodeListResponse> {
  return client().get<ClusterNodeListResponse>(
    `${CLUSTERS_PATH}/${encodeURIComponent(clusterId)}/nodes`,
    { params: { page: params.page ?? 1, pageSize: params.pageSize ?? 50 } },
  )
}

// ---------------------------------------------------------------------------
// 敏感凭据注册（SecretReference，禁止在意图载荷中出现明文）
// ---------------------------------------------------------------------------

/** base64 编码（用于把明文 kubeconfig / CloudCore 凭据交给服务端加密落库） */
export function base64Encode(text: string): string {
  if (typeof btoa === 'function') return btoa(text)
  // 浏览器外（SSR/测试）退化处理：逐字符 utf8→base64
  const bytes = new TextEncoder().encode(text)
  let binary = ''
  bytes.forEach((b) => { binary += String.fromCharCode(b) })
  return btoa(binary)
}

/**
 * 注册一个租户级敏感凭据，返回解析后的 SecretReference。
 * 明文仅在此处一次性上送，服务端 AES-256-GCM 加密落库，意图中只引用引用。
 */
export async function registerClusterSecret(
  request: RegisterClusterSecretRequest,
): Promise<SecretRegistrationResponse> {
  return client().post<SecretRegistrationResponse>(SECRETS_REGISTER_PATH, request)
}

/** 由注册响应构造意图使用的 SecretReference */
export function toSecretReference(reg: SecretRegistrationResponse): SecretReference {
  return { provider: reg.provider, scope: reg.scope, name: reg.name, version: reg.version }
}

/** 由集群 kind 推导意图 spec.targetKind（服务端必填枚举） */
export function targetKindFor(kind: ClusterSummary['kind']): RuntimeIntentTargetKind {
  return kind === 'edge' ? 'EdgeRuntimeTarget' : 'KubernetesTarget'
}

// ---------------------------------------------------------------------------
// 写动作：统一 RuntimeIntent 提交（Operation 唯一写入口）
// ---------------------------------------------------------------------------

function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `cluster-${crypto.randomUUID()}`
  }
  return `cluster-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}

function newCorrelationId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `00000000-0000-4000-8000-${Date.now().toString(16).padStart(12, '0')}`
}

export interface SubmitIntentOptions {
  idempotencyKey?: string
  correlationId?: string
  /** STALE challenge 非预选确认（KERNEL-019）；缺省时不附加 riskConfirmation */
  riskConfirmation?: RiskConfirmation
}

export async function submitRuntimeIntent(
  envelope: RuntimeIntentEnvelope,
  options: SubmitIntentOptions = {},
): Promise<RuntimeIntentRecord> {
  const idempotencyKey = options.idempotencyKey ?? envelope.metadata.idempotencyKey ?? newIdempotencyKey()
  const correlationId = options.correlationId ?? envelope.metadata.correlationId ?? newCorrelationId()
  const payload: RuntimeIntentEnvelope = {
    ...envelope,
    metadata: { idempotencyKey, correlationId },
    spec: options.riskConfirmation
      ? { ...envelope.spec, riskConfirmation: options.riskConfirmation }
      : envelope.spec,
  }
  return client().post<RuntimeIntentRecord>(INTENTS_PATH, payload, {
    headers: {
      'Idempotency-Key': idempotencyKey,
      'X-Correlation-ID': correlationId,
      // 服务端以 X-Trace-Id 优先确定受信 correlationId（request_id 中间件），
      // 必须与 body metadata.correlationId 一致，否则触发 CONTEXT_BODY_MISMATCH。
      'X-Trace-Id': correlationId,
    },
  })
}

export interface RuntimeIntentBatchReceipt {
  batch: { id: string; status: string; total_children: number; succeeded_children: number; failed_children: number }
  replayed: boolean
}

/** 批量解除纳管：平台创建父批次，并为每个目标编排 DeleteRuntimeTarget 子意图。 */
export async function submitBatchDeleteRuntimeTargets(targetIds: string[]): Promise<RuntimeIntentBatchReceipt> {
  const idempotencyKey = newIdempotencyKey()
  const correlationId = newCorrelationId()
  return client().post<RuntimeIntentBatchReceipt>(INTENT_BATCHES_PATH, {
    targetIds,
    idempotencyKey,
    correlationId,
  }, {
    headers: { 'Idempotency-Key': idempotencyKey, 'X-Correlation-ID': correlationId, 'X-Trace-Id': correlationId },
  })
}

/** 从 ApiError 解析 STALE challenge 上下文（ProblemDetails 扩展字段）。
 * 使用结构化检测而非 `instanceof ApiError`：插件 bundle 与 Shell 可能持有
 * 不同副本，`instanceof` 会因双重复制失效；错误均由 @hnb/api-client 构造，
 * 携带 code/problem 字段。 */
export function staleChallengeFromError(err: unknown): StaleChallenge | null {
  if (typeof err !== 'object' || err === null) return null
  const e = err as { code?: unknown; problem?: Record<string, unknown> | null }
  if (e.code !== 'STALE_CONFIRMATION_REQUIRED') return null
  const p = e.problem
  if (!p || typeof p.confirmation !== 'string' || !p.confirmation) return null
  return {
    confirmation: p.confirmation,
    policyOutcome:
      p.policyOutcome === 'require_approval' ||
      p.policyOutcome === 'queued_offline' ||
      p.policyOutcome === 'deny'
        ? p.policyOutcome
        : 'allow',
    lastKnownStateAt: typeof p.lastKnownStateAt === 'string' ? p.lastKnownStateAt : undefined,
    lifecycleState: typeof p.lifecycleState === 'string' ? p.lifecycleState : undefined,
    healthState: typeof p.healthState === 'string' ? p.healthState : undefined,
    connectivityState: typeof p.connectivityState === 'string' ? p.connectivityState : undefined,
    targetId: typeof p.targetId === 'string' ? p.targetId : undefined,
    action: typeof p.action === 'string' ? p.action : undefined,
    retryable: typeof p.retryable === 'boolean' ? p.retryable : undefined,
  }
}

/** 构建创建 KubernetesTarget 的 RuntimeIntent（凭据为已注册的 SecretReference） */
export function buildCreateIntent(
  displayName: string,
  credentialSecretRef: SecretReference,
  opts: { kubernetesVersion?: string; parameters?: Record<string, string | number | boolean | null> } = {},
): RuntimeIntentEnvelope {
  const spec: RuntimeIntentSpec = {
    targetKind: 'KubernetesTarget',
    displayName,
    credentialSecretRef,
  }
  if (opts.kubernetesVersion) spec.kubernetesVersion = opts.kubernetesVersion
  if (opts.parameters && Object.keys(opts.parameters).length) spec.parameters = opts.parameters
  return {
    apiVersion: 'hnb.io/v1',
    kind: 'CreateKubernetesTarget',
    metadata: { idempotencyKey: newIdempotencyKey(), correlationId: newCorrelationId() },
    spec,
  }
}

/** 构建纳管已有集群（KubernetesTarget / EdgeRuntimeTarget）的 RuntimeIntent */
export function buildImportIntent(
  kind: 'kubernetes' | 'edge',
  displayName: string,
  credentialSecretRef: SecretReference,
  opts: { cloudCoreEndpoint?: string; parameters?: Record<string, string | number | boolean | null> } = {},
): RuntimeIntentEnvelope {
  const spec: RuntimeIntentSpec = {
    targetKind: kind === 'edge' ? 'EdgeRuntimeTarget' : 'KubernetesTarget',
    displayName,
    credentialSecretRef,
  }
  if (kind === 'edge' && opts.cloudCoreEndpoint) spec.cloudCoreEndpoint = opts.cloudCoreEndpoint
  if (opts.parameters && Object.keys(opts.parameters).length) spec.parameters = opts.parameters
  return {
    apiVersion: 'hnb.io/v1',
    kind: 'ImportRuntimeTarget',
    metadata: { idempotencyKey: newIdempotencyKey(), correlationId: newCorrelationId() },
    spec,
  }
}

/** 构建升级集群的 RuntimeIntent（需 targetId + expectedVersion + desiredVersion） */
export function buildUpgradeIntent(
  cluster: ClusterSummary,
  desiredVersion: string,
): RuntimeIntentEnvelope {
  return {
    apiVersion: 'hnb.io/v1',
    kind: 'UpgradeRuntimeTarget',
    metadata: { idempotencyKey: newIdempotencyKey(), correlationId: newCorrelationId() },
    spec: {
      targetKind: targetKindFor(cluster.kind),
      targetId: cluster.clusterId,
      expectedVersion: cluster.expectedVersion,
      desiredVersion,
    },
  }
}

/** 构建解除纳管的 RuntimeIntent（需 targetId + expectedVersion；服务端以 fencing token / UID 前置条件保护） */
export function buildDeleteIntent(cluster: ClusterSummary): RuntimeIntentEnvelope {
  return {
    apiVersion: 'hnb.io/v1',
    kind: 'DeleteRuntimeTarget',
    metadata: { idempotencyKey: newIdempotencyKey(), correlationId: newCorrelationId() },
    spec: {
      targetKind: targetKindFor(cluster.kind),
      targetId: cluster.clusterId,
      expectedVersion: cluster.expectedVersion,
    },
  }
}

/** 用户选项（创建集群高级设置中的告警联系人选择） */
export interface WizardUserOption {
  id: string
  username: string
}

export async function listWizardUsers(): Promise<WizardUserOption[]> {
  const res = await client().get<{ items: Array<{ id: string; username: string }>; total: number }>(
    '/api/v1/users',
    { params: { page: 1, pageSize: 200 } },
  )
  return (res.items ?? []).map((u) => ({ id: u.id, username: u.username }))
}
