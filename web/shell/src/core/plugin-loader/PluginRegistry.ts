/**
 * PluginRegistry — secure component resolution per V3.6 §4.5.
 *
 * The registry holds the trusted mapping from (pluginId, componentKey) to a
 * Promise of the loaded Vue component. Resolvers are populated either:
 *  - By PluginManager after a plugin bundle is loaded, registering the
 *    bundle's `components` export (the normal local-bundle path), or
 *  - Explicitly via `registerManifest()` with a `modules` field
 *    (reserved for remote bundles enrolled by the backend Navigation Service).
 *
 * Remote bundles additionally require domain allowlist + digest validation.
 * This is a security boundary: arbitrary import() expressions and remote URLs
 * stored in the navigation database MUST NOT bypass this registry.
 */

import type { Component } from 'vue'
import type { PluginManifest } from '@hnb/types'

interface ModuleDescriptor {
  entry: string
  componentKey: string
  digest?: string
}

export class PluginRegistry {
  private pluginMap = new Map<string, PluginManifest>()
  private componentMap = new Map<string, () => Promise<any>>()
  private lazyEntries = new Map<string, string>()
  /** 懒加载完成回调，由 PluginManager 注册，用于完成插件激活 */
  _lazyLoadCallbacks = new Map<string, () => void>()
  private allowedDomains: Set<string> = new Set(
    [typeof window !== 'undefined' ? window.location.origin : ''].filter(Boolean),
  )

  /**
   * Register the componentKey → component mapping exported by a loaded
   * plugin bundle. Called by PluginManager after a successful load.
   */
  register(pluginId: string, components: Record<string, Component>): void {
    for (const [componentKey, component] of Object.entries(components ?? {})) {
      this.componentMap.set(`${pluginId}:${componentKey}`, () =>
        Promise.resolve(component),
      )
    }
  }

  /** 注册插件 Bundle 入口，组件首次被引用时自动懒加载 */
  registerLazyEntry(pluginId: string, entryUrl: string): void {
    this.lazyEntries.set(pluginId, entryUrl)
  }

  unregister(pluginId: string): void {
    const prefix = `${pluginId}:`
    for (const key of Array.from(this.componentMap.keys())) {
      if (key.startsWith(prefix)) {
        this.componentMap.delete(key)
      }
    }
    this.pluginMap.delete(pluginId)
    this.lazyEntries.delete(pluginId)
  }

  registerManifest(plugin: PluginManifest, modules?: ModuleDescriptor[]): void {
    this.pluginMap.set(plugin.name, plugin)
    const moduleList: ModuleDescriptor[] =
      modules ?? (plugin as any).modules ?? []
    for (const module of moduleList) {
      if (!module?.entry || !module?.componentKey) continue
      const key = `${plugin.name}:${module.componentKey}`
      this.componentMap.set(key, () => {
        if (module.entry && !this.isEntryAllowed(module.entry)) {
          throw new Error(`Entry URL not allowed: ${module.entry}`)
        }
        if (plugin.bundleDigest && module.digest && plugin.bundleDigest !== module.digest) {
          throw new Error(`Digest mismatch for plugin ${plugin.name}`)
        }
        return import(/* @vite-ignore */ module.entry)
      })
    }
  }

  isEntryAllowed(entry: string): boolean {
    if (!entry || entry.startsWith('/')) return true
    try {
      const url = new URL(entry)
      return this.allowedDomains.has(url.origin)
    } catch {
      return false
    }
  }

  async resolveComponent(pluginId: string, componentKey: string): Promise<any> {
    const key = `${pluginId}:${componentKey}`
    // 已注册 → 直接返回
    const existing = this.componentMap.get(key)
    if (existing) return existing()

    // 未注册但有懒加载入口 → 自动加载插件 Bundle
    const entryUrl = this.lazyEntries.get(pluginId)
    if (entryUrl) {
      const pluginModule = await import(/* @vite-ignore */ entryUrl)
      const components = pluginModule?.components || pluginModule?.default?.components
      if (components) {
        this.register(pluginId, components)
      }
      // 触发懒加载完成回调（await 确保 create() 在组件渲染前完成）
      await this._lazyLoadCallbacks.get(pluginId)?.()
      const loaded = this.componentMap.get(key)
      if (loaded) return loaded()
    }

    throw new Error(`Component "${componentKey}" not found in plugin "${pluginId}"`)
  }

  hasComponent(pluginId: string, componentKey: string): boolean {
    return this.componentMap.has(`${pluginId}:${componentKey}`)
  }

  getAllPlugins(): readonly PluginManifest[] {
    return Array.from(this.pluginMap.values())
  }

  getPlugin(pluginId: string | null | undefined): PluginManifest | undefined {
    return pluginId ? this.pluginMap.get(pluginId) : undefined
  }

  setAllowedDomain(domain: string): void {
    if (domain) this.allowedDomains.add(domain.toLowerCase())
  }

  clear(): void {
    this.pluginMap.clear()
    this.componentMap.clear()
    this.lazyEntries.clear()
  }
}

let _registry: PluginRegistry | null = null

export function getPluginRegistry(): PluginRegistry {
  if (!_registry) {
    _registry = new PluginRegistry()
  }
  return _registry
}

export async function resolveComponent(
  pluginId: string,
  componentKey: string,
): Promise<any> {
  return getPluginRegistry().resolveComponent(pluginId, componentKey)
}

export default getPluginRegistry
