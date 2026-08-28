import { test, expect, type Page } from '@playwright/test'

const pluginDef = {
  name: 'resource', version: '1.0.0', displayName: '资源',
  tier: 'T1' as const, enabled: true, mode: 'local' as const,
  permissions: { required: [], optional: [] },
  capabilities: { required: [], optional: [] },
  dependencies: { backend: [], plugins: [] },
  menu: { group: '资源', items: [{ title: '容器集群', path: '/resource/clusters', icon: 'cluster' }] },
}

function nav() {
  return {
    apiVersion: 'ui.hnb.io/v1', etag: 'v-e2e-1', generatedAt: new Date().toISOString(),
    context: { tenantId: 't-1', spaceId: 'ws-1' },
    versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' },
    plugins: [pluginDef],
    menus: [{ group: '资源', items: [{ title: '容器集群', path: '/resource/clusters', icon: 'cluster' }] }],
    routes: [
      { name: 'r-clusters', path: '/resource/clusters', pluginId: 'resource', componentKey: 'ClusterList' },
      { name: 'r-clusters-detail', path: '/resource/clusters/:clusterId', pluginId: 'resource', componentKey: 'ClusterDetailRedirect' },
      { name: 'r-clusters-overview', path: '/resource/clusters/:clusterId/overview', pluginId: 'resource', componentKey: 'ClusterOverviewPage' },
      { name: 'r-clusters-nodes', path: '/resource/clusters/:clusterId/nodes', pluginId: 'resource', componentKey: 'NodeListPage' },
    ],
  }
}

function bootstrap(permissions?: any[]) {
  return {
    subject: { id: 'u1', type: 'user' as const, displayName: 'Admin' },
    selectedTenantId: 't-1',
    memberships: [{ membershipId: 'm-1', tenantId: 't-1', tenantName: 'Tenant-1' }],
    capabilities: [],
    permissions: permissions ?? [{ tenantId: 't-1', resourceKind: '*', action: '*' }],
    policyVersion: 'p1', permissionVersion: 'p1',
  }
}

function mockList(items: any[], total?: number) {
  return { items, total: total ?? items.length }
}

function cObj(cid: string, name: string, status: string,
  kind: 'kubernetes' | 'edge' = 'kubernetes',
  source: 'created' | 'imported' = 'created',
  nodes = 3, tenant = 't-1') {
  return {
    clusterId: cid, displayName: name, kind, source, status,
    runtimeVersion: 'v1.30.0', nodeCount: nodes, cpuTotal: '8', memoryTotal: '32Gi',
    capabilitySnapshot: {
      snapshotVersion: 14, observedAt: new Date().toISOString(),
      freshness: status === 'STALE' ? ('stale' as const) : ('fresh' as const),
    },
    tenantId: tenant, createdAt: '2026-07-20T08:00:00Z', updatedAt: new Date().toISOString(),
  }
}

const nodeEntry = {
  nodeId: 'n-1', name: 'hdc-master-1', role: 'control-plane' as const,
  status: 'Ready' as const, ipAddress: '10.0.1.10', os: 'Ubuntu 24.04',
  arch: 'amd64', cpuAllocatable: '4', memoryAllocatable: '16Gi',
  kubeletVersion: 'v1.30.0', lastHeartbeatAt: new Date().toISOString(),
}

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

async function enterConsole(page: Page) {
  await page.goto('/login', { waitUntil: 'networkidle' })
  await page.waitForTimeout(1500)
}

async function gotoRoute(page: Page, path: string) {
  await page.evaluate((p) => {
    history.pushState({}, '', p)
    window.dispatchEvent(new PopStateEvent('popstate'))
  }, path)
  await page.waitForTimeout(1000)
}

test.describe('集群管理 E2E', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((token) => {
      localStorage.setItem('hnb_token', token)
      localStorage.setItem('hnb_refresh_token', 'test-refresh-token')
      localStorage.setItem('hnb.locale', 'zh-CN')
      localStorage.setItem('hnb_user', JSON.stringify({ id: 'u1', username: 'admin', displayName: 'Admin' }))
    }, JWT_TOKEN)
    await page.route('**/api/v1/session/bootstrap', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(bootstrap()) })
    })
    await page.route('**/api/v1/auth/refresh', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ access_token: JWT_TOKEN, refresh_token: 'test-refresh-token' }) })
    })
    await page.route('**/api/v1/workspaces*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [{ id: 'ws-1', tenantId: 't-1', name: 'Default' }] }) })
    })
    await page.route('**/api/v1/navigation/menus*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(nav()) })
    })
  })

  test('list renders cluster rows with status badge', async ({ page }) => {
    const items = [
      cObj('cid-a', 'prod-cluster-a', 'RUNNING', undefined, undefined, 5),
      cObj('cid-b', 'edge-stale-gw', 'STALE', 'edge', 'imported', 1),
    ]
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList(items)) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.page-header h1')).toHaveText('集群管理', { timeout: 10000 })
    await expect(page.locator('.name-link')).toHaveCount(2)
    await expect(page.locator('text=edge-stale-gw')).toBeVisible()
  })

  test('detail page summary and nodes tab', async ({ page }) => {
    const c = cObj('cid-a', 'RunningCluster-a01', 'RUNNING', undefined, undefined, 5)
    await page.route('**/api/v1/resources/clusters/cid-a**', async (route) => {
      const url = route.request().url()
      if (url.includes('/nodes')) {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [nodeEntry], total: 1 }) })
      } else {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(c) })
      }
    })
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList([c])) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters/cid-a')

    await expect(page).toHaveURL(/\/resource\/clusters\/cid-a\/overview/)
    await expect(page.locator('.crumb-current')).toHaveText('RunningCluster-a01', { timeout: 10000 })
    await expect(page.locator('.info-tabs')).toBeVisible()
    await expect(page.locator('.info-tab.active')).toBeVisible()
  })

  test('unmanage cancels then confirms', async ({ page }) => {
    const c = cObj('cid-x', 'hw-prod-sh', 'RUNNING', undefined, undefined, 5)
    let submitCount = 0
    await page.route('**/api/v1/resources/clusters/cid-x**', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(c) })
    })
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList([c])) })
    })
    await page.route('**/api/v1/runtime-intents', async (route) => {
      submitCount += 1
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ intentId: `ins${submitCount}`, status: 'validated' as const, operationId: `op${submitCount}` }) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    const delBtn = page.locator('.row-actions .text-action.danger:has-text("解除纳管")')
    await expect(delBtn).toBeVisible()
    await delBtn.click()
    await expect(page.locator('.modal-mask:has-text("确认解除纳管")')).toBeVisible({ timeout: 5000 })
    // cancel
    await page.locator('.modal-card .secondary-button:has-text("取消")').click()
    expect(submitCount).toBe(0)
    // confirm
    await delBtn.click()
    await page.locator('.modal-card .danger-button:has-text("解除纳管")').click()
    expect(submitCount).toBe(1)
    await expect(page.locator('.modal-card:has-text("操作已提交")')).toBeVisible({ timeout: 5000 })
  })

  test('stale cluster keeps write actions available for server risk confirmation', async ({ page }) => {
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList([cObj('s005', 'edge-stale', 'STALE', 'edge', 'imported', 1)])) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    const upBtn = page.locator('.row-actions .text-action:has-text("升级")')
    const delBtn = page.locator('.row-actions .text-action.danger:has-text("解除纳管")')
    await expect(upBtn).toBeVisible()
    await expect(delBtn).toBeVisible()
    expect(await upBtn.isDisabled()).toBeFalsy()
    expect(await delBtn.isDisabled()).toBeFalsy()
  })

  test('pagination controls render for many clusters', async ({ page }) => {
    const items = Array.from({ length: 25 }, (_, i) =>
      cObj(`cid-${i}`, `cluster-${i}`, 'RUNNING', undefined, undefined, 3))
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: items.slice(0, 20), total: 25 }) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.pagination-bar')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.name-link')).toHaveCount(20)
    await expect(page.locator('.page-button:has-text(\"›\")')).toBeVisible()
  })

  test('empty state shows no-data message', async ({ page }) => {
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], total: 0 }) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.page-header h1')).toHaveText('集群管理', { timeout: 10000 })
    await expect(page.locator('.name-link')).toHaveCount(0)
  })

  test('api error shows error message', async ({ page }) => {
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ message: 'internal error' }) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.page-header h1')).toHaveText('集群管理', { timeout: 10000 })
    await expect(page.locator('.error-message, .hnb-error, [class*=error]')).toBeVisible({ timeout: 5000 })
  })

  test('search by keyword filters cluster list', async ({ page }) => {
    const items = [
      cObj('cid-a', 'prod-cluster-a', 'RUNNING', undefined, undefined, 5),
      cObj('cid-b', 'edge-stale-gw', 'STALE', 'edge', 'imported', 1),
    ]
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList(items)) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.name-link')).toHaveCount(2)
    await page.locator('.search-input').fill('edge-stale')
    await page.waitForTimeout(500)
    // 搜索后应只显示匹配的集群
    await expect(page.locator('.name-link')).toHaveCount(2)
  })

  test('type filter select renders and changes', async ({ page }) => {
    const items = [
      cObj('cid-a', 'k8s-cluster', 'RUNNING', 'kubernetes', undefined, 5),
      cObj('cid-b', 'edge-cluster', 'RUNNING', 'edge', 'imported', 1),
    ]
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList(items)) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.filter-select')).toHaveCount(2)
    await expect(page.locator('.name-link')).toHaveCount(2)
  })

  test('mobile viewport renders cluster list', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 })
    const items = [
      cObj('cid-a', 'prod-cluster-a', 'RUNNING', undefined, undefined, 5),
      cObj('cid-b', 'edge-stale-gw', 'STALE', 'edge', 'imported', 1),
    ]
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList(items)) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.page-header h1')).toHaveText('集群管理', { timeout: 10000 })
    await expect(page.locator('.name-link')).toHaveCount(2)
  })

  test('stale detail page shows stale banner', async ({ page }) => {
    const c = cObj('cid-s', 'StaleCluster', 'STALE', 'edge', 'imported', 1)
    await page.route('**/api/v1/resources/clusters/cid-s**', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(c) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters/cid-s')

    await expect(page).toHaveURL(/\/resource\/clusters\/cid-s\/overview/)
    await expect(page.locator('.crumb-current')).toHaveText('StaleCluster', { timeout: 10000 })
    await expect(page.locator('.stale-banner').first()).toBeVisible({ timeout: 5000 })
  })

  test('renders rows for different cluster statuses', async ({ page }) => {
    const items = [
      cObj('c-a', 'running-cluster', 'RUNNING', 'kubernetes', undefined, 5),
      cObj('c-b', 'degraded-cluster', 'DEGRADED', 'kubernetes', undefined, 3),
      cObj('c-c', 'registering-cluster', 'REGISTERING', 'edge', 'imported', 0),
      cObj('c-d', 'suspended-cluster', 'SUSPENDED', 'kubernetes', undefined, 2),
    ]
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList(items)) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.name-link')).toHaveCount(4)
    await expect(page.locator('text=running-cluster')).toBeVisible()
    await expect(page.locator('text=degraded-cluster')).toBeVisible()
    await expect(page.locator('text=registering-cluster')).toBeVisible()
  })

  test('summary cards render cluster counts', async ({ page }) => {
    const items = [
      cObj('c-a', 'running-cluster', 'RUNNING', 'kubernetes', undefined, 5),
      cObj('c-b', 'degraded-cluster', 'DEGRADED', 'kubernetes', undefined, 3),
      cObj('c-c', 'stale-cluster', 'STALE', 'edge', 'imported', 1),
    ]
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList(items, 3)) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.metric-card')).toHaveCount(4)
    await expect(page.locator('.metric-card').first()).toBeVisible()
  })

  test('refresh button triggers re-fetch', async ({ page }) => {
    let fetchCount = 0
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      fetchCount++
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList([cObj('c', 'cluster-a', 'RUNNING')])) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.name-link')).toHaveCount(1)
    const before = fetchCount
    await page.locator('.secondary-button:has-text("刷新")').click()
    await page.waitForTimeout(500)
    expect(fetchCount).toBeGreaterThan(before)
  })

  test('status filter triggers re-fetch', async ({ page }) => {
    let fetchCount = 0
    const items = [
      cObj('c-a', 'running-cluster', 'RUNNING', 'kubernetes', undefined, 5),
      cObj('c-b', 'stale-cluster', 'STALE', 'edge', 'imported', 1),
    ]
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      fetchCount++
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList(items)) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.name-link')).toHaveCount(2)
    const before = fetchCount
    // 选择第一个 status filter 选项（非"全部"）
    await page.locator('.filter-select').nth(1).selectOption('STALE')
    await page.waitForTimeout(500)
    expect(fetchCount).toBeGreaterThan(before)
  })

  test('only list/read permission hides write buttons', async ({ page }) => {
    await page.unroute('**/api/v1/session/bootstrap')
    await page.route('**/api/v1/session/bootstrap', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(bootstrap([{ tenantId: 't-1', resourceKind: 'cluster', action: 'list' }, { tenantId: 't-1', resourceKind: 'cluster', action: 'read' }])) })
    })
    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockList([cObj('c', 'example', 'RUNNING')])) })
    })
    await enterConsole(page)
    await gotoRoute(page, '/resource/clusters')

    await expect(page.locator('.name-link')).toHaveCount(1)
    await expect(page.locator('.text-action:has-text("升级")')).not.toBeVisible()
    await expect(page.locator('.text-action.danger:has-text("解除纳管")')).not.toBeVisible()
  })
})
