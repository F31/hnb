import type { HNBPlugin, PluginContext, MenuItem } from '@hnb/types'
import { definePlugin } from '@hnb/plugin-sdk'
import SystemLayout from './pages/SystemLayout.vue'
import Settings from './pages/Settings.vue'
import UserList from './pages/UserList.vue'
import RoleList from './pages/RoleList.vue'
import TenantList from './pages/TenantList.vue'
import WorkspaceList from './pages/WorkspaceList.vue'
import OperationApproval from './pages/OperationApproval.vue'
import AuditLog from './pages/AuditLog.vue'
import ExtensionList from './pages/ExtensionList.vue'
import messages from './locales'
import { setSystemApiClient } from './systemApi'
export * from './systemApi'

const plugin: HNBPlugin = definePlugin({
  name: 'system',
  version: '1.0.0',
  displayName: '系统',
  tier: 'T1',
  enabled: true,
  mode: 'local',

  components: {
    SystemLayout,
    Settings,
    UserList,
    RoleList,
    TenantList,
    WorkspaceList,
    OperationApproval,
    AuditLog,
    ExtensionList,
  },

  async create(ctx: PluginContext) {
    ctx.i18n.registerMessages(messages)
    setSystemApiClient(ctx.apiClient)
    const t = ctx.i18n.t
    return {
      menuItems: [
        { title: t('menu.settings'), path: '/system/settings', icon: 'settings' },
        { title: t('menu.users'), path: '/system/users', icon: 'user' },
        { title: t('menu.roles'), path: '/system/roles', icon: 'role' },
        { title: t('menu.tenants'), path: '/system/tenants', icon: 'tenant' },
        { title: t('menu.approvals'), path: '/system/approvals', icon: 'approval' },
        { title: t('menu.audit'), path: '/system/audit', icon: 'audit' },
        { title: t('menu.extensions'), path: '/system/extensions', icon: 'extension' },
      ],
      async onActivate() {
        console.log('[System] plugin activated')
      },
      async onDeactivate() {
        console.log('[System] plugin deactivated')
      },
    }
  }
})

export default plugin