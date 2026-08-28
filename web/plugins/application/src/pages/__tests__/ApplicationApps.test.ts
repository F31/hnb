import { mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import ApplicationApps from '../ApplicationApps.vue'
import messages from '../../locales'

const apiMocks = vi.hoisted(() => ({
  listGroups: vi.fn().mockResolvedValue([]),
  createGroup: vi.fn().mockResolvedValue({ id: 'group-1', name: 'payments' }),
}))

vi.mock('../../marketApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../marketApi')>(),
  ...apiMocks,
}))

const wrappers: VueWrapper[] = []

async function mountPage(path = '/application/monolith') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/application/:kind', component: ApplicationApps }],
  })
  await router.push(path)
  await router.isReady()

  const locale = messages['en-US'] as any
  const applicationLocale = { ...locale, common: { ...locale.common, loading: 'Loading' } }
  const wrapper = mount(ApplicationApps, {
    attachTo: document.body,
    global: {
      plugins: [
        router,
        createI18n({
          legacy: false,
          locale: 'en-US',
          messages: { 'en-US': { application: applicationLocale, common: applicationLocale.common } },
        }),
      ],
    },
  })
  wrappers.push(wrapper)
  await nextTick()
  await nextTick()
  return wrapper
}

function drawer() {
  return document.querySelector<HTMLElement>('.application-drawer')!
}

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.innerHTML = ''
  document.body.style.overflow = ''
  vi.clearAllMocks()
})

describe('ApplicationApps drawers', () => {
  it('opens the ConfigMap create drawer and keeps validation in the drawer', async () => {
    const wrapper = await mountPage()
    await wrapper.findAll('.app-tab').find((tab) => tab.text() === 'Configuration')!.trigger('click')
    await wrapper.find('.action-bar .primary-button').trigger('click')

    expect(drawer().querySelector('.application-drawer__title')?.textContent).toBe('Add ConfigMap')
    expect(document.querySelector('.dialog-overlay')).toBeNull()

    drawer().querySelectorAll<HTMLButtonElement>('.application-drawer__footer button')[1].click()
    await nextTick()
    expect(drawer().textContent).toContain('Config name is required')
    expect(drawer().textContent).toContain('Version is required')
  })

  it('opens ConfigMap metadata detail and edit drawers from table actions', async () => {
    const wrapper = await mountPage()
    await wrapper.findAll('.app-tab').find((tab) => tab.text() === 'Configuration')!.trigger('click')
    await nextTick()

    document.querySelector<HTMLAnchorElement>('.config-link')!.click()
    await nextTick()
    expect(drawer().querySelector('.application-drawer__title')?.textContent).toBe('View YAML')
    expect(drawer().textContent).toContain('sql')
    expect(drawer().textContent).toContain('spacefbponvcx')
    expect(drawer().querySelectorAll('.application-drawer__footer button')).toHaveLength(1)

    drawer().querySelector<HTMLButtonElement>('.application-drawer__close')!.click()
    await nextTick()
    const edit = Array.from(document.querySelectorAll<HTMLAnchorElement>('.action-link')).find((link) => link.textContent === 'Edit')!
    edit.click()
    await nextTick()

    expect(drawer().querySelector('.application-drawer__title')?.textContent).toBe('Edit ConfigMaps')
    expect(drawer().querySelector<HTMLInputElement>('input[placeholder="Enter name"]')?.value).toBe('sql')
  })

  it('opens the application group drawer and submits through the existing API', async () => {
    const wrapper = await mountPage('/application/microservices')
    await wrapper.find('.panel-toolbar .primary-button').trigger('click')
    expect(drawer().querySelector('.application-drawer__title')?.textContent).toBe('Create App Group')

    drawer().querySelectorAll<HTMLButtonElement>('.application-drawer__footer button')[1].click()
    await nextTick()
    expect(drawer().textContent).toContain('请输入名称')

    const name = drawer().querySelector<HTMLInputElement>('input[placeholder="Enter name"]')!
    name.value = 'payments'
    name.dispatchEvent(new Event('input', { bubbles: true }))
    drawer().querySelectorAll<HTMLButtonElement>('.application-drawer__footer button')[1].click()
    await vi.waitFor(() => expect(apiMocks.createGroup).toHaveBeenCalledWith({ name: 'payments', group_type: 'custom' }))
    await vi.waitFor(() => expect(document.querySelector('.application-drawer')).toBeNull())
  })

  it('continues delegating application creation to AppCreateWizard', async () => {
    const wrapper = await mountPage()
    await wrapper.find('.panel-toolbar .primary-button').trigger('click')
    expect(wrapper.findComponent({ name: 'AppCreateWizard' }).exists()).toBe(true)
  })
})
