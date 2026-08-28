import { mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import AppMarket from '../AppMarket.vue'
import messages from '../../locales'
import * as api from '../../marketApi'

vi.mock('../../marketApi', () => ({
  listHarborProjects: vi.fn(),
  listProducts: vi.fn(),
  listRepositories: vi.fn(),
  listReleases: vi.fn(),
  deleteProduct: vi.fn(),
}))

const wrappers: VueWrapper[] = []

async function flushPage() {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

function mountMarket() {
  const i18n = createI18n({
    legacy: false,
    locale: 'en-US',
    messages: { 'en-US': { application: messages['en-US']! } },
  })
  const wrapper = mount(AppMarket, { attachTo: document.body, global: { plugins: [i18n] } })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.mocked(api.listHarborProjects).mockResolvedValue(['hnb'])
  vi.mocked(api.listProducts).mockResolvedValue({
    items: [{ id: 'product-1', name: 'mysql', display_name: 'MySQL', category: 'database', status: 'published', description: 'Database' }],
    total: 1,
    page: 1,
    pageSize: 9,
  })
  vi.mocked(api.listRepositories).mockResolvedValue([])
  vi.mocked(api.listReleases).mockResolvedValue([
    { id: 'release-1', product_id: 'product-1', version: '8.0.36', status: 'published' },
  ])
})

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
  document.body.style.overflow = ''
  vi.clearAllMocks()
})

describe('AppMarket dialogs', () => {
  it('opens normal and wide non-confirm modes in ApplicationDrawer', async () => {
    const wrapper = mountMarket()
    await flushPage()

    await wrapper.get('.scope-tabs .primary-button').trigger('click')
    expect(document.querySelector<HTMLElement>('.application-drawer')?.style.width).toBe('560px')
    expect(document.querySelector('.modal-mask')).toBeNull()

    document.querySelector<HTMLButtonElement>('.application-drawer__close')!.click()
    await nextTick()
    await wrapper.get('.product-title-button').trigger('click')
    await flushPage()

    expect(document.querySelector<HTMLElement>('.application-drawer')?.style.width).toBe('880px')
    expect(document.querySelector('.detail-layout')?.textContent).toContain('8.0.36')
  })

  it('keeps confirmation in the custom presentation and resolves both actions', async () => {
    const wrapper = mountMarket()
    await flushPage()

    await wrapper.get('.icon-action.danger').trigger('click')
    expect(document.querySelector('.modal-mask')).not.toBeNull()
    expect(document.querySelector('.application-drawer')).toBeNull()

    document.querySelector<HTMLButtonElement>('.modal-actions .secondary-button')!.click()
    await nextTick()
    expect(document.querySelector('.modal-mask')).toBeNull()
    expect(api.deleteProduct).not.toHaveBeenCalled()

    await wrapper.get('.icon-action.danger').trigger('click')
    document.querySelector<HTMLButtonElement>('.modal-actions .danger-button')!.click()
    await flushPage()
    expect(document.querySelector('.modal-mask')).toBeNull()
    expect(api.deleteProduct).toHaveBeenCalledWith('product-1')
  })

  it('renders submission validation through the shared drawer error state', async () => {
    const wrapper = mountMarket()
    await flushPage()

    await wrapper.get('.scope-tabs .primary-button').trigger('click')
    document.querySelector<HTMLButtonElement>('.application-drawer__footer .primary-button')!.click()
    await nextTick()

    expect(document.querySelector('[role="alert"]')?.textContent).toContain('Enter a product key.')
  })
})
