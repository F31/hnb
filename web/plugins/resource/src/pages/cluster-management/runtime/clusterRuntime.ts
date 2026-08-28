/**
 * 集群管理 Schema 引擎运行时类型与共享辅助（UI 规范 V2.6 §7/§13）。
 *
 * 实际 construction 由 composables/useClusterRuntime.ts 的 buildClusterRuntime
 * 在 Vue setup() 内完成（运行时读取 Pinia stores + 单例 ApiClient）；本文件
 * 只承载共享类型与工具函数。
 */
import type {
  ActionEngine,
  ComponentRegistry,
  DataSourceManager,
  ExtensionRegistry,
  SchemaEngine,
} from '@hnb/schema-engine'
import type { ApiClient, EventBus, PermissionStore } from '@hnb/types'

export interface ClusterRuntime {
  schemaEngine: SchemaEngine
  registry: ComponentRegistry
  dataSources: DataSourceManager
  extensionRegistry: ExtensionRegistry
  actionEngine: ActionEngine
  conditionContext: { permissions: Set<string>; context: { contextKey: string } }
  invalidateContext: () => void
  apiClient: ApiClient
  eventBus: EventBus
  contextKey: string
  notify?: (message: string) => void
  confirm?: (message: string) => Promise<boolean>
}

/** 从 PermissionStore 读取集群相关权限码并产生 conditionContext 使用的 Set。 */
export function permissionsFromStore(store: PermissionStore | null | undefined): Set<string> {
  const out = new Set<string>()
  if (!store || typeof store.hasPermission !== 'function') return out
  const tryCodes = [
    '*',
    'cluster:list',
    'cluster:read',
    'cluster:create',
    'cluster:update',
    'cluster:delete',
    'operation:list',
    'operation:read',
  ]
  for (const code of tryCodes) {
    if (store.hasPermission(code)) out.add(code)
  }
  return out
}
