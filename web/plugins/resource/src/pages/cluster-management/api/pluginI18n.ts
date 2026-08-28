/**
 * 插件 i18n 单例：供非 Vue 模块（service adapter / utils）获取翻译函数。
 * 由 plugin create(ctx) 注入；未注入时返回 key 本身（测试/兜底，不抛错）。
 *
 * 组件内 useI18n().t 使用带 `resource.` 前缀的完整键；而 ctx.i18n.t 会自动
 * 追加 `resource.` 前缀，因此这里按 pluginId 剥离前缀后转发。
 */
export type PluginT = (key: string, params?: Record<string, unknown>) => string

let t: PluginT | null = null

export function setPluginI18nT(pluginId: string, fn: PluginT): void {
  t = (key: string, params?: Record<string, unknown>) => {
    const stripped = key.startsWith(`${pluginId}.`) ? key.slice(pluginId.length + 1) : key
    return fn(stripped, params)
  }
}

/** 翻译（未初始化时返回 key，避免测试与极端路径抛错） */
export function pluginT(key: string, params?: Record<string, unknown>): string {
  return t ? t(key, params) : key
}
