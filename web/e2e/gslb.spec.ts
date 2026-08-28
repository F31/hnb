import { test, expect, type Page } from '@playwright/test'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

/**
 * GSLB 全流程 E2E（OpenSpec gslb-traffic-resilience T10.3）：
 * 列表 → 详情 → 只读演练（报告展示）→ 切换（审批门控）→ 审批 → 回切。
 * 后端响应经 route mock；验证页面动作全部走 RuntimeIntent 提交与审批门控提示。
 */

const SERVICE_ID = '00000000-0000-4000-8000-0000000000a1'
const POOL_A = '00000000-0000-4000-8000-0000000000b1'
const POOL_B = '00000000-0000-4000-8000-0000000000b2'

const pluginDef = {
  name: 'resource', version: '1.0.0', displayName: '资源',
  tier: 'T1' as const, enabled: true, mode: 'local' as const,
  permissions: { required: [], optional: [] },
  capabilities: { required: [], optional: [] },
  dependencies: { backend: [], plugins: [] },
  menu: { group: '资源', items: [{ title: 'GSLB', path: '/resource/gslb', icon: 'globe' }] },
}

function nav() {
  return {
    apiVersion: 'ui.hnb.io/v1', etag: 'v-e2e-gslb', generatedAt: new Date().toISOString(),
    context: { tenantId: 't-1', spaceId: 'ws-1' },
    versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' },
    plugins: [pluginDef],
    menus: [{ group: '资源', items: [{ title: 'GSLB', path: '/resource/gslb', icon: 'globe' }] }],
    routes: [{ name: 'r-gslb', path: '/resource/gslb', pluginId: 'resource', componentKey: 'GSLB' }],
  }
}

function bootstrap() {
  return {
    subject: { id: 'u1', type: 'user' as const, displayName: 'Admin' },
    selectedTenantId: 't-1',
    memberships: [{ membershipId: 'm-1', tenantId: 't-1', tenantName: 'Tenant-1' }],
    capabilities: [],
    permissions: [{ tenantId: 't-1', resourceKind: '*', action: '*' }],
    policyVersion: 'p1', permissionVersion: 'p1',
  }
}

function readModel(overrides: Record<string, unknown> = {}) {
  return {
    serviceId: SERVICE_ID, tenantId: 't-1', domain: 'api.hnb.cloud',
    activePoolId: POOL_A, lifecycleState: 'Active',
    healthyPools: [POOL_A, POOL_B], currentDnsTargets: ['cluster-a'],
    observedAt: new Date().toISOString(), ...overrides,
  }
}

function switchRequest(kind: string, status: string, requireApproval: boolean) {
  const now = new Date().toISOString()
  return {
    id: '00000000-0000-4000-8000-0000000000c1', tenantId: 't-1', serviceId: SERVICE_ID,
    intentKind: kind, intentDigest: 'a'.repeat(64), idempotencyKey: 'k-e2e',
    correlationId: '00000000-0000-4000-8000-0000000000c2',
    requireApproval, status, operationId: '00000000-0000-4000-8000-0000000000c3',
    createdAt: now, updatedAt: now,
  }
}

const drillReport = {
  id: '00000000-0000-4000-8000-0000000000d1', tenantId: 't-1', serviceId: SERVICE_ID,
  requestId: '00000000-0000-4000-8000-0000000000c1', verdict: 'Ready',
  report: {
    serviceId: SERVICE_ID, domain: 'api.hnb.cloud',
    activePoolId: POOL_A, targetPoolId: POOL_B,
    currentTargets: ['cluster-a'], projectedTargets: ['cluster-b'],
    checks: [{ name: 'target-pool-has-members', passed: true }],
    verdict: 'Ready', generatedAt: new Date().toISOString(),
  },
  createdAt: new Date().toISOString(),
}

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

test.describe('GSLB 全流程 E2E', () => {
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

  test('列表 → 详情 → 演练 → 审批切换 → 回切', async ({ page }) => {
    let drillCount = 0
    const submittedKinds: string[] = []
    let approved = false

    await page.route('**/api/v1/gslb/services/*/drills*', async (route) => {
      const items = drillCount > 0 ? [drillReport] : []
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items, total: items.length }) })
    })
    await page.route('**/api/v1/gslb/services/*/intents*', async (route) => {
      const body = route.request().postDataJSON() as { kind: string; tenantId: string }
      submittedKinds.push(body.kind)
      // 前端必须携带租户上下文（后端 fail-closed 校验）
      expect(body.tenantId).toBe('t-1')
      if (body.kind === 'gslb.drill') {
        drillCount += 1
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(switchRequest(body.kind, 'DrillCompleted', false)) })
      } else {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(switchRequest(body.kind, 'PendingApproval', true)) })
      }
    })
    await page.route('**/api/v1/gslb/switch-requests/*/approve*', async (route) => {
      approved = true
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(switchRequest('gslb.failover', 'Approved', true)) })
    })
    await page.route(`**/api/v1/gslb/services/${SERVICE_ID}*`, async (route) => {
      const detail = approved
        ? readModel({ activePoolId: POOL_B, currentDnsTargets: ['cluster-b'] })
        : readModel()
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(detail) })
    })
    await page.route('**/api/v1/gslb/services*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [readModel()], total: 1 }) })
    })

    // 列表
    await enterConsole(page)
    await gotoRoute(page, '/resource/gslb')
    await expect(page.locator('.gslb-table tbody tr')).toHaveCount(1, { timeout: 10000 })
    await expect(page.locator('text=api.hnb.cloud')).toBeVisible()

    // 详情
    await page.locator('.gslb-table .btn-small').first().click()
    await expect(page.locator('.gslb-detail')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.gslb-drill-empty')).toBeVisible()

    // 演练：只读，报告展示
    await page.locator('button:has-text("故障演练")').click()
    await expect(page.locator('.gslb-request')).toContainText('DrillCompleted', { timeout: 5000 })
    await expect(page.locator('.gslb-drill .state-pill').first()).toHaveText('可切换', { timeout: 5000 })
    await expect(page.locator('.gslb-drill-checks li').first()).toContainText('target-pool-has-members')

    // 切换：审批门控提示
    await page.locator('button:has-text("故障转移")').click()
    await expect(page.locator('.gslb-request')).toContainText('PendingApproval', { timeout: 5000 })
    await expect(page.locator('.gslb-request')).toContainText('需审批')

    // 审批（审批动作在 Operation Center / API 完成，这里经页面内 fetch 驱动审批端点，
    // 以便命中 route mock——page.request 不走页面路由拦截）
    const approveOk = await page.evaluate(async () => {
      const res = await fetch('/api/v1/gslb/switch-requests/00000000-0000-4000-8000-0000000000c1/approve', { method: 'POST' })
      return res.ok
    })
    expect(approveOk).toBe(true)
    expect(approved).toBe(true)

    // 回切：显式人工确认（同样审批门控）
    await page.locator('button:has-text("回切")').click()
    await expect(page.locator('.gslb-request')).toContainText('PendingApproval', { timeout: 5000 })

    expect(submittedKinds).toEqual(['gslb.drill', 'gslb.failover', 'gslb.switchback'])
  })
})
