/**
 * PluginManager — per V3.6 §4.4
 *
 * Orchestrates plugin lifecycle: discovered → loading → loaded → activating → active.
 * Separates loading (module resolution) from activation (calling onActivate).
 * Implements error isolation — one plugin failing should not break the whole app.
 */

import type { PluginInstance, PluginManifest } from '@hnb/types'
import { createApiClient } from '@hnb/api-client'
import { PluginLoader } from '@/core/plugin-loader/PluginLoader'
import { getPluginRegistry } from '@/core/plugin-loader/PluginRegistry'
import { getEventBus } from '@/core/event-bus'
import { usePluginStore } from '@/stores/pluginStore'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'
import { usePermissionStore } from '@/stores/permissionStore'
import { createCapabilityManager } from '@/core/capability'
import { getExtensionRegistry } from '@/core/extension/ExtensionRegistry'
import { createPluginI18n } from '@/i18n'
import { getRouterManager } from '@/core/router/RouterManager'

export interface PluginLifecycleState {
  discovered: boolean
  loading: boolean
  loaded: boolean
  activating: boolean
  active: boolean
  inactive: boolean
  unloaded: boolean
  error: Error | null
}

/** 生命周期钩子超时（V3.6 §4.4：幂等、超时、错误隔离） */
const LIFECYCLE_TIMEOUT_MS = 10_000

export class PluginManager {
  private loader: PluginLoader
  private store = usePluginStore()
  private eventBus = getEventBus()
  private pluginStates = new Map<string, PluginLifecycleState>()
  private activePlugins = new Set<string>()
  private abortControllers = new Map<string, AbortController>()
  private manifests = new Map<string, PluginManifest>()
  /** Remote Bundle 默认关闭（V3.6 §13.4），由部署配置显式开启 */
  private remoteBundlesEnabled =
    (import.meta as any).env?.VITE_ENABLE_REMOTE_BUNDLES === 'true'

  constructor(loader?: PluginLoader) {
    this.loader = loader || new PluginLoader()
  }

  setRemoteBundlesEnabled(enabled: boolean): void {
    this.remoteBundlesEnabled = enabled
  }

  discover(): PluginManifest[] {
    return getPluginRegistry()
      .getAllPlugins()
      .filter((p) => p.enabled)
  }

  async loadPlugin(manifest: PluginManifest): Promise<PluginInstance> {
    const pluginId = manifest.name
    this._setState(pluginId, { loading: true, error: null })
    this.store.setLoading(pluginId, true)

    try {
      this.manifests.set(pluginId, manifest)
      const mode = manifest.mode ?? 'local'
      if (mode === 'remote' && !this.remoteBundlesEnabled) {
        throw new Error(
          `Remote bundles are disabled; refusing to load remote plugin "${pluginId}"`,
        )
      }
      const registry = getPluginRegistry()
      registry.registerManifest(manifest)
      const context = this._createPluginContext(pluginId)
      const instance = mode === 'remote'
        ? await this.loader.loadRemotePlugin(manifest, context)
        : await this.loader.loadLocalPlugin(manifest, context)
      const components = this.loader.getModule(pluginId)?.components
      if (components) registry.register(pluginId, components)

      this._setState(pluginId, { loaded: true })
      this.store.add(instance)
      console.log('[PluginManager] plugin registered:', pluginId)
      return instance
    } catch (error) {
      this._setState(pluginId, { error: error as Error })
      this.store.setError(pluginId, error as Error)
      this.eventBus.emit('plugin:error', { name: pluginId, error })
      throw error
    } finally {
      this._setState(pluginId, { loading: false })
      this.store.setLoading(pluginId, false)
    }
  }

  async activatePlugin(pluginId: string): Promise<void> {
    const state = this._getOrMakeState(pluginId)
    if (state.active) return

    const manifest = this.manifests.get(pluginId)
    // 权限与 Capability 检查（防御性，不构成安全边界）必须先于激活状态写入。
    if (manifest) {
      const requiredPermissions = manifest.permissions?.required ?? []
      if (requiredPermissions.length > 0) {
        const permissionStore = usePermissionStore()
        if (!permissionStore.hasAllPermissions(requiredPermissions)) {
          throw new Error(
            `Plugin "${pluginId}" missing required permissions: ${requiredPermissions.join(', ')}`,
          )
        }
      }
      const requiredCapabilities = manifest.capabilities?.required ?? []
      if (requiredCapabilities.length > 0) {
        const ok = await createCapabilityManager().checkRequired(requiredCapabilities)
        if (!ok) {
          throw new Error(
            `Plugin "${pluginId}" missing required capabilities: ${requiredCapabilities.join(', ')}`,
          )
        }
      }
    }

    const instance = this.loader.getPlugin(pluginId)
    if (!instance) throw new Error(`Plugin "${pluginId}" is not loaded`)
    this._setState(pluginId, { activating: true })
    try {
      if (instance.onActivate) {
        await this._withTimeout(
          instance.onActivate(),
          LIFECYCLE_TIMEOUT_MS,
          `onActivate(${pluginId})`,
        )
      }
      this._setState(pluginId, { active: true, inactive: false })
      this.activePlugins.add(pluginId)
      this.store.activate(pluginId)
      console.log('[PluginManager] plugin activated:', pluginId)
      this.eventBus.emit('plugin:activated', { name: pluginId })
    } finally {
      this._setState(pluginId, { activating: false })
    }
  }

  async deactivatePlugin(pluginId: string): Promise<void> {
    const state = this._getOrMakeState(pluginId)
    if (!state.active) {
      this.unloadPlugin(pluginId)
      return
    }

    this._setState(pluginId, { activating: false, inactive: true })

    try {
      const instance = this.loader.getPlugin(pluginId)
      if (instance?.onDeactivate) {
        await this._withTimeout(
          instance.onDeactivate(),
          LIFECYCLE_TIMEOUT_MS,
          `onDeactivate(${pluginId})`,
        )
      }
      this.unloadPlugin(pluginId)
      this.eventBus.emit('plugin:deactivated', { name: pluginId })
    } catch (error) {
      console.warn('[PluginManager] onDeactivate failed for', pluginId, error)
      this.unloadPlugin(pluginId)
      throw error
    }
  }

  /**
   * 上下文变化通知（V3.6 §4.4 onContextChange）：同一租户内切换
   * space/environment 时插件保持激活，需要收到新上下文。
   * 单个插件失败或超时不影响其他插件。
   */
  async notifyContextChange(ctx: import('@hnb/types').HNBContext): Promise<void> {
    const tasks = Array.from(this.activePlugins).map(async (pluginId) => {
      const instance = this.loader.getPlugin(pluginId)
      if (!instance?.onContextChange) return
      try {
        await this._withTimeout(
          instance.onContextChange(ctx),
          LIFECYCLE_TIMEOUT_MS,
          `onContextChange(${pluginId})`,
        )
      } catch (error) {
        console.warn('[PluginManager] onContextChange failed for', pluginId, error)
        this.eventBus.emit('plugin:error', { name: pluginId, error })
      }
    })
    await Promise.allSettled(tasks)
  }

  private unloadPlugin(pluginId: string): void {
    this.activePlugins.delete(pluginId)
    // 触发 abortSignal，取消插件进行中的请求与订阅
    this.abortControllers.get(pluginId)?.abort()
    this.abortControllers.delete(pluginId)
    // 回收该插件在扩展点上的全部贡献
    getExtensionRegistry().removeByPlugin(pluginId)
    getPluginRegistry().unregister(pluginId)
    this.store.deactivate(pluginId)
    this._setState(pluginId, { unloaded: true, loaded: false, active: false })
  }

  async activateRequiredPlugins(
    manifests: PluginManifest[],
  ): Promise<PluginInstance[]> {
    const failed: Record<string, Error> = {}

    for (const manifest of manifests) {
      try {
        await this.loadPlugin(manifest)
        await this.activatePlugin(manifest.name)
      } catch (error) {
        failed[manifest.name] = error as Error
        console.error('[PluginManager] activation failed for', manifest.name, error)
      }
    }

    if (Object.keys(failed).length > 0) {
      console.warn('[PluginManager] some plugins failed to activate:', failed)
      this.eventBus.emit('plugins:partial-load-failure', { failed: Object.keys(failed) })
    }

    return Array.from(this.activePlugins)
      .map((pid) => this.loader.getPlugin(pid))
      .filter((p): p is PluginInstance => !!p)
  }

  getActivatedPlugins(): PluginInstance[] {
    return Array.from(this.activePlugins)
      .map((pid) => this.loader.getPlugin(pid))
      .filter((p): p is PluginInstance => !!p)
  }

  getAllManifests(): readonly PluginManifest[] {
    return getPluginRegistry().getAllPlugins()
  }

  reset(): void {
    // Best-effort deactivation without awaiting — clearing state is the priority
    for (const pid of Array.from(this.activePlugins)) {
      this.deactivatePlugin(pid).catch(() => undefined)
    }
    // 兜底：停用流程之外仍持有 controller 的插件也一并中止
    for (const controller of this.abortControllers.values()) {
      controller.abort()
    }
    this.abortControllers.clear()
    this.loader.clear()
    this.store.clear()
    getPluginRegistry().clear()
    getExtensionRegistry().clear()
    this.activePlugins.clear()
    this.pluginStates.clear()
    this.eventBus.emit('plugin:reset')
  }

  getState(pluginId: string): PluginLifecycleState {
    return this._getOrMakeState(pluginId)
  }

  private _getOrMakeState(pluginId: string): PluginLifecycleState {
    if (!this.pluginStates.has(pluginId)) {
      this.pluginStates.set(pluginId, this._initialState())
    }
    return this.pluginStates.get(pluginId)!
  }

  private _initialState(): PluginLifecycleState {
    return {
      discovered: false,
      loading: false,
      loaded: false,
      activating: false,
      active: false,
      inactive: false,
      unloaded: false,
      error: null,
    }
  }

  private _setState(pluginId: string, partial: Partial<PluginLifecycleState>): void {
    Object.assign(this._getOrMakeState(pluginId), partial)
  }

  private async _withTimeout<T>(promise: Promise<T> | T, ms: number, label: string): Promise<T> {
    let timer: ReturnType<typeof setTimeout> | undefined
    try {
      return await Promise.race([
        promise,
        new Promise<T>((_, reject) => {
          timer = setTimeout(
            () => reject(new Error(`${label} timed out after ${ms}ms`)),
            ms,
          )
        }),
      ])
    } finally {
      if (timer) clearTimeout(timer)
    }
  }

  private _createPluginContext(pluginId: string): import('@hnb/types').PluginContext {
    // 每个插件独立的 AbortController：unload/reset 时触发，
    // apiClient 默认信号与插件的 fetch 都会随之取消。
    const controller = new AbortController()
    this.abortControllers.set(pluginId, controller)

    const authStore = useAuthStore()
    const contextStore = useContextStore()
    return {
      auth: authStore,
      context: contextStore,
      permission: usePermissionStore(),
      eventBus: this.eventBus,
      apiClient: createApiClient({
        getToken: () => authStore.token,
        refreshToken: () => authStore.refreshTokenAction(),
        beforeRequest: () => authStore.ensureFreshToken(30),
        onUnauthorized: () => { authStore.sessionExpired = true },
        getContext: () => contextStore.current,
        signal: controller.signal,
      }),
      capability: createCapabilityManager(),
      extensions: getExtensionRegistry(),
      i18n: createPluginI18n(pluginId),
      abortSignal: controller.signal,
      navigate: (path: string) => getRouterManager().getRouter().push(path),
    }
  }
}

let _manager: PluginManager | null = null

export function getPluginManager(loader?: PluginLoader): PluginManager {
  if (!_manager) {
    _manager = new PluginManager(loader)
  }
  return _manager
}

export default getPluginManager
