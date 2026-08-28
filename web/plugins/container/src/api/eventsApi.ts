import type { ApiClient } from '@hnb/types'

export interface WorkloadEvent {
  id: string
  updatedAt: string
  type: string
  object: string
  reason: string
  message: string
}

let apiClient: ApiClient | null = null
const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

export function setContainerEventsClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('container events api client is not initialized')
  return apiClient
}

function proxyUrl(clusterId: string, path: string): string {
  return `/api/v1/proxy/${encodeURIComponent(clusterId)}/${path.replace(/^\//, '')}`
}

function record(value: unknown): Record<string, any> {
  return value && typeof value === 'object' ? value as Record<string, any> : {}
}

export function mapWorkloadEvent(raw: unknown): WorkloadEvent {
  const item = record(raw)
  const metadata = record(item.metadata)
  const involvedObject = record(item.involvedObject ?? item.regarding)
  const series = record(item.series)
  const name = String(involvedObject.name ?? '')
  const kind = String(involvedObject.kind ?? '')
  return {
    id: String(metadata.uid ?? metadata.name ?? `${kind}-${name}-${item.reason ?? ''}`),
    updatedAt: String(series.lastObservedTime ?? item.eventTime ?? item.lastTimestamp ?? item.firstTimestamp ?? metadata.creationTimestamp ?? ''),
    type: String(item.type ?? 'Normal'),
    object: [kind, name].filter(Boolean).join('/'),
    reason: String(item.reason ?? ''),
    message: String(item.message ?? item.note ?? ''),
  }
}

function eventTime(item: WorkloadEvent): number {
  const value = Date.parse(item.updatedAt)
  return Number.isNaN(value) ? 0 : value
}

export function filterAndSortEvents(items: WorkloadEvent[], objectNames: string[]): WorkloadEvent[] {
  const names = new Set(objectNames)
  return items.filter((item) => names.has(item.object.split('/').at(-1) ?? '')).sort((left, right) => eventTime(right) - eventTime(left))
}

export async function listWorkloadEvents(clusterId: string, namespace: string, objectNames: string[]): Promise<WorkloadEvent[]> {
  if (!objectNames.length) return []
  if (USE_FIXTURES) {
    const { workloadEventsFixture } = await import('./fixtures/events')
    return filterAndSortEvents(workloadEventsFixture.filter((item) => item.namespace === namespace).map((item) => ({ ...item.event })), objectNames)
  }
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/events`))
  return filterAndSortEvents((data.items ?? []).map(mapWorkloadEvent), objectNames)
}
