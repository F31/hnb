/**
 * 集群管理组件测试辅助：构造 i18n（resource 命名空间）+ 注入
 * clusterApi 模块单例（apiClient / eventBus / permissionStore / contextStore）。
 */
import { createI18n } from 'vue-i18n'
import type { ApiClient, CapabilityManager, ContextStore, EventBus, PermissionStore } from '@hnb/types'
import pluginMessages from '../../../locales'

export function createTestI18n(locale: 'zh-CN' | 'en-US' = 'zh-CN') {
  const pack = (pluginMessages[locale] ?? pluginMessages['zh-CN']) as Record<string, unknown>
  return createI18n({
    legacy: false,
    locale,
    fallbackLocale: 'zh-CN',
    messages: { [locale]: { resource: pack } },
  })
}

export function stubApiClient(overrides: Partial<ApiClient> = {}): ApiClient {
  const client: ApiClient = {
    get: async <T,>(_url: string, _config?: unknown): Promise<T> => ({} as T),
    post: async <T,>(_url: string, _data?: unknown, _config?: unknown): Promise<T> => ({} as T),
    put: async <T,>(_url: string, _data?: unknown, _config?: unknown): Promise<T> => ({} as T),
    patch: async <T,>(_url: string, _data?: unknown, _config?: unknown): Promise<T> => ({} as T),
    delete: async <T,>(_url: string, _config?: unknown): Promise<T> => ({} as T),
    ...overrides,
  }
  return client
}

export function stubContextStore(tenantId = 'tenant-a', spaceId = 'space-a'): ContextStore {
  return {
    current: { tenantId, spaceId, environmentId: 'env-a', clusterId: '' },
  } as unknown as ContextStore
}

export function stubPermissionStore(permissions: string[] = ['*']): PermissionStore {
  return {
    hasPermission: (code: string) => permissions.includes(code) || permissions.includes('*'),
    setPermissions: () => {},
  } as unknown as PermissionStore
}

export function stubEventBus(): EventBus {
  return {
    on: () => () => {},
    once: () => () => {},
    emit: () => {},
    off: () => {},
  } as unknown as EventBus
}

export function stubCapabilityManager(enabled: string[] = []): CapabilityManager {
  return {
    hasCapability: async (name: string) => enabled.includes(name),
    hasAllCapabilities: async (names: string[]) => names.every((n) => enabled.includes(n)),
    checkRequired: async (names: string[]) => names.every((n) => enabled.includes(n)),
    refresh: async () => {},
  } as unknown as CapabilityManager
}
