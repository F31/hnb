import { test, expect } from '@playwright/test'

test.describe('Login', () => {
  test('successful login redirects to tenant select', async ({ page }) => {
    await page.route(/\/api\/v1\/auth\/login/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          access_token: 'test-token',
          refresh_token: 'test-refresh',
          user_id: 'u1',
          username: 'admin',
          displayName: 'Admin',
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

    await page.goto('/')
    await page.waitForURL('**/login')
    await page.fill('#username', 'admin')
    await page.fill('#password', 'password')
    await page.click('button[type="submit"]')
    await page.waitForURL('**/tenant-select')
    expect(page.url()).toContain('/tenant-select')
  })

  test('failed login shows error message', async ({ page }) => {
    await page.route(/\/api\/v1\/auth\/login/, async (route) => {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ message: 'Invalid credentials' }),
      })
    })

    await page.goto('/login')
    await page.fill('#username', 'admin')
    await page.fill('#password', 'wrong')
    await page.click('button[type="submit"]')
    await expect(page.locator('text=Invalid credentials')).toBeVisible()
    expect(page.url()).toContain('/login')
  })
})