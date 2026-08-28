import type { HNBPlugin, PluginContext, MenuItem } from '@hnb/types'
import { definePlugin } from '@hnb/plugin-sdk'
import ResourceLayout from './pages/ResourceLayout.vue'
import NodeList from './pages/NodeList.vue'
import GPUResources from './pages/GPUResources.vue'
import Network from './pages/Network.vue'
import NetworkManagementPage from './pages/cluster-management/NetworkManagementPage.vue'
import Storage from './pages/Storage.vue'
import GSLB from './pages/GSLB.vue'
import ClusterList from './pages/cluster-management/ClusterList.vue'
import ClusterDetailRedirect from './pages/cluster-management/ClusterDetailRedirect.vue'
import ClusterOverviewPage from './pages/cluster-management/ClusterOverviewPage.vue'
import ClusterMonitoringPage from './pages/cluster-management/ClusterMonitoringPage.vue'
import NodeListPage from './pages/cluster-management/NodeListPage.vue'
import NodeDetailPage from './pages/cluster-management/NodeDetailPage.vue'
import NodeDetailRedirect from './pages/cluster-management/NodeDetailRedirect.vue'
import EdgeNodeGroupsPage from './pages/cluster-management/EdgeNodeGroupsPage.vue'
import TenantAllocationPage from './pages/cluster-management/TenantAllocationPage.vue'
import PluginInstancesPage from './pages/cluster-management/PluginInstancesPage.vue'
import SecurityConfigurationPage from './pages/cluster-management/SecurityConfigurationPage.vue'
import PluginMarketPage from './pages/cluster-management/PluginMarketPage.vue'
import BackupRestorePage from './pages/cluster-management/BackupRestorePage.vue'
import ClusterPlaceholder from './pages/cluster-management/ClusterPlaceholder.vue'
import ClusterRegisterWizard from './pages/cluster-management/components/ClusterRegisterWizard.vue'
import ClusterSummaryCards from './pages/cluster-management/components/ClusterSummaryCards.vue'
import OperationList from './pages/cluster-management/OperationList.vue'
import OperationDetail from './pages/cluster-management/OperationDetail.vue'
import { setGslbApiClient, setGslbContextStore } from './gslbApi'
import messages from './locales'
import {
  setClusterApiClient,
  setClusterCapabilityManager,
  setClusterContextStore,
  setClusterEventBus,
  setClusterNavigate,
  setClusterPermissionStore,
} from './pages/cluster-management/api/clusterApi'
import { setOperationApiClient } from './pages/cluster-management/api/operationApi'
import { setAgentOnboardingApiClient } from './pages/cluster-management/api/agentOnboardingApi'
import { setPluginI18nT } from './pages/cluster-management/api/pluginI18n'

/**
 * 集群管理功能灰度开关（默认开启）。
 * 构建时设置 VITE_FEATURE_RESOURCE_CLUSTER_MGMT=false 可降级为占位页面，
 * 路由仍可达但仅渲染"功能未开启"文案，所有写动作入口隐藏。
 */
const clusterMgmtEnabled = import.meta.env.VITE_FEATURE_RESOURCE_CLUSTER_MGMT !== 'false'
// Advanced pages remain disabled until their production APIs are deployed.
// This avoids presenting fixture-backed or empty screens as usable controls.
const clusterAdvancedEnabled = import.meta.env.VITE_FEATURE_RESOURCE_CLUSTER_ADVANCED === 'true'
const clusterMonitoringEnabled = import.meta.env.VITE_FEATURE_RESOURCE_CLUSTER_MONITORING === 'true'

const plugin: HNBPlugin = definePlugin({
  name: 'resource',
  version: '1.0.0',
  displayName: '资源',
  tier: 'T1',
  enabled: true,
  mode: 'local',

  components: {
    ResourceLayout,
    ClusterList: clusterMgmtEnabled ? ClusterList : ClusterPlaceholder,
    ClusterDetailRedirect: clusterMgmtEnabled ? ClusterDetailRedirect : ClusterPlaceholder,
    ClusterOverviewPage: clusterMgmtEnabled ? ClusterOverviewPage : ClusterPlaceholder,
    ClusterMonitoringPage: clusterMgmtEnabled && clusterMonitoringEnabled ? ClusterMonitoringPage : ClusterPlaceholder,
    NodeListPage: clusterMgmtEnabled ? NodeListPage : ClusterPlaceholder,
    NodeDetailPage: clusterMgmtEnabled ? NodeDetailPage : ClusterPlaceholder,
    NodeDetailRedirect: clusterMgmtEnabled ? NodeDetailRedirect : ClusterPlaceholder,
    EdgeNodeGroupsPage: clusterMgmtEnabled && clusterAdvancedEnabled ? EdgeNodeGroupsPage : ClusterPlaceholder,
    TenantAllocationPage: clusterMgmtEnabled && clusterAdvancedEnabled ? TenantAllocationPage : ClusterPlaceholder,
    PluginInstancesPage: clusterMgmtEnabled && clusterAdvancedEnabled ? PluginInstancesPage : ClusterPlaceholder,
    SecurityConfigurationPage: clusterMgmtEnabled && clusterAdvancedEnabled ? SecurityConfigurationPage : ClusterPlaceholder,
    PluginMarketPage: clusterMgmtEnabled && clusterAdvancedEnabled ? PluginMarketPage : ClusterPlaceholder,
    BackupRestorePage: clusterMgmtEnabled && clusterAdvancedEnabled ? BackupRestorePage : ClusterPlaceholder,
    ClusterRegisterWizard,
    ClusterSummaryCards,
    OperationList,
    OperationDetail,
    NodeList,
    GPUResources,
    Network,
    NetworkManagementPage: clusterMgmtEnabled ? NetworkManagementPage : ClusterPlaceholder,
    Storage,
    GSLB,
  },

  async create(ctx: PluginContext) {
    ctx.i18n.registerMessages(messages)
    setClusterApiClient(ctx.apiClient)
    setGslbApiClient(ctx.apiClient)
    setGslbContextStore(ctx.context)
    setOperationApiClient(ctx.apiClient)
    setAgentOnboardingApiClient(ctx.apiClient)
    setClusterContextStore(ctx.context)
    setClusterPermissionStore(ctx.permission)
    setClusterEventBus(ctx.eventBus)
    setClusterCapabilityManager(ctx.capability)
    setClusterNavigate(ctx.navigate)
    setPluginI18nT('resource', ctx.i18n.t)

    const t = ctx.i18n.t
    return {
      routes: [
        { path: '/resource', componentKey: 'ResourceLayout', pluginId: 'resource' },
        { path: '/resource/clusters', componentKey: 'ClusterList', pluginId: 'resource', permission: 'cluster:list' },
        { path: '/resource/clusters/:clusterId', componentKey: 'ClusterDetailRedirect', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/overview', componentKey: 'ClusterOverviewPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/monitoring', componentKey: 'ClusterMonitoringPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes', componentKey: 'NodeListPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes/:nodeId', componentKey: 'NodeDetailRedirect', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes/:nodeId/basic', componentKey: 'NodeDetailPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes/:nodeId/monitoring', componentKey: 'NodeDetailPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes/:nodeId/disks', componentKey: 'NodeDetailPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes/:nodeId/nics', componentKey: 'NodeDetailPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes/:nodeId/pods', componentKey: 'NodeDetailPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/nodes/:nodeId/virtual-machines', componentKey: 'NodeDetailPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/edge-node-groups', componentKey: 'EdgeNodeGroupsPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/tenant-allocations', componentKey: 'TenantAllocationPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/clusters/:clusterId/plugin-instances', componentKey: 'PluginInstancesPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/security/configuration', componentKey: 'SecurityConfigurationPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/security/configuration/vulnerability-database', componentKey: 'SecurityConfigurationPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/security/configuration/vulnerability-scan', componentKey: 'SecurityConfigurationPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/operations', componentKey: 'OperationList', pluginId: 'resource', permission: 'operation:list' },
        { path: '/resource/operations/:operationId', componentKey: 'OperationDetail', pluginId: 'resource', permission: 'operation:read' },
        { path: '/resource/nodes', componentKey: 'NodeList', pluginId: 'resource' },
        { path: '/resource/gpu', componentKey: 'GPUResources', pluginId: 'resource' },
        { path: '/resource/network', componentKey: 'NetworkManagementPage', pluginId: 'resource' },
        { path: '/resource/storage', componentKey: 'Storage', pluginId: 'resource' },
        { path: '/resource/gslb', componentKey: 'GSLB', pluginId: 'resource' },
        { path: '/resource/plugin-market', componentKey: 'PluginMarketPage', pluginId: 'resource', permission: 'cluster:read' },
        { path: '/resource/backup-restore', componentKey: 'BackupRestorePage', pluginId: 'resource', permission: 'cluster:read' },
      ],
      menuItems: [
        { title: t('menu.clusters'), path: '/resource/clusters', icon: 'cluster', permission: 'cluster:list' },
        { title: t('menu.operations'), path: '/resource/operations', icon: 'operation', permission: 'operation:list' },
        { title: t('menu.network'), path: '/resource/network', icon: 'network' },
        { title: t('menu.storage'), path: '/resource/storage', icon: 'storage' },
        { title: t('menu.gslb'), path: '/resource/gslb', icon: 'globe' },
        ...(clusterAdvancedEnabled ? [
          { title: t('menu.pluginMarket'), path: '/resource/plugin-market', icon: 'plugin', permission: 'cluster:read' },
          { title: t('menu.security'), path: '/security/configuration', icon: 'shield', permission: 'cluster:read' },
        ] : []),
      ],
      async onActivate() {
        console.log('[Resource] plugin activated')
      },
      async onDeactivate() {
        console.log('[Resource] plugin deactivated')
      },
    }
  }
})

export default plugin
