import type { ApiClient } from '@hnb/types'

export type LogWorkloadType = 'deployment' | 'statefulset' | 'daemonset' | 'job' | 'cronjob'

export interface LogPod {
  name: string
  containers: string[]
}

export interface LogWorkload {
  name: string
  namespace: string
  type: LogWorkloadType
  selector: Record<string, string>
  pods?: LogPod[]
}

export interface LogQuery {
  clusterId: string
  namespace: string
  pod: string
  container: string
  tailLines: number
}

const WORKLOAD_PATHS: Record<LogWorkloadType, string> = {
  deployment: 'apis/apps/v1/namespaces/{namespace}/deployments',
  statefulset: 'apis/apps/v1/namespaces/{namespace}/statefulsets',
  daemonset: 'apis/apps/v1/namespaces/{namespace}/daemonsets',
  job: 'apis/batch/v1/namespaces/{namespace}/jobs',
  cronjob: 'apis/batch/v1/namespaces/{namespace}/cronjobs',
}

let apiClient: ApiClient | null = null
const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

export function setContainerLogsClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('container logs api client is not initialized')
  return apiClient
}

function proxyUrl(clusterId: string, path: string): string {
  return `/api/v1/proxy/${encodeURIComponent(clusterId)}/${path.replace(/^\//, '')}`
}

function stringMap(value: unknown): Record<string, string> {
  if (!value || typeof value !== 'object') return {}
  return Object.fromEntries(Object.entries(value as Record<string, unknown>).map(([key, item]) => [key, String(item)]))
}

export function selectorString(selector: Record<string, string>): string {
  return Object.entries(selector).map(([key, value]) => `${key}=${value}`).join(',')
}

export function extractPodContainers(pod: unknown): string[] {
  const spec = (pod as any)?.spec ?? {}
  return [
    ...(Array.isArray(spec.initContainers) ? spec.initContainers : []),
    ...(Array.isArray(spec.containers) ? spec.containers : []),
    ...(Array.isArray(spec.ephemeralContainers) ? spec.ephemeralContainers : []),
  ].map((container: any) => String(container.name ?? '')).filter(Boolean)
}

export async function listLogWorkloads(clusterId: string, namespace: string, type: LogWorkloadType): Promise<LogWorkload[]> {
  if (USE_FIXTURES) {
    const { logWorkloadsFixture } = await import('./fixtures/logs')
    return logWorkloadsFixture.filter((item) => item.namespace === namespace && item.type === type).map((item) => ({ ...item, selector: { ...item.selector }, pods: item.pods?.map((pod) => ({ ...pod, containers: [...pod.containers] })) }))
  }
  const path = WORKLOAD_PATHS[type].replace('{namespace}', encodeURIComponent(namespace))
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, path))
  return (data.items ?? []).map((raw: any) => ({
    name: String(raw?.metadata?.name ?? ''), namespace, type,
    selector: stringMap(raw?.spec?.selector?.matchLabels ?? raw?.spec?.jobTemplate?.spec?.selector?.matchLabels),
  })).filter((item) => item.name)
}

export async function listLogPods(clusterId: string, namespace: string, workload: LogWorkload): Promise<LogPod[]> {
  if (USE_FIXTURES) return (workload.pods ?? []).map((pod) => ({ ...pod, containers: [...pod.containers] }))
  const data = await client().get<{ items?: unknown[] }>(proxyUrl(clusterId, `api/v1/namespaces/${encodeURIComponent(namespace)}/pods`), {
    params: { labelSelector: selectorString(workload.selector) || undefined },
  })
  return (data.items ?? []).map((raw: any) => ({ name: String(raw?.metadata?.name ?? ''), containers: extractPodContainers(raw) })).filter((pod) => pod.name)
}

export async function fetchContainerLogs(query: LogQuery): Promise<string> {
  if (USE_FIXTURES) {
    const { fixtureLogText } = await import('./fixtures/logs')
    return fixtureLogText(query.namespace, query.pod, query.container, query.tailLines)
  }
  const api = client()
  if (!api.requestRaw) throw new Error('raw API responses are not supported by this console runtime')
  const response = await api.requestRaw('GET', proxyUrl(query.clusterId, `api/v1/namespaces/${encodeURIComponent(query.namespace)}/pods/${encodeURIComponent(query.pod)}/log`), undefined, {
    headers: { Accept: '*/*' },
    params: { container: query.container, tailLines: query.tailLines, timestamps: true, follow: false },
  })
  const text = await response.text()
  if (!response.ok) throw new Error(text.trim() || `log request failed: ${response.status}`)
  return text
}
