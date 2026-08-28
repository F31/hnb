import { DOMWrapper, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import AccessForm from '../AccessForm.vue'
import NetworkDrawer from '../../network/NetworkDrawer.vue'
import messages from '../../../locales'

const containerApi = vi.hoisted(() => ({
  listNamespaces: vi.fn(),
  listWorkspaceClusters: vi.fn(),
}))
const accessApi = vi.hoisted(() => ({
  listAccessIngresses: vi.fn(),
  listAccessNetworkPolicies: vi.fn(),
  listAccessServices: vi.fn(),
  saveAccessIngress: vi.fn(),
  saveAccessNetworkPolicy: vi.fn(),
  saveAccessService: vi.fn(),
}))

vi.mock('../../../api/containerApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../api/containerApi')>(),
  ...containerApi,
}))
vi.mock('../../../api/accessApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../api/accessApi')>(),
  ...accessApi,
}))
vi.mock('../Access.vue', () => ({ default: { template: '<div data-testid="access-list" />' } }))

async function mountForm(path = '/container/instances/access/service/create'): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/container/instances/access', component: { template: '<div />' } },
      { path: '/container/instances/access/service/create', component: AccessForm },
    ],
  })
  await router.push(path)
  await router.isReady()
  const pack = messages['en-US'] as Record<string, unknown>
  const wrapper = mount(AccessForm, {
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
  return { wrapper, router }
}

describe('Container Access routed form drawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    containerApi.listWorkspaceClusters.mockResolvedValue([
      { id: 'cluster-a', name: 'a', status: 'online', target_type: 'kubernetes' },
    ])
    containerApi.listNamespaces.mockResolvedValue([])
    accessApi.listAccessServices.mockResolvedValue([])
    accessApi.listAccessIngresses.mockResolvedValue([])
    accessApi.listAccessNetworkPolicies.mockResolvedValue([])
    accessApi.saveAccessService.mockResolvedValue(undefined)
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('opens over the Access list and returns to the matching list tab when cancelled', async () => {
    const { wrapper, router } = await mountForm()
    const drawer = wrapper.getComponent(NetworkDrawer)

    expect(wrapper.get('[data-testid="access-list"]')).toBeTruthy()
    expect(drawer.props('modelValue')).toBe(true)

    drawer.vm.$emit('cancel')
    await flushPromises()

    expect(router.currentRoute.value.fullPath).toBe('/container/instances/access?tab=service')
    wrapper.unmount()
  })

  it.each(['form submit', 'drawer confirm'])('uses save for %s', async (action) => {
    const { wrapper, router } = await mountForm()
    const drawer = wrapper.getComponent(NetworkDrawer)
    const form = new DOMWrapper(document.querySelector('form') as HTMLFormElement)
    await form.get('input[type="text"]').setValue('my-service')

    if (action === 'form submit') await form.trigger('submit')
    else drawer.vm.$emit('confirm')
    await flushPromises()

    expect(accessApi.saveAccessService).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.fullPath).toBe('/container/instances/access?tab=service&saved=1')
    wrapper.unmount()
  })
})
