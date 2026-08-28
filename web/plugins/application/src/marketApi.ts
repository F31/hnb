import type { ApiClient } from '@hnb/types'

let apiClient: ApiClient | null = null

export function setMarketApiClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('market api client is not initialized')
  return apiClient
}

export interface MarketProduct {
  id?: string
  name: string
  display_name?: string
  category: string
  status: string
  publisher_id?: string
  description?: string
  labels?: Record<string, string>
  visibility?: string
  release_count?: number
  version_count?: number
}

export interface MarketReleaseManifest {
  artifacts?: unknown[]
  [key: string]: unknown
}

export interface MarketRelease {
  id?: string
  product_id?: string
  product?: string
  version: string
  status: string
  manifest_digest?: string
  release_notes?: string
  manifest?: MarketReleaseManifest | null
}

export interface MarketArtifact {
  id?: string
  name: string
  artifact_type: string
  verification_status: string
  lifecycle_state: string
  size_bytes?: number
  digest?: string
}

export interface MarketRepository {
  id?: string
  name: string
  type?: string
  backend?: string
  endpoint?: string
  service_tier?: string
  authority_role?: string
  secret_reference?: string
  lifecycle_state?: string
  updated_at?: string
}

type CollectionResponse<T> = T[] | { items?: T[]; data?: T[] } | null

function collectionItems<T>(response: CollectionResponse<T>): T[] {
  if (Array.isArray(response)) return response
  return response?.items ?? response?.data ?? []
}

export interface UploadPolicy {
  resumable: boolean
  chunk_size: number
  max_concurrency: number
  max_retries: number
  resume_storage_key: string
}

export interface TransferPolicy extends UploadPolicy {
  strategy: string
  endpoint: string
}

export interface UploadSessionResponse {
  session_id: string
  harbor_url: string
  repository: string
  robot_name: string
  robot_token: string
  expires_at: string
  transfer_endpoint?: string
  strategy?: string
  upload_policy?: UploadPolicy
  transfer?: TransferPolicy
}

export interface TransferStatusResponse {
  session_id: string
  status: string
  uploaded_bytes: number
  total_bytes: number
  completed_parts: number[]
  expires_at: string
}

export interface TransferCompleteResponse {
  artifact_id: string
  digest: string
  size_bytes: number
  registry_url: string
}

export type ProductListResponse = {
  items: MarketProduct[]
  total: number
  page: number
  pageSize: number
}

export async function listProducts(params: { q?: string; category?: string; scope?: string; page?: number; pageSize?: number } = {}) {
  const response = await client().get<ProductListResponse>('/api/v1/market/products', { params })
  return response
}

export async function listReleases(productId: string) {
  const response = await client().get<CollectionResponse<MarketRelease>>(`/api/v1/market/products/${productId}/releases`)
  return collectionItems(response)
}

export async function listReleaseArtifacts(releaseId: string) {
  const response = await client().get<CollectionResponse<MarketArtifact>>(`/api/v1/market/releases/${releaseId}/artifacts`)
  return collectionItems(response)
}

export async function listUnassignedArtifacts() {
  const response = await client().get<CollectionResponse<MarketArtifact>>('/api/v1/market/artifacts', { params: { unassigned: true } })
  return collectionItems(response)
}

export async function attachArtifact(releaseId: string, artifactId: string) {
  return client().post(`/api/v1/market/releases/${releaseId}/artifacts/${artifactId}`)
}

export async function detachArtifact(releaseId: string, artifactId: string) {
  return client().delete(`/api/v1/market/releases/${releaseId}/artifacts/${artifactId}`)
}

export async function listRepositories() {
  const response = await client().get<CollectionResponse<MarketRepository>>('/api/v1/market/artifact-storage/profiles')
  return collectionItems(response)
}

export async function createProduct(payload: unknown) {
  return client().post('/api/v1/market/products', payload)
}

export async function updateProduct(id: string, payload: unknown) {
  return client().patch(`/api/v1/market/products/${id}`, payload)
}

export async function deleteProduct(id: string) {
  return client().delete(`/api/v1/market/products/${id}`)
}

export async function createRelease(productId: string, payload: unknown) {
  return client().post<MarketRelease>(`/api/v1/market/products/${productId}/releases`, payload)
}

export async function updateRelease(id: string, payload: unknown) {
  return client().patch<MarketRelease>(`/api/v1/market/releases/${id}`, payload)
}

export async function deleteRelease(id: string) {
  return client().delete(`/api/v1/market/releases/${id}`)
}

export async function publishRelease(id: string) {
  return client().post(`/api/v1/market/releases/${id}/publish`)
}

export async function createApplication(payload: unknown) {
  return client().post('/api/v1/market/applications', payload)
}

export async function createUploadSession(payload: { filename: string; artifact_type: string; size_bytes: number; release_id?: string }) {
  return client().post<UploadSessionResponse>('/api/v1/market/artifacts/session', payload)
}

export async function confirmArtifact(payload: unknown) {
  return client().post('/api/v1/market/artifacts/confirm', payload)
}

export async function getTransferStatus(endpoint: string) {
  return client().get<TransferStatusResponse>(endpoint)
}

export async function uploadTransferPart(endpoint: string, partNumber: number, chunk: Blob) {
  return client().put(`${endpoint}/parts/${partNumber}`, chunk, { headers: { 'Content-Type': 'application/octet-stream' }, timeoutMs: 300_000 })
}

export async function completeTransfer(endpoint: string) {
  return client().post<TransferCompleteResponse>(`${endpoint}/complete`)
}

export async function abortTransfer(endpoint: string) {
  return client().post(`${endpoint}/abort`)
}

// Artifacts are immutable: PATCH /api/v1/market/artifacts/{id} returns 405.
// To change a field, detach + GC the artifact and re-upload. This helper is
// kept as a deprecated stub so any leftover callers fail loudly rather than
// silently succeeding.
export async function updateArtifact(id: string, payload: unknown) {
  await client().patch(`/api/v1/market/artifacts/${id}`, payload)
  throw new Error('artifacts are immutable; use POST /api/v1/market/artifacts/{id}/gc and re-upload')
}

export async function gcArtifact(id: string, payload: unknown = {}) {
  return client().post(`/api/v1/market/artifacts/${id}/gc`, payload)
}

export async function createRepository(payload: unknown) {
  return client().post('/api/v1/market/artifact-storage/profiles', payload)
}

export async function updateRepository(id: string, payload: unknown) {
  return client().patch(`/api/v1/market/artifact-storage/profiles/${id}`, payload)
}

export async function deleteRepository(id: string) {
  return client().delete(`/api/v1/market/artifact-storage/profiles/${id}`)
}

export async function listSecurityProfiles() {
  const r = await client().get<any[]>('/api/v1/market/security/profiles')
  return Array.isArray(r) ? r : []
}

export async function createSecurityProfile(payload: unknown) {
  return client().post('/api/v1/market/security/profiles', payload)
}

export async function deleteSecurityProfile(id: string) {
  return client().delete(`/api/v1/market/security/profiles/${id}`)
}

export async function listSecurityReports() {
  const r = await client().get<{ items?: any[] }>('/api/v1/market/security/reports')
  return r?.items ?? []
}

export async function getDBStatus() {
  const r = await client().get<any[]>('/api/v1/market/security/db-status')
  return Array.isArray(r) ? r : []
}

export async function listHarborProjects() {
  const r = await client().get<string[]>('/api/v1/market/harbor/projects')
  return Array.isArray(r) ? r : ['hnb']
}

export interface AppGroup {
  id: string
  name: string
  description?: string
  namespace?: string
  group_type: string
  status: string
  app_count: number
  created_at: string
  updated_at: string
}

export async function listGroups(): Promise<AppGroup[]> {
  const r = await client().get<AppGroup[]>('/api/v1/market/applications/groups')
  return Array.isArray(r) ? r : []
}

export async function createGroup(payload: { name: string; description?: string; group_type?: string }): Promise<AppGroup> {
  return client().post<AppGroup>('/api/v1/market/applications/groups', payload)
}

export async function getGroup(id: string): Promise<AppGroup> {
  return client().get<AppGroup>(`/api/v1/market/applications/groups/${id}`)
}

export async function updateGroup(id: string, payload: Partial<AppGroup>): Promise<AppGroup> {
  return client().patch<AppGroup>(`/api/v1/market/applications/groups/${id}`, payload)
}

export async function deleteGroup(id: string): Promise<void> {
  return client().delete(`/api/v1/market/applications/groups/${id}`)
}
