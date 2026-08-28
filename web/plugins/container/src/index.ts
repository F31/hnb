import type { HNBPlugin, PluginContext, MenuItem } from '@hnb/types'
import { definePlugin } from '@hnb/plugin-sdk'
import ContainerLayout from './pages/ContainerLayout.vue'
import Workloads from './pages/cluster-instance/Workloads.vue'
import Namespaces from './pages/cluster-instance/Namespaces.vue'
import Storage from './pages/cluster-instance/Storage.vue'
import Access from './pages/cluster-instance/Access.vue'
import AccessForm from './pages/cluster-instance/AccessForm.vue'
import Config from './pages/cluster-instance/Config.vue'
import Logs from './pages/cluster-instance/Logs.vue'
import Events from './pages/cluster-instance/Events.vue'
import SecurityOverview from './pages/security/SecurityOverview.vue'
import SecurityProtection from './pages/security/SecurityProtection.vue'
import SecurityReport from './pages/security/SecurityReport.vue'
import SecurityConfig from './pages/security/SecurityConfig.vue'
import NetworkPage from './pages/network/NetworkPage.vue'
import messages from './locales'
import { setContainerApiClient, setContainerContextStore } from './api/containerApi'
import { setContainerNetworkClient, setContainerNetworkContext } from './api/containerNetworkApi'
import { setContainerStorageClient } from './api/storageApi'
import { setContainerAccessClient } from './api/accessApi'
import { setContainerLogsClient } from './api/logsApi'
import { setContainerConfigClient } from './api/configApi'
import { setContainerEventsClient } from './api/eventsApi'
import { setContainerSecurityClient } from './api/securityApi'

const plugin: HNBPlugin = definePlugin({
  name: 'container',
  version: '1.0.0',
  displayName: '容器',
  tier: 'T1',
  enabled: true,
  mode: 'local',

  components: {
    ContainerLayout,
    Workloads,
    Namespaces,
    Storage,
    Access,
    AccessForm,
    Config,
    Logs,
    Events,
    SecurityOverview,
    SecurityProtection,
    SecurityReport,
    SecurityConfig,
    NetworkPage,
  },

  async create(ctx: PluginContext) {
    ctx.i18n.registerMessages(messages)
    setContainerApiClient(ctx.apiClient)
    setContainerContextStore(ctx.context)
    setContainerNetworkClient(ctx.apiClient)
    setContainerNetworkContext(ctx.context)
    setContainerStorageClient(ctx.apiClient)
    setContainerAccessClient(ctx.apiClient)
    setContainerLogsClient(ctx.apiClient)
    setContainerConfigClient(ctx.apiClient)
    setContainerEventsClient(ctx.apiClient)
    setContainerSecurityClient(ctx.apiClient)
    const t = ctx.i18n.t
    return {
      routes: [
        { path: '/container', componentKey: 'ContainerLayout' },
        { path: '/container/instances/workloads', componentKey: 'Workloads' },
        { path: '/container/instances/namespaces', componentKey: 'Namespaces' },
        { path: '/container/storage', componentKey: 'Storage' },
        { path: '/container/instances/access', componentKey: 'Access' },
        { path: '/container/instances/access/service/create', componentKey: 'AccessForm' },
        { path: '/container/instances/access/service/:name/edit', componentKey: 'AccessForm' },
        { path: '/container/instances/access/ingress/create', componentKey: 'AccessForm' },
        { path: '/container/instances/access/ingress/:name/edit', componentKey: 'AccessForm' },
        { path: '/container/instances/access/network-policy/create', componentKey: 'AccessForm' },
        { path: '/container/instances/access/network-policy/:name/edit', componentKey: 'AccessForm' },
        { path: '/container/instances/config', componentKey: 'Config' },
        { path: '/container/instances/logs', componentKey: 'Logs' },
        { path: '/container/instances/events', componentKey: 'Events' },
        { path: '/container/network', componentKey: 'NetworkPage' },
        { path: '/container/security/overview', componentKey: 'SecurityOverview' },
        { path: '/container/security/protection', componentKey: 'SecurityProtection' },
        { path: '/container/security/report', componentKey: 'SecurityReport' },
        { path: '/container/security/config', componentKey: 'SecurityConfig' },
      ],
      menuItems: [
        {
          title: t('menu.instances'),
          path: '/container/instances',
          icon: 'cluster',
          children: [
            { title: t('menu.workloads'), path: '/container/instances/workloads', icon: 'workload' },
            { title: t('menu.namespaces'), path: '/container/instances/namespaces', icon: 'namespace' },
            { title: t('menu.storage'), path: '/container/storage', icon: 'storage' },
            { title: t('menu.access'), path: '/container/instances/access', icon: 'access' },
            { title: t('menu.network'), path: '/container/network', icon: 'network' },
            { title: t('menu.config'), path: '/container/instances/config', icon: 'config' },
            { title: t('menu.logs'), path: '/container/instances/logs', icon: 'logs' },
            { title: t('menu.events'), path: '/container/instances/events', icon: 'events' },
          ]
        },
        {
          title: t('menu.security'),
          path: '/container/security',
          icon: 'security',
          children: [
            { title: t('menu.securityOverview'), path: '/container/security/overview', icon: 'overview' },
            { title: t('menu.securityProtection'), path: '/container/security/protection', icon: 'protection' },
            { title: t('menu.securityReport'), path: '/container/security/report', icon: 'report' },
            { title: t('menu.securityConfig'), path: '/container/security/config', icon: 'config' },
          ]
        },
      ],
      async onActivate() {
        console.log('[Container] plugin activated')
      },
      async onDeactivate() {
        console.log('[Container] plugin deactivated')
      },
    }
  }
})

export default plugin
