/**
 * @hnb/plugin-sdk — HNB 插件开发 SDK
 *
 * 为插件作者提供统一的入口定义工具与类型再导出，
 * 避免各插件自行约定入口对象形状。
 */

import type { HNBPlugin, PluginInstance, PluginMessages } from '@hnb/types'

/**
 * 定义插件入口。当前为带完整类型检查的恒等函数，
 * 后续 SDK 能力（注册辅助、默认生命周期）在此收口。
 */
export function definePlugin(plugin: HNBPlugin): HNBPlugin {
  return plugin
}

/**
 * 定义插件实例，约束生命周期钩子签名。
 */
export function definePluginInstance(instance: PluginInstance): PluginInstance {
  return instance
}

/**
 * 定义插件语言包（带类型检查的恒等函数）。
 * 键不带插件前缀；在 create(ctx) 中经 ctx.i18n.registerMessages 注册后，
 * 模板中以 $t('<pluginId>.<key>') 访问。
 */
export function definePluginMessages(messages: PluginMessages): PluginMessages {
  return messages
}

/**
 * 生成带插件命名空间前缀的日志器，便于在控制台过滤插件日志。
 */
export function createPluginLogger(pluginId: string) {
  const prefix = `[hnb-plugin:${pluginId}]`
  return {
    debug: (...args: unknown[]) => console.debug(prefix, ...args),
    info: (...args: unknown[]) => console.info(prefix, ...args),
    warn: (...args: unknown[]) => console.warn(prefix, ...args),
    error: (...args: unknown[]) => console.error(prefix, ...args),
  }
}

export type * from '@hnb/types'
