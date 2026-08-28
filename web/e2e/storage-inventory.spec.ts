import { expect, test, type Page, type Route } from '@playwright/test'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

const plugins = ['resource', 'container'].map((name) => ({
  name,
  version: '1.0.0',
  displayName: name,
  tier: 'T1' as const,
  enabled: true,
  mode: 'local' as const,
  permissions: { required: [], optional: [] },
  capabilities: { required: [], optional: [] },
  dependencies: { backend: [], plugins: [] },
  menu: { group: name, items: [] },
}))

const navigation = {
  apiVersion: 'ui.hnb.io/v1',
  etag: 'storage-e2e-1',
  generatedAt: '2026-08-10T00:00:00Z',
  context: { tenantId: 'tenant-1', spaceId: 'workspace-1' },
  versions: { permission: 'p1', pluginCatalog: 'c1', navigation: 'n1' },
  plugins,
  menus: [{
    group: 'Storage',
    items: [
      { title: 'Resource Storage', path: '/resource/storage', icon: 'storage' },
      { title: 'Container Storage', path: '/container/storage', icon: 'storage' },
    ],
  }],
  routes: [
    { name: 'resource-storage', path: '/resource/storage', pluginId: 'resource', componentKey: 'Storage' },
    { name: 'container-storage', path: '/container/storage', pluginId: 'container', componentKey: 'Storage' },
    { name: 'container-storage-legacy', path: '/container/instances/storage', pluginId: 'container', componentKey: 'Storage', redirect: '/container/storage' },
  ],
}

const overview = {
  schemaVersion: '1.0.0',
  source: 'runtime_target_storage_inventory',
  observedAt: '2026-08-10T01:00:00Z',
  freshness: 'Stale',
  counts: { backends: 1, offerings: 1, driverInstallations: 1, targets: 1, bindings: 1 },
  capacityStates: { Known: 2, Elastic: 1, Unknown: 3, NotReported: 4 },
}

const backend = {
  schemaVersion: '1.0.0', id: 'backend-1', tenantId: 'tenant-1', providerType: 'generic-csi', displayName: 'Primary storage',
  connectionState: 'Connected', healthState: 'Healthy', source: 'storage-projection', observedAt: '2026-08-10T01:00:00Z', freshness: 'Stale',
  capacity: { status: 'Known', value: 10737418240, unit: 'By', source: 'CSIStorageCapacity', observedAt: '2026-08-10T01:00:00Z', freshness: 'Stale' },
  secretReference: { name: 'redacted' }, conditions: [], version: 1, createdAt: '2026-08-01T00:00:00Z', updatedAt: '2026-08-10T01:00:00Z',
}

const offering = {
  schemaVersion: '1.0.0', id: 'fast/block', scope: 'Tenant', name: 'Fast block', serviceMode: 'Block', accessModes: ['ReadWriteOnce'],
  volumeExpansion: 'Supported', snapshots: 'Supported', clones: 'Unknown', protectionClass: 'gold', conditions: [], version: 1,
  createdAt: '2026-08-01T00:00:00Z', updatedAt: '2026-08-10T01:00:00Z',
}

const binding = {
  schemaVersion: '1.0.0', id: 'binding-1', tenantId: 'tenant-1', offeringId: 'fast/block', offeringVersion: 1,
  targetId: 'cluster-a', storageClassName: 'sc fast', storageClassUid: 'sc-uid', storageClassResourceVersion: '7',
  syncState: 'Active', isDefault: false, source: 'storage-projection', observedAt: '2026-08-10T01:00:00Z', freshness: 'Fresh',
  conditions: [], version: 1, createdAt: '2026-08-01T00:00:00Z', updatedAt: '2026-08-10T01:00:00Z',
}

async function fulfill(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

async function bootstrap(page: Page, initialPath = '/tenant-select'): Promise<void> {
  await page.addInitScript((token) => {
    localStorage.setItem('hnb_token', token)
    localStorage.setItem('hnb_refresh_token', 'test-refresh-token')
    localStorage.setItem('hnb.locale', 'en-US')
    localStorage.setItem('hnb_user', JSON.stringify({ id: 'u1', username: 'admin', displayName: 'Admin' }))
  }, JWT_TOKEN)
  await page.route('**/api/v1/auth/refresh', (route) => fulfill(route, { access_token: JWT_TOKEN, refresh_token: 'test-refresh-token' }))
  await page.route('**/api/v1/session/bootstrap', (route) => fulfill(route, {
    subject: { id: 'u1', type: 'user', displayName: 'Admin' }, selectedTenantId: 'tenant-1',
    memberships: [{ membershipId: 'm1', tenantId: 'tenant-1', tenantName: 'Tenant 1' }], capabilities: [],
    permissions: [{ tenantId: 'tenant-1', resourceKind: '*', action: '*' }], policyVersion: 'p1', permissionVersion: 'p1',
  }))
  await page.route('**/api/v1/workspaces*', (route) => fulfill(route, { data: [{ id: 'workspace-1', tenantId: 'tenant-1', name: 'Default' }] }))
  await page.route('**/api/v1/navigation/menus*', (route) => fulfill(route, navigation))
  await page.route('**/api/v1/storage/provider-schemas*', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [], total: 0 }))
  const consoleInitialized = page.waitForEvent('console', {
    predicate: (message) => message.text().includes('[App] Initialization complete'),
    timeout: 60000,
  })
  await page.goto(initialPath, { waitUntil: 'domcontentloaded' })
  await consoleInitialized
}

async function gotoRoute(page: Page, path: string): Promise<void> {
  const title = path.startsWith('/resource/') ? 'Resource Storage' : 'Container Storage'
  await page.getByText(title, { exact: true }).click()
}

async function mockEmptyCollections(page: Page): Promise<void> {
  await page.route('**/api/v1/storage/backends*', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [], total: 0 }))
  await page.route('**/api/v1/storage/offerings', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [], total: 0 }))
  await page.route('**/api/v1/storage/driver-installations*', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [], total: 0 }))
}

test.describe('Resource Storage read-only inventory', () => {
  test.setTimeout(90000)

  test('direct links survive startup and refresh while the legacy route preserves context', async ({ page }) => {
    await page.route('**/api/v1/storage/overview*', (route) => fulfill(route, overview))
    await mockEmptyCollections(page)

    const resourcePath = '/resource/storage?target=cluster-a#systems'
    await bootstrap(page, resourcePath)
    await expect(page).toHaveURL(resourcePath)
    await expect(page.getByText('Read-only supply view')).toBeVisible({ timeout: 10000 })

    const refreshed = page.waitForEvent('console', {
      predicate: (message) => message.text().includes('[App] Initialization complete'),
      timeout: 60000,
    })
    await page.reload({ waitUntil: 'domcontentloaded' })
    await refreshed
    await expect(page).toHaveURL(resourcePath)

    const initialized = page.waitForEvent('console', {
      predicate: (message) => message.text().includes('[App] Initialization complete'),
      timeout: 60000,
    })
    await page.goto('/container/instances/storage?cluster=cluster-a&target=cluster-a&namespace=default&offering=fast%2Fblock&storageClass=sc+fast#pvc')
    await initialized
    await expect(page).toHaveURL((url) => (
      url.pathname === '/container/storage'
      && url.searchParams.get('cluster') === 'cluster-a'
      && url.searchParams.get('target') === 'cluster-a'
      && url.searchParams.get('namespace') === 'default'
      && url.searchParams.get('offering') === 'fast/block'
      && url.searchParams.get('storageClass') === 'sc fast'
      && url.hash === '#pvc'
    ))
  })

  test('desktop shows loading, stale capacity states, and preserves offering link context', async ({ page }) => {
    await bootstrap(page)
    let releaseOverview!: () => void
    const overviewGate = new Promise<void>((resolve) => { releaseOverview = resolve })
    await page.route('**/api/v1/storage/overview*', async (route) => {
      await overviewGate
      await fulfill(route, overview)
    })
    await page.route('**/api/v1/storage/backends*', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [backend], total: 1 }))
    await page.route('**/api/v1/storage/offerings/fast%2Fblock/bindings*', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [binding], total: 1 }))
    await page.route('**/api/v1/storage/offerings', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [offering], total: 1 }))
    await page.route('**/api/v1/storage/driver-installations*', (route) => fulfill(route, { schemaVersion: '1.0.0', items: [], total: 0 }))

    await gotoRoute(page, '/resource/storage')
    await expect(page.getByRole('region', { name: 'Loading storage overview' })).toBeVisible({ timeout: 10000 })
    releaseOverview()
    await expect(page.getByText('Read-only supply view')).toBeVisible()
    await expect(page.getByText('Stale').first()).toBeVisible()
    for (const label of ['Known', 'Elastic', 'Unknown', 'Not reported']) {
      await expect(page.getByText(label).first()).toBeVisible()
    }

    await page.getByRole('tab', { name: 'Storage Systems' }).click()
    await expect(page.getByText('Primary storage')).toBeVisible()
    await expect(page.getByText('CSIStorageCapacity')).toBeVisible()
    await page.getByRole('tab', { name: 'Storage Services' }).click()
    const link = page.getByRole('link', { name: 'sc fast (cluster-a)' })
    await expect(link).toHaveAttribute('href', /target=cluster-a.*cluster=cluster-a.*offering=fast%2Fblock.*storageClass=sc\+fast/)
    const offeringHref = await link.getAttribute('href')

    await page.route('**/api/v1/workspaces/workspace-1/clusters*', (route) => fulfill(route, { data: [{ id: 'cluster-a', name: 'Cluster A', status: 'online', target_type: 'kubernetes' }] }))
    await page.route('**/api/v1/workspaces/workspace-1/namespaces*', (route) => fulfill(route, { data: [] }))
    await page.route('**/api/v1/proxy/cluster-a/**', (route) => {
      const storageClass = route.request().url().includes('storageclasses')
        ? [{ metadata: { name: 'sc fast', creationTimestamp: '2026-08-01T00:00:00Z' }, provisioner: 'csi.example', reclaimPolicy: 'Retain', allowVolumeExpansion: true }]
        : []
      return fulfill(route, { items: storageClass })
    })
    // Warm the lazy Container route before exercising the plain offering href.
    await gotoRoute(page, '/container/storage')
    await expect(page.getByRole('tablist', { name: 'Storage Resources' })).toBeVisible({ timeout: 10000 })
    await gotoRoute(page, '/resource/storage')
    await page.evaluate((href) => {
      history.pushState({}, '', href)
      window.dispatchEvent(new PopStateEvent('popstate'))
    }, offeringHref)
    await expect(page).toHaveURL(/\/container\/storage\?.*offering=fast%2Fblock/)
    await expect(page.getByText('Storage offering context: fast/block')).toBeVisible({ timeout: 10000 })
    await expect(page.locator('input[type="search"]')).toHaveValue('sc fast')
  })

  test('desktop renders explicit empty inventory states', async ({ page }) => {
    await bootstrap(page)
    await page.route('**/api/v1/storage/overview*', (route) => fulfill(route, { ...overview, freshness: 'Unknown', counts: { backends: 0, offerings: 0, driverInstallations: 0, targets: 0, bindings: 0 }, capacityStates: { Known: 0, Elastic: 0, Unknown: 0, NotReported: 0 } }))
    await mockEmptyCollections(page)
    await gotoRoute(page, '/resource/storage')

    await page.getByRole('tab', { name: 'Storage Systems' }).click()
    await expect(page.getByText('No storage systems')).toBeVisible({ timeout: 10000 })
    await page.getByRole('tab', { name: 'Storage Services' }).click()
    await expect(page.getByText('No storage services')).toBeVisible()
    await page.getByRole('tab', { name: 'Drivers & Connectors' }).click()
    await expect(page.getByText('No drivers or connectors')).toBeVisible()
  })

  test('desktop renders a projection error without leaking a fake inventory', async ({ page }) => {
    await bootstrap(page)
    await page.route('**/api/v1/storage/overview*', (route) => fulfill(route, { code: 'STORAGE_PROJECTION_READ_FAILED' }, 503))
    await mockEmptyCollections(page)
    await gotoRoute(page, '/resource/storage')

    await expect(page.getByText('Failed to load storage data')).toBeVisible({ timeout: 10000 })
    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
    await expect(page.locator('.metric-card')).toHaveCount(0)
  })

  test('mobile renders the read-only overview and capacity inventory', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 })
    await bootstrap(page)
    await page.route('**/api/v1/storage/overview*', (route) => fulfill(route, overview))
    await mockEmptyCollections(page)
    await gotoRoute(page, '/resource/storage')

    await expect(page.getByText('Read-only supply view')).toBeVisible({ timeout: 10000 })
    await expect(page.locator('.capacity-card')).toHaveCount(4)
    await expect(page.getByRole('tab', { name: 'Storage Systems' })).toBeVisible()
    const cardsFitViewport = await page.locator('.capacity-card').evaluateAll((cards) => cards.every((card) => {
      const bounds = card.getBoundingClientRect()
      return bounds.left >= 0 && bounds.right <= window.innerWidth
    }))
    expect(cardsFitViewport).toBe(true)
  })
})
