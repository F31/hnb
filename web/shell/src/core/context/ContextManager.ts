import type { ContextStore } from '@hnb/types'
import { useContextStore } from '@/stores/contextStore'
import { usePermissionStore } from '@/stores/permissionStore'
import { getNavigationManager } from '@/core/navigation/NavigationManager'
import { getPluginManager } from '@/core/plugin/PluginManager'
import { getRouterManager } from '@/core/router/RouterManager'
import { fetchSessionBootstrap, scopedPermissionsToCodes } from '@/core/api/session'

export function createContextManager(): ContextStore {
  return useContextStore()
}

/**
 * 原子化租户切换编排：先停用全部插件、注销动态路由、清空导航与权限缓存，
 * 再由 contextStore.switchTenant 递增 generation、中止进行中请求并加载
 * 新租户的工作空间。
 *
 * 编排放在这里而不是 contextStore 内部，是为了避免 store → manager → store
 * 的循环依赖：各 manager 均不引用本模块，调用方（TenantSelect）单向依赖这里。
 */
export async function switchTenantAtomic(newTenantId: string): Promise<number> {
  const contextStore = useContextStore()
  const permissionStore = usePermissionStore()

  // 停用全部插件并清空 Registry / pluginStore
  getPluginManager().reset()
  // 从 vue-router 真实移除上一租户的动态路由
  getRouterManager().unregisterDynamicRoutes()
  // 清空 navigationStore 与 L1 缓存（含 ETag），避免旧租户菜单残留
  getNavigationManager().clear()
  permissionStore.clear()

  // generation++、中止进行中请求、清空上下文并加载新租户工作空间
  const gen = await contextStore.switchTenant(newTenantId)

  // 切换后重取权限快照：bootstrap 以受信上下文（JWT）为准，与工作空间无关。
  // 清空后若不重填，前端权限门禁将全部 fail-closed（写按钮等权限控制 UI 全隐藏）。
  try {
    const bootstrap = await fetchSessionBootstrap()
    const codes = scopedPermissionsToCodes(bootstrap.permissions ?? [])
    permissionStore.setPermissions(codes)
    permissionStore.setVersion(bootstrap.permissionVersion || bootstrap.policyVersion || '')
  } catch (err) {
    console.warn('[switchTenantAtomic] failed to refresh permission snapshot:', err)
  }

  return gen
}
