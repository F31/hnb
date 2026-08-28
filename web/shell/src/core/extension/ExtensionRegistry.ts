/**
 * ExtensionRegistry — 扩展点注册中心（V3.6 §5.3 扩展点机制）
 *
 * Shell 或插件通过 registerPoint 声明扩展点（如 'dashboard.widgets'），
 * 插件通过 contribute 向扩展点贡献内容。插件卸载时其全部贡献被回收，
 * 贡献变更通过事件总线广播，消费方可据此刷新 UI。
 */

import type {
  ExtensionContribution,
  ExtensionRegistry as ExtensionRegistryApi,
} from '@hnb/types'
import { getEventBus } from '../event-bus'

interface ExtensionPoint {
  name: string
  /** 声明方：'shell' 或插件 id，仅用于诊断展示 */
  owner: string
}

export class ExtensionRegistry implements ExtensionRegistryApi {
  private points = new Map<string, ExtensionPoint>()
  private contributions = new Map<string, ExtensionContribution[]>()
  private eventBus = getEventBus()

  /** 声明扩展点。重复声明同名扩展点属于编程错误，直接抛错。 */
  registerPoint(name: string, owner = 'shell'): void {
    if (this.points.has(name)) {
      throw new Error(`[ExtensionRegistry] extension point already registered: ${name}`)
    }
    this.points.set(name, { name, owner })
  }

  /** 幂等的扩展点声明：已存在时直接返回，供插件在 onActivate 中安全调用。 */
  ensurePoint(name: string, owner = 'shell'): void {
    if (!this.points.has(name)) {
      this.points.set(name, { name, owner })
    }
  }

  hasPoint(name: string): boolean {
    return this.points.has(name)
  }

  /** 向扩展点贡献内容。目标扩展点必须已声明，防止拼写错误的"幽灵贡献"。 */
  contribute<T>(point: string, contribution: ExtensionContribution<T>): void {
    if (!this.points.has(point)) {
      throw new Error(`[ExtensionRegistry] unknown extension point: ${point}`)
    }
    const list = this.contributions.get(point) ?? []
    list.push(contribution as ExtensionContribution)
    this.contributions.set(point, list)
    this.eventBus.emit('extension:changed', { point, pluginId: contribution.pluginId })
  }

  /** 读取扩展点贡献：priority 大的在前，同优先级按注册顺序。 */
  getContributions<T>(point: string): ExtensionContribution<T>[] {
    const list = this.contributions.get(point) ?? []
    return [...list].sort(
      (a, b) => (b.priority ?? 0) - (a.priority ?? 0),
    ) as ExtensionContribution<T>[]
  }

  /** 回收某插件的全部贡献（插件 deactivate/unload 时调用）。 */
  removeByPlugin(pluginId: string): void {
    for (const [point, list] of this.contributions) {
      const remaining = list.filter((c) => c.pluginId !== pluginId)
      if (remaining.length !== list.length) {
        if (remaining.length === 0) {
          this.contributions.delete(point)
        } else {
          this.contributions.set(point, remaining)
        }
        this.eventBus.emit('extension:changed', { point, pluginId })
      }
    }
  }

  clear(): void {
    this.points.clear()
    this.contributions.clear()
  }
}

let _registry: ExtensionRegistry | null = null

export function getExtensionRegistry(): ExtensionRegistry {
  if (!_registry) {
    _registry = new ExtensionRegistry()
  }
  return _registry
}

export default getExtensionRegistry
