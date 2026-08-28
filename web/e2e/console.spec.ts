import { test, expect } from '@playwright/test'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

/**
 * 控制台初始化冒烟：登录态 → 工作空间 → 导航菜单 → 布局渲染。
 * 全部 API 走 mock，验证"菜单仅来自 /api/v1/navigation/menus"的链路（V3.6 §6）。
 */

function navigationResponse(locale: string) {
  const isEnglish = locale === 'en-US'
  return {
  apiVersion: 'ui.hnb.io/v1',
  etag: 'v-e2e-1',
  generatedAt: '2026-01-01T00:00:00Z',
  context: { tenantId: 't-1', spaceId: 'ws-1' },
  versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' },
  plugins: [],
  menus: [
    {
      group: isEnglish ? 'System' : '系统管理',
      items: [
        { title: isEnglish ? 'Settings' : '系统设置', path: '/system/settings', icon: 'settings' },
        { title: isEnglish ? 'Audit Log' : '审计日志', path: '/system/audit', icon: 'audit' },
      ],
    },
    {
      group: isEnglish ? 'Applications' : '应用',
      items: [
        { title: isEnglish ? 'Monolith Apps' : '单体应用', path: '/application/monolith', icon: 'app' },
        { title: isEnglish ? 'Microservice Apps' : '微服务应用', path: '/application/microservices', icon: 'mesh' },
      ],
    },
  ],
  routes: [],
  }
}

test.describe('Console smoke', () => {
  test('loads menus from navigation API and renders layout', async ({ page }) => {
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

    await page.route(/\/api\/v1\/workspaces/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [{ id: 'ws-1', tenantId: 't-1', name: 'Default Workspace' }],
        }),
      })
    })

    await page.route(/\/api\/v1\/session\/bootstrap/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          subject: { id: 'u1', type: 'user', displayName: 'Admin' },
          selectedTenantId: 't-1',
          memberships: [{ membershipId: 'm-1', tenantId: 't-1', tenantName: 'Tenant 1' }],
          capabilities: [],
          permissions: [{ tenantId: 't-1', resourceKind: '*', action: '*' }],
          policyVersion: 'p1',
          permissionVersion: 'p1',
        }),
      })
    })

    await page.route(/\/api\/v1\/navigation\/menus/, async (route) => {
      const url = new URL(route.request().url())
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(navigationResponse(url.searchParams.get('locale') ?? 'zh-CN')),
      })
    })

    const initialNavigation = page.waitForResponse(/\/api\/v1\/navigation\/menus/)
    await page.goto('/tenant-select')
    // 租户选择页自动进入首个工作空间，等待导航菜单加载完成
    await initialNavigation

    // 导航数据就绪后进入控制台路由，布局应渲染 mock 下发的菜单
    await page.goto('/dashboard')
    await expect(page.locator('.workspace-switcher')).toHaveCount(0)
    await expect(page.locator('.layout-sidebar')).toBeVisible({ timeout: 10000 })
    await expect(page.locator('text=系统设置')).toBeVisible()
    await expect(page.locator('.menu-group-title')).toHaveText('系统管理')
    await expect(page.locator('.layout-header .user-info')).toHaveText('Admin')

    await page.getByRole('button', { name: '应用' }).click()
    await expect(page.locator('.menu-group-title')).toHaveText('应用')
    await expect(page.locator('text=单体应用')).toBeVisible()
    await expect(page.locator('text=微服务应用')).toBeVisible()

    const englishNavigation = page.waitForResponse((response) => {
      const url = new URL(response.url())
      return url.pathname === '/api/v1/navigation/menus' && url.searchParams.get('locale') === 'en-US'
    })
    await page.getByRole('button', { name: 'EN' }).click()
    await englishNavigation
    await expect(page.locator('text=Monolith Apps')).toBeVisible()
    await expect(page.locator('.menu-group-title')).toHaveText('Applications')
    await expect(page.getByRole('button', { name: '中' })).toBeVisible()
  })
})
