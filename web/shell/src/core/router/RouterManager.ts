/**
 * RouterManager — per V3.6 §4 (Router Manager)
 *
 * Manages dynamic route registration from NavigationResponse and provides
 * navigation guards for plugin availability, authorization, and tenant selection.
 */

import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
  type Router,
} from 'vue-router'
import type { NavigationRoute } from '@hnb/types'
import { getPluginRegistry } from '@/core/plugin-loader/PluginRegistry'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'
import { usePermissionStore } from '@/stores/permissionStore'
import { usePluginStore } from '@/stores/pluginStore'

function buildNotFoundRoute(): RouteRecordRaw {
  return {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('@/pages/NotFound.vue'),
  }
}

function buildBaseRoutes(): RouteRecordRaw[] {
  return [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/layout/LoginPage.vue'),
      meta: { public: true },
    },
    {
      path: '/tenant-select',
      name: 'TenantSelect',
      component: () => import('@/pages/TenantSelect.vue'),
      meta: { public: true },
    },
    {
      path: '/plugin-error',
      name: 'PluginError',
      component: () => import('@/pages/ErrorPage.vue'),
      meta: { public: true },
    },
    {
      path: '/plugin-unavailable',
      name: 'PluginUnavailable',
      component: () => import('@/pages/ErrorPage.vue'),
      meta: { public: true },
    },
    {
      path: '/license-required',
      name: 'LicenseRequired',
      component: () => import('@/pages/ErrorPage.vue'),
      meta: { public: true },
    },
    {
      path: '/403',
      name: 'Forbidden',
      component: () => import('@/pages/ErrorPage.vue'),
      meta: { public: true },
    },
    {
      path: '/dashboard',
      name: 'Dashboard',
      component: () => import('@/pages/Dashboard.vue'),
    },
    buildNotFoundRoute(),
  ]
}

export class RouterManager {
  private router: Router
  private dynamicRoutesRegistered = false
  private dynamicRouteRemovers: Array<() => void> = []

  constructor() {
    this.router = createRouter({
      history: createWebHistory(),
      routes: buildBaseRoutes(),
    })
    this.setupGuards()
  }

  private setupGuards(): void {
    this.router.beforeEach((to, _from) => {
      const authStore = useAuthStore()
      const contextStore = useContextStore()
      const pluginStore = usePluginStore()
      const permissionStore = usePermissionStore()

      // 1. Public routes (login, error pages, tenant-select) bypass auth checks
      if (to.meta?.public) {
        return true
      }

      // 2. Require authentication for everything else
      if (!authStore.isAuthenticated) {
        return { name: 'Login', query: { redirect: to.fullPath } }
      }

      // 3. Require tenant selection before accessing any console route.
      // 以 contextStore 为准（租户选择的权威状态）：工作空间绑定后、
      // 导航菜单异步加载完成前，路由应放行，由 App 的 loading 态兜底；
      // 若依据 navStore.current 判断，enterWorkspace 的 push 会被弹回
      // TenantSelect，而 navManager.clear() 又使进行中的 loadMenus 失效，
      // 形成无法离开租户选择页的死锁。
      const tenantId = contextStore.tenantId
      if (!tenantId && to.name !== 'TenantSelect' && this.dynamicRoutesRegistered) {
        return { name: 'TenantSelect' }
      }

      // 4. If route is tied to a plugin, check activation state.
      //    schemaId 路由由 SchemaPage 独立渲染，不依赖任何插件。
      const pluginId = to.meta?.pluginId as string | undefined
      const schemaId = to.meta?.schemaId as string | undefined
      if (pluginId && !schemaId) {
        if (pluginStore.hasError(pluginId)) {
          return { name: 'PluginError' }
        }
        if (pluginStore.isLoading(pluginId)) {
          // Wait briefly for plugin to finish loading, then retry
          return new Promise<void>((resolve) => {
            setTimeout(() => resolve(), 50)
          }).then(() => undefined)
        }
        if (!pluginStore.isActivated(pluginId)) {
          return { name: 'PluginUnavailable' }
        }
      }

      // 5. Client-side permission check (defense in depth — backend remains authoritative)
      const routePermission = to.meta?.permissionCode as string | undefined
      if (routePermission && permissionStore.permissions.length > 0 && !permissionStore.hasPermission(routePermission)) {
        console.warn(`[RouterManager] permission denied: required "${routePermission}", available: [${permissionStore.permissions.join(', ')}]`)
        return { name: 'Forbidden' }
      }

      return true
    })

    this.router.afterEach((to) => {
      if (!to.meta?.public) {
        console.debug('[RouterManager] navigated to', to.path)
      }
    })
  }

  /**
   * Register dynamic routes from NavigationResponse. Each route uses
   * PluginRegistry.resolveComponent() for safe component resolution.
   */
  registerDynamicRoutes(routes: NavigationRoute[]): void {
    if (this.dynamicRoutesRegistered) {
      console.warn('[RouterManager] dynamic routes already registered; skipping')
      return
    }

    const registry = getPluginRegistry()
    const pendingFullPath = this.router.currentRoute.value.name === 'NotFound'
      ? this.router.currentRoute.value.fullPath
      : null
    const records: RouteRecordRaw[] = routes
      .filter((r) => r && r.name && r.path && r.pluginId && (r.componentKey || r.schemaId))
      .map((route): RouteRecordRaw => {
        const meta = {
          pluginId: route.pluginId,
          permissionCode: route.permission,
          schemaId: route.schemaId,
        }
        if (route.redirect) {
          return {
            path: route.path,
            name: route.name,
            redirect: (to) => ({ path: route.redirect!, query: to.query, hash: to.hash }),
            meta,
          }
        }
        return {
          path: route.path,
          name: route.name,
          component: route.schemaId
            ? () => import('@/pages/SchemaPage.vue')
            // 过滤已保证 componentKey 存在（schemaId 路由走 SchemaPage）；
            // 契约中 componentKey 可选，这里给空串兜底避免类型错误。
            : () => registry.resolveComponent(route.pluginId, route.componentKey ?? ''),
          meta,
        }
      })

    // Keep the catch-all last so browser URLs resolve to routes loaded from
    // NavigationResponse. Named menu navigation would otherwise mask this.
    this.router.removeRoute('NotFound')
    for (const record of records) {
      // addRoute 返回对应路由的移除函数，保存下来供卸载使用。
      // 注册前先按 name 移除同名旧路由，避免重复注册造成冲突。
      if (record.name && this.router.hasRoute(record.name)) {
        this.router.removeRoute(record.name)
      }
      this.dynamicRouteRemovers.push(this.router.addRoute(record))
    }
    this.router.addRoute(buildNotFoundRoute())
    this.dynamicRoutesRegistered = true
    console.log(`[RouterManager] registered ${records.length} dynamic routes`)

    // A browser refresh resolves plugin paths against the base catch-all
    // before navigation metadata is available. Re-resolve that preserved URL
    // after dynamic routes are registered instead of losing it to a default.
    if (pendingFullPath) {
      const resolved = this.router.resolve(pendingFullPath)
      if (resolved.name && resolved.name !== 'NotFound') {
        void this.router.replace({
          name: resolved.name,
          params: resolved.params,
          query: resolved.query,
          hash: resolved.hash,
          force: true,
        })
      }
    }
    records.forEach((r) => {
      if (r.path?.startsWith?.('/system/')) {
        console.log(`[RouterManager] route ${r.path}: permissionCode=${r.meta?.permissionCode}`)
      }
    })
  }

  /**
   * Remove all dynamic routes from the router instance
   * (called on tenant switch / logout).
   */
  unregisterDynamicRoutes(): void {
    for (const remove of this.dynamicRouteRemovers) {
      remove()
    }
    this.dynamicRouteRemovers = []
    this.dynamicRoutesRegistered = false
  }

  /**
   * Reconcile dynamic routes with a new route set (e.g. after a tenant
   * switch). Simple implementation: tear down all existing dynamic routes,
   * then register the new set. Removal happens first so route-name
   * conflicts between the old and new sets cannot occur.
   */
  reconcile(routes: NavigationRoute[]): void {
    this.unregisterDynamicRoutes()
    this.registerDynamicRoutes(routes)
  }

  getRoutes(): RouteRecordRaw[] {
    return this.router.getRoutes() as unknown as RouteRecordRaw[]
  }

  navigate(name: string, params?: Record<string, any>): void {
    this.router.push({ name, params })
  }

  getLocation() {
    return this.router.currentRoute.value
  }

  getRouter(): Router {
    return this.router
  }
}

let _routerManager: RouterManager | null = null

export function getRouterManager(_initialRoutes?: RouteRecordRaw[]): RouterManager {
  if (!_routerManager) {
    _routerManager = new RouterManager()
  }
  return _routerManager
}

export default getRouterManager
