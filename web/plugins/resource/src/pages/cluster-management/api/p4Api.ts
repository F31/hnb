/**
 * P4 service adapter：边缘节点组 / 项目分配 / 插件实例 / 安全配置。
 * 页面只依赖 typed 函数；开发 fixture，生产返回空态/空列表。
 */
import type {
  EdgeNodeGroup,
  MarketPlugin,
  PluginInstance,
  PluginInstanceListResponse,
  PluginInstancePayload,
  PluginVersionCatalog,
  TenantAllocation,
} from '../types/p4'
import {
  edgeNodeGroupsFixture,
  pluginInstancesFixture,
  pluginMarketCatalogFixture,
  pluginVersionCatalogFixture,
  tenantAllocationsFixture,
  vulnerabilityDbStatusFixture,
} from './fixtures/p4'
import { pluginT } from './pluginI18n'

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

// ---------------------------------------------------------------------------
// 边缘节点组
// ---------------------------------------------------------------------------

export async function getEdgeNodeGroups(
  _clusterId: string,
  params: { keyword?: string } = {},
): Promise<EdgeNodeGroup[]> {
  if (!USE_FIXTURES) return []
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  if (!kw) return edgeNodeGroupsFixture
  return edgeNodeGroupsFixture.filter((g) => g.name.toLowerCase().includes(kw))
}

// ---------------------------------------------------------------------------
// 租户分配
// ---------------------------------------------------------------------------

export async function getTenantAllocations(
  _clusterId: string,
  params: { keyword?: string } = {},
): Promise<TenantAllocation[]> {
  if (!USE_FIXTURES) return []
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  if (!kw) return tenantAllocationsFixture
  return tenantAllocationsFixture.filter((a) => a.tenantName.toLowerCase().includes(kw))
}

/** 删除租户分配（生产暂未实现 → 抛"未开放"） */
export async function deleteTenantAllocation(_clusterId: string, tenantName: string): Promise<void> {
  if (USE_FIXTURES) return
  throw new Error(pluginT('resource.clusterMgmt.error.tenantAllocDeleteUnavailable'))
}

/** 更新租户配额（生产暂未实现 → 抛"未开放"） */
export async function updateTenantAllocation(
  _clusterId: string,
  _tenantName: string,
  _payload: unknown,
): Promise<void> {
  if (USE_FIXTURES) return
  throw new Error(pluginT('resource.clusterMgmt.error.tenantAllocUpdateUnavailable'))
}

// ---------------------------------------------------------------------------
// 插件实例
// ---------------------------------------------------------------------------

export async function getPluginInstances(
  _clusterId: string,
  params: { page?: number; pageSize?: number; keyword?: string } = {},
): Promise<PluginInstanceListResponse> {
  if (!USE_FIXTURES) return { items: [], total: 0 }
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  const filtered = kw
    ? pluginInstancesFixture.filter((p) => p.applicationName.toLowerCase().includes(kw))
    : pluginInstancesFixture
  const page = params.page ?? 1
  const pageSize = params.pageSize ?? 10
  const start = (page - 1) * pageSize
  return { items: filtered.slice(start, start + pageSize), total: filtered.length }
}

/** 插件目录：返回所有可用插件（版本联动下拉） */
export async function getPluginVersionCatalog(_clusterId: string): Promise<PluginVersionCatalog[]> {
  if (!USE_FIXTURES) return []
  return pluginVersionCatalogFixture
}

export async function createPluginInstance(
  _clusterId: string,
  _payload: PluginInstancePayload,
): Promise<void> {
  if (USE_FIXTURES) return
  throw new Error(pluginT('resource.clusterMgmt.error.pluginCreateUnavailable'))
}

export async function updatePluginInstance(
  _clusterId: string,
  _applicationName: string,
  _payload: PluginInstancePayload,
): Promise<void> {
  if (USE_FIXTURES) return
  throw new Error(pluginT('resource.clusterMgmt.error.pluginUpdateUnavailable'))
}

export async function deletePluginInstance(_clusterId: string, applicationName: string): Promise<void> {
  if (USE_FIXTURES) return
  throw new Error(pluginT('resource.clusterMgmt.error.pluginDeleteUnavailable'))
}

// ---------------------------------------------------------------------------
// 插件市场
// ---------------------------------------------------------------------------

/** 插件市场目录（当前后端暂缺 → 开发 fixture，生产空态） */
export async function getPluginMarketCatalog(): Promise<MarketPlugin[]> {
  if (!USE_FIXTURES) return []
  return pluginMarketCatalogFixture
}

/** 安装插件（fixture 更新目录 installed；生产暂未实现 → 抛"未开放"） */
export async function installPlugin(pluginName: string, _version: string): Promise<void> {
  if (USE_FIXTURES) {
    const target = pluginMarketCatalogFixture.find((p) => p.name === pluginName)
    if (target) target.installed = true
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.pluginInstallUnavailable'))
}

/** 卸载插件（fixture 更新目录 installed；生产暂未实现 → 抛"未开放"） */
export async function uninstallPlugin(pluginName: string): Promise<void> {
  if (USE_FIXTURES) {
    const target = pluginMarketCatalogFixture.find((p) => p.name === pluginName)
    if (target) target.installed = false
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.pluginUninstallUnavailable'))
}

// ---------------------------------------------------------------------------
// 安全配置（漏洞库）
// ---------------------------------------------------------------------------

export interface VulnerabilityDbStatus {
  label: string
  updatedAt: string
}

export async function getVulnerabilityDbStatus(): Promise<VulnerabilityDbStatus | null> {
  if (!USE_FIXTURES) return null
  return vulnerabilityDbStatusFixture
}

/** 上传漏洞库 .tgz（模拟进度；生产走 POST /api/v1/security/vulnerability-database） */
export async function uploadVulnerabilityDatabase(
  file: File,
  onProgress?: (percent: number) => void,
): Promise<void> {
  if (!USE_FIXTURES) {
    const form = new FormData()
    form.append('file', file)
    await (await import('./clusterApi')).getClusterApiClient().post('/api/v1/security/vulnerability-database', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    return
  }
  // fixture：模拟分步进度
  for (let i = 10; i <= 100; i += 20) {
    onProgress?.(i)
    await new Promise((resolve) => setTimeout(resolve, 200))
  }
}
