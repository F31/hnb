import type { Component } from 'vue'
import type { HNBContext } from './context'
import type { MenuItem, RouteConfig } from './manifest'

export interface HNBPlugin {
  name: string
  version: string
  displayName: string
  tier: 'T0' | 'T1' | 'T2'
  enabled: boolean
  mode: 'local' | 'remote'

  /**
   * componentKey → Vue 组件的可信映射。插件入口必须导出该属性，
   * 既保证组件代码不被摇树优化掉，也为 PluginRegistry 提供解析来源。
   */
  components: Record<string, Component>

  create(ctx: PluginContext): Promise<PluginInstance>
}

export interface PluginInstance {
  name?: string
  routes?: RouteConfig[]
  menuItems?: MenuItem[]
  onActivate?(): Promise<void>
  onDeactivate?(): Promise<void>
  onContextChange?(ctx: HNBContext): Promise<void>
}

export interface PluginContext {
  auth: AuthStore
  context: ContextStore
  permission: PermissionStore
  eventBus: EventBus
  apiClient: ApiClient
  capability: CapabilityManager
  /** 扩展点注册中心：插件可向 Shell/其他插件声明的扩展点贡献内容 */
  extensions: ExtensionRegistry
  /** i18n 适配器：语言包按插件 id 命名空间隔离，t() 自动加前缀 */
  i18n: PluginI18n
  /**
   * 插件生命周期中止信号：deactivate/unload/租户切换时触发，
   * 插件应以此取消进行中的请求、订阅与定时器。
   */
  abortSignal: AbortSignal
  /** 路由导航：插件通过此函数进行 SPA 内跳转，避免全页刷新 */
  navigate: (path: string) => void
}

/** 插件语言包：键不带插件前缀，注册时由 Shell 统一加命名空间 */
export interface PluginMessages {
  'zh-CN'?: Record<string, unknown>
  'en-US'?: Record<string, unknown>
}

export interface PluginI18n {
  /** 当前语言（如 'zh-CN'） */
  readonly locale: string
  /** 翻译插件命名空间下的 key（自动加 '<pluginId>.' 前缀），缺失时回退 zh-CN/原文 */
  t(key: string): string
  /** 注册插件语言包（键空间为插件 id，与其他插件天然隔离） */
  registerMessages(messages: PluginMessages): void
}

/**
 * 首页仪表盘 Widget 合约（扩展点 'dashboard.widgets' 的贡献类型）。
 * 插件通过 ctx.extensions.contribute('dashboard.widgets', payload) 添加。
 */
export interface DashboardWidget {
  /** 标题（如 "集群数量"） */
  title: string
  /** 数值 */
  value: string | number
  /** 单位 */
  unit?: string
  /** 排序权重，大的在前 */
  priority?: number
}

export interface ExtensionContribution<T = unknown> {
  /** 贡献来源插件 id，卸载时据此回收 */
  pluginId: string
  payload: T
  /** 排序权重，大的在前；缺省 0 */
  priority?: number
}

export interface ExtensionRegistry {
  registerPoint(name: string, owner?: string): void
  ensurePoint(name: string, owner?: string): void
  hasPoint(name: string): boolean
  contribute<T>(point: string, contribution: ExtensionContribution<T>): void
  getContributions<T>(point: string): ExtensionContribution<T>[]
  removeByPlugin(pluginId: string): void
}

export interface AuthStore {
  readonly isAuthenticated: boolean
  readonly token: string | null
  readonly refreshToken: string | null
  readonly user: User | null
  login(username: string, password: string): Promise<void>
  logout(): Promise<void>
  refreshTokenAction(): Promise<void>
  restoreSession(): void
  setPermissions(permissions: string[]): void
}

export interface User {
  id: string
  username: string
  displayName: string
  email?: string
  roles: string[]
  permissions: string[]
}

export interface ContextStore {
  readonly current: HNBContext
  setSpace(spaceId: string, gen?: number): Promise<void>
  setFullContext(ctx: HNBContext, gen?: number): void
  reset(): void
  matches(ctx: HNBContext): boolean
}

export interface PermissionStore {
  hasPermission(permission: string): boolean
  hasAllPermissions(permissions: string[]): boolean
  hasAnyPermission(permissions: string[]): boolean
}

export interface EventBus {
  on(event: string, handler: (...args: any[]) => void): void
  off(event: string, handler: (...args: any[]) => void): void
  emit(event: string, ...args: any[]): void
}

export interface ApiClient {
	requestRaw?(method: string, url: string, data?: unknown, config?: any): Promise<Response>
  get<T>(url: string, config?: any): Promise<T>
  post<T>(url: string, data?: any, config?: any): Promise<T>
  put<T>(url: string, data?: any, config?: any): Promise<T>
  delete<T>(url: string, config?: any): Promise<T>
  patch<T>(url: string, data?: any, config?: any): Promise<T>
}

export interface CapabilityManager {
  hasCapability(name: string): Promise<boolean>
  hasAllCapabilities(names: string[]): Promise<boolean>
  checkRequired(required: string[]): Promise<boolean>
  refresh(): Promise<void>
}
