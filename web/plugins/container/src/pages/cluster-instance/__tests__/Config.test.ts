import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import Config from '../Config.vue'
import messages from '../../../locales'

const containerApi = vi.hoisted(() => ({
  listNamespaces: vi.fn(),
  listWorkspaceClusters: vi.fn(),
}))
const configApi = vi.hoisted(() => ({
  createSecret: vi.fn(),
  deleteConfigMap: vi.fn(),
  deleteSecret: vi.fn(),
  listConfigMaps: vi.fn(),
  listSecrets: vi.fn(),
  saveConfigMap: vi.fn(),
}))

vi.mock('../../../api/containerApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../api/containerApi')>(),
  ...containerApi,
}))
vi.mock('../../../api/configApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../api/configApi')>(),
  ...configApi,
}))

let wrapper: VueWrapper | undefined

async function mountConfig(): Promise<VueWrapper> {
  const pack = messages['en-US'] as Record<string, unknown>
  wrapper = mount(Config, {
    attachTo: document.body,
    global: {
      plugins: [createI18n({
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

describe('Container Config drawers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    containerApi.listWorkspaceClusters.mockResolvedValue([
      { id: 'cluster-a', name: 'cluster-a', status: 'online', target_type: 'kubernetes' },
    ])
    containerApi.listNamespaces.mockResolvedValue([])
    configApi.listConfigMaps.mockResolvedValue([])
    configApi.listSecrets.mockResolvedValue([])
    configApi.saveConfigMap.mockResolvedValue(undefined)
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    document.body.innerHTML = ''
  })

  it('creates a ConfigMap from the drawer and preserves dynamic data rows', async () => {
    const page = await mountConfig()

    await page.get('.config-toolbar button').trigger('click')
    expect(document.querySelector('.hnb-drawer')).not.toBeNull()
    document.querySelector<HTMLElement>('.hnb-drawer-layer')!.click()
    await flushPromises()
    expect(document.querySelector('.hnb-drawer')).not.toBeNull()

    const addButton = Array.from(document.querySelectorAll<HTMLButtonElement>('.add-row'))[0]
    addButton.click()
    await flushPromises()
    expect(document.querySelectorAll('.data-row')).toHaveLength(2)

    const removeButtons = document.querySelectorAll<HTMLButtonElement>('.data-actions button[aria-label="Remove row"]')
    removeButtons[1].click()
    await flushPromises()
    expect(document.querySelectorAll('.data-row')).toHaveLength(1)

    const form = document.querySelector<HTMLFormElement>('.config-form')!
    const inputs = form.querySelectorAll<HTMLInputElement>('input')
    inputs[0].value = 'app-config'
    inputs[0].dispatchEvent(new Event('input'))
    inputs[1].value = 'setting'
    inputs[1].dispatchEvent(new Event('input'))
    const value = form.querySelector<HTMLTextAreaElement>('textarea')!
    value.value = 'enabled'
    value.dispatchEvent(new Event('input'))
    await flushPromises()

    const confirm = Array.from(document.querySelectorAll<HTMLButtonElement>('.hnb-drawer__footer button')).find((button) => button.textContent?.trim() === 'Confirm')!
    confirm.click()
    await flushPromises()

    expect(configApi.saveConfigMap).toHaveBeenCalledWith(
      'cluster-a',
      { name: 'app-config', namespace: 'argocd', data: { setting: 'enabled' } },
      undefined,
      {},
    )
    expect(document.querySelector('.hnb-drawer')).toBeNull()
  })

  it('shows redacted Secret YAML in a hide-confirm drawer', async () => {
    configApi.listSecrets.mockResolvedValue([{
      name: 'registry-secret',
      namespace: 'argocd',
      type: 'Opaque',
      dataKeys: ['password'],
      createdAt: '2026-01-01T00:00:00Z',
      protected: false,
    }])
    const page = await mountConfig()

    await page.findAll('[role="tab"]')[1].trigger('click')
    await flushPromises()
    await page.get('.name-link').trigger('click')
    await flushPromises()

    const yaml = document.querySelector<HTMLTextAreaElement>('.yaml-view')!
    expect(yaml.value).toContain('password: <redacted>')
    expect(document.querySelectorAll('.hnb-drawer__footer button')).toHaveLength(1)
    expect(document.querySelector('.hnb-drawer__footer')?.textContent).toContain('Cancel')
    expect(document.querySelector('.hnb-drawer__footer')?.textContent).not.toContain('Confirm')
  })
})
