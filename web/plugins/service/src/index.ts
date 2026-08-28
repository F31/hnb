import type { HNBPlugin, PluginContext, MenuItem } from '@hnb/types'
import { definePlugin } from '@hnb/plugin-sdk'
import ServiceLayout from './pages/ServiceLayout.vue'
import DataService from './pages/DataService.vue'
import MessageService from './pages/MessageService.vue'
import Governance from './pages/Governance.vue'
import Gateway from './pages/Gateway.vue'
import messages from './locales'

const plugin: HNBPlugin = definePlugin({
  name: 'service',
  version: '1.0.0',
  displayName: '云原生服务',
  tier: 'T1',
  enabled: true,
  mode: 'local',

  components: {
    ServiceLayout,
    DataService,
    MessageService,
    Governance,
    Gateway,
  },

  async create(ctx: PluginContext) {
    ctx.i18n.registerMessages(messages)
    const t = ctx.i18n.t
    return {
      routes: [
        { path: '/service', componentKey: 'ServiceLayout' },
        { path: '/service/data', componentKey: 'DataService' },
        { path: '/service/messaging', componentKey: 'MessageService' },
        { path: '/service/governance', componentKey: 'Governance' },
        { path: '/service/gateway', componentKey: 'Gateway' },
      ],
      menuItems: [
        { title: t('menu.data'), path: '/service/data', icon: 'database' },
        { title: t('menu.messaging'), path: '/service/messaging', icon: 'message' },
        { title: t('menu.governance'), path: '/service/governance', icon: 'mesh' },
        { title: t('menu.gateway'), path: '/service/gateway', icon: 'gateway' },
      ],
      async onActivate() {
        console.log('[Service] plugin activated')
      },
      async onDeactivate() {
        console.log('[Service] plugin deactivated')
      },
    }
  }
})

export default plugin