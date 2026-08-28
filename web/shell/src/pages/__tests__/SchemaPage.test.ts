import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import SchemaPage from '../SchemaPage.vue'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'
import { usePermissionStore } from '@/stores/permissionStore'

const JWT_TOKEN = 'header.eyJzdWIiOiJ1MSIsImV4cCI6OTk5OTk5OTk5OSwiaWF0IjoxMDAwMDAwMDAwfQ.signature'

vi.mock('vue-router', () => ({
  useRoute: () => ({ meta: { schemaId: 'cluster-list' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

function schemaFixture(minShellVersion?: string) {
  return {
    apiVersion: 'ui.hnb.io/v1',
    kind: 'PageSchema',
    metadata: {
      id: 'cluster-list',
      revision: 1,
      minShellVersion,
      texts: { title: 'Clusters', refresh: 'Refresh' },
    },
    spec: {
      template: 'list',
      titleKey: 'title',
      layout: { type: 'grid' },
      endpoints: [{ id: 'clusters.list', path: '/api/v1/clusters', method: 'GET' }],
      dataSources: [{ id: 'clusters', type: 'paginatedQuery', endpointId: 'clusters.list', responseMapping: { items: 'data.items', total: 'data.total' } }],
      actions: [{ id: 'refresh', type: 'api', labelKey: 'refresh', permission: 'schema:read', request: { method: 'GET', endpointId: 'clusters.list' } }],
      regions: [{ id: 'table', componentType: 'DataTable', span: 12, props: { columns: [{ key: 'name', title: 'Name' }], dataSource: 'clusters', actions: ['refresh'] }, condition: { all: [{ permission: 'schema:read' }] } }],
    },
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  const auth = useAuthStore()
  auth.token = JWT_TOKEN
  const context = useContextStore()
  context.setFullContext({ tenantId: 'tenant-a', spaceId: 'space-a' })
  usePermissionStore().setPermissions(['schema:read'])
  vi.restoreAllMocks()
})

describe('SchemaPage', () => {
  it('loads schema, queries dataSource and executes an action through trusted endpoints', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify(schemaFixture()), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { items: [{ name: 'cluster-a' }], total: 1 } }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }))

    const wrapper = mount(SchemaPage)
    await flushPromises()
    await wrapper.find('.region-actions button').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Clusters')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/schema/page/cluster-list', expect.objectContaining({ method: 'GET' }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/clusters?page=1&pageSize=20', expect.objectContaining({ method: 'GET' }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/clusters', expect.objectContaining({ method: 'GET' }))
  })

  it('shows upgrade guidance for incompatible schema', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(new Response(JSON.stringify(schemaFixture('99.0.0')), { status: 200, headers: { 'Content-Type': 'application/json' } }))

    const wrapper = mount(SchemaPage)
    await flushPromises()

    expect(wrapper.text()).toContain('当前 Shell 版本过低')
    expect(wrapper.find('.region-actions button').exists()).toBe(false)
  })
})
