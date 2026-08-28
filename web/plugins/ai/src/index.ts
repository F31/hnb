import type { HNBPlugin, PluginContext, MenuItem } from '@hnb/types'
import { definePlugin } from '@hnb/plugin-sdk'
import AILayout from './pages/AILayout.vue'
import ModelRegistry from './pages/ModelRegistry.vue'
import Inference from './pages/Inference.vue'
import AIGateway from './pages/AIGateway.vue'
import messages from './locales'

const plugin: HNBPlugin = definePlugin({
  name: 'ai',
  version: '1.0.0',
  displayName: 'AI',
  tier: 'T2',
  enabled: true,
  mode: 'local',

  components: {
    AILayout,
    ModelRegistry,
    Inference,
    AIGateway,
  },

  async create(ctx: PluginContext) {
    ctx.i18n.registerMessages(messages)
    const t = ctx.i18n.t
    return {
      routes: [
        { path: '/ai', componentKey: 'AILayout' },
        { path: '/ai/models', componentKey: 'ModelRegistry' },
        { path: '/ai/inference', componentKey: 'Inference' },
        { path: '/ai/gateway', componentKey: 'AIGateway' },
      ],
      menuItems: [
        { title: t('menu.models'), path: '/ai/models', icon: 'model' },
        { title: t('menu.inference'), path: '/ai/inference', icon: 'inference' },
        { title: t('menu.gateway'), path: '/ai/gateway', icon: 'ai-gateway' },
      ],
      async onActivate() {
        console.log('[AI] plugin activated')
      },
      async onDeactivate() {
        console.log('[AI] plugin deactivated')
      },
    }
  }
})

export default plugin