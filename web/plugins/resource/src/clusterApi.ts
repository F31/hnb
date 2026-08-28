import type { ApiClient } from '@hnb/types'

let client: ApiClient

export function setClusterApiClient(c: ApiClient) {
  client = c
}

interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

async function apiGet<T>(url: string, config?: any): Promise<T> {
  const res = await client.get<ApiResponse<T>>(url, config)
  return res.data as T
}

async function apiPost<T>(url: string, data?: any, config?: any): Promise<T> {
  const res = await client.post<ApiResponse<T>>(url, data, config)
  return res.data as T
}

async function apiDelete(url: string, config?: any): Promise<void> {
  await client.delete<ApiResponse<any>>(url, config)
}

export interface ClusterRecord {
  id: string
  tenant_id: string
  name: string
  display_name?: string
  target_type: string
  distribution: string
  connection_type: string
  status: string
  labels?: Record<string, string>
  is_active: boolean
  created_at: string
}

export interface CreateClusterPayload {
  name: string
  target_type: string
  connection_type: string
}

export async function listClusters(): Promise<ClusterRecord[]> {
  return apiGet<ClusterRecord[]>('/api/v1/clusters')
}

export async function getCluster(id: string): Promise<ClusterRecord> {
  return apiGet<ClusterRecord>(`/api/v1/clusters/${id}`)
}

export async function createCluster(payload: CreateClusterPayload): Promise<ClusterRecord> {
  return apiPost('/api/v1/clusters', payload)
}

export async function deleteCluster(id: string): Promise<void> {
  return apiDelete(`/api/v1/clusters/${id}`)
}