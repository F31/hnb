import { test, expect } from '@playwright/test'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

/**
 * dev 模式插件加载验收（双 Vue 实例根治的回归测试）。
 *
 * dev 下 /modules/<name>/index.js 由 vite 中间件重写到插件源码，
 * 裸导入 'vue' 统一走 vite 的 vue 实例。若回退到 import map 的
 * /vendor 拷贝（双实例），插件组件挂载时 inject/reactive 会断裂，
 * 本用例将失败。
 */

const NAVIGATION_RESPONSE = {
  apiVersion: 'ui.hnb.io/v1',
  etag: 'v-e2e-plugin-1',
  generatedAt: '2026-01-01T00:00:00Z',
  context: { tenantId: 't-1', spaceId: 'ws-1' },
  versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' },
  plugins: [
    {
      name: 'dashboard',
      version: '1.0.0',
      displayName: '首页',
      tier: 'T0',
      enabled: true,
      mode: 'local',
      permissions: { required: [] },
      capabilities: { required: [] },
      dependencies: { backend: [] },
      menu: { group: '总览', items: [] },
    },
  ],
  menus: [
    {
      group: '总览',
      items: [{ title: '平台总览', path: '/p/dashboard', icon: 'dashboard' }],
    },
  ],
  routes: [
    {
      name: 'PluginDashboard',
      path: '/p/dashboard',
      pluginId: 'dashboard',
      componentKey: 'Dashboard',
    },
  ],
}

test.describe('Plugin modules (dev)', () => {
  test('local plugin loads through vite and renders schema-driven page', async ({ page }) => {
    await page.addInitScript((token) => {
      localStorage.setItem('hnb_token', token)
      localStorage.setItem('hnb_refresh_token', 'test-refresh-token')
      // 固定中文 locale，避免 headless 浏览器 en-US 影响文案断言
      localStorage.setItem('hnb.locale', 'zh-CN')
      localStorage.setItem(
        'hnb_user',
        JSON.stringify({ id: 'u1', username: 'admin', displayName: 'Admin' }),
      )
    }, JWT_TOKEN)

    await page.route(/\/api\/v1\/session\/bootstrap/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          subject: { id: 'u1', type: 'user', displayName: 'Admin' },
          selectedTenantId: 't-1',
          memberships: [{ membershipId: 'm-1', tenantId: 't-1', tenantName: 'Tenant 1' }],
          capabilities: [], permissions: [{ tenantId: 't-1', resourceKind: '*', action: '*' }],
          policyVersion: 'p1', permissionVersion: 'p1',
        }),
      })
    })

    await page.route(/\/api\/v1\/workspaces/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [{ id: 'ws-1', tenantId: 't-1', name: 'Default Workspace' }],
        }),
      })
    })

    await page.route(/\/api\/v1\/navigation\/menus/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(NAVIGATION_RESPONSE),
      })
    })

    // 插件激活完成的信号（首次经 vite 转换较慢，放宽超时）
    const pluginActivated = page.waitForEvent('console', {
      predicate: (msg) => msg.text().includes('plugin activated: dashboard'),
      timeout: 60000,
    })

    const initialNavigation = page.waitForResponse(/\/api\/v1\/navigation\/menus/)
    await page.goto('/tenant-select')
    await initialNavigation
    await pluginActivated

    // 验证 Shell 首页消费了 dashboard 插件贡献的仪表盘 Widget
    await expect(page.locator('.extension-grid')).toBeVisible({ timeout: 15000 })
    await expect(page.locator('.extension-card')).toHaveCount(4)
    await expect(page.locator('.extension-card').first()).toContainText('集群数量')

    // 单入口分组直接由一级导航进入插件路由。
    await page.getByRole('button', { name: '总览' }).click()
    await expect(page).toHaveURL(/\/p\/dashboard/)
    await expect(page.locator('.page-title')).toHaveText('平台运行总览', { timeout: 15000 })
    await expect(page.locator('.metric-card')).toHaveCount(4)
    await expect(page.locator('.metric-card').first()).toContainText('集群数量')
    // 无 Schema 校验错误与区块占位符
    await expect(page.locator('.page-error')).toHaveCount(0)
    await expect(page.locator('.region-placeholder')).toHaveCount(0)
  })
})
