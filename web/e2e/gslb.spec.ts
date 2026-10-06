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
    // 审批中心（切换记录页签）的列表数据：随意图提交/审批动作演进
    const requests: ReturnType<typeof switchRequest>[] = []

    await page.route('**/api/v1/gslb/services/*/drills*', async (route) => {
      const items = drillCount > 0 ? [drillReport] : []
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items, total: items.length }) })
    })
    await page.route('**/api/v1/gslb/services/*/pools*', async (route) => {
      const pools = {
        items: [
          { poolId: POOL_A, name: 'active', priority: 0, isActive: !approved, healthy: true, members: [{ clusterId: 'cluster-a', weight: 100 }] },
          { poolId: POOL_B, name: 'backup', priority: 1, isActive: approved, healthy: true, members: [{ clusterId: 'cluster-b', weight: 100 }] },
        ],
        total: 2,
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(pools) })
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
        const req = switchRequest(body.kind, 'PendingApproval', true)
        requests.push(req)
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(req) })
      }
    })
    await page.route('**/api/v1/gslb/switch-requests/*/approve*', async (route) => {
      approved = true
      requests.forEach((r) => { if (r.status === 'PendingApproval') r.status = 'Approved' })
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(switchRequest('gslb.failover', 'Approved', true)) })
    })
    await page.route('**/api/v1/gslb/switch-requests*', async (route) => {
      if (route.request().method() !== 'GET') return route.fallback()
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: requests, total: requests.length }) })
    })
    // 单条查询（审批后轮询用）：按 id 返回当前状态；列表 URL 带 query 不会命中此 mock 的语义
    await page.route('**/api/v1/gslb/switch-requests/*', async (route) => {
      const url = route.request().url()
      if (url.includes('/approve') || url.includes('/reject')) return route.fallback()
      const id = url.split('/switch-requests/')[1]?.split(/[/?]/)[0]
      const req = requests.find((r) => r.id === id)
      if (!req) return route.fulfill({ status: 404, contentType: 'application/json', body: '{}' })
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(req) })
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

    // 详情（页签式结构）
    await page.locator('.gslb-table .btn-small').first().click()
    await expect(page.locator('.gslb-detail')).toBeVisible({ timeout: 5000 })

    // 演练报告页签：初始为空
    await page.locator('.gslb-tab', { hasText: '演练报告' }).click()
    await expect(page.locator('.gslb-drill-empty')).toBeVisible()

    // 演练：只读，报告展示
    await page.locator('.gslb-tab', { hasText: '概览' }).click()
    await page.locator('button:has-text("故障演练")').click()
    await expect(page.locator('.gslb-request')).toContainText('DrillCompleted', { timeout: 5000 })
    await page.locator('.gslb-tab', { hasText: '演练报告' }).click()
    await expect(page.locator('.gslb-drill .state-pill').first()).toHaveText('可切换', { timeout: 5000 })
    await expect(page.locator('.gslb-drill-checks li').first()).toContainText('target-pool-has-members')

    // 切换：审批门控提示
    await page.locator('.gslb-tab', { hasText: '概览' }).click()
    await page.locator('button:has-text("故障转移")').click()
    await expect(page.locator('.gslb-request')).toContainText('PendingApproval', { timeout: 5000 })
    await expect(page.locator('.gslb-request')).toContainText('需审批')

    // 审批闭环：切换记录页签内直接批准（表格状态列渲染中文标签）
    await page.locator('.gslb-tab', { hasText: '切换记录' }).click()
    await expect(page.locator('.gslb-switch-table tbody tr').first()).toContainText('待审批', { timeout: 5000 })
    await page.locator('button:has-text("批准")').first().click()
    await expect(page.locator('.gslb-switch-table tbody tr').first()).toContainText('已批准', { timeout: 5000 })
    expect(approved).toBe(true)

    // 回切：显式人工确认（同样审批门控）
    await page.locator('.gslb-tab', { hasText: '概览' }).click()
    await page.locator('button:has-text("回切")').click()
    await expect(page.locator('.gslb-request')).toContainText('PendingApproval', { timeout: 5000 })

    expect(submittedKinds).toEqual(['gslb.drill', 'gslb.failover', 'gslb.switchback'])
  })

  test('创建服务向导：填表 → 提交 → 列表出现新服务并进入详情', async ({ page }) => {
    const NEW_ID = '00000000-0000-4000-8000-0000000000e1'
    let createdBody: Record<string, any> | null = null
    let created = false

    const createdModel = () => readModel({
      serviceId: NEW_ID,
      domain: 'newapi.hnb.cloud',
      activePoolId: undefined,
      healthyPools: [],
      currentDnsTargets: [],
      healthCheck: { protocol: 'http', path: '/healthz', intervalSeconds: 10, timeoutSeconds: 3, failureThreshold: 3 },
    })

    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [{ clusterId: 'cluster-a', displayName: '集群 A' }], total: 1 }),
      })
    })
    await page.route(`**/api/v1/gslb/services/${NEW_ID}*`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(createdModel()) })
    })
    await page.route('**/api/v1/gslb/services', async (route) => {
      if (route.request().method() === 'POST') {
        createdBody = route.request().postDataJSON()
        created = true
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(createdModel()) })
        return
      }
      const items = created ? [createdModel()] : []
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items, total: items.length }) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/gslb')
    await expect(page.locator('.gslb-empty-panel')).toBeVisible({ timeout: 10000 })

    await page.locator('button:has-text("创建 GSLB 服务")').click()
    await expect(page.locator('.wizard-dialog')).toBeVisible({ timeout: 5000 })

    await page.getByPlaceholder('如 api-global').fill('api-global')
    await page.getByPlaceholder('api.example.com').fill('newapi.hnb.cloud')
    await page.getByPlaceholder('pool-0').fill('primary')
    await page.locator('.wizard-member-cluster').selectOption('cluster-a')

    await page.locator('.wizard-dialog button[type="submit"]').click()

    expect(createdBody).not.toBeNull()
    expect(createdBody!).toMatchObject({
      name: 'api-global',
      domain: 'newapi.hnb.cloud',
      requireApproval: true,
      healthCheck: { protocol: 'http', path: '/healthz', intervalSeconds: 10, timeoutSeconds: 3, failureThreshold: 3 },
    })
    expect(createdBody!.pools).toEqual([
      { name: 'primary', priority: 0, members: [{ clusterId: 'cluster-a', weight: 100 }] },
    ])

    // 成功后刷新列表并自动选中新服务；概览展示健康检查摘要
    await expect(page.locator('.wizard-dialog')).toHaveCount(0)
    await expect(page.locator('.gslb-table tbody tr')).toHaveCount(1, { timeout: 10000 })
    await expect(page.locator('.gslb-detail')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.gslb-detail')).toContainText('newapi.hnb.cloud')
    await expect(page.locator('.gslb-detail')).toContainText('/healthz')
  })

  test('调权流程：地址池页签编辑权重并提交 weight-update 意图', async ({ page }) => {
    let intentBody: Record<string, any> | null = null

    await page.route('**/api/v1/gslb/services/*/intents*', async (route) => {
      intentBody = route.request().postDataJSON()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(switchRequest('gslb.weight-update', 'Succeeded', false)),
      })
    })
    await page.route('**/api/v1/gslb/services/*/pools*', async (route) => {
      const pools = {
        items: [
          {
            poolId: POOL_A, name: 'active', priority: 0, isActive: true, healthy: true,
            members: [
              { clusterId: 'cluster-a', weight: 100 },
              { clusterId: 'cluster-b', weight: 0 },
            ],
          },
        ],
        total: 1,
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(pools) })
    })
    await page.route(`**/api/v1/gslb/services/${SERVICE_ID}*`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(readModel()) })
    })
    await page.route('**/api/v1/gslb/services*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [readModel()], total: 1 }) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/gslb')
    await expect(page.locator('.gslb-table tbody tr')).toHaveCount(1, { timeout: 10000 })

    await page.locator('.gslb-table .btn-small').first().click()
    await expect(page.locator('.gslb-detail')).toBeVisible({ timeout: 5000 })

    await page.locator('.gslb-tab', { hasText: '地址池' }).click()
    await expect(page.locator('.gslb-pool')).toHaveCount(1, { timeout: 5000 })

    await page.locator('button:has-text("调整权重")').click()
    await expect(page.locator('.gslb-weight-edit')).toBeVisible()
    await expect(page.locator('.gslb-weight-sum')).toContainText('100 / 100')

    // 灰度调权：cluster-b 0 → 40（总和仍 ≤100 时先调 cluster-a）
    await page.locator('.gslb-weight-row').nth(0).locator('input').fill('60')
    await page.locator('.gslb-weight-row').nth(1).locator('input').fill('40')
    await expect(page.locator('.gslb-weight-sum')).toContainText('100 / 100')

    await page.locator('button:has-text("提交调权")').click()

    expect(intentBody).not.toBeNull()
    expect(intentBody!.kind).toBe('gslb.weight-update')
    expect(intentBody!.weights).toEqual({ 'cluster-a': 60, 'cluster-b': 40 })

    // 免审批，灰度生效：终态直接回显
    await expect(page.locator('.gslb-request')).toContainText('Succeeded', { timeout: 5000 })
  })

  test('编辑服务：预填健康检查 → 合并式 PATCH 保存 → 刷新投影', async ({ page }) => {
    let patchBody: Record<string, any> | null = null
    let patchHeaders: Record<string, string> = {}
    const healthCheck = { protocol: 'http', path: '/healthz', intervalSeconds: 15, timeoutSeconds: 5, failureThreshold: 3 }

    await page.route(`**/api/v1/gslb/services/${SERVICE_ID}*`, async (route) => {
      const method = route.request().method()
      if (method === 'PATCH') {
        patchBody = route.request().postDataJSON()
        patchHeaders = route.request().headers()
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: SERVICE_ID, tenantId: 't-1', name: 'api-v2', domain: 'api.hnb.cloud',
            routingMode: 'failover', activePoolId: POOL_A, lifecycleState: 'Active',
            requireApproval: false, healthCheck, updatedAt: new Date().toISOString(), pools: [],
          }),
        })
        return
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(readModel({ healthCheck })) })
    })
    await page.route('**/api/v1/gslb/services*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [readModel({ healthCheck })], total: 1 }) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/gslb')
    await expect(page.locator('.gslb-table tbody tr')).toHaveCount(1, { timeout: 10000 })
    await page.locator('.gslb-table .btn-small').first().click()
    await expect(page.locator('.gslb-detail')).toBeVisible({ timeout: 5000 })

    await page.locator('button:has-text("编辑服务")').click()
    await expect(page.locator('.wizard-dialog')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.wizard-dialog h3')).toHaveText('编辑 GSLB 服务')

    // 域名只读；健康检查从投影预填
    await expect(page.locator('.wizard-dialog input[disabled]')).toHaveValue('api.hnb.cloud')
    await expect(page.getByPlaceholder('/healthz')).toHaveValue('/healthz')

    // 合并式 PATCH：改名 + 关闭审批；名称留空则不提交 name 字段
    await page.getByPlaceholder('留空则保持现有名称不变').fill('api-v2')
    await page.locator('label', { hasText: '切换操作需要审批' }).locator('select').selectOption('false')
    await page.locator('.wizard-dialog button[type="submit"]').click()

    await expect(page.locator('.wizard-dialog')).toHaveCount(0, { timeout: 5000 })
    expect(patchBody).not.toBeNull()
    expect(patchBody!.name).toBe('api-v2')
    expect(patchBody!.requireApproval).toBe(false)
    expect(patchBody!.healthCheck).toEqual(healthCheck)
    expect(patchBody!.domain).toBeUndefined()
    expect(patchHeaders['if-match']).toBe('*')
    expect(patchHeaders['x-correlation-id']).toBeTruthy()
    expect(patchHeaders['idempotency-key']).toMatch(/^gslb-update-/)
  })

  test('编辑池配置：预填现有池 → PUT 整组替换；active 池被删时展示 409 错误', async ({ page }) => {
    let putBody: Record<string, any> | null = null
    let rejectOnce = true

    await page.route('**/api/v1/resources/clusters*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [{ clusterId: 'cluster-a', displayName: '集群 A' }], total: 1 }),
      })
    })
    await page.route(`**/api/v1/gslb/services/${SERVICE_ID}/pools*`, async (route) => {
      if (route.request().method() === 'PUT') {
        if (rejectOnce) {
          rejectOnce = false
          putBody = route.request().postDataJSON()
          await route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({ code: 'CONFLICT', message: 'active pool "active" must be kept in the new configuration' }),
          })
          return
        }
        putBody = route.request().postDataJSON()
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ items: [{ poolId: POOL_A, name: 'active', priority: 0, isActive: true, healthy: true, members: [{ clusterId: 'cluster-a', weight: 100 }] }], total: 1 }),
        })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [{ poolId: POOL_A, name: 'active', priority: 0, isActive: true, healthy: true, members: [{ clusterId: 'cluster-a', weight: 100 }] }],
          total: 1,
        }),
      })
    })
    await page.route(`**/api/v1/gslb/services/${SERVICE_ID}*`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(readModel()) })
    })
    await page.route('**/api/v1/gslb/services*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [readModel()], total: 1 }) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/gslb')
    await expect(page.locator('.gslb-table tbody tr')).toHaveCount(1, { timeout: 10000 })
    await page.locator('.gslb-table .btn-small').first().click()
    await expect(page.locator('.gslb-detail')).toBeVisible({ timeout: 5000 })

    await page.locator('.gslb-tab', { hasText: '地址池' }).click()
    await expect(page.locator('.gslb-pool')).toHaveCount(1, { timeout: 5000 })

    await page.locator('button:has-text("编辑池配置")').click()
    await expect(page.locator('.wizard-dialog')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.wizard-dialog h3')).toHaveText('编辑地址池配置')
    // 预填现有池名
    await expect(page.getByPlaceholder('pool-0')).toHaveValue('active')

    // 首次提交：把主池改名（相当于从配置中移除 active 池名），后端 409 展示错误信息
    await page.getByPlaceholder('pool-0').fill('renamed')
    await page.locator('.wizard-dialog button[type="submit"]').click()
    await expect(page.locator('.wizard-error')).toContainText('active pool "active" must be kept', { timeout: 5000 })
    expect(putBody!.pools).toEqual([
      { name: 'renamed', priority: 0, members: [{ clusterId: 'cluster-a', weight: 100 }] },
    ])

    // 改回主池名后再次提交成功：对话框关闭
    await page.getByPlaceholder('pool-0').fill('active')
    await page.locator('.wizard-dialog button[type="submit"]').click()
    await expect(page.locator('.wizard-dialog')).toHaveCount(0, { timeout: 5000 })
    expect(putBody!.pools).toEqual([
      { name: 'active', priority: 0, members: [{ clusterId: 'cluster-a', weight: 100 }] },
    ])
  })

  test('删除服务：Active 需强制删除 → 409 展示在途请求错误 → 成功后返回列表', async ({ page }) => {
    let deleted = false
    let deleteCalls = 0
    let deleteUrl = ''
    let deleteHeaders: Record<string, string> = {}

    await page.route(`**/api/v1/gslb/services/${SERVICE_ID}*`, async (route) => {
      if (route.request().method() === 'DELETE') {
        deleteCalls += 1
        deleteUrl = route.request().url()
        deleteHeaders = route.request().headers()
        if (deleteCalls === 1) {
          await route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({ code: 'CONFLICT', message: 'service has in-flight switch requests' }),
          })
          return
        }
        deleted = true
        await route.fulfill({ status: 204 })
        return
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(readModel()) })
    })
    await page.route('**/api/v1/gslb/services*', async (route) => {
      const items = deleted ? [] : [readModel()]
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items, total: items.length }) })
    })

    await enterConsole(page)
    await gotoRoute(page, '/resource/gslb')
    await expect(page.locator('.gslb-table tbody tr')).toHaveCount(1, { timeout: 10000 })
    await page.locator('.gslb-table .btn-small').first().click()
    await expect(page.locator('.gslb-detail')).toBeVisible({ timeout: 5000 })

    await page.locator('button:has-text("删除服务")').click()
    await expect(page.locator('.gslb-dialog')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.gslb-dialog')).toContainText('api.hnb.cloud')
    // Active 状态：须勾选强制删除，确认按钮先禁用
    await expect(page.locator('.gslb-dialog')).toContainText('强制删除')
    await expect(page.locator('.gslb-dialog button.danger')).toBeDisabled()
    await page.locator('.gslb-dialog-check input').check()
    await expect(page.locator('.gslb-dialog button.danger')).toBeEnabled()

    // 首次删除：后端 409（在途切换请求），错误展示在对话框内且对话框保持打开
    await page.locator('.gslb-dialog button.danger').click()
    await expect(page.locator('.gslb-dialog-error')).toContainText('in-flight switch requests', { timeout: 5000 })
    await expect(page.locator('.gslb-dialog')).toBeVisible()
    expect(deleteUrl).toContain('force=true')
    expect(deleteHeaders['idempotency-key']).toMatch(/^gslb-delete-/)

    // 再次确认：删除成功，返回列表（空态），详情关闭
    await page.locator('.gslb-dialog button.danger').click()
    await expect(page.locator('.gslb-dialog')).toHaveCount(0, { timeout: 5000 })
    await expect(page.locator('.gslb-detail')).toHaveCount(0)
    await expect(page.locator('.gslb-empty-panel')).toBeVisible({ timeout: 10000 })
    expect(deleteCalls).toBe(2)
  })
})
