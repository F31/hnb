/**
 * 容器安全治理数据访问层。
 * 定基线/看风险/做检测/出报告；开发 fixture，生产空态。
 */
import type { ApiClient } from '@hnb/types'
import type {
  ClusterAlertRules,
  ClusterProtectionTopology,
  DetectionEvent,
  NamespaceIsolationCoverage,
  NetworkAnomalyEvent,
  SecurityBaselineConfig,
  SecurityDashboardData,
  SecurityOverviewMetrics,
  VulnerabilityDatabaseRecord,
  VulnerabilityScanProject,
  VulnerabilityScanRule,
  SecurityReportItem,
} from './securityTypes'
import {
  anomalyEventsFixture,
  detectionEventsFixture,
  isolationCoverageFixture,
  securityBaselineFixture,
  securityDashboardFixture,
  securityOverviewMetricsFixture,
  securityReportsFixture,
  vulnerabilityDatabaseRecordsFixture,
  vulnerabilityScanProjectsFixture,
} from './fixtures/security'

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'
let apiClient: ApiClient | null = null
const fixtureProtection = new Map<string, boolean>()
const fixtureAlertRules = new Map<string, ClusterAlertRules>()

export function setContainerSecurityClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('container security api client is not initialized')
  return apiClient
}

function proxyUrl(clusterId: string, path: string): string {
  return `/api/v1/proxy/${encodeURIComponent(clusterId)}/${path.replace(/^\//, '')}`
}

export async function getClusterProtectionTopology(clusterId: string): Promise<ClusterProtectionTopology> {
  const [versionResult, nodesResult] = await Promise.allSettled([
    client().get<Record<string, any>>(proxyUrl(clusterId, 'version')),
    client().get<{ items?: any[] }>(proxyUrl(clusterId, 'api/v1/nodes')),
  ])
  const version = versionResult.status === 'fulfilled' ? String(versionResult.value.gitVersion ?? '') : ''
  const nodes = nodesResult.status === 'fulfilled' ? nodesResult.value.items ?? [] : []
  const architectures = new Set<string>()
  const systems = new Set<string>()
  let controlPlaneNodes = 0
  for (const node of nodes) {
    const info = node?.status?.nodeInfo ?? {}
    if (info.architecture) architectures.add(String(info.architecture).toUpperCase())
    if (info.osImage || info.operatingSystem) systems.add(String(info.osImage || info.operatingSystem))
    const labels = node?.metadata?.labels ?? {}
    if ('node-role.kubernetes.io/control-plane' in labels || 'node-role.kubernetes.io/master' in labels) controlPlaneNodes++
  }
  return {
    version,
    architecture: [...architectures].join(', '),
    operatingSystem: [...systems].join(', '),
    controlPlaneNodes,
    workerNodes: Math.max(0, nodes.length - controlPlaneNodes),
  }
}

export function getClusterProtectionEnabled(clusterId: string): boolean {
  return fixtureProtection.get(clusterId) ?? false
}

export async function setClusterProtectionEnabled(clusterId: string, enabled: boolean): Promise<void> {
  if (!USE_FIXTURES) throw new Error('集群安全防护接口未开放')
  fixtureProtection.set(clusterId, enabled)
}

export function getClusterAlertRules(clusterId: string): ClusterAlertRules {
  return fixtureAlertRules.get(clusterId) ?? { vulnerabilityLevel: 'severe', runtimeEvents: true, imageVulnerabilities: true, notification: 'console' }
}

export async function saveClusterAlertRules(clusterId: string, rules: ClusterAlertRules): Promise<void> {
  if (!USE_FIXTURES) throw new Error('安全告警规则接口未开放')
  fixtureAlertRules.set(clusterId, { ...rules })
}

export async function getVulnerabilityDatabaseRecords(): Promise<VulnerabilityDatabaseRecord[]> {
  if (!USE_FIXTURES) return []
  return vulnerabilityDatabaseRecordsFixture.map((item) => ({ ...item }))
}

export async function uploadVulnerabilityDatabase(file: File, updatedBy = 'admin'): Promise<VulnerabilityDatabaseRecord> {
  if (!file.name.toLowerCase().endsWith('.tgz')) throw new Error('only .tgz files are supported')
  if (!USE_FIXTURES) throw new Error('漏洞库上传接口未开放')
  const now = new Date().toISOString()
  const record = { version: file.name.replace(/\.tgz$/i, ''), updatedBy, createdAt: now, updatedAt: now }
  vulnerabilityDatabaseRecordsFixture.unshift(record)
  return { ...record }
}

export async function getVulnerabilityScanProjects(): Promise<VulnerabilityScanProject[]> {
  if (!USE_FIXTURES) return []
  return vulnerabilityScanProjectsFixture.map((item) => ({ ...item }))
}

export async function saveVulnerabilityScanRules(projectIds: string[], rule: VulnerabilityScanRule): Promise<void> {
  if (!USE_FIXTURES) throw new Error('漏洞扫描设置接口未开放')
  for (const item of vulnerabilityScanProjectsFixture) if (projectIds.includes(item.id)) Object.assign(item, rule)
}

export function recentSecurityDates(end = new Date()): string[] {
  return Array.from({ length: 7 }, (_, index) => {
    const date = new Date(end)
    date.setHours(0, 0, 0, 0)
    date.setDate(date.getDate() - (6 - index))
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    return `${year}-${month}-${day}`
  })
}

function emptySecurityDashboard(): SecurityDashboardData {
  return {
    images: { private: 0, public: 0 },
    vulnerabilities: { critical: 0, severe: 0, medium: 0, low: 0, negligible: 0, unknown: 0 },
    runtimeEvents: {
      containerEscape: { pending: 0, handled: 0, ignored: 0 },
      processAnomaly: { pending: 0, handled: 0, ignored: 0 },
      environmentDetection: { pending: 0, handled: 0, ignored: 0 },
    },
    protection: { totalClusters: { enabled: 0, disabled: 0 }, protectedClusterNodes: { enabled: 0, disabled: 0 } },
    trend: recentSecurityDates().map((date) => ({ date, containerEscape: 0, processAnomaly: 0, environmentDetection: 0 })),
  }
}

export async function getSecurityDashboard(): Promise<SecurityDashboardData> {
  const source = USE_FIXTURES ? securityDashboardFixture : emptySecurityDashboard()
  return JSON.parse(JSON.stringify(source)) as SecurityDashboardData
}

export async function getSecurityOverviewMetrics(): Promise<SecurityOverviewMetrics | null> {
  if (!USE_FIXTURES) return null
  return securityOverviewMetricsFixture
}

export async function getIsolationCoverage(): Promise<NamespaceIsolationCoverage[]> {
  if (!USE_FIXTURES) return []
  return isolationCoverageFixture
}

export async function getNetworkAnomalyEvents(): Promise<NetworkAnomalyEvent[]> {
  if (!USE_FIXTURES) return []
  return anomalyEventsFixture
}

export async function getDetectionEvents(): Promise<DetectionEvent[]> {
  if (!USE_FIXTURES) return []
  return detectionEventsFixture
}

export async function getSecurityBaseline(): Promise<SecurityBaselineConfig | null> {
  if (!USE_FIXTURES) return null
  return securityBaselineFixture
}

export async function saveSecurityBaseline(config: SecurityBaselineConfig): Promise<void> {
  if (USE_FIXTURES) {
    Object.assign(securityBaselineFixture, config)
    return
  }
  throw new Error('安全基线保存接口未开放')
}

export async function getSecurityReports(): Promise<SecurityReportItem[]> {
  if (!USE_FIXTURES) return []
  return securityReportsFixture
}
