<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { useNavigationStore } from '@/stores/navigationStore'
import { usePluginStore } from '@/stores/pluginStore'
import { storeToRefs } from 'pinia'
import type { DashboardWidget } from '@hnb/types'
import { getExtensionRegistry } from '@/core/extension/ExtensionRegistry'
import { getEventBus } from '@/core/event-bus'

const router = useRouter()
const authStore = useAuthStore()
const navStore = useNavigationStore()
const pluginStore = usePluginStore()

const { isAuthenticated } = storeToRefs(authStore)
const { tenantId, spaceId } = storeToRefs(navStore)

const loadingCount = computed(() => pluginStore.loadingCount)
const activePlugins = computed(() => pluginStore.getAllActive.length)

/** 插件贡献的仪表盘 Widget */
const extensionWidgets = ref<DashboardWidget[]>([])

function refreshWidgets() {
  extensionWidgets.value = getExtensionRegistry()
    .getContributions<DashboardWidget>('dashboard.widgets')
    .sort((a, b) => (b.payload.priority ?? 0) - (a.payload.priority ?? 0))
    .map((c) => c.payload)
}

onMounted(() => {
  refreshWidgets()
  getEventBus().on('extension:changed', refreshWidgets)
})
</script>

<template>
  <div class="dashboard">
    <header class="dashboard-header">
      <h1>概览</h1>
      <div v-if="spaceId" class="context-info">
        租户: {{ tenantId || '-' }} · 空间: {{ spaceId }}
      </div>
    </header>

    <main class="dashboard-content">
      <div v-if="loadingCount > 0" class="status-card">
        <p>正在加载插件 ({{ loadingCount }})...</p>
      </div>

      <template v-else>
        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-value">{{ activePlugins }}</div>
            <div class="stat-label">已激活插件</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">-</div>
            <div class="stat-label">运行中应用</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">-</div>
            <div class="stat-label">集群节点</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">-</div>
            <div class="stat-label">系统健康</div>
          </div>
        </div>

        <!-- 扩展点示范：插件贡献的仪表盘 Widget（V3.6 §5.3） -->
        <div v-if="extensionWidgets.length > 0" class="stats-grid extension-grid">
          <div
            v-for="w in extensionWidgets"
            :key="w.title"
            class="stat-card extension-card"
          >
            <div class="stat-value">{{ w.value }}{{ w.unit ? '' : '' }}</div>
            <div v-if="w.unit" class="stat-unit">{{ w.unit }}</div>
            <div class="stat-label">{{ w.title }}</div>
          </div>
        </div>

        <div class="welcome-card">
          <h2>欢迎使用 HNB 控制台</h2>
          <p v-if="authStore.user" class="user-info">
            当前用户: {{ authStore.user.displayName }}
          </p>
          <p class="hint">请从左侧导航菜单选择功能模块开始操作，或安装更多 CapabilityPack 扩展平台能力。</p>
        </div>
      </template>
    </main>
  </div>
</template>

<style scoped>
.dashboard {
  padding: 24px;
  color: #eef2f7;
  height: 100%;
  overflow-y: auto;
}
.dashboard-header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: 24px;
}
h1 { margin: 0; font-size: 22px; color: #fff; }
.context-info { color: #7a8a9a; font-size: 13px; }
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.stat-card {
  background: #11161d;
  border: 1px solid #293443;
  border-radius: 8px;
  padding: 24px;
  text-align: center;
}
.stat-value { font-size: 32px; font-weight: 700; color: #7188ff; margin-bottom: 4px; }
.stat-unit { font-size: 13px; color: #7a8a9a; margin-top: -4px; margin-bottom: 4px; }
.stat-label { font-size: 13px; color: #7a8a9a; }
.welcome-card {
  background: #11161d;
  border: 1px solid #293443;
  border-radius: 8px;
  padding: 32px;
}
.welcome-card h2 { margin: 0 0 12px; font-size: 18px; color: #fff; }
.user-info { color: #b9c2d0; font-size: 14px; margin: 0 0 8px; }
.hint { color: #7a8a9a; font-size: 13px; margin: 0; }
.status-card {
  padding: 32px; text-align: center;
  border: 1px solid #293443; border-radius: 8px;
  background: #11161d; color: #7a8a9a;
}
</style>