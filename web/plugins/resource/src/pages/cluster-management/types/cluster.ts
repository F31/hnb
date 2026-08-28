/**
 * 集群管理领域类型（resource.clusterManagement）。
 *
 * 数据来源：
 *  - 列表/详情/节点均为只读 Read Model 投影（CQRS，白皮书 §3.5），
 *    请求路径不实时遍历 RuntimeTarget；
 *  - 写动作统一经 RuntimeIntent 提交（白皮书 §3.2 Operation 唯一写入口）。
 *
 * 字段命名与 `contracts/schema/platform/v1/runtime-intent.schema.json`
 * 及运行时目标 Read Model 投影保持一致（Schema First）。
 */

/** 集群类型：KubernetesTarget / EdgeRuntimeTarget / ContainerEngineTarget */
export type ClusterKind = 'kubernetes' | 'edge' | 'container-engine'

import type {
  ClusterIntentKind as GeneratedClusterIntentKind,
  SecretReference as GeneratedSecretReference,
} from '@hnb/contracts/console'

/** 来源：平台创建 or 纳管已有集群 */
export type ClusterSource = 'created' | 'imported'

/**
 * 集群状态（RT-005 新鲜度 / RuntimeTarget 生命周期状态机投影）。
 * 语义色统一由服务端字典 `resource.cluster.status` 治理，前端不自定义。
 */
export type ClusterStatus =
	| 'UNKNOWN'
	| 'REGISTERING'
	| 'PROVISIONING'
	| 'UPGRADING'
	| 'RUNNING'
	| 'DEGRADED'
	| 'STALE'
	| 'FAILED'
  | 'DELETING'
  | 'TERMINATED'

export type ClusterFreshness = 'fresh' | 'stale'

/** STALE 四维状态（RT-005：lifecycle/health/connectivity/freshness 独立维度） */
export interface ClusterFourDimState {
  lifecycleState?: string
  healthState?: string
  connectivityState?: string
  freshnessState?: string
  lastKnownStateAt?: string
}

/** 服务端 STALE challenge 决策（KERNEL-019 §9.7） */
export type StalePolicyOutcome = 'allow' | 'require_approval' | 'queued_offline' | 'deny'

/** STALE challenge 确认载荷（RiskConfirmation，非预选确认） */
export interface RiskConfirmation {
  acknowledged: true
  confirmation: string
}

/**
 * 服务端 STALE challenge 上下文（来自 ProblemDetails 扩展字段，
 * `STALE_CONFIRMATION_REQUIRED` 时由 ApiError.problem 解析）。
 * 仅呈现服务端提供的数据，前端不猜测四维状态。
 */
export interface StaleChallenge {
  confirmation: string
  policyOutcome: StalePolicyOutcome
  lastKnownStateAt?: string
  lifecycleState?: string
  healthState?: string
  connectivityState?: string
  targetId?: string
  action?: string
  retryable?: boolean
}

/** 能力快照（RT-003）：带时间戳，超过新鲜度阈值进入 STALE */
export interface CapabilitySnapshot {
  snapshotVersion: number
  observedAt: string
  freshness: ClusterFreshness
}

/** 集群 Read Model 投影（列表与详情共用） */
export interface ClusterSummary {
  clusterId: string
  displayName: string
  kind: ClusterKind
  source: ClusterSource
  status: ClusterStatus
  runtimeVersion: string
  nodeCount: number
  cpuTotal: string
  memoryTotal: string
  capabilitySnapshot: CapabilitySnapshot
  tenantId: string
  environmentId?: string
  createdAt: string
  updatedAt: string
  /**
   * 乐观锁投影版本（runtime_targets.projection_version）。
   * 升级/解除纳管等写动作须回传该值作为 expectedVersion（>=1 才可操作）。
   */
  expectedVersion?: number
}

/** 集群节点只读视图（RT-006/RT-007：Agent 上报或 CloudCore 代理） */
export interface ClusterNodeInfo {
  nodeId: string
  name: string
  role: 'control-plane' | 'worker' | 'edge'
  status: 'Ready' | 'NotReady' | 'Unknown'
  ipAddress?: string
  os: string
  arch: string
  cpuAllocatable: string
  memoryAllocatable: string
  kubeletVersion: string
  lastHeartbeatAt: string
}

export interface ClusterListResponse {
  items: ClusterSummary[]
  total: number
	/** Server aggregate over all records matching the current filters. */
	summary?: ClusterListAggregate
}

export interface ClusterListAggregate {
  total: number
  running: number
  degraded: number
  stale: number
}

export interface ClusterNodeListResponse {
  items: ClusterNodeInfo[]
  total: number
}

/** 列表查询参数（服务端分页，映射到受信 Query 参数） */
export interface ClusterListParams {
  page?: number
  pageSize?: number
  keyword?: string
  kind?: ClusterKind | ''
  status?: ClusterStatus | ''
}

/**
 * 集群写动作 RuntimeIntent kind。
 *
 * 说明：当前冻结契约 `runtime-intent.schema.json` 的 kind 枚举仅覆盖
 * Release 生命周期；集群 kind 由对应后端 change 扩展（Schema First 演进），
 * 前端先按统一信封提交，服务端按扩展契约接收并规划 ExecutionPlan。
 */
export type ClusterIntentKind = GeneratedClusterIntentKind

/** 敏感凭据只使用 SecretReference，前端不得出现明文 kubeconfig/CloudCore 凭据 */
export type SecretReference = GeneratedSecretReference

/** RuntimeIntent spec.targetKind（服务端 bffIntentSpec 必填枚举） */
export type RuntimeIntentTargetKind = 'KubernetesTarget' | 'EdgeRuntimeTarget'

/**
 * 集群写动作 RuntimeIntent spec。
 *
 * 与服务端 `bffIntentSpec`（apiserver）及 `engine.IntentSpec`（platform-api）
 * 字段名一一对齐；两者均以 `DisallowUnknownFields` 解码，禁止附带
 * targetRef/scopeRef 等遗留字段（会导致 400）。
 *
 *  - 创建/纳管：targetKind + displayName + credentialSecretRef（纳管 edge 另需 cloudCoreEndpoint）
 *  - 升级/解除纳管：targetKind + targetId(UUID) + expectedVersion(>=1)，升级另需 desiredVersion
 */
export interface RuntimeIntentSpec {
  targetKind: RuntimeIntentTargetKind
  displayName?: string
  credentialSecretRef?: SecretReference
  kubernetesVersion?: string
  cloudCoreEndpoint?: string
  nodeGroupMappings?: Record<string, string>
  targetId?: string
  expectedVersion?: number
  desiredVersion?: string
  parameters?: Record<string, string | number | boolean | null>
  /** STALE challenge 非预选确认（KERNEL-019）；仅服务端下发 token 后回传 */
  riskConfirmation?: RiskConfirmation
}

/** 统一 RuntimeIntent 信封（与平台契约字段对齐） */
export interface RuntimeIntentEnvelope {
  apiVersion: 'hnb.io/v1'
  kind: ClusterIntentKind
  metadata: {
    idempotencyKey: string
    correlationId: string
  }
  spec: RuntimeIntentSpec
}

/** POST /api/v1/secrets:register 请求体（值必须为 base64） */
export interface RegisterClusterSecretRequest {
  purpose: 'kubeconfig' | 'cloudcore-client'
  scope: string
  name: string
  value: string
}

/** POST /api/v1/secrets:register 响应（解析后的 SecretReference） */
export interface SecretRegistrationResponse {
  apiVersion: string
  provider: string
  scope: string
  name: string
  version: string
  purpose: string
}

export type RuntimeIntentStatus =
  | 'received'
  | 'validated'
  | 'planned'
  | 'operationCommitted'
  | 'rejected'

/** POST /api/v1/runtime-intents 响应（复用平台契约） */
export interface RuntimeIntentRecord {
  intentId: string
  status: RuntimeIntentStatus
  semanticDigest: string
  intent: unknown
  executionPlanId?: string
  operationId?: string
  createdAt: string
}

/** 集群操作权限码（对齐 scopedPermissionsToCodes 的 `resourceKind:action`） */
export const CLUSTER_PERMISSIONS = {
  list: 'cluster:list',
  read: 'cluster:read',
  create: 'cluster:create',
	update: 'cluster:update',
	delete: 'cluster:delete',
	execute: 'cluster:execute',
} as const

// ---------------------------------------------------------------------------
// 集群详情控制台领域类型（UI 规范 V2.6 / restore-cluster-detail-console §5）
// ---------------------------------------------------------------------------

/** 集群详情展示状态（OpenSpec design §5） */
export type ClusterDetailStatus = 'running' | 'abnormal' | 'unknown'

/**
 * 集群详情基本信息（集群信息 > 集群详情）。
 * 部分字段（OS 版本 / CPU 架构 / VIP / CIDR 等）当前后端 Read Model 未投影，
 * 缺失时由 adapter 补 `--`；service adapter 负责字段映射，页面不拼接 URL。
 */
export interface ClusterDetail {
  id: string
  name: string
  kubernetesVersion: string
  createdAt: string
  osVersion: string
  cpuArchitecture: string
  description?: string
  status: ClusterDetailStatus
  stale?: boolean
  controlPlaneSchedulingEnabled: boolean
  clusterType: string
  /** 集群来源（created=平台创建 / imported=导入纳管）；仅 imported 的 Kubernetes 集群需要手工部署 cluster-agent */
  source?: ClusterSource
  managementVip?: string
  clusterVip?: string
  podCidr?: string
  serviceCidr?: string
  clusterDomain?: string
  kubeOvnJoinCidr?: string
}

/** 插件/平台能力运行状态（OpenSpec cluster-overview：运行中/已安装/未安装/异常/未知） */
export type ClusterPluginStatusKind =
  | 'running'
  | 'installed'
  | 'not-installed'
  | 'abnormal'
  | 'unknown'

export interface ClusterPluginStatus {
  key: string
  displayName: string
  status: ClusterPluginStatusKind
}

/** 节点资源摘要（集群信息 > 节点信息摘要，OpenSpec cluster-overview） */
export interface NodeSummary {
  id: string
  name: string
  role: 'worker' | 'controller' | 'edge' | string
  type: string
  status: string
  managementIp?: string
  clusterIp?: string
  nodeGroup?: string
  cpuCores: number
  memoryGiB: number
  gpuResource?: string
  vramGiB?: number
  createdAt: string
}
