/**
 * 集群监控领域类型（restore-cluster-detail-console cluster-monitoring）。
 * 生产数据由集群监控 BFF 提供；显式开启 fixture 时仅用于本地演示。
 */

/** 五级告警严重度摘要 */
export interface MonitoringAlertCounts {
  critical: number
  major: number
  minor: number
  warning: number
  event: number
}

/** CPU / 内存可调度资源概览（总量 / 使用率 / 分配率 / 超分率） */
export interface ResourceUsageSummary {
  total: number
  usagePercent: number
  used: number
  allocationPercent: number
  allocated: number
  overcommitPercent: number
  overcommitted: number
}

/** 集群监控摘要：告警 + 规模统计 + 可调度资源 */
export interface ClusterMonitoringSummary {
  alerts: MonitoringAlertCounts
  namespaceCount: number
  projectCount: number
  schedulableNodeCount: number
  cpu: ResourceUsageSummary
  memory: ResourceUsageSummary
}

/** 时间序列数据点 */
export interface MetricPoint {
  timestamp: string
  value: number
}

/** 指标序列（含名称 / 单位 / 数据点） */
export interface MetricSeries {
  name: string
  unit: string
  points: MetricPoint[]
}

/** 监控指标 key：CPU / 内存 / GPU / 显存 使用率 */
export type MonitoringMetricKey = 'cpuUsage' | 'memoryUsage' | 'gpuUsage' | 'vramUsage'

/** 监控时间范围查询参数（URL query 持久化） */
export interface MonitoringRange {
  start: string
  end: string
}
