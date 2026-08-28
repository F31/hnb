<script setup lang="ts">
/**
 * ClusterOverviewPage — 集群信息 > 集群详情 概览页（OpenSpec cluster-overview）。
 *
 * Schema 驱动：在 setup 内构造 cluster Schema 引擎运行时（ComponentRegistry /
 * DataSourceManager / ActionEngine），provide 给注册组件；PageRenderer 负责
 * region 编排、条件求值与区块级错误隔离（UI 规范 V2.6 §7）。
 */
import { computed, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { PageRenderer, provideDataSourceManager } from '@hnb/schema-engine'
import { clusterDetailOverviewSchema } from './schemas/cluster.overview'
import { buildClusterRuntime } from './composables/useClusterRuntime'
import { deriveContextKey, usePluginContext } from './composables/usePluginContext'
import ClusterDetailLayout from './components/ClusterDetailLayout.vue'
import ClusterInfoTabs from './components/ClusterInfoTabs.vue'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const contextKey = computed(() => deriveContextKey(pluginCtx.contextStore.current))

const runtime = buildClusterRuntime({
  apiClient: pluginCtx.apiClient,
  eventBus: pluginCtx.eventBus,
  permissionStore: pluginCtx.permissionStore,
  contextKey: contextKey.value,
  allowedEndpointPrefixes: [
    '/api/v1/resources/clusters',
    '/api/v1/resources/clusters/',
    '/api/v1/dictionaries',
    '/api/v1/dictionaries/',
    '/api/v1/runtime-intents',
    '/api/v1/operations',
    '/api/v1/operations/',
  ],
  navigate: pluginCtx.navigate,
  confirm: pluginCtx.confirm,
  notify: pluginCtx.notify,
  openOverlay: (actionId: string) => {
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('cluster-action-overlay', { detail: { actionId } }))
    }
  },
})

provideDataSourceManager(runtime.dataSources)

const schema = computed(() => clusterDetailOverviewSchema)
const texts = computed(() => ({
  'resource.clusterMgmt.detail.title': t('resource.clusterMgmt.detail.title'),
  'resource.clusterMgmt.detail.desc': t('resource.clusterMgmt.detail.desc'),
}))

watch(contextKey, () => runtime.invalidateContext())
onBeforeUnmount(() => runtime.invalidateContext())
</script>

<template>
  <ClusterDetailLayout>
    <ClusterInfoTabs />
    <PageRenderer
      :schema="schema"
      :registry="runtime.registry"
      :data-sources="runtime.dataSources"
      :action-engine="runtime.actionEngine"
      :extension-registry="runtime.extensionRegistry"
      :condition-context="runtime.conditionContext"
      :texts="texts"
    />
  </ClusterDetailLayout>
</template>
