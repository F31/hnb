import { test, expect } from '@playwright/test'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

/**
 * Schema 服务端下发联调 E2E（V2.5 §7）。
 *
 * Shell 从 /api/v1/schema/page/{schemaId} 获取 PageSchema，
 * 经 SchemaEngine 校验后由 PageRenderer 渲染——不依赖任何插件，
 * 覆盖 dataSource 查询、action 执行和受控 visibility。
 */

const SCHEMA_OVERVIEW = {
  apiVersion: 'ui.hnb.io/v1',
  kind: 'PageSchema',
  metadata: { id: 'schema-overview', revision: 1, texts: { 'page.title': '平台运行总览', refresh: '刷新集群' } },
  spec: {
    template: 'list',
    titleKey: 'page.title',
    layout: { type: 'grid' },
    endpoints: [{ id: 'clusters.list', path: '/api/v1/clusters', method: 'GET' }],
    dataSources: [{ id: 'clusters', type: 'paginatedQuery', endpointId: 'clusters.list', responseMapping: { items: 'data.items', total: 'data.total' } }],
    actions: [{ id: 'refresh', type: 'api', labelKey: 'refresh', permission: 'schema:read', request: { method: 'GET', endpointId: 'clusters.list' } }],
    regions: [
      { id: 'clusters', componentType: 'DataTable', span: 12, props: { columns: [{ key: 'name', title: '名称' }], dataSource: 'clusters', actions: ['refresh'] }, condition: { all: [{ permission: 'schema:read' }] } },
      { id: 'admin-only', componentType: 'MetricCard', span: 6, props: { title: '管理员指标', value: 1 }, condition: { all: [{ permission: 'schema:admin' }] } },
    ],
  },
}

const NAVIGATION_RESPONSE = {
  apiVersion: 'ui.hnb.io/v1',
  etag: 'v-e2e-schema-1',
  generatedAt: '2026-01-01T00:00:00Z',
  context: { tenantId: 't-1', spaceId: 'ws-1' },
  versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' },
  plugins: [],
  menus: [
    {
      group: '总览',
      items: [{ title: 'Schema 总览', path: '/overview', icon: 'dashboard' }],
    },
  ],
  routes: [
    {
      name: 'SchemaOverview',
      path: '/overview',
      pluginId: 'shell',
      componentKey: 'Placeholder',
      schemaId: 'schema-overview',
    },
  ],
}

test.describe('Schema page (server-delivered)', () => {
  test('fetches schema from API and renders via PageRenderer', async ({ page }) => {
    await page.addInitScript((token) => {
      localStorage.setItem('hnb_token', token)
      localStorage.setItem('hnb_refresh_token', 'test-refresh-token')
      localStorage.setItem('hnb.locale', 'zh-CN')
      localStorage.setItem(
        'hnb_user',
        JSON.stringify({ id: 'u1', username: 'admin', displayName: 'Admin', permissions: ['schema:read'] }),
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
          capabilities: [],
          permissions: [{ tenantId: 't-1', resourceKind: 'schema', action: 'read' }],
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

    await page.route(/\/api\/v1\/schema\/page\/schema-overview/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(SCHEMA_OVERVIEW),
      })
    })

    let clusterCalls = 0
    await page.route(/\/api\/v1\/clusters/, async (route) => {
      clusterCalls += 1
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: { items: [{ name: 'cluster-a' }], total: 1 } }),
      })
    })

    const initialNavigation = page.waitForResponse(/\/api\/v1\/navigation\/menus/)
    await page.goto('/tenant-select')
    await initialNavigation
    // 单入口分组直接由一级导航进入 Schema 路由，不渲染冗余侧边栏。
    await page.getByRole('button', { name: '总览' }).click()
    await expect(page).toHaveURL(/\/overview/)

    // Schema 页面应通过 PageRenderer 渲染出服务端下发的组件并查询 dataSource
    await expect(page.locator('.page-title')).toHaveText('平台运行总览', { timeout: 15000 })
    await expect(page.locator('text=cluster-a')).toBeVisible()
    await expect(page.locator('text=管理员指标')).toHaveCount(0)
    await page.locator('.region-actions button', { hasText: '刷新集群' }).click()
    await expect.poll(() => clusterCalls).toBeGreaterThanOrEqual(2)
    // 校验与区块错误均为 0
    await expect(page.locator('.page-error')).toHaveCount(0)
    await expect(page.locator('.region-placeholder')).toHaveCount(0)
  })
})
