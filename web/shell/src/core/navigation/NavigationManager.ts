/**
 * NavigationManager — per V3.6 §4 (Navigation Manager)
 *
 * Single source of truth for navigation: always queries backend API, never
 * falls back to static manifest (V3.6 §2.1 — security requirement).
 */

import type {
  NavigationResponse,
  MenuGroup,
  NavigationContext,
  NavigationCacheEntry,
} from '@hnb/types'
import { useNavigationStore } from '@/stores/navigationStore'
import { useAuthStore } from '@/stores/authStore'
import { usePermissionStore } from '@/stores/permissionStore'
import { getEventBus } from '@/core/event-bus'
import { getShellApiClient } from '@/core/api/client'
import { getLocale } from '@/i18n'

const L1_CACHE_TTL_MS = 30_000
const DEFAULT_WATCH_INTERVAL_MS = 60_000
const LKG_KEY_PREFIX = 'hnb.nav.lkg'

export class NavigationManager {
  private store = useNavigationStore()
  private authStore = useAuthStore()
  private eventBus = getEventBus()
  private lastFetchTime = 0
  private lastContext: { tenantId: string; spaceId?: string; locale?: string } | null = null
  private lastEtag = ''
  /** 代际计数：clear()/租户切换后使进行中的迟到响应失效 */
  private fetchGeneration = 0
  private watcherTimer: ReturnType<typeof setInterval> | null = null

  /**
   * Load menus from backend, applying a short L1 in-memory cache to avoid
   * spamming the API on rapid navigation events. The L1 cache is scoped to
   * the requesting tenant/space — a context switch never hits stale entries.
   */
  async loadMenus(context: NavigationContext): Promise<MenuGroup[]> {
    const tenantId = context.tenantId
    if (!tenantId) throw new Error('Tenant ID required for navigation fetch')
    const locale = context.locale ?? currentLocale()

    if (this.isCacheValid(context)) {
      console.debug('[NavigationManager] L1 cache hit')
      return this.store.items
    }

    const gen = this.fetchGeneration
    try {
      const response = await this.fetchMenusFromBackend(tenantId, context.spaceId, locale)

      // 迟到响应丢弃：fetch 期间发生 clear()/租户切换时不写回旧数据
      if (gen !== this.fetchGeneration) {
        console.debug('[NavigationManager] discarding stale response')
        return this.store.items
      }

      const entry: NavigationCacheEntry = {
        cacheKeyHash: '',
        userIdHash: '',
        context: { tenantId, spaceId: response.context?.spaceId ?? context.spaceId, locale },
        versions: response.versions,
        etag: response.etag,
        generatedAt: response.generatedAt,
        expiresAt: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
        payload: response,
      }
      this.store.replace({
        current: entry,
        etag: response.etag,
        versions: response.versions as Record<string, string>,
      })
      this.persistLkg(entry)

      // 权限版本变化：更新本地版本并广播，订阅方可触发权限刷新（V3.6 §10.4）
      const permissionStore = usePermissionStore()
      const permissionVersion = response.versions?.permission ?? ''
      if (permissionVersion && permissionVersion !== permissionStore.version) {
        permissionStore.setVersion(permissionVersion)
        this.eventBus.emit('permission:updated', { version: permissionVersion })
      }

      this.lastFetchTime = Date.now()
      this.lastContext = { tenantId, spaceId: response.context?.spaceId ?? context.spaceId, locale }

      // 仅在版本真正变化时发布事件（304 / 重复刷新不打扰订阅方）
      if (response.etag && response.etag !== this.lastEtag) {
        this.lastEtag = response.etag
        this.eventBus.emit('navigation:updated', {
          tenantId,
          version: response.versions?.navigation,
        })
      }

      return response.menus ?? []
    } catch (error) {
      console.error('[NavigationManager] failed to load menus:', error)
      if (this.store.current?.context?.tenantId === tenantId) {
        console.warn('[NavigationManager] serving last known good menus')
        return this.store.items
      }
      // 内存未命中时尝试本地持久化的 LKG 快照（V3.6 §6.5：
      // 仅使用与当前身份、上下文、版本严格匹配的快照，不绕过服务端鉴权）
      const lkg = this.loadLkg(context)
      if (lkg) {
        console.warn('[NavigationManager] serving persisted last known good menus')
        this.store.replace({
          current: lkg,
          etag: lkg.etag,
          versions: lkg.versions as Record<string, string>,
        })
        return this.store.items
      }
      throw error
    }
  }

  /**
   * LKG 缓存键：userId / tenantId / spaceId / locale / appBuildVersion（V3.6 §6.5）
   */
  private lkgKey(context: NavigationContext): string {
    const userId = this.authStore.user?.id ?? 'anonymous'
    const locale = context.locale ?? currentLocale()
    const build = (import.meta as any).env?.VITE_APP_VERSION ?? 'dev'
    return [LKG_KEY_PREFIX, userId, context.tenantId, context.spaceId ?? '-', locale, build].join('.')
  }

  private persistLkg(entry: NavigationCacheEntry): void {
    const tenantId = entry.context.tenantId
    if (!tenantId) return
    try {
      localStorage.setItem(
        this.lkgKey({ tenantId, spaceId: entry.context.spaceId }),
        JSON.stringify(entry),
      )
    } catch (e) {
      console.warn('[NavigationManager] failed to persist LKG snapshot:', e)
    }
  }

  private loadLkg(context: NavigationContext): NavigationCacheEntry | null {
    try {
      const raw = localStorage.getItem(this.lkgKey(context))
      if (!raw) return null
      const entry = JSON.parse(raw) as NavigationCacheEntry
      // 严格上下文匹配：跨租户/空间的快照一律拒绝
      if (entry?.context?.tenantId !== context.tenantId) return null
      if ((entry.context.spaceId ?? undefined) !== (context.spaceId ?? undefined)) return null
      if ((entry.context.locale ?? currentLocale()) !== (context.locale ?? currentLocale())) return null
      return entry
    } catch {
      return null
    }
  }

  private async fetchMenusFromBackend(
    tenantId: string,
    spaceId: string | undefined,
    locale: string,
  ): Promise<NavigationResponse> {
    const headers: Record<string, string> = {}
    if (this.store.etag) {
      headers['If-None-Match'] = this.store.etag
    }

    const response = await getShellApiClient().requestRaw('GET', '/api/v1/navigation/menus', undefined, {
      headers,
      params: { tenant: tenantId, spaceId, locale },
    })

    if (response.status === 304) {
      if (!this.store.current) {
        throw new Error('304 received but no cached navigation available')
      }
      return this.store.current.payload
    }

    if (!response.ok) {
      throw new Error(`Navigation API failed: ${response.status} ${response.statusText}`)
    }

    const data = (await response.json()) as NavigationResponse
    const etagFromHeader = response.headers.get('etag')
    if (etagFromHeader) {
      data.etag = etagFromHeader
    }
    return data
  }

  private isCacheValid(context: NavigationContext): boolean {
    if (!this.store.current || !this.lastContext) return false
    if (this.lastContext.tenantId !== context.tenantId) return false
    if ((context.locale ?? currentLocale()) !== (this.lastContext.locale ?? currentLocale())) return false
    if (
      context.spaceId !== undefined &&
      this.lastContext.spaceId !== undefined &&
      this.lastContext.spaceId !== context.spaceId
    ) {
      return false
    }
    return Date.now() - this.lastFetchTime < L1_CACHE_TTL_MS
  }

  getMenus(): MenuGroup[] {
    return this.store.items
  }

  getResponse(): NavigationResponse | null {
    return this.store.current?.payload ?? null
  }

  getVersion(): string {
    return this.store.versions?.navigation ?? ''
  }

  getPermissionVersion(): string {
    return this.store.versions?.permission ?? ''
  }

  getPluginCatalogVersion(): string {
    return this.store.versions?.pluginCatalog ?? ''
  }

  invalidateCache(): void {
    this.lastFetchTime = 0
  }

  async forceRefresh(context: NavigationContext): Promise<MenuGroup[]> {
    this.invalidateCache()
    return this.loadMenus(context)
  }

  /**
   * 周期性版本比对（V3.6 §6.4 的降级通道）：在后端 SSE/WebSocket 通知
   * 就绪前，以 ETag 条件请求轮询，版本变化时通过 navigation:updated 事件
   * 驱动前端刷新。页面不可见时暂停轮询（V2.5 §13.5）。
   */
  startVersionWatcher(
    context: NavigationContext,
    intervalMs: number = DEFAULT_WATCH_INTERVAL_MS,
  ): void {
    this.stopVersionWatcher()
    this.watcherTimer = setInterval(() => {
      if (typeof document !== 'undefined' && document.hidden) return
      this.loadMenus(context).catch((err) => {
        console.warn('[NavigationManager] version watcher refresh failed:', err)
      })
    }, intervalMs)
  }

  stopVersionWatcher(): void {
    if (this.watcherTimer) {
      clearInterval(this.watcherTimer)
      this.watcherTimer = null
    }
  }

  clear(): void {
    this.fetchGeneration++
    this.stopVersionWatcher()
    this.lastEtag = ''
    this.store.clear()
    this.lastFetchTime = 0
    this.lastContext = null
  }
}

function currentLocale(): string {
  return getLocale()
}

let _navManager: NavigationManager | null = null

export function getNavigationManager(): NavigationManager {
  if (!_navManager) {
    _navManager = new NavigationManager()
  }
  return _navManager
}

export default getNavigationManager
