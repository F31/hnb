/**
 * 容器安全治理领域类型（安全概览 / 基线配置 / 安全防护 / 安全报告）。
 * 边界：本模块只做定基线、看风险、做检测、出报告；策略实操在容器网络层。
 * 后端端点暂缺，adapter + fixture 支撑（生产空态）。
 */

/** 安全概览指标 */
export interface SecurityOverviewMetrics {
  totalNamespaces: number
  nakedNamespaces: number
  exposedServices: number
  exposedIngresses: number
  networkAnomalyEvents24h: number
  runtimeRisks: number
}

export interface SecurityDashboardData {
  images: { private: number; public: number }
  vulnerabilities: { critical: number; severe: number; medium: number; low: number; negligible: number; unknown: number }
  runtimeEvents: {
    containerEscape: { pending: number; handled: number; ignored: number }
    processAnomaly: { pending: number; handled: number; ignored: number }
    environmentDetection: { pending: number; handled: number; ignored: number }
  }
  protection: {
    totalClusters: { enabled: number; disabled: number }
    protectedClusterNodes: { enabled: number; disabled: number }
  }
  trend: Array<{ date: string; containerEscape: number; processAnomaly: number; environmentDetection: number }>
}

/** 命名空间网络隔离覆盖 */
export interface NamespaceIsolationCoverage {
  namespace: string
  policyCount: number
  hasDenyAll: boolean
}

/** 网络异常行为事件 */
export interface NetworkAnomalyEvent {
  id: string
  time: string
  type: 'anomaly-outbound' | 'port-scan' | 'dns-tunnel'
  source: string
  target: string
  severity: 'high' | 'medium' | 'low'
  status: 'detected' | 'blocked'
}

/** 检测事件（运行时 / 网络） */
export interface DetectionEvent {
  id: string
  time: string
  category: 'runtime' | 'network'
  event: string
  pod: string
  severity: 'high' | 'medium' | 'low'
  status: 'detected' | 'blocked'
}

export type ClusterProtectionState = 'enabled' | 'disabled' | 'partial'

export interface ClusterProtectionTopology {
  version: string
  architecture: string
  operatingSystem: string
  controlPlaneNodes: number
  workerNodes: number
}

export interface ClusterAlertRules {
  vulnerabilityLevel: 'critical' | 'severe' | 'medium'
  runtimeEvents: boolean
  imageVulnerabilities: boolean
  notification: 'console' | 'email' | 'webhook'
}

export interface VulnerabilityDatabaseRecord {
  version: string
  updatedBy: string
  createdAt: string
  updatedAt: string
}

export interface VulnerabilityScanRule {
  autoScan: boolean
  scheduledScan: boolean
  frequency: 'daily' | 'weekly' | 'monthly' | ''
  scanTime: string
}

export interface VulnerabilityScanProject extends VulnerabilityScanRule {
  id: string
  name: string
}

/** 安全基线配置（配置的是规则要求，而非具体策略实例） */
export interface SecurityBaselineConfig {
  pssLevel: 'privileged' | 'baseline' | 'restricted'
  forceNetworkPolicyOnNewNamespace: boolean
  denyAllBaseline: boolean
  ingressExposure: 'approval' | 'self-service'
  admissionRules: boolean
}

/** 安全报告 */
export interface SecurityReportItem {
  type: 'vulnerability' | 'runtime' | 'isolation' | 'anomaly'
  title: string
  generatedAt: string
  summary: string
}
