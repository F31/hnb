/**
 * ExtensionRegistry — 声明式扩展点注册与校验（V2.5 §9 扩展机制）。
 *
 * 扩展点按受控命名空间注册（如 `resource.cluster.detail.tabs`）。每次注册
 * 校验：命名空间合法、组件类型已在 ComponentRegistry、声明的最小 Shell
 * 版本兼容、绑定权限非任意通配符。渲染层只渲染已注册且通过校验的扩展，
 * 未知/越权/版本不兼容的扩展一律 fail-closed。
 */

import { compareVersion, SHELL_VERSION } from './SchemaEngine'
import type { ComponentRegistry } from './ComponentRegistry'

const NAMESPACE_PATTERN = /^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)*$/

export interface ExtensionPointDefinition {
  /** 受控命名空间，如 `resource.cluster.detail.tabs` */
  namespace: string
  componentType: string
  /** 显示优先级（升序渲染） */
  order?: number
  /** 该扩展需要的权限，空表示无额外权限（渲染时仍按调用方上下文判断） */
  permission?: string
  /** 该扩展组件要求的最小 Shell 版本 */
  minShellVersion?: string
  props?: Record<string, unknown>
  pluginId?: string
}

export interface ExtensionValidationResult {
  valid: boolean
  errors: string[]
}

export class ExtensionRegistry {
  private extensions = new Map<string, ExtensionPointDefinition>()

  constructor(private registry: ComponentRegistry) {}

  register(def: ExtensionPointDefinition): ExtensionValidationResult {
    const errors: string[] = []
    if (!def?.namespace || !NAMESPACE_PATTERN.test(def.namespace)) {
      errors.push(`invalid extension namespace "${def?.namespace}"`)
    }
    if (!def.componentType) {
      errors.push('componentType is required')
    } else if (!this.registry.has(def.componentType)) {
      errors.push(`componentType "${def.componentType}" is not registered`)
    }
    if (def.permission && (def.permission === '*' || def.permission.trim() === '')) {
      errors.push('extension permission must not be a wildcard or empty')
    }
    if (def.minShellVersion && compareVersion(def.minShellVersion, SHELL_VERSION) > 0) {
      errors.push(`extension requires Shell >= ${def.minShellVersion}, current ${SHELL_VERSION}`)
    }
    if (errors.length > 0) {
      return { valid: false, errors }
    }
    this.extensions.set(def.namespace, { ...def, order: def.order ?? 0 })
    return { valid: true, errors: [] }
  }

  unregister(namespace: string): void {
    this.extensions.delete(namespace)
  }

  unregisterPlugin(pluginId: string): void {
    for (const [ns, def] of Array.from(this.extensions.entries())) {
      if (def.pluginId === pluginId) this.extensions.delete(ns)
    }
  }

  has(namespace: string): boolean {
    return this.extensions.has(namespace)
  }

  /** 返回按 order 排序且调用方拥有权限的全部扩展定义 */
  list(namespace: string, permissions?: readonly string[]): ExtensionPointDefinition[] {
    const def = this.extensions.get(namespace)
    if (!def) return []
    if (def.permission && !hasPermission(permissions, def.permission)) return []
    return [def]
  }

  /** 列出命名空间前缀下的全部已注册扩展（如 `resource.cluster.detail.*`） */
  listPrefix(prefix: string, permissions?: readonly string[]): ExtensionPointDefinition[] {
    return Array.from(this.extensions.values())
      .filter((def) => def.namespace === prefix || def.namespace.startsWith(prefix + '.'))
      .filter((def) => !def.permission || hasPermission(permissions, def.permission))
      .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
  }

  clear(): void {
    this.extensions.clear()
  }
}

function hasPermission(permissions: readonly string[] | undefined, permission: string): boolean {
  if (!permissions) return false
  return permissions.includes(permission) || permissions.includes('*')
}

export function createExtensionRegistry(registry: ComponentRegistry): ExtensionRegistry {
  return new ExtensionRegistry(registry)
}
