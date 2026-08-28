import { test, expect } from '@playwright/test'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

test.describe('Error Pages', () => {
  test('404 page shows for non-existent route', async ({ page }) => {
    // Auth is needed to prevent App.vue from redirecting to login
    await page.addInitScript((token) => {
      localStorage.setItem('hnb_token', token)
      localStorage.setItem('hnb_refresh_token', 'test-refresh-token')
      localStorage.setItem('hnb_user', JSON.stringify({
        id: 'u1', username: 'admin', displayName: 'Admin',
      }))
    }, JWT_TOKEN)

    await page.route(/\/api\/v1\/session\/bootstrap/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          subject: { id: 'u1', type: 'user', displayName: 'Admin' }, selectedTenantId: 't-1',
          memberships: [], capabilities: [], permissions: [], policyVersion: 'p1', permissionVersion: 'p1',
        }),
      })
    })

    await page.route(/\/api\/v1\/workspaces/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [{ id: 'ws-1', tenantId: 't-1', name: 'Default' }],
        }),
      })
    })

    await page.route(/\/api\/v1\/navigation\/menus/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          apiVersion: 'ui.hnb.io/v1', etag: 'errors-e2e-1', generatedAt: new Date().toISOString(),
          context: { tenantId: 't-1', spaceId: 'ws-1' },
          versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' },
          plugins: [], menus: [], routes: [],
        }),
      })
    })

    const consoleInitialized = page.waitForEvent('console', {
      predicate: (message) => message.text().includes('[App] Initialization complete'),
    })
    await page.goto('/login')
    await consoleInitialized
    await page.evaluate(() => {
      history.pushState({}, '', '/nonexistent-route')
      window.dispatchEvent(new PopStateEvent('popstate'))
    })
    await expect(page.locator('h1')).toContainText('404')
    await expect(page.locator('text=页面不存在')).toBeVisible()
  })

  test('unauthenticated access redirects to login', async ({ page }) => {
    await page.goto('/dashboard')
    await page.waitForURL('**/login')
    expect(page.url()).toContain('/login')
  })
})
