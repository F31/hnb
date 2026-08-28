/**
 * Shell i18n 基础设施（vue-i18n Composition 模式）。
 *
 * 语言优先级：localStorage('hnb.locale') > zh-CN。
 * 插件如需多语言，可通过 plugin-sdk 的 useShellI18n 访问同一实例，
 * 并以命名空间（插件 id）注册自己的语言包，避免键冲突。
 */

import { createI18n } from 'vue-i18n'
import type { PluginI18n, PluginMessages } from '@hnb/types'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'

export const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]
export const DEFAULT_LOCALE: SupportedLocale = 'zh-CN'
const LOCALE_STORAGE_KEY = 'hnb.locale'

function detectLocale(): SupportedLocale {
  const stored =
    typeof localStorage !== 'undefined' ? localStorage.getItem(LOCALE_STORAGE_KEY) : null
  if (stored === 'zh-CN' || stored === 'en-US') return stored
  return DEFAULT_LOCALE
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})

/** 切换语言并持久化选择。 */
export function setLocale(locale: SupportedLocale): void {
  i18n.global.locale.value = locale
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  }
}

export function getLocale(): SupportedLocale {
  return i18n.global.locale.value as SupportedLocale
}

/**
 * 为插件注册命名空间语言包：mergeLocaleMessage('<pluginId>', messages)。
 * 插件内通过 $t('<pluginId>.xxx') 访问，键空间天然隔离。
 */
export function registerPluginMessages(
  pluginId: string,
  messages: { 'zh-CN'?: Record<string, unknown>; 'en-US'?: Record<string, unknown> },
): void {
  for (const locale of SUPPORTED_LOCALES) {
    const pack = messages[locale]
    if (pack) {
      i18n.global.mergeLocaleMessage(locale, { [pluginId]: pack })
    }
  }
}

/**
 * 插件上下文 i18n 适配器（V3.6 能力经 PluginContext 下发）：
 * 语言包注册与 t() 均以插件 id 为命名空间，插件无需感知全局键结构。
 * 组件模板中可直接使用 $t('<pluginId>.key')（共享全局 vue-i18n 实例）。
 */
export function createPluginI18n(pluginId: string): PluginI18n {
  return {
    get locale() {
      return i18n.global.locale.value
    },
    t(key: string): string {
      return i18n.global.t(`${pluginId}.${key}`)
    },
    registerMessages(messages: PluginMessages): void {
      registerPluginMessages(pluginId, messages)
    },
  }
}

export default i18n
