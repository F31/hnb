/**
 * useClusterRuntime — 在 Vue setup() 内构造 cluster Schema 引擎运行时。
 *
 * 运行时由本 composable 在页面组件 setup 内构造；plugin create(ctx) 仅负责把
 * apiClient / permissionStore / contextStore / navigate 注入模块单例，页面
 * 组件 setup 内调用本 composable 即可取得完整 ClusterRuntime。
 */
import { onBeforeUnmount } from 'vue'
import {
  ActionEngine,
  ComponentRegistry,
  DataSourceManager,
  ExtensionRegistry,
  SchemaEngine,
  createComponentRegistry,
  createDataSourceManager,
  createExtensionRegistry,
  registerBuiltinComponents,
} from '@hnb/schema-engine'
import type { ApiClient, EventBus, PermissionStore } from '@hnb/types'
import {
  CLUSTER_LIST_DATASOURCES,
  CLUSTER_LIST_ENDPOINTS,
} from '../schemas/cluster.endpoints'
import { clusterDetailTabsExtensions } from '../schemas/extensions'
import { registerClusterComponents } from '../runtime/registerClusterComponents'
import { permissionsFromStore, type ClusterRuntime } from '../runtime/clusterRuntime'
import { setClusterApiClient, setClusterPermissionStore } from '../api/clusterApi'

export interface UseClusterRuntimeOptions {
  apiClient: ApiClient
  eventBus: EventBus
  permissionStore: PermissionStore
  contextKey: string
  allowedEndpointPrefixes: string[]
  navigate?: (path: string) => void
  confirm?: (message: string) => Promise<boolean>
  notify?: (message: string) => void
  openOverlay?: (actionId: string, ctx: Record<string, unknown>) => void
}

export function buildClusterRuntime(options: UseClusterRuntimeOptions): ClusterRuntime {
  setClusterApiClient(options.apiClient)
  setClusterPermissionStore(options.permissionStore)

  const dataSources = createDataSourceManager(options.apiClient)
  for (const prefix of options.allowedEndpointPrefixes) dataSources.allowEndpoint(prefix)
  for (const endpoint of CLUSTER_LIST_ENDPOINTS) dataSources.registerEndpoint(endpoint)
  for (const ds of CLUSTER_LIST_DATASOURCES) dataSources.registerDataSource(ds)

  const registry = createComponentRegistry()
  registerBuiltinComponents(registry)
  registerClusterComponents(registry)

  const extensionRegistry = createExtensionRegistry(registry)
  for (const def of clusterDetailTabsExtensions) extensionRegistry.register(def)

  const permissions = permissionsFromStore(options.permissionStore)

  const actionEngine = new ActionEngine({
    apiClient: options.apiClient,
    eventBus: options.eventBus,
    dataSources,
    hasPermission: (perm) => permissions.has(perm) || permissions.has('*'),
    navigate: options.navigate
      ? (name: string, params?: Record<string, string>) => {
          if (name === 'cluster-detail' && params?.clusterId) {
            options.navigate?.(`/resource/clusters/${encodeURIComponent(params.clusterId)}`)
          } else if (name === 'operation-detail' && params?.operationId) {
            options.navigate?.(`/resource/operations/${encodeURIComponent(params.operationId)}`)
          }
        }
      : undefined,
    confirm: options.confirm,
    openOverlay: (action) =>
      options.openOverlay?.(action.id, action as unknown as Record<string, unknown>),
    notify: options.notify,
  })

  return {
    schemaEngine: new SchemaEngine(),
    registry,
    dataSources,
    extensionRegistry,
    actionEngine,
    conditionContext: {
      permissions,
      context: { contextKey: options.contextKey },
    },
    invalidateContext: () => dataSources.invalidateContext(),
    apiClient: options.apiClient,
    eventBus: options.eventBus,
    contextKey: options.contextKey,
    notify: options.notify,
    confirm: options.confirm,
  }
}

export { ComponentRegistry, DataSourceManager, ExtensionRegistry }
