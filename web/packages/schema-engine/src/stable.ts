/**
 * 稳定序列化工具（V2.6 §8.2 / §13.6.3）。
 *
 * 递归稳定序列化任意值：对象键排序、数组有序、原始值 JSON 编码。
 * 与 `JSON.stringify(value, replacerArray)` 不同，嵌套对象的键不会被
 * 数组 replacer 过滤掉，避免不同嵌套结构碰撞出相同缓存键。
 *
 * 用于：
 *  - DataSourceManager 响应缓存 / in-flight 去重键（§13.6.3）
 *  - ComponentRegistry validateProps 校验缓存键（§8.2）
 */

export function stableJSON(value: unknown): string {
  if (value === undefined) return ''
  if (value === null) return 'null'
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(',')}]`
  if (typeof value === 'object') {
    const entries = Object.keys(value as Record<string, unknown>)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${stableJSON((value as Record<string, unknown>)[k])}`)
    return `{${entries.join(',')}}`
  }
  return JSON.stringify(value)
}
