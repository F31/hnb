import type { ApiClient } from '@hnb/types'

let apiClient: ApiClient | null = null

export function setDeliveryApiClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('delivery api client is not initialized')
  return apiClient
}

function unwrap<T>(response: any): T {
  return (response?.data ?? response) as T
}

export interface DeliveryReadiness {
  ready: boolean
  revision: number
  execution: string
  observation: string
  externalProbe: string
  volume: string
  reasons: string[]
  checkedAt: string
}

export interface DeliveryPreview {
  retainVolumes: boolean
  requiresDataCleanupAuthorization: boolean
  resources: Array<{ kind: string; name: string; action: string; note?: string }>
}

export interface OperationRecord {
  id?: string
  operationId?: string
  status?: string
  type?: string
  steps?: Array<{ name: string; status: string; failure?: { message?: string } }>
  failure?: { message?: string }
}

export async function previewPlan(composition: unknown): Promise<unknown> {
  return unwrap<unknown>(await client().post('/api/v1/application-plans:preview', composition))
}

export async function getReadiness(id: string): Promise<DeliveryReadiness> {
  return unwrap<DeliveryReadiness>(await client().get(`/api/v1/application-deliveries/${id}/readiness`))
}

export async function getDeletionPreview(id: string): Promise<DeliveryPreview> {
  return unwrap<DeliveryPreview>(await client().get(`/api/v1/application-deliveries/${id}/deletion-preview`))
}

export async function runAction(id: string, payload: Record<string, unknown>): Promise<OperationRecord> {
  return unwrap<OperationRecord>(await client().post(`/api/v1/application-deliveries/${id}/actions`, payload))
}

export async function approveOperation(id: string): Promise<OperationRecord> {
  return unwrap<OperationRecord>(await client().post(`/api/v1/operations/${id}/actions/approve`, { reason: 'Approved in Portal' }))
}

export async function getOperation(id: string): Promise<OperationRecord> {
  return unwrap<OperationRecord>(await client().get(`/api/v1/operations/${id}`))
}
