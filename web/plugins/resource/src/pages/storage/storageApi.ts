import type { ApiClient } from '@hnb/types'
import type {
  StorageAlertRule,
  StorageBackendList,
  StorageClassBindingList,
  StorageOverview,
  ProviderBackendSchemaList,
  StorageBackend,
  StorageBackendInput,
  WorkloadStorageOfferingList,
} from '@hnb/contracts/storage'
import { getClusterApiClient } from '../cluster-management/api/clusterApi'

const STORAGE_PATH = '/api/v1/storage'

function client(): ApiClient {
  return getClusterApiClient()
}

export function getStorageOverview(): Promise<StorageOverview> {
  return client().get<StorageOverview>(`${STORAGE_PATH}/overview`)
}

export function listStorageBackends(): Promise<StorageBackendList> {
  return client().get<StorageBackendList>(`${STORAGE_PATH}/backends`)
}

export function listStorageProviderSchemas(): Promise<ProviderBackendSchemaList> {
  return client().get<ProviderBackendSchemaList>(`${STORAGE_PATH}/provider-schemas`)
}

export function createStorageBackend(input: StorageBackendInput): Promise<StorageBackend> {
  return client().post<StorageBackend>(`${STORAGE_PATH}/backends`, input, {
    headers: { 'Idempotency-Key': crypto.randomUUID() },
  })
}

export function listStorageOfferings(): Promise<WorkloadStorageOfferingList> {
  return client().get<WorkloadStorageOfferingList>(`${STORAGE_PATH}/offerings`)
}

export function listStorageOfferingBindings(offeringId: string): Promise<StorageClassBindingList> {
  return client().get<StorageClassBindingList>(`${STORAGE_PATH}/offerings/${encodeURIComponent(offeringId)}/bindings`)
}

export interface StorageAlertRuleList {
  schemaVersion: string
  items: StorageAlertRule[]
}

/** 存储告警规则列表：后端 GET /api/v1/storage/alert-rules 为真实实现 */
export function listStorageAlertRules(): Promise<StorageAlertRuleList> {
  return client().get<StorageAlertRuleList>(`${STORAGE_PATH}/alert-rules`)
}
