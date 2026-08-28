<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'
import { getNavigationManager } from '@/core/navigation/NavigationManager'
import { getPluginManager } from '@/core/plugin/PluginManager'
import { getRouterManager } from '@/core/router/RouterManager'
import { getLayoutManager } from '@/layout/LayoutManager'
import { getEventBus } from '@/core/event-bus'
import { getExtensionRegistry } from '@/core/extension/ExtensionRegistry'
import { fetchSessionBootstrap, scopedPermissionsToCodes } from '@/core/api/session'
import LayoutShell from '@/layout/LayoutShell.vue'
import type { HNBContext, PluginManifest } from '@hnb/types'
import { usePermissionStore } from '@/stores/permissionStore'
import { getLocale } from '@/i18n'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const context = useContextStore()
const navManager = getNavigationManager()
const pluginManager = getPluginManager()
const routerManager = getRouterManager()
const layoutManager = getLayoutManager()
const eventBus = getEventBus()
const permissionStore = usePermissionStore()

let appGeneration = 0
let lastAppliedNavVersion = ''
const loading = ref(false)
const errorShown = ref(false)
const errorMessage = ref('')

// 服务端导航版本变化（版本轮询或后端通知触发 navigation:updated）：
// 重新应用菜单与动态路由。与 initializeConsole 通过版本号去重，避免重复 reconcile。
eventBus.on('navigation:updated', ({ version }: { tenantId?: string; version?: string }) => {
  // loadMenus emits this event during initial startup. initializeConsole owns
  // that path so plugins are activated before routes can trigger navigation.
  if (loading.value) return
  const next = version ?? navManager.getVersion()
  if (!next || next === lastAppliedNavVersion) return
  lastAppliedNavVersion = next
  layoutManager.render(navManager.getMenus())
  const response = navManager.getResponse()
  if (response?.routes?.length) {
    routerManager.reconcile(response.routes)
  }
})

// 显式重载通道（如后端通知服务收到 NATS 变更事件后转发）
eventBus.on('navigation:reload', () => {
  if (context.tenantId && context.spaceId) {
    navManager
      .forceRefresh({ tenantId: context.tenantId, spaceId: context.spaceId, locale: getLocale() })
      .catch((err) => console.warn('[App] navigation reload failed:', err))
  }
})

eventBus.on('locale:changed', ({ locale }: { locale: string }) => {
  if (!context.tenantId || !context.spaceId) return
  navManager
    .forceRefresh({ tenantId: context.tenantId, spaceId: context.spaceId, locale })
    .then((menus) => {
      layoutManager.render(menus)
      const response = navManager.getResponse()
      if (response?.routes?.length) routerManager.reconcile(response.routes)
      lastAppliedNavVersion = navManager.getVersion()
      navManager.startVersionWatcher({ tenantId: context.tenantId!, spaceId: context.spaceId, locale })
    })
    .catch((err) => console.warn('[App] locale navigation reload failed:', err))
})

const themeOverrides = {
  common: {
    primaryColor: '#637bff',
    primaryColorHover: '#4f65e0',
    primaryColorPressed: '#3b4fc0',
    bodyColor: '#0b0f14',
    cardColor: '#141b24',
    modalColor: '#141b24',
    popoverColor: '#1a2330',
    tableColor: '#141b24',
    inputColor: '#1a2330',
    textColorBase: '#eef2f7',
    textColor1: '#eef2f7',
    textColor2: '#b9c2d0',
    textColor3: '#7a8a9a',
    borderColor: '#293443',
    dividerColor: '#293443',
    hoverColor: '#1a2330',
    actionColor: '#1a2330',
    clearColor: '#141b24',
    placeholderColor: '#7a8a9a',
    fontSize: '14px',
    borderRadius: '6px',
  },
} as const

const isPublicRoute = computed(() => route.meta?.public === true)

onMounted(async () => {
  try {
    auth.restoreSession()
    if (!auth.isAuthenticated) {
      router.push({ name: 'Login' })
      return
    }
    await auth.ensureFreshToken()
    auth.startSlidingSession()
    const gen = ++appGeneration
    if (gen !== appGeneration) return
    await loadSessionBootstrap()
    const workspaces = await context.loadWorkspaces(gen)
    if (!context.spaceId && workspaces.length > 0) {
      await context.setSpace(workspaces[0].id, context.switchGeneration)
    }
  } catch (err) {
    console.error('[App] Initialization failed:', err)
    if (!auth.isAuthenticated) {
      router.push({ name: 'Login' })
      return
    }
    errorShown.value = true
    errorMessage.value = err instanceof Error ? err.message : String(err)
  }
})

watch(
  () => [context.tenantId, context.spaceId] as const,
  ([tenantId, spaceId]) => {
    if (tenantId && spaceId) {
      initializeConsole({ tenantId, spaceId })
    }
  },
)

watch(
  () => auth.sessionExpired,
  (expired) => {
    if (expired) {
      console.warn('[App] session expired, redirecting to login')
      setTimeout(() => {
        auth.logout()
        router.push({ name: 'Login' })
      }, 5000)
    }
  },
)

async function initializeConsole(ctx: HNBContext) {
  const gen = ++appGeneration
  loading.value = true
  errorShown.value = false

  try {
    if (!ctx.tenantId || !ctx.spaceId) {
      await router.push({ name: 'TenantSelect' })
      return
    }
    // 注册 Shell 内置扩展点，供各插件在 create 中贡献内容。
    // 必须在 notifyContextChange / activateRequiredPlugins 前完成。
    const extensionRegistry = getExtensionRegistry()
    extensionRegistry.ensurePoint('dashboard.widgets', 'shell')

    // 同租户内切换 space 时插件保持激活，通知新上下文（V3.6 §4.4）；
    // 跨租户切换时插件已在 switchTenantAtomic 中重置，本调用为空操作。
    await pluginManager.notifyContextChange(ctx)
    const menus = await navManager.loadMenus({
      tenantId: ctx.tenantId,
      spaceId: ctx.spaceId,
      locale: getLocale(),
    })
    layoutManager.render(menus)

    const response = navManager.getResponse()
    // 契约中 plugins 为精简引用（NavigationPlugin，V2.6 §6.2），本地插件
    // 激活时依赖 loader 对缺省字段（entry/mode 等）的默认处理，因此这里
    // 按 PluginManifest 语义读取；后端仍为权威（PluginRegistry 校验）。
    const requiredPluginIds: string[] = (response?.plugins || []).map((p) => p.name)

    if (requiredPluginIds.length > 0) {
      const manifests = (response?.plugins || []) as unknown as PluginManifest[]
      await pluginManager.activateRequiredPlugins(manifests)
    }

    if (response?.routes?.length) {
      // reconcile 会先移除旧动态路由再注册新集合，租户切换后可安全重入。
      routerManager.reconcile(response.routes)
    }

    // 记录已应用版本并启动版本轮询（ETag 条件请求，页面不可见时暂停）
    lastAppliedNavVersion = navManager.getVersion()
    navManager.startVersionWatcher({ tenantId: ctx.tenantId, spaceId: ctx.spaceId, locale: getLocale() })

    eventBus.emit('console:initialized', { tenantId: ctx.tenantId })
    console.log('[App] Initialization complete')
  } catch (err) {
    if (gen === appGeneration) {
      console.error('[App] Initialization failed:', err)
      if (!auth.isAuthenticated) {
        router.push({ name: 'Login' })
        return
      }
      errorShown.value = true
      errorMessage.value = err instanceof Error ? err.message : String(err)
    }
  } finally {
    if (gen === appGeneration) loading.value = false
  }
}

async function loadSessionBootstrap() {
  const bootstrap = await fetchSessionBootstrap()
  const permissionCodes = scopedPermissionsToCodes(bootstrap.permissions ?? [])
  permissionStore.setPermissions(permissionCodes)
  permissionStore.setVersion(bootstrap.permissionVersion || bootstrap.policyVersion || '')
  auth.setPermissions(permissionCodes)
  if (!context.tenantId && bootstrap.selectedTenantId) {
    context.setFullContext({ tenantId: bootstrap.selectedTenantId, locale: getLocale() })
  }
}

function retry() {
  errorShown.value = false
  errorMessage.value = ''
  navManager.clear()
  pluginManager.reset()
  if (auth.isAuthenticated && context.tenantId && context.spaceId) {
    initializeConsole({ tenantId: context.tenantId, spaceId: context.spaceId, locale: getLocale() })
  } else {
    router.push({ name: 'Login' })
  }
}

function goLogin() {
  auth.logout()
  router.push({ name: 'Login' })
}
</script>

<template>
  <n-config-provider :theme-overrides="themeOverrides">
    <div class="app-root">
      <div v-if="errorShown" class="error-display">
        <h1>{{ $t('shell.loadFailed') }}</h1>
        <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
        <p v-else>{{ $t('shell.refreshHint') }}</p>
        <button class="btn-retry" @click="retry">{{ $t('shell.retry') }}</button>
      </div>

      <div v-else-if="auth.sessionExpired" class="session-expired-overlay">
        <div class="session-expired-card">
          <h1>{{ $t('shell.sessionExpired') }}</h1>
          <p>{{ $t('shell.sessionExpiredHint') }}</p>
          <button class="btn-login" @click="goLogin()">{{ $t('shell.login') }}</button>
        </div>
      </div>

      <div v-else-if="loading" class="loading-overlay">
        <div class="spinner"></div>
        <p>{{ $t('shell.loadingConsole') }}</p>
      </div>

      <LayoutShell v-else-if="!isPublicRoute" />
      <router-view v-else />
    </div>
  </n-config-provider>
</template>

<style>
html, body, #app {
  width: 100%;
  height: 100%;
  margin: 0;
  overflow: hidden;
  background: #0b0f14;
}
.n-config-provider {
  height: 100%;
  min-height: 0;
  overflow: hidden;
}
* { box-sizing: border-box; }
.app-root { width: 100%; height: 100%; overflow: hidden; }
.loading-overlay {
  position: fixed; inset: 0;
  background: rgba(11, 15, 20, 0.85);
  display: flex; flex-direction: column;
  align-items: center; justify-content: center;
  color: #eef2f7; z-index: 9999;
}
.spinner {
  width: 48px; height: 48px;
  border: 4px solid #293443;
  border-top: 4px solid #7188ff;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 16px;
}
@keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
.error-display {
  height: 100%;
  display: flex; flex-direction: column;
  align-items: center; justify-content: center;
  color: #ff8585; font-size: 18px;
  background: #0b0f14; padding: 40px;
}
.error-message { color: #b9c2d0; margin: 8px 0 24px; font-size: 14px; }
.btn-retry {
  padding: 10px 24px;
  background: #637bff; color: #fff;
  border: 0; border-radius: 6px; cursor: pointer;
  font-size: 14px;
}
.btn-retry:hover { background: #4f65e0; }
.session-expired-overlay {
  position: fixed; inset: 0;
  background: rgba(11, 15, 20, 0.9);
  display: flex; align-items: center; justify-content: center;
  z-index: 9999;
}
.session-expired-card {
  background: #1a2332; border: 1px solid #ff8585;
  border-radius: 12px; padding: 40px; text-align: center;
  color: #eef2f7;
}
.session-expired-card h1 { margin: 0 0 12px; color: #ff8585; font-size: 20px; }
.session-expired-card p { margin: 0 0 24px; color: #b9c2d0; font-size: 14px; }
.btn-login {
  padding: 10px 24px;
  background: #637bff; color: #fff;
  border: 0; border-radius: 6px; cursor: pointer;
  font-size: 14px;
}
.btn-login:hover { background: #4f65e0; }
</style>
