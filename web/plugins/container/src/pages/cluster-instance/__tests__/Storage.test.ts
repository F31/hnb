import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import Storage from '../Storage.vue'
import NetworkDrawer from '../../network/NetworkDrawer.vue'
import messages from '../../../locales'
import { createI18n } from 'vue-i18n'

const containerApi = vi.hoisted(() => ({
  listNamespaces: vi.fn(),
  listWorkspaceClusters: vi.fn(),
}))
const storageApi = vi.hoisted(() => ({
  listPersistentVolumes: vi.fn(),
  listPersistentVolumeClaims: vi.fn(),
  listStorageClasses: vi.fn(),
}))

vi.mock('../../../api/containerApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../api/containerApi')>(),
  ...containerApi,
}))
vi.mock('../../../api/storageApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../api/storageApi')>(),
  ...storageApi,
}))

async function mountStorage(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/container/storage', component: Storage }],
  })
  await router.push(path)
  await router.isReady()
  const pack = messages['en-US'] as Record<string, unknown>
  const wrapper = mount(Storage, {
    global: {
      plugins: [router, createI18n({
        legacy: false,
        locale: 'en-US',
        fallbackLocale: 'en-US',
        messages: { 'en-US': { container: pack } },
      })],
    },
  })
  await flushPromises()
  return wrapper
}

describe('Container Storage query context', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    containerApi.listWorkspaceClusters.mockResolvedValue([
      { id: 'cluster-a', name: 'a', status: 'online', target_type: 'kubernetes' },
      { id: 'cluster-b', name: 'b', status: 'online', target_type: 'kubernetes' },
    ])
    containerApi.listNamespaces.mockResolvedValue([])
    storageApi.listPersistentVolumes.mockResolvedValue([])
    storageApi.listPersistentVolumeClaims.mockResolvedValue([])
    storageApi.listStorageClasses.mockResolvedValue([
      { name: 'sc-fast', provisioner: 'csi.test', reclaimPolicy: 'Retain', allowVolumeExpansion: true, poolPolicy: '', createdAt: '', parameters: {}, labels: {} },
      { name: 'sc-slow', provisioner: 'csi.test', reclaimPolicy: 'Retain', allowVolumeExpansion: false, poolPolicy: '', createdAt: '', parameters: {}, labels: {} },
    ])
  })

  it('initializes cluster and StorageClass filtering from Resource binding query parameters', async () => {
    const wrapper = await mountStorage('/container/storage?target=cluster-b&cluster=cluster-b&offering=fast%2Fblock&storageClass=sc-fast')

    expect(storageApi.listStorageClasses).toHaveBeenCalledWith('cluster-b')
    expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('Storage Classes')
    expect((wrapper.get('input[type="search"]').element as HTMLInputElement).value).toBe('sc-fast')
    expect(wrapper.text()).toContain('Storage offering context: fast/block')
    expect(wrapper.text()).toContain('sc-fast')
    expect(wrapper.text()).not.toContain('sc-slow')
  })

  it('toggles StorageClass details in a read-only drawer', async () => {
    storageApi.listStorageClasses.mockResolvedValue([
      { name: 'sc-fast', provisioner: 'csi.test', reclaimPolicy: 'Retain', allowVolumeExpansion: true, poolPolicy: '', createdAt: '', parameters: { server: 'nfs.test' }, labels: { tier: 'fast' } },
    ])
    const wrapper = await mountStorage('/container/storage?storageClass=sc-fast')
    const detailButton = wrapper.get('.expand-button')

    expect(wrapper.find('.detail-row').exists()).toBe(false)
    expect(detailButton.attributes('aria-expanded')).toBe('false')

    await detailButton.trigger('click')

    const detailDrawer = wrapper.findAllComponents(NetworkDrawer).find((drawer) => drawer.props('title') === 'Storage Classes: sc-fast')
    expect(detailDrawer).toBeDefined()
    expect(detailDrawer?.props('modelValue')).toBe(true)
    expect(detailDrawer?.props('hideConfirm')).toBe(true)
    expect(document.body.textContent).toContain('server=nfs.test')
    expect(document.body.textContent).toContain('tier=fast')
    expect(detailButton.attributes('aria-expanded')).toBe('true')

    await detailButton.trigger('click')
    expect(detailDrawer?.props('modelValue')).toBe(false)
    expect(detailButton.attributes('aria-expanded')).toBe('false')
  })

  it('preserves direct-use defaults and does not select an unavailable target', async () => {
    const wrapper = await mountStorage('/container/storage?target=not-authorized')

    expect(storageApi.listStorageClasses).toHaveBeenCalledWith('cluster-a')
    expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('Persistent Volumes')
    expect((wrapper.get('input[type="search"]').element as HTMLInputElement).value).toBe('')
  })

  it('initializes the PVC namespace from compatibility query context', async () => {
    containerApi.listNamespaces.mockResolvedValue([{ name: 'team-a' }])
    const wrapper = await mountStorage('/container/storage?cluster=cluster-a&namespace=team-a')

    expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('Persistent Volume Claims')
    expect((wrapper.get('.compact-field select').element as HTMLSelectElement).value).toBe('cluster-a')
    expect((wrapper.findAll('.compact-field select')[1].element as HTMLSelectElement).value).toBe('team-a')
  })
})
