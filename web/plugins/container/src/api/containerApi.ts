/**
 * 容器插件数据访问层（Container 插件）。
 *
 * 与 resource/application 插件一致：在 plugin create(ctx) 中通过
 * setContainerApiClient / setContainerContextStore 注入实例，组件只消费
 * 本模块的封装函数，不直接 fetch。所有请求统一走 @hnb/api-client，
 * 携带上下文头 / traceId / 401 单飞刷新。
 */

import type { ApiClient, ContextStore } from '@hnb/types'

export interface ContainerNamespaceQuota {
  cpu?: number
  memory?: number
  storage?: number
  vgpu?: number
  vram?: number
  gpu?: number
}

export interface ContainerNamespace {
  id: string
  workspace_id: string
  cluster_id?: string
  name: string
  description?: string
  quota?: ContainerNamespaceQuota
  status: string
  created_at: string
  updated_at?: string
}

export interface ContainerCluster {
  id: string
  name: string
  display_name?: string
  status: string
  target_type: string
  shared?: boolean
  created_at?: string
}

let apiClient: ApiClient | null = null
let contextStore: ContextStore | null = null

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

export function setContainerApiClient(client: ApiClient): void {
  apiClient = client
}

export function setContainerContextStore(store: ContextStore): void {
  contextStore = store
}

export function getContainerContextStore(): ContextStore {
  if (!contextStore) throw new Error('container context store is not initialized')
  return contextStore
}

function client(): ApiClient {
  if (!apiClient) throw new Error('container api client is not initialized')
  return apiClient
}

/** 当前激活的工作空间（HNBContext.spaceId），缺省抛错提醒用户切换上下文 */
function currentWorkspaceId(): string {
  const spaceId = contextStore?.current?.spaceId
  if (!spaceId) throw new Error('no active workspace context')
  return spaceId
}

/** api-client 返回完整响应壳 {code,message,data}，本函数解包出数组；兼容直接返回数组的代理通道 */
function unwrapArray<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[]
  const d = data as { data?: T[]; items?: T[] } | null
  return d?.data ?? d?.items ?? []
}

/** 解包单对象响应壳，返回 data 字段 */
function unwrapObject<T>(data: unknown): T {
  const d = data as { data?: T } | null
  if (d && d.data !== undefined) return d.data as T
  return data as T
}

export async function listWorkspaceClusters(workspaceId?: string): Promise<ContainerCluster[]> {
  const wsId = workspaceId ?? currentWorkspaceId()
  const data = await client().get<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/clusters`)
  return unwrapArray<ContainerCluster>(data)
}

export async function listNamespaces(options: { workspaceId?: string; clusterId?: string } = {}): Promise<ContainerNamespace[]> {
  const wsId = options.workspaceId ?? currentWorkspaceId()
  const data = await client().get<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces`, {
    params: { cluster_id: options.clusterId || undefined },
  })
  return unwrapArray<ContainerNamespace>(data)
}

export interface CreateNamespacePayload {
  name: string
  description?: string
  cluster_id?: string
  quota?: ContainerNamespaceQuota
}

export interface UpdateNamespacePayload {
  description?: string
  cluster_id?: string
  quota?: ContainerNamespaceQuota
}

export async function createNamespace(
  payload: CreateNamespacePayload,
  workspaceId?: string,
): Promise<ContainerNamespace> {
  const wsId = workspaceId ?? currentWorkspaceId()
  return unwrapObject<ContainerNamespace>(await client().post<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces`, payload))
}

export async function getNamespace(namespaceId: string, workspaceId?: string): Promise<ContainerNamespace> {
  const wsId = workspaceId ?? currentWorkspaceId()
  return unwrapObject<ContainerNamespace>(await client().get<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces/${encodeURIComponent(namespaceId)}`))
}

export async function updateNamespace(
  namespaceId: string,
  payload: UpdateNamespacePayload,
  workspaceId?: string,
): Promise<ContainerNamespace> {
  const wsId = workspaceId ?? currentWorkspaceId()
  return unwrapObject<ContainerNamespace>(await client().put<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces/${encodeURIComponent(namespaceId)}`, payload))
}

export async function deleteNamespace(namespaceId: string, workspaceId?: string): Promise<void> {
  const wsId = workspaceId ?? currentWorkspaceId()
  await client().delete(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces/${encodeURIComponent(namespaceId)}`)
}

export async function getNamespaceQuotaRemaining(workspaceId?: string): Promise<ContainerNamespaceQuota> {
  const wsId = workspaceId ?? currentWorkspaceId()
  return unwrapObject<ContainerNamespaceQuota>(await client().get<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces/quota-remaining`))
}

export interface NamespaceMember {
  binding_id: string
  subject_id: string
  user_id: string
  username: string
  display_name: string
  email: string
  role_id: string
  role_name: string
  granted_at: string
}

export interface TenantUser {
  subject_id: string
  user_id: string
  username: string
  display_name: string
  email: string
}

export async function listNamespaceMembers(namespaceId: string, workspaceId?: string): Promise<NamespaceMember[]> {
  const wsId = workspaceId ?? currentWorkspaceId()
  const data = await client().get<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces/${encodeURIComponent(namespaceId)}/members`)
  return unwrapArray<NamespaceMember>(data)
}

export async function addNamespaceMember(namespaceId: string, subjectId: string, workspaceId?: string): Promise<void> {
  const wsId = workspaceId ?? currentWorkspaceId()
  await client().post(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces/${encodeURIComponent(namespaceId)}/members`, { subject_id: subjectId })
}

export async function removeNamespaceMember(namespaceId: string, subjectId: string, workspaceId?: string): Promise<void> {
  const wsId = workspaceId ?? currentWorkspaceId()
  await client().delete(`/api/v1/workspaces/${encodeURIComponent(wsId)}/namespaces/${encodeURIComponent(namespaceId)}/members/${encodeURIComponent(subjectId)}`)
}

export async function listTenantUsers(workspaceId?: string): Promise<TenantUser[]> {
  const wsId = workspaceId ?? currentWorkspaceId()
  const data = await client().get<unknown>(`/api/v1/workspaces/${encodeURIComponent(wsId)}/users`)
  return unwrapArray<TenantUser>(data)
}

export type WorkloadType = 'deployment' | 'statefulset' | 'daemonset' | 'job' | 'cronjob' | 'pod'

export interface WorkloadListEnvelope {
  kind?: string
  items?: unknown[]
  metadata?: { resourceVersion?: string; continue?: string }
}

export interface WorkloadQuery {
  clusterId: string
  type: WorkloadType
  namespace?: string
}

const WORKLOAD_K8S_PATH: Record<WorkloadType, string> = {
  deployment: '/apis/apps/v1/namespaces/{ns}/deployments',
  statefulset: '/apis/apps/v1/namespaces/{ns}/statefulsets',
  daemonset: '/apis/apps/v1/namespaces/{ns}/daemonsets',
  job: '/apis/batch/v1/namespaces/{ns}/jobs',
  cronjob: '/apis/batch/v1/namespaces/{ns}/cronjobs',
  pod: '/api/v1/namespaces/{ns}/pods',
}

function workloadPath(type: WorkloadType, namespace?: string): string {
  const base = WORKLOAD_K8S_PATH[type]
  return base.replace('{ns}', namespace && namespace !== '*' ? encodeURIComponent(namespace) : '')
}

function proxyUrl(clusterId: string, path: string): string {
  return `/api/v1/proxy/${encodeURIComponent(clusterId)}/${path.replace(/^\//, '')}`
}

export async function listWorkloads(query: WorkloadQuery): Promise<unknown[]> {
  if (USE_FIXTURES) {
    const { workloadsFixture } = await import('./fixtures/workload')
    const items = workloadsFixture[query.type] ?? []
    if (query.namespace && query.namespace !== '*') {
      return items.filter((item) => (item.metadata as Record<string, unknown>)?.namespace === query.namespace)
    }
    return items
  }
  const path = workloadPath(query.type, query.namespace)
  const data = await client().get<WorkloadListEnvelope>(proxyUrl(query.clusterId, path))
  return data?.items ?? []
}

export async function getWorkload(query: WorkloadQuery & { name: string }): Promise<unknown> {
  if (USE_FIXTURES) {
    const { workloadsFixture } = await import('./fixtures/workload')
    const items = workloadsFixture[query.type] ?? []
    return items.find((item) => (item.metadata as Record<string, unknown>)?.name === query.name) ?? null
  }
  const path = `${workloadPath(query.type, query.namespace)}/${encodeURIComponent(query.name)}`
  return client().get(proxyUrl(query.clusterId, path))
}

export async function deleteWorkload(query: WorkloadQuery & { name: string }): Promise<void> {
  if (USE_FIXTURES) return
  const path = `${workloadPath(query.type, query.namespace)}/${encodeURIComponent(query.name)}`
  await client().delete(proxyUrl(query.clusterId, path))
}

export async function restartWorkload(query: WorkloadQuery & { name: string }): Promise<unknown> {
  if (USE_FIXTURES) return { status: 'ok' }
  const path = `${workloadPath(query.type, query.namespace)}/${encodeURIComponent(query.name)}`
  return client().patch(proxyUrl(query.clusterId, path), {
    spec: { template: { metadata: { annotations: { 'kubectl.kubernetes.io/restartedAt': new Date().toISOString() } } } },
  })
}
