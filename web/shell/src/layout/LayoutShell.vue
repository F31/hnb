<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'
import { switchTenantAtomic } from '@/core/context'
import { getLayoutManager } from '@/layout/LayoutManager'
import RecursiveMenu from '@/layout/RecursiveMenu.vue'
import type { MenuGroup, MenuItem } from '@hnb/types'
import { getEventBus } from '@/core/event-bus'
import { setLocale, type SupportedLocale } from '@/i18n'

const router = useRouter()
const auth = useAuthStore()
const context = useContextStore()
const layoutManager = getLayoutManager()
const eventBus = getEventBus()
const { locale } = useI18n()

const menus = computed(() => layoutManager.menus.value)
const workspaces = computed(() => context.workspaces)
const showWorkspaceSwitcher = computed(() => workspaces.value.length > 1)
const currentSpaceId = computed(() => context.spaceId)
const currentPath = computed(() => router.currentRoute.value.path)
const nextLocale = computed<SupportedLocale>(() => (locale.value === 'zh-CN' ? 'en-US' : 'zh-CN'))
const selectedGroupKey = ref<string | null>(null)

function firstLeafPath(items: MenuItem[]): string | undefined {
  for (const item of items) {
    if (item.children?.length) {
      const childPath = firstLeafPath(item.children)
      if (childPath) return childPath
    }
    if (item.path) return item.path
  }
  return undefined
}

function groupPath(group: MenuGroup): string | undefined {
  return firstLeafPath(group.items)
}

function groupKey(group: MenuGroup): string {
  return groupPath(group) ?? group.group
}

function groupTitle(group: MenuGroup): string {
  return group.group
}

function groupMatches(group: MenuGroup): boolean {
  return group.items.some((item) => itemMatches(item))
}

function itemMatches(item: MenuItem): boolean {
  if (item.path && currentPath.value.startsWith(item.path)) return true
  return item.children?.some((child) => itemMatches(child)) ?? false
}

function isTopGroupActive(group: MenuGroup): boolean {
  return activeGroup.value === group
}

const routeMatchedGroup = computed(() => menus.value.find((group) => groupMatches(group)))
const selectedGroup = computed(() =>
  selectedGroupKey.value ? menus.value.find((group) => groupKey(group) === selectedGroupKey.value) : undefined,
)
const activeGroup = computed(() => routeMatchedGroup.value ?? selectedGroup.value ?? menus.value[0])
const showSidebar = computed(() => {
  const group = activeGroup.value
  if (!group) return false
  if (group.items.length !== 1) return true
  const only = group.items[0]
  return !!only.children?.length
})

async function handleSpaceSwitch(event: Event) {
  const target = event.target as HTMLSelectElement
  const spaceId = target.value
  if (!spaceId) return
  const ws = workspaces.value.find((w) => w.id === spaceId)
  if (!ws) return
  if (ws.tenantId && ws.tenantId !== context.tenantId) {
    // 工作空间属于其他租户：先走原子化租户切换（清理插件/路由/缓存），
    // 再绑定新工作空间。
    await switchTenantAtomic(ws.tenantId)
  }
  // setSpace 校验 gen === switchGeneration，直接传当前 generation，
  // 与 TenantSelect 的做法保持一致。
  context.setSpace(spaceId, context.switchGeneration)
}

function navigateTop(group: MenuGroup) {
  selectedGroupKey.value = groupKey(group)
  const path = groupPath(group)
  if (path) router.push(path)
}

watch(routeMatchedGroup, (group) => {
  if (group) selectedGroupKey.value = groupKey(group)
})

watch(menus, (groups) => {
  if (!groups.some((group) => groupKey(group) === selectedGroupKey.value)) {
    selectedGroupKey.value = null
  }
})

function toggleLocale() {
  const value = nextLocale.value
  setLocale(value)
  context.setFullContext({ ...context.current, locale: value })
  eventBus.emit('locale:changed', { locale: value })
}

async function logout() {
  await auth.logout()
  context.reset()
  router.push({ name: 'Login' })
}
</script>

<template>
  <div class="layout-shell">
    <header class="layout-header">
      <div class="logo">
        <span class="logo-accent">HNB</span> Console
      </div>
      <div class="workspace-switcher" v-if="showWorkspaceSwitcher">
        <label for="workspace-select">{{ $t('shell.workspace') }}</label>
        <select
          id="workspace-select"
          :value="currentSpaceId"
          @change="handleSpaceSwitch"
        >
          <option v-for="ws in workspaces" :key="ws.id" :value="ws.id">
            {{ ws.displayName || ws.name }}
          </option>
        </select>
      </div>
      <nav class="top-nav" aria-label="一级导航">
        <button
          v-for="group in menus"
          :key="group.group"
          class="top-nav-item"
          :class="{ active: isTopGroupActive(group) }"
          type="button"
          @click="navigateTop(group)"
        >
          {{ groupTitle(group) }}
        </button>
      </nav>
      <div class="header-actions">
        <span class="user-info" v-if="auth.user">{{ auth.user.displayName }}</span>
        <button class="btn-language" type="button" @click="toggleLocale">{{ $t('shell.languageToggle') }}</button>
        <button class="btn-logout" @click="logout">{{ $t('shell.logout') }}</button>
      </div>
    </header>

    <div class="layout-main">
      <aside v-if="showSidebar" class="layout-sidebar" role="navigation">
        <nav class="sidebar-nav">
          <div v-if="menus.length === 0" class="empty-sidebar">
            <p>{{ $t('shell.emptyMenu') }}</p>
          </div>
          <div v-else-if="activeGroup" class="menu-group">
            <div class="menu-group-title">{{ groupTitle(activeGroup) }}</div>
            <RecursiveMenu :menu-items="activeGroup.items" />
          </div>
        </nav>
      </aside>
      <main class="layout-content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped>
.layout-shell {
  display: flex; flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
  overflow: hidden;
  background: #0b0f14; color: #eef2f7;
}
.layout-header {
  height: 56px; background: #121820;
  border-bottom: 1px solid #293441;
  display: flex; align-items: center;
  padding: 0 20px; gap: 16px;
}
.logo { font-size: 18px; font-weight: 700; flex: 0 0 auto; white-space: nowrap; }
.logo-accent { color: #7188ff; margin-right: 8px; }
.top-nav {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 100%;
  flex: 1;
  min-width: 0;
  overflow-x: auto;
}
.top-nav-item {
  height: 36px;
  padding: 0 14px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #b9c2d0;
  cursor: pointer;
  font-size: 14px;
}
.top-nav-item:hover { background: #1f2833; color: #fff; }
.top-nav-item.active { background: #26335f; color: #fff; }
.workspace-switcher {
  display: flex; align-items: center; gap: 8px;
  font-size: 13px; color: #b9c2d0;
  flex: 0 0 auto;
}
.workspace-switcher select {
  background: #202936; color: #fff;
  padding: 6px 10px; border-radius: 4px;
  border: 1px solid #3b4658; font-size: 13px;
}
.header-actions {
  margin-left: auto;
  display: flex; align-items: center; gap: 12px;
}
.user-info { color: #b9c2d0; font-size: 13px; }
.btn-logout,
.btn-language {
  background: transparent;
  border: 1px solid #4a5568;
  color: #b9c2d0;
  padding: 6px 14px;
  border-radius: 6px; cursor: pointer; font-size: 13px;
}
.btn-language { min-width: 44px; }
.btn-logout:hover, .btn-language:hover { border-color: #7188ff; color: #fff; }
.layout-main {
  display: flex; flex: 1; min-height: 0; overflow: hidden;
}
.layout-sidebar {
  width: 240px;
  background: #11161d;
  border-right: 1px solid #293441;
  padding: 12px 0;
  overflow-y: auto;
  flex-shrink: 0;
}
.menu-group { margin-bottom: 8px; }
.menu-group-title {
  font-size: 11px; color: #6b7a8a;
  text-transform: uppercase;
  padding: 12px 16px 4px;
  font-weight: 600; letter-spacing: 0.5px;
}
.empty-sidebar { padding: 16px; color: #6b7a8a; font-size: 13px; }
.layout-content {
  flex: 1; min-width: 0; padding: 0;
  overflow-y: auto;
  background: #0b0f14;
}

@media (max-width: 768px) {
  .layout-header { gap: 8px; padding: 0 12px; }
  .top-nav { overflow-x: auto; flex: 1; }
  .top-nav-item { flex: 0 0 auto; padding: 0 10px; }
  .workspace-switcher label { display: none; }
  .layout-sidebar { width: 200px; }
  .layout-content { padding: 0; }
}
</style>
