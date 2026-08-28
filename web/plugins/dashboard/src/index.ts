import type { HNBPlugin, PluginContext, MenuItem, DashboardWidget } from '@hnb/types'
import { definePlugin } from '@hnb/plugin-sdk'
import Dashboard from './pages/Dashboard.vue'
import ApprovalList from './pages/ApprovalList.vue'
import RecentOps from './pages/RecentOps.vue'
import messages from './locales'

const plugin: HNBPlugin = definePlugin({
  name: 'dashboard',
  version: '1.0.0',
  displayName: '首页',
  tier: 'T0',
  enabled: true,
  mode: 'local',

  components: {
    Dashboard,
    ApprovalList,
    RecentOps,
  },

  async create(ctx: PluginContext) {
    ctx.i18n.registerMessages(messages)
    const t = ctx.i18n.t

    const widgets: DashboardWidget[] = [
      { title: t('clusterCount'), value: 12, unit: '', priority: 100 },
      { title: t('cpu'), value: 600, unit: t('cores'), priority: 90 },
      { title: t('gpu'), value: 16, unit: t('cards'), priority: 80 },
      { title: t('storage'), value: '7.1TB', priority: 70 },
    ]
    for (const w of widgets) {
      ctx.extensions.contribute('dashboard.widgets', { pluginId: 'dashboard', payload: w })
    }

    return {
      routes: [
        { path: '/dashboard', componentKey: 'Dashboard' },
        { path: '/dashboard/approvals', componentKey: 'ApprovalList' },
        { path: '/dashboard/recent', componentKey: 'RecentOps' },
      ],
      menuItems: [
        { title: t('menu.overview'), path: '/dashboard', icon: 'dashboard' },
        { title: t('menu.approvals'), path: '/dashboard/approvals', icon: 'check' },
        { title: t('menu.recent'), path: '/dashboard/recent', icon: 'history' },
      ],
      async onActivate() {
        console.log('[Dashboard] plugin activated')
      },
      async onDeactivate() {
        console.log('[Dashboard] plugin deactivated')
      },
    }
  },
})

export default plugin
