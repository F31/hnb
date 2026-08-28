/**
 * PluginLoader — performs module resolution and route registration.
 *
 * V3.6 conformance:
 *  - Menus are loaded ONLY from `/api/v1/navigation/menus` (handled by NavigationManager).
 *  - Routes are loaded ONLY via PluginRegistry.resolveComponent() — never arbitrary import().
 *  - Remote bundles require whitelist + digest + signature validation (TODO Phase B).
 */

import type {
  HNBPlugin,
  PluginManifest,
  PluginInstance,
  PluginContext,
  RouteConfig,
} from '@hnb/types'
import type { Router } from 'vue-router'
import { getEventBus } from '@/core/event-bus'
import { getPluginRegistry } from '@/core/plugin-loader/PluginRegistry'

async function sha256Hex(text: string): Promise<string> {
  const data = new TextEncoder().encode(text)
  const digest = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

/**
 * 拉取远程 Bundle 源码并校验 SHA-256 Digest（V3.6 §4.2），
 * 校验通过后经 Blob URL 动态导入，避免直接执行未校验的远程 URL。
 */
async function importRemoteBundle(entry: string, expectedDigest: string): Promise<any> {
  const res = await fetch(entry)
  if (!res.ok) {
    throw new Error(`Failed to fetch remote bundle: ${res.status}`)
  }
  const code = await res.text()
  const actual = `sha256:${await sha256Hex(code)}`
  if (actual !== expectedDigest) {
    throw new Error(
      `Remote bundle digest mismatch: expected ${expectedDigest}, got ${actual}`,
    )
  }
  const blobUrl = URL.createObjectURL(new Blob([code], { type: 'text/javascript' }))
  try {
    return await import(/* @vite-ignore */ blobUrl)
  } finally {
    URL.revokeObjectURL(blobUrl)
  }
}

export class PluginLoader {
  private plugins = new Map<string, PluginInstance>()
  private manifests = new Map<string, PluginManifest>()
  private modules = new Map<string, HNBPlugin>()
  private styleElements = new Map<string, HTMLLinkElement>()
  private router: Router | null = null

  /** 缓存插件实例（用于懒加载模式，先注册实例再按需加载 Bundle） */
  _cachePlugin(pluginId: string, instance: PluginInstance): void {
    this.plugins.set(pluginId, instance)
  }

  setRouter(router: Router): void {
    this.router = router
  }

  /**
   * Load a local plugin bundle. The bundle is fetched from
   * `/modules/<name>/index.js` which is served by nginx (see Dockerfile).
   */
  async loadLocalPlugin(
    manifest: PluginManifest,
    context: PluginContext,
  ): Promise<PluginInstance> {
    if (!manifest.enabled) {
      throw new Error(`Plugin "${manifest.name}" is disabled`)
    }

    this.manifests.set(manifest.name, manifest)

    try {
      this.attachLocalPluginStyle(manifest.name)
      const version = encodeURIComponent(manifest.version || 'dev')
      const url = `/modules/${manifest.name}/index.js?v=${version}&t=${Date.now()}`
      const mod: any = await import(/* @vite-ignore */ url)
      if (!mod?.default && !mod?.create) {
        throw new Error(`Plugin "${manifest.name}" missing default export or create()`)
      }
      const pluginModule: HNBPlugin = mod.default || mod
      const instance: PluginInstance = await pluginModule.create(context)
      if (!instance?.name) {
        instance.name = manifest.name
      }
      this.plugins.set(manifest.name, instance)
      this.modules.set(manifest.name, pluginModule)

      const routesToRegister =
        instance.routes?.length
          ? instance.routes
          : (manifest.routes || [])
      if (routesToRegister.length > 0) {
        this.registerRoutes(routesToRegister)
      }

      getEventBus().emit('plugin:loaded', { name: manifest.name })
      return instance
    } catch (e) {
      this.detachPluginStyle(manifest.name)
      console.error(`[PluginLoader] failed to load local plugin "${manifest.name}":`, e)
      getEventBus().emit('plugin:error', { name: manifest.name, error: e })
      throw e
    }
  }

  /**
   * Load a remote plugin bundle after whitelist/digest validation.
   * Remote bundles are disabled by default (V3.6 §13.4).
   * 签名验证（cosign/证书链）留待 Phase C 对接后端验签服务；
   * 当前强制 Digest 校验：bundleDigest 缺失的远程插件拒绝加载。
   */
  async loadRemotePlugin(
    manifest: PluginManifest,
    context: PluginContext,
  ): Promise<PluginInstance> {
    if (!manifest.entry) {
      throw new Error(`Remote plugin "${manifest.name}" missing entry`)
    }
    const registry = getPluginRegistry()
    if (!registry.isEntryAllowed(manifest.entry)) {
      throw new Error(`Remote entry not allowed: ${manifest.entry}`)
    }
    if (!manifest.bundleDigest) {
      throw new Error(`Remote plugin "${manifest.name}" missing bundleDigest`)
    }

    try {
      const mod: any = await importRemoteBundle(manifest.entry, manifest.bundleDigest)
      const pluginModule: HNBPlugin = mod.default || mod
      const instance: PluginInstance = await pluginModule.create(context)
      if (!instance?.name) instance.name = manifest.name
      this.plugins.set(manifest.name, instance)
      this.modules.set(manifest.name, pluginModule)

      const routesToRegister = instance.routes?.length ? instance.routes : (manifest.routes || [])
      if (routesToRegister.length > 0) {
        this.registerRoutes(routesToRegister)
      }
      getEventBus().emit('plugin:loaded', { name: manifest.name })
      return instance
    } catch (e) {
      console.error(`[PluginLoader] remote plugin "${manifest.name}" failed:`, e)
      getEventBus().emit('plugin:error', { name: manifest.name, error: e })
      throw e
    }
  }

  private registerRoutes(routes: RouteConfig[]): void {
    if (!this.router) return
    const registry = getPluginRegistry()
    for (const route of routes) {
      if (!route.path || !route.componentKey) {
        console.warn(
          '[PluginLoader] skipping route with missing path/componentKey:',
          route,
        )
        continue
      }
      const pluginId = route.pluginId || ''
      this.router.addRoute({
        path: route.path,
        name: route.path,
        component: () => registry.resolveComponent(
          pluginId,
          route.componentKey,
        ),
        meta: {
          pluginId,
          permissionCode: route.permissionCode,
        },
      })
    }
  }

  getPlugin(name: string): PluginInstance | undefined {
    return this.plugins.get(name)
  }

  getModule(name: string): HNBPlugin | undefined {
    return this.modules.get(name)
  }

  getAllPlugins(): PluginInstance[] {
    return Array.from(this.plugins.values())
  }

  private attachLocalPluginStyle(pluginId: string): void {
    // In development Vite injects styles from the plugin source module. Library
    // builds emit a separate index.css that dynamic import() does not load.
    if (import.meta.env.DEV || this.styleElements.has(pluginId)) return

    const link = document.createElement('link')
    link.rel = 'stylesheet'
    const version = encodeURIComponent(this.manifests.get(pluginId)?.version || 'dev')
    link.href = `/modules/${pluginId}/index.css?v=${version}&t=${Date.now()}`
    link.dataset.hnbPluginStyle = pluginId
    document.head.appendChild(link)
    this.styleElements.set(pluginId, link)
  }

  private detachPluginStyle(pluginId: string): void {
    this.styleElements.get(pluginId)?.remove()
    this.styleElements.delete(pluginId)
  }

  clear(): void {
    for (const pluginId of this.styleElements.keys()) {
      this.detachPluginStyle(pluginId)
    }
    this.plugins.clear()
    this.manifests.clear()
    this.modules.clear()
  }
}

export default PluginLoader
