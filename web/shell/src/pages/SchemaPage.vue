<script setup lang="ts">
/**
 * SchemaPage — 服务端 Schema 驱动页面（V2.5 §7）。
 *
 * 接收路由 meta.schemaId，从 /api/v1/schema/page/{schemaId} 获取
 * PageSchema，经 SchemaEngine 校验后由 PageRenderer 渲染。
 * 组件解析使用本地 ComponentRegistry（内置受信组件），无需插件参与。
 *
 * 错误处理：
 *  - 网络/解析失败 → 内联错误提示
 *  - Schema 校验失败（版本不兼容等）→ 显示 SchemaError 信息
 *  - 单区块错误由 RegionWrapper 隔离
 */

import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  SchemaEngine,
  SchemaError,
  PageRenderer,
  createActionEngine,
  createComponentRegistry,
  createDataSourceManager,
  createExtensionRegistry,
  registerBuiltinComponents,
} from '@hnb/schema-engine'
import type { PageSchema } from '@hnb/schema-engine'
import { createApiClient } from '@hnb/api-client'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'
import { usePermissionStore } from '@/stores/permissionStore'
import { getEventBus } from '@/core/event-bus'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const contextStore = useContextStore()
const permissionStore = usePermissionStore()

const schemaId = computed(() => route.meta?.schemaId as string | undefined)

const loading = ref(true)
const error = ref<string | null>(null)
const schema = ref<PageSchema | null>(null)
const texts = ref<Record<string, string>>({})

const registry = createComponentRegistry()
registerBuiltinComponents(registry)
const engine = new SchemaEngine()
const apiClient = createApiClient({
  getToken: () => authStore.token,
  refreshToken: () => authStore.refreshTokenAction(),
  beforeRequest: () => authStore.ensureFreshToken(30),
  onUnauthorized: () => { authStore.sessionExpired = true },
  getContext: () => ({ tenantId: contextStore.tenantId, spaceId: contextStore.spaceId }),
})
const dataSources = createDataSourceManager(apiClient)
// 受信端点 allowlist（8.2）：Schema 只能引用这些服务端声明的端点前缀。
for (const prefix of [
  '/api/v1/resources/clusters',
  '/api/v1/resources/operations',
  '/api/v1/operations',
  '/api/v1/runtime-intents',
  '/api/v1/dictionaries/cluster.status',
  '/api/v1/clusters',
]) {
  dataSources.allowEndpoint(prefix)
}
// tenant/context 切换时推进 generation，丢弃在途（迟到）响应（8.4）。
watch(
  () => [contextStore.tenantId, contextStore.spaceId],
  () => {
    dataSources.invalidateContext()
  },
)
const extensionRegistry = createExtensionRegistry(registry)
// 声明式扩展点（8.3）：集群详情 tabs 等受控扩展在初始化时注册。
// 命名空间非法 / 组件未注册 / 通配权限 / 版本不兼容都会被拒绝（fail-closed）。
extensionRegistry.register({
  namespace: 'resource.cluster.detail.tabs.overview',
  componentType: 'DetailPanel',
  order: 1,
  permission: 'cluster:read',
  minShellVersion: '2.5.0',
})
extensionRegistry.register({
  namespace: 'resource.cluster.detail.tabs.config',
  componentType: 'DescriptionList',
  order: 3,
  permission: 'cluster:read',
  minShellVersion: '2.5.0',
})

const actionEngine = createActionEngine({
  apiClient,
  eventBus: getEventBus(),
  dataSources,
  hasPermission: (permission) => permissionStore.hasPermission(permission) || (authStore.user?.permissions ?? []).includes(permission) || (authStore.user?.permissions ?? []).includes('*'),
  navigate: (name, params) => router.push({ name, params }),
  confirm: async (message) => window.confirm(message),
  openOverlay: (action) => window.dispatchEvent(new CustomEvent('hnb:overlay', { detail: { actionId: action.id, type: action.type } })),
  notify: (message) => window.dispatchEvent(new CustomEvent('hnb:notify', { detail: { message } })),
})
const conditionContext = computed(() => ({
  permissions: Array.from(new Set([
    ...permissionStore.permissions,
    ...(authStore.user?.permissions ?? []),
  ])),
  context: {
    tenantId: contextStore.tenantId,
    spaceId: contextStore.spaceId,
  },
}))

onMounted(async () => {
  if (!schemaId.value) {
    error.value = 'Schema ID not provided'
    loading.value = false
    return
  }
  try {
    const raw = await apiClient.get<PageSchema>(`/api/v1/schema/page/${schemaId.value}`)
    // 校验信封 + spec.regions
    engine.validatePageSchema(raw)
    schema.value = raw
    texts.value = raw.metadata?.texts ?? {}
  } catch (err: unknown) {
    if (err instanceof SchemaError) {
      const schemaErr = err as SchemaError
      error.value = schemaErr.code === 'INCOMPATIBLE'
        ? `当前 Shell 版本过低，请升级后访问该页面：${schemaErr.message}`
        : `Schema 校验失败 (${schemaErr.code}): ${schemaErr.message}`
    } else if (err instanceof Error) {
      error.value = err.message
    } else {
      error.value = String(err)
    }
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="schema-page">
    <div v-if="loading" class="schema-loading">加载页面 Schema...</div>

    <div v-else-if="error" class="schema-error" role="alert">
      <h2>页面加载失败</h2>
      <p>{{ error }}</p>
    </div>

    <PageRenderer
      v-else-if="schema"
      :schema="schema"
      :registry="registry"
      :texts="texts"
      :data-sources="dataSources"
      :action-engine="actionEngine"
      :condition-context="conditionContext"
      :extension-registry="extensionRegistry"
    />

    <div v-else class="schema-empty">无效的页面 Schema</div>
  </div>
</template>

<style scoped>
.schema-page { padding: 16px; }
.schema-loading { color: #7a8a9a; font-size: 14px; padding: 40px; text-align: center; }
.schema-error { color: #f04438; padding: 40px; text-align: center; }
.schema-error h2 { margin: 0 0 12px; font-size: 18px; }
.schema-error p { font-size: 13px; color: #f97066; }
.schema-empty { color: #7a8a9a; padding: 40px; text-align: center; }
</style>
