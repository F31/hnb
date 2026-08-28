import type {
  HNBPlugin,
  PluginContext,
  PluginInstance,
} from './plugin'
import type { HNBContext } from './context'

export type { HNBPlugin, PluginContext, PluginInstance }

/**
 * 导航契约类型（单一真源）：从生成的 console-openapi 契约导入，
 * 与 `contracts/openapi/console/v1/openapi.yaml` 同源，禁止在本文件
 * 手写重复定义（V2.6 §6 / 白皮书 §11.1 Schema First）。
 * 注：import type 会被 esbuild/rollup 完全擦除，无运行时跨目录依赖。
 */
import type { NavigationResponse } from '../../../../contracts/generated/typescript/console-openapi/models/NavigationResponse'
import type { NavigationRoute } from '../../../../contracts/generated/typescript/console-openapi/models/NavigationRoute'
import type { NavigationMenu } from '../../../../contracts/generated/typescript/console-openapi/models/NavigationMenu'
import type { NavigationItem } from '../../../../contracts/generated/typescript/console-openapi/models/NavigationItem'

export type { NavigationResponse, NavigationRoute }

/** 菜单分组（契约 NavigationMenu）：group + items */
export type MenuGroup = NavigationMenu

/** 菜单项（契约 NavigationItem）：path 可空——父级分组项仅携带 children */
export type MenuItem = NavigationItem

export interface PluginManifest {
  name: string
  version: string
  displayName: string
  description?: string
  tier: 'T0' | 'T1' | 'T2' | 'T3'
  enabled: boolean
  mode: 'local' | 'remote'
  icon?: string
  entry?: string
  bundleDigest?: string

  permissions: {
    required: string[]
    optional?: string[]
  }

  capabilities: {
    required: string[]
    optional?: string[]
  }

  dependencies: {
    backend: string[]
    plugins?: string[]
  }

  lifecycle?: {
    onInstall?: string
    onEnable?: string
    onDisable?: string
    onUninstall?: string
  }

  menu: {
    group: string
    items: MenuItem[]
  }

  routes?: RouteConfig[]

  exposes?: {
    components?: Record<string, string>
  }

  modules?: Array<{
    entry: string
    componentKey: string
    digest?: string
  }>
}

export interface RouteConfig {
  path: string
  componentKey: string
  pluginId?: string
  permissionCode?: string
  permission?: string
  redirectPath?: string
  redirect?: string
  capability?: string
  keepAlive?: boolean
  /** 服务端下发的 PageSchema id；非空时忽略 componentKey，由 SchemaPage 统一渲染 */
  schemaId?: string
  meta?: {
    title?: string
  }
}

// L2 cache entry (JetStream KV / browser fallback)
export interface NavigationCacheEntry {
  cacheKeyHash: string
  userIdHash: string
  context: HNBContext
  versions: Record<string, string>
  etag: string
  generatedAt: string
  expiresAt: string
  payload: NavigationResponse
}

export interface NavigationStoreState {
  current: NavigationCacheEntry | null
  etag: string
  versions: Record<string, string>
  tenantId?: string
  spaceId?: string
}

/** 导航请求上下文（前端侧；响应中的 context 见契约 NavigationResponse.context） */
export interface NavigationContext {
  tenantId: string
  spaceId?: string
  locale?: string
}
