import type { ApiClient } from '@hnb/types'

let client: ApiClient

export function setSystemApiClient(c: ApiClient) {
  console.log('[systemApi] setSystemApiClient called')
  client = c
}

interface ApiResponse<T> {
  code: number
  message: string
  data: T
  total?: number
}

export async function apiGet<T>(url: string, config?: any): Promise<T> {
  if (!client) throw new Error('[systemApi] client not initialized — setSystemApiClient was never called')
  const res = await client.get<ApiResponse<T>>(url, config)
  console.log(`[systemApi] GET ${url} raw response:`, res, '| data:', res.data)
  return res.data as T
}

export async function apiPost<T>(url: string, data?: any, config?: any): Promise<T> {
  const res = await client.post<ApiResponse<T>>(url, data, config)
  return res.data as T
}

export async function apiPut<T>(url: string, data?: any, config?: any): Promise<T> {
  if (!client) throw new Error('[systemApi] client not initialized')
  const res = await client.put<ApiResponse<T>>(url, data, config)
  return res.data as T
}

async function apiPatch<T>(url: string, data?: any, config?: any): Promise<T> {
  const res = await client.patch<ApiResponse<T>>(url, data, config)
  return res.data as T
}

async function apiDelete(url: string, config?: any): Promise<void> {
  await client.delete<ApiResponse<any>>(url, config)
}

export interface UserRecord {
  id: string
  username: string
  email?: string
  phone?: string
  display_name?: string
  source: string
  is_active: boolean
  created_at: string
  updated_at?: string
}

export interface CreateUserPayload {
  username: string
  password: string
  email?: string
  phone?: string
  display_name?: string
}

export interface UpdateUserPayload {
  email?: string
  phone?: string
  display_name?: string
  is_active?: boolean
}

export interface RoleRecord {
  id: string
  name: string
  display_name?: string
  scope: string
  rules?: { verbs: string[]; resources: string[] }[]
  built_in: boolean
  created_at?: string
}

export interface RoleBindingRecord {
  id: string
  role_id: string
  user_id: string
  scope: string
  scope_id?: string
  created_at?: string
}

export interface BindRolePayload {
  user_id: string
  role_id: string
  scope: string
  scope_id?: string
}

export interface TenantRecord {
  id: string
  name: string
  display_name: string
  status: string
  created_at: string
  updated_at: string
}

export interface CreateTenantPayload {
  name: string
  display_name?: string
  quota?: Quota
}

export interface UpdateTenantPayload {
  display_name?: string
  status?: string
}

export interface WorkspaceRecord {
  id: string
  name: string
  display_name?: string
  tenant_id: string
  is_active: boolean
  created_at: string
}

export interface CreateWorkspacePayload {
  name: string
  display_name?: string
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
}

function normalizeResponse<T>(raw: any): PaginatedResponse<T> {
  if (raw && typeof raw === 'object' && 'items' in raw) {
    return { items: raw.items ?? [], total: raw.total ?? 0 }
  }
  if (Array.isArray(raw)) {
    return { items: raw as T[], total: raw.length }
  }
  return { items: [], total: 0 }
}

export async function listUsers(page?: number, pageSize?: number): Promise<PaginatedResponse<UserRecord>> {
  const params: Record<string, string> = {}
  if (page) params.page = String(page)
  if (pageSize) params.page_size = String(pageSize)
  const raw = await apiGet<any>('/api/v1/users', { params })
  return normalizeResponse<UserRecord>(raw)
}

export async function getUser(id: string): Promise<UserRecord> {
  return apiGet<UserRecord>(`/api/v1/users/${id}`)
}

export async function createUser(payload: CreateUserPayload): Promise<{ id: string; username: string }> {
  return apiPost('/api/v1/users', payload)
}

export async function updateUser(id: string, payload: UpdateUserPayload): Promise<UserRecord> {
  return apiPatch(`/api/v1/users/${id}`, payload)
}

export async function deleteUser(id: string): Promise<void> {
  return apiDelete(`/api/v1/users/${id}`)
}

export async function resetPassword(id: string, password: string): Promise<void> {
  await apiPost(`/api/v1/users/${id}/reset-password`, { password })
}

export interface CreateRolePayload {
  name: string
  display_name?: string
  scope: string
  verbs: string[]
  resources: string[]
}

export async function listRoles(): Promise<RoleRecord[]> {
  return apiGet<RoleRecord[]>('/api/v1/roles')
}

export async function createRole(payload: CreateRolePayload): Promise<RoleRecord> {
  return apiPost('/api/v1/roles', payload)
}

export async function deleteRole(id: string): Promise<void> {
  return apiDelete(`/api/v1/roles/${id}`)
}

export async function listRoleBindings(userId?: string): Promise<PaginatedResponse<RoleBindingRecord>> {
  const params: Record<string, string> = {}
  if (userId) params.user_id = userId
  const raw = await apiGet<any>('/api/v1/role-bindings', { params })
  return normalizeResponse<RoleBindingRecord>(raw)
}

export async function bindRole(payload: BindRolePayload): Promise<void> {
  await apiPost('/api/v1/role-bindings', payload)
}

export async function unbindRole(userId: string, scope: string, scopeId: string): Promise<void> {
  await apiDelete(`/api/v1/role-bindings/${userId}/${scope}/${scopeId}`)
}

export async function listTenants(page?: number, pageSize?: number): Promise<PaginatedResponse<TenantRecord>> {
  const params: Record<string, string> = {}
  if (page) params.page = String(page)
  if (pageSize) params.page_size = String(pageSize)
  const raw = await apiGet<any>('/api/v1/tenants', { params })
  return normalizeResponse<TenantRecord>(raw)
}

export async function getTenant(id: string): Promise<TenantRecord> {
  return apiGet<TenantRecord>(`/api/v1/tenants/${id}`)
}

export async function createTenant(payload: CreateTenantPayload): Promise<TenantRecord> {
  return apiPost('/api/v1/tenants', payload)
}

export async function updateTenant(id: string, payload: UpdateTenantPayload): Promise<TenantRecord> {
  return apiPatch(`/api/v1/tenants/${id}`, payload)
}

export async function deleteTenant(id: string): Promise<void> {
  return apiDelete(`/api/v1/tenants/${id}`)
}

export async function listTenantWorkspaces(tenantId: string): Promise<WorkspaceRecord[]> {
  return apiGet<WorkspaceRecord[]>(`/api/v1/tenants/${tenantId}/workspaces`)
}

export async function createTenantWorkspace(tenantId: string, payload: CreateWorkspacePayload): Promise<WorkspaceRecord> {
  return apiPost(`/api/v1/tenants/${tenantId}/workspaces`, payload)
}

export async function listWorkspaces(): Promise<WorkspaceRecord[]> {
  return apiGet<WorkspaceRecord[]>('/api/v1/workspaces')
}

export interface Quota {
  cpu: number
  memory: number
  storage: number
  vgpu: number
  vram: number
  gpu: number
}

export async function getTenantQuota(tenantId: string): Promise<Quota> {
  return apiGet<Quota>(`/api/v1/tenants/${tenantId}/quota`)
}

export async function updateTenantQuota(tenantId: string, quota: Quota): Promise<void> {
  return apiPut(`/api/v1/tenants/${tenantId}/quota`, quota)
}

export async function getWorkspaceQuota(workspaceId: string): Promise<Quota> {
  return apiGet<Quota>(`/api/v1/workspaces/${workspaceId}/quota`)
}

export async function updateWorkspaceQuota(workspaceId: string, quota: Quota): Promise<void> {
  return apiPut(`/api/v1/workspaces/${workspaceId}/quota`, quota)
}

export interface AuditLogRecord {
  id: string
  timestamp: string
  user_id: string
  tenant_id: string
  action: string
  resource_type: string
  status_code: number
  method: string
  path: string
  remote_addr: string
  user_agent: string
}

export async function listAuditLogs(page?: number, pageSize?: number): Promise<AuditLogRecord[]> {
  const params: Record<string, string> = {}
  if (page) params.page = String(page)
  if (pageSize) params.page_size = String(pageSize)
  return apiGet<AuditLogRecord[]>('/api/v1/audit-logs', { params })
}

export interface ExtensionRecord {
  id: string
  name: string
  version: string
  display_name?: string
  description?: string
  enabled: boolean
  created_at?: string
}

export async function listExtensions(): Promise<ExtensionRecord[]> {
  return apiGet<ExtensionRecord[]>('/api/v1/extensions')
}

export interface ClusterRecord {
  id: string
  name: string
  display_name?: string
  target_type: string
  status: string
  is_active: boolean
  created_at: string
}

export async function bindWorkspaceCluster(workspaceId: string, clusterId: string): Promise<void> {
  await apiPost(`/api/v1/workspaces/${workspaceId}/bind-cluster`, { cluster_id: clusterId })
}

export async function unbindWorkspaceCluster(workspaceId: string, clusterId: string): Promise<void> {
  await apiDelete(`/api/v1/workspaces/${workspaceId}/clusters/${clusterId}`)
}

export async function listWorkspaceClusters(workspaceId: string): Promise<ClusterRecord[]> {
  return apiGet<ClusterRecord[]>(`/api/v1/workspaces/${workspaceId}/clusters`)
}
