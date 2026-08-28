import type { HNBPlugin, PluginContext, MenuItem } from '@hnb/types'
import { definePlugin } from '@hnb/plugin-sdk'
import ApplicationLayout from './pages/ApplicationLayout.vue'
import ApplicationApps from './pages/ApplicationApps.vue'
import EnvManager from './pages/EnvManager.vue'
import AppMarket from './pages/AppMarket.vue'
import AppTemplates from './pages/AppTemplates.vue'
import ObservabilityLayout from './pages/ObservabilityLayout.vue'
import AppAnalysis from './pages/observability/AppAnalysis.vue'
import Topology from './pages/observability/Topology.vue'
import SmartGuard from './pages/observability/SmartGuard.vue'
import TimeTravel from './pages/observability/TimeTravel.vue'
import messages from './locales'
import { setMarketApiClient } from './marketApi'

const plugin: HNBPlugin = definePlugin({
  name: 'application',
  version: '1.0.0',
  displayName: '应用工厂',
  tier: 'T1',
  enabled: true,
  mode: 'local',

  components: {
    ApplicationLayout,
    ApplicationApps,
    EnvManager,
    AppMarket,
    AppTemplates,
    ObservabilityLayout,
    AppAnalysis,
    Topology,
    SmartGuard,
    TimeTravel,
  },

  async create(ctx: PluginContext) {
    ctx.i18n.registerMessages(messages)
    setMarketApiClient(ctx.apiClient)
    const t = ctx.i18n.t
    return {
      routes: [
        { path: '/application', componentKey: 'ApplicationLayout' },
        { path: '/application/monolith', componentKey: 'ApplicationApps' },
        { path: '/application/microservices', componentKey: 'ApplicationApps' },
        { path: '/application/environments', componentKey: 'EnvManager' },
        { path: '/application/market', componentKey: 'AppMarket' },
        { path: '/application/templates', componentKey: 'AppTemplates' },
        { path: '/application/observability', componentKey: 'ObservabilityLayout' },
        { path: '/application/observability/analysis', componentKey: 'AppAnalysis' },
        { path: '/application/observability/topology', componentKey: 'Topology' },
        { path: '/application/observability/guard', componentKey: 'SmartGuard' },
        { path: '/application/observability/timeline', componentKey: 'TimeTravel' },
      ],
      menuItems: [
        { title: t('menu.monolith'), path: '/application/monolith', icon: 'app' },
        { title: t('menu.microservice'), path: '/application/microservices', icon: 'mesh' },
        { title: t('menu.env'), path: '/application/environments', icon: 'env' },
        { title: t('menu.market'), path: '/application/market', icon: 'market' },
        { title: t('menu.templates'), path: '/application/templates', icon: 'template' },
        { title: t('menu.observability'), path: '/application/observability', icon: 'eye' },
      ],
      async onActivate() {
        console.log('[Application] plugin activated')
      },
      async onDeactivate() {
        console.log('[Application] plugin deactivated')
      },
    }
  }
})

export default plugin
