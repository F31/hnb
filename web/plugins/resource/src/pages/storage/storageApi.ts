import type { ApiClient } from '@hnb/types'
import type {
  StorageBackendList,
  StorageClassBindingList,
  StorageDriverInstallationList,
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

export function listStorageDriverInstallations(): Promise<StorageDriverInstallationList> {
  return client().get<StorageDriverInstallationList>(`${STORAGE_PATH}/driver-installations`)
}
