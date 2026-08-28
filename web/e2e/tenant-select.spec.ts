import { test, expect } from '@playwright/test'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

test.describe('Tenant Select', () => {
  test('shows workspace selector for authenticated user', async ({ page }) => {
    let workspaceApiCalled = false
    await page.addInitScript((token) => {
      localStorage.setItem('hnb_token', token)
      localStorage.setItem('hnb_refresh_token', 'test-refresh-token')
      localStorage.setItem('hnb_user', JSON.stringify({
        id: 'u1', username: 'admin', displayName: 'Admin',
      }))
    }, JWT_TOKEN)

    await page.route('**/api/v1/auth/refresh', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ access_token: JWT_TOKEN, refresh_token: 'test-refresh-token' }) })
    })
    await page.route('**/api/v1/session/bootstrap', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ subject: { id: 'u1' }, selectedTenantId: 't-1' }) })
    })
    await page.route('**/api/v1/workspaces*', async (route) => {
      workspaceApiCalled = true
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [{ id: 'ws-1', tenantId: 't-1', name: 'Default Workspace' }],
        }),
      })
    })
    await page.route('**/api/v1/navigation/menus*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ apiVersion: 'ui.hnb.io/v1', etag: 'v1', context: { tenantId: 't-1', spaceId: 'ws-1' }, versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' }, plugins: [], menus: [], routes: [] }) })
    })

    // 通过 /login 初始化 SPA（避免 TenantSelect 自动导航导致竞态）
    await page.goto('/login', { waitUntil: 'load', timeout: 25000 })
    await page.waitForTimeout(1000)
    await page.evaluate(() => {
      history.pushState({}, '', '/tenant-select')
      window.dispatchEvent(new PopStateEvent('popstate'))
    })
    await page.waitForTimeout(500)
    expect(workspaceApiCalled).toBe(true)
  })
})