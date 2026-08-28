/**
 * 容器安全治理 fixture。
 * 仅构建时显式设置 VITE_CLUSTER_DETAIL_USE_FIXTURES=true 时启用。
 */
import type {
  DetectionEvent,
  NamespaceIsolationCoverage,
  NetworkAnomalyEvent,
  SecurityBaselineConfig,
  SecurityDashboardData,
  SecurityOverviewMetrics,
  SecurityReportItem,
  VulnerabilityDatabaseRecord,
  VulnerabilityScanProject,
} from '../securityTypes'

function dateLabel(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const trend = Array.from({ length: 7 }, (_, index) => {
  const date = new Date()
  date.setHours(0, 0, 0, 0)
  date.setDate(date.getDate() - (6 - index))
  return { date: dateLabel(date), containerEscape: 0, processAnomaly: 0, environmentDetection: 0 }
})

export const securityDashboardFixture: SecurityDashboardData = {
  images: { private: 2, public: 5 },
  vulnerabilities: { critical: 0, severe: 0, medium: 0, low: 0, negligible: 0, unknown: 0 },
  runtimeEvents: {
    containerEscape: { pending: 0, handled: 0, ignored: 0 },
    processAnomaly: { pending: 0, handled: 0, ignored: 0 },
    environmentDetection: { pending: 0, handled: 0, ignored: 0 },
  },
  protection: {
    totalClusters: { enabled: 0, disabled: 1 },
    protectedClusterNodes: { enabled: 0, disabled: 0 },
  },
  trend,
}

export const securityOverviewMetricsFixture: SecurityOverviewMetrics = {
  totalNamespaces: 8,
  nakedNamespaces: 3,
  exposedServices: 5,
  exposedIngresses: 2,
  networkAnomalyEvents24h: 12,
  runtimeRisks: 4,
}

export const isolationCoverageFixture: NamespaceIsolationCoverage[] = [
  { namespace: 'default', policyCount: 2, hasDenyAll: true },
  { namespace: 'rd', policyCount: 1, hasDenyAll: true },
  { namespace: 'ai', policyCount: 1, hasDenyAll: false },
  { namespace: 'dr', policyCount: 0, hasDenyAll: false },
  { namespace: 'test', policyCount: 0, hasDenyAll: false },
  { namespace: 'vm-973fd1ef', policyCount: 0, hasDenyAll: false },
]

export const anomalyEventsFixture: NetworkAnomalyEvent[] = [
  { id: 'NA-001', time: '2026-08-07 10:05:00', type: 'port-scan', source: '10.20.1.55', target: '10.20.1.10', severity: 'high', status: 'blocked' },
  { id: 'NA-002', time: '2026-08-07 09:40:00', type: 'anomaly-outbound', source: 'pod:web-7f8d', target: '185.220.101.2', severity: 'high', status: 'detected' },
  { id: 'NA-003', time: '2026-08-07 08:15:00', type: 'dns-tunnel', source: 'pod:app-3b2a', target: 'dns-tunnel.example.com', severity: 'medium', status: 'detected' },
]

export const detectionEventsFixture: DetectionEvent[] = [
  { id: 'DE-001', time: '2026-08-07 10:12:00', category: 'network', event: '异常外联连接已知恶意 IP', pod: 'web-7f8d', severity: 'high', status: 'detected' },
  { id: 'DE-002', time: '2026-08-07 09:50:00', category: 'runtime', event: '容器内检测到异常进程', pod: 'app-3b2a', severity: 'high', status: 'blocked' },
  { id: 'DE-003', time: '2026-08-07 09:20:00', category: 'runtime', event: '敏感文件访问告警', pod: 'db-1c1e', severity: 'medium', status: 'detected' },
  { id: 'DE-004', time: '2026-08-07 08:40:00', category: 'network', event: '端口扫描行为', pod: 'cron-5d2a', severity: 'medium', status: 'blocked' },
]

export const securityBaselineFixture: SecurityBaselineConfig = {
  pssLevel: 'baseline',
  forceNetworkPolicyOnNewNamespace: true,
  denyAllBaseline: true,
  ingressExposure: 'approval',
  admissionRules: true,
}

export const securityReportsFixture: SecurityReportItem[] = [
  { type: 'isolation', title: '网络隔离审计报告', generatedAt: '2026-08-07 00:00:00', summary: '全平台 8 个命名空间中 3 个无任何 NetworkPolicy 保护（裸奔）' },
  { type: 'anomaly', title: '网络异常事件报告', generatedAt: '2026-08-07 00:00:00', summary: '近 24 小时检测到 12 起网络异常行为，其中 5 起已阻断' },
  { type: 'runtime', title: '运行时安全事件报告', generatedAt: '2026-08-06 00:00:00', summary: '近 7 天记录 4 起运行时风险事件' },
  { type: 'vulnerability', title: '漏洞扫描报告', generatedAt: '2026-08-05 00:00:00', summary: '镜像漏洞扫描完成，发现高危漏洞 2 个' },
]

export const vulnerabilityDatabaseRecordsFixture: VulnerabilityDatabaseRecord[] = []

export const vulnerabilityScanProjectsFixture: VulnerabilityScanProject[] = [
  { id: 'zyrs', name: 'zyrs', autoScan: true, scheduledScan: false, frequency: '', scanTime: '' },
  { id: 'test', name: 'test', autoScan: false, scheduledScan: false, frequency: '', scanTime: '' },
  { id: 'test11', name: 'test11', autoScan: false, scheduledScan: false, frequency: '', scanTime: '' },
  { id: 'text', name: 'text', autoScan: true, scheduledScan: false, frequency: '', scanTime: '' },
  { id: 'dilab', name: 'dilab', autoScan: false, scheduledScan: false, frequency: '', scanTime: '' },
  { id: 'rd', name: 'rd', autoScan: true, scheduledScan: false, frequency: '', scanTime: '' },
]
