/**
 * ComponentRegistry — 受信组件注册表（V2.5 §8）。
 *
 * 服务端 Schema 只能引用此处注册的 componentType；未注册类型由渲染层
 * 显示安全错误占位符（区块级隔离），不得动态执行任何下发代码。
 *
 * props 校验实现 JSON Schema 子集：type / required / properties / enum /
 * additionalProperties:false。完整 JSON Schema 能力可在后续版本替换为
 * 标准校验器，接口保持不变。
 */

import type { Component } from 'vue'
import { stableJSON } from './stable'

export interface JsonSchemaSubset {
  type?: 'object' | 'string' | 'number' | 'boolean' | 'array'
  required?: string[]
  properties?: Record<string, JsonSchemaSubset>
  enum?: unknown[]
  additionalProperties?: boolean
}

export interface ComponentDefinition {
  type: string
  component: Component
  version?: string
  pluginId?: string
  propsSchema?: JsonSchemaSubset
  minShellVersion?: string
}

export interface PropsValidationResult {
  valid: boolean
  errors: string[]
}

/** props 校验缓存条目上限：超出后按插入顺序淘汰最旧（近似 LRU） */
const MAX_PROPS_VALIDATION_CACHE = 500

function setBounded<K, V>(map: Map<K, V>, key: K, value: V, max: number): void {
  if (map.has(key)) map.delete(key)
  map.set(key, value)
  while (map.size > max) {
    const oldest = map.keys().next().value
    if (oldest === undefined) break
    map.delete(oldest)
  }
}

function validateValue(
  schema: JsonSchemaSubset,
  value: unknown,
  path: string,
  errors: string[],
): void {
  if (value === undefined || value === null) return
  if (schema.enum && !schema.enum.includes(value)) {
    errors.push(`${path}: value must be one of ${JSON.stringify(schema.enum)}`)
    return
  }
  switch (schema.type) {
    case 'string':
      if (typeof value !== 'string') errors.push(`${path}: expected string`)
      break
    case 'number':
      if (typeof value !== 'number') errors.push(`${path}: expected number`)
      break
    case 'boolean':
      if (typeof value !== 'boolean') errors.push(`${path}: expected boolean`)
      break
    case 'array':
      if (!Array.isArray(value)) errors.push(`${path}: expected array`)
      break
    case 'object':
      if (typeof value !== 'object' || Array.isArray(value)) {
        errors.push(`${path}: expected object`)
        return
      }
      validateObject(schema, value as Record<string, unknown>, path, errors)
      break
  }
}

function validateObject(
  schema: JsonSchemaSubset,
  props: Record<string, unknown>,
  path: string,
  errors: string[],
): void {
  for (const key of schema.required ?? []) {
    if (props[key] === undefined || props[key] === null) {
      errors.push(`${path}${key}: required property missing`)
    }
  }
  if (schema.additionalProperties === false && schema.properties) {
    for (const key of Object.keys(props)) {
      if (!(key in schema.properties)) {
        errors.push(`${path}${key}: unknown property not allowed`)
      }
    }
  }
  for (const [key, sub] of Object.entries(schema.properties ?? {})) {
    validateValue(sub, props[key], `${path}${key}.`, errors)
  }
}

export class ComponentRegistry {
  private definitions = new Map<string, ComponentDefinition>()
  private propsValidationCache = new Map<string, PropsValidationResult>()

  register(def: ComponentDefinition): void {
    if (!def?.type || !def.component) {
      throw new Error('ComponentDefinition requires type and component')
    }
    this.definitions.set(def.type, def)
    this.propsValidationCache.clear()
  }

  unregister(type: string): void {
    this.definitions.delete(type)
    this.propsValidationCache.clear()
  }

  /** 注销某插件注册的全部组件（插件停用时调用，V2.5 §8.3） */
  unregisterPlugin(pluginId: string): void {
    for (const [type, def] of Array.from(this.definitions.entries())) {
      if (def.pluginId === pluginId) this.definitions.delete(type)
    }
    this.propsValidationCache.clear()
  }

  has(type: string): boolean {
    return this.definitions.has(type)
  }

  resolve(type: string): Component | null {
    return this.definitions.get(type)?.component ?? null
  }

  getDefinition(type: string): ComponentDefinition | undefined {
    return this.definitions.get(type)
  }

  validateProps(type: string, props: Record<string, unknown>): PropsValidationResult {
    const def = this.definitions.get(type)
    if (!def?.propsSchema) return { valid: true, errors: [] }
    // V2.6 §8.2：缓存键使用稳定序列化（嵌套键不丢失），
    // 相同 props 跳过递归校验；与 DataSourceManager 共用 stableJSON。
    const key = `${type}::${stableJSON(props)}`
    const cached = this.propsValidationCache.get(key)
    if (cached) return cached
    const errors: string[] = []
    validateValue({ ...def.propsSchema, type: def.propsSchema.type ?? 'object' }, props, '', errors)
    const result: PropsValidationResult = { valid: errors.length === 0, errors }
    setBounded(this.propsValidationCache, key, result, MAX_PROPS_VALIDATION_CACHE)
    return result
  }

  clear(): void {
    this.definitions.clear()
    this.propsValidationCache.clear()
  }
}

export function createComponentRegistry(): ComponentRegistry {
  return new ComponentRegistry()
}
