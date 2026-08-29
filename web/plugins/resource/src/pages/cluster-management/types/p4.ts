/**
 * P4 领域类型：边缘节点组 / 项目分配 / 插件实例 / 安全配置。
 * 后端相应端点暂缺，由 service adapter + fixture 提供（OpenSpec P4）。
 */

// ---------------------------------------------------------------------------
// 边缘节点组
// ---------------------------------------------------------------------------

export interface EdgeNodeGroup {
  name: string
  status: 'running' | 'abnormal' | 'unknown'
  nodeCount: number
  description?: string
}

// ---------------------------------------------------------------------------
// 租户分配（容器资源配额）
// ---------------------------------------------------------------------------

/** 单项配额：额度 / 已用 / 百分比 */
export interface AllocationMetric {
  limit: number
  used: number
  percent: number
}

export interface TenantAllocation {
  tenantName: string
  cpu: AllocationMetric
  memory: AllocationMetric
  storage: AllocationMetric
  virtualGpu: AllocationMetric
  virtualVram: AllocationMetric
  physicalGpu: AllocationMetric
}

// ---------------------------------------------------------------------------
// 插件实例
// ---------------------------------------------------------------------------

export type PluginInstanceStatus = 'running' | 'abnormal' | 'unknown'

export interface PluginInstance {
  applicationName: string
  description: string
  pluginName: string
  pluginVersion: string
  status: PluginInstanceStatus
  createdAt: string
  valuesYaml?: string
}

export interface PluginInstanceListResponse {
  items: PluginInstance[]
  total: number
}

/** 新建/更新实例提交载荷 */
export interface PluginInstancePayload {
  applicationName: string
  pluginName: string
  pluginVersion: string
  values: string
}

/** 插件目录：插件名 → 可用版本 */
export interface PluginVersionCatalog {
  pluginName: string
  versions: string[]
}

// ---------------------------------------------------------------------------
// 插件市场
// ---------------------------------------------------------------------------

export interface MarketPlugin {
  name: string
  version: string
  description: string
  category: string
  installed: boolean
  displayName?: string
  kind?: string
  provider?: string
}
