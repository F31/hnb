/**
 * usePluginContext — 在 Vue setup() 内提供插件级共享句柄。
 *
 * 全部经 clusterApi / clusterDetailApi 模块单例获取（由 plugin create(ctx)
 * 注入），不依赖 `@/stores` 别名（插件 tsconfig 不含该别名）。navigate 使用
 * Shell 注入的 `getClusterNavigate()`（router.push），confirm / notify /
 * openOverlay 由本 composable 提供默认实现，调用方可直接使用。
 */
import {
  getClusterApiClient,
  getClusterContextStore,
  getClusterEventBus,
  getClusterNavigate,
  getClusterPermissionStore,
  getClusterCapabilityManager,
} from '../api/clusterApi'
import type { ApiClient, CapabilityManager, ContextStore, EventBus, PermissionStore } from '@hnb/types'

export interface PluginContextHandle {
  apiClient: ApiClient
  eventBus: EventBus
  permissionStore: PermissionStore
  contextStore: ContextStore
  capability: CapabilityManager
  navigate: (path: string) => void
  confirm: (message: string) => Promise<boolean>
  notify: (message: string) => void
  openOverlay: (actionId: string, ctx: Record<string, unknown>) => void
}

/**
 * 注意：contextStore 是 Vue reactive store，可作为 watch 依赖。
 */
export function usePluginContext(): PluginContextHandle {
  return {
    apiClient: getClusterApiClient(),
    eventBus: getClusterEventBus(),
    permissionStore: getClusterPermissionStore(),
    contextStore: getClusterContextStore(),
    capability: getClusterCapabilityManager(),
    navigate: getClusterNavigate(),
    confirm,
    notify,
    openOverlay,
  }
}

async function confirm(message: string): Promise<boolean> {
  if (typeof window !== 'undefined' && typeof window.confirm === 'function') {
    return window.confirm(message)
  }
  return true
}

function notify(message: string): void {
  console.info('[cluster-management]', message)
}

function openOverlay(actionId: string, _ctx: Record<string, unknown>): void {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(
      new CustomEvent('cluster-action-overlay', { detail: { actionId } }),
    )
  }
}

/** 派生 contextKey 字符串（用于 DataSourceManager cacheKey 隔离） */
export function deriveContextKey(ctx: {
  tenantId?: string
  spaceId?: string
}): string {
  return ctx?.tenantId ? `${ctx.tenantId}::${ctx.spaceId ?? ''}` : 'default::'
}
