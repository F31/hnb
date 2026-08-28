import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import messages from '../../locales'
import SecurityPanel from '../SecurityPanel.vue'

const marketApi = vi.hoisted(() => ({
  getDBStatus: vi.fn(),
  listSecurityReports: vi.fn(),
}))

vi.mock('../../marketApi', () => marketApi)

let wrapper: VueWrapper | undefined

async function mountPanel(): Promise<VueWrapper> {
  const pack = messages['en-US'] as Record<string, unknown>
  wrapper = mount(SecurityPanel, {
    attachTo: document.body,
    global: {
      plugins: [createI18n({
        legacy: false,
        locale: 'en-US',
        messages: { 'en-US': { application: pack, common: { close: 'Close' } } },
      })],
    },
  })
  await flushPromises()
  return wrapper
}

function drawerButtons(): NodeListOf<HTMLButtonElement> {
  return document.querySelectorAll('.application-drawer__footer button')
}

async function selectFile(file: File): Promise<void> {
  const input = document.querySelector<HTMLInputElement>('.application-drawer input[type="file"]')!
  Object.defineProperty(input, 'files', { configurable: true, value: [file] })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  marketApi.getDBStatus.mockResolvedValue([])
  marketApi.listSecurityReports.mockResolvedValue([])
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('SecurityPanel vulnerability database drawer', () => {
  it('opens the update workflow while records remain page content', async () => {
    const page = await mountPanel()

    expect(page.find('.vulndb-list').exists()).toBe(true)
    await page.find('.vuln-db-update-button').trigger('click')

    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    expect(document.querySelector('.application-drawer__title')?.textContent).toBe('Vulnerability Database Update')
    expect(drawerButtons()[1].disabled).toBe(true)
  })

  it('rejects an invalid file and keeps confirm disabled', async () => {
    const page = await mountPanel()
    await page.find('.vuln-db-update-button').trigger('click')
    await selectFile(new File(['invalid'], 'trivy-db.zip'))

    expect(document.querySelector('[role="alert"]')?.textContent).toBe('Please upload a tgz file')
    expect(document.querySelector('.file-info')).toBeNull()
    expect(drawerButtons()[1].disabled).toBe(true)
  })

  it('keeps the TODO confirm open and cancel closes and resets the workflow', async () => {
    const log = vi.spyOn(console, 'log').mockImplementation(() => undefined)
    const page = await mountPanel()
    await page.find('.vuln-db-update-button').trigger('click')
    await selectFile(new File(['database'], 'trivy-offline.db.tgz'))

    expect(drawerButtons()[1].disabled).toBe(false)
    drawerButtons()[1].click()
    await flushPromises()
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    expect(log).toHaveBeenCalledWith('confirm', { file: 'trivy-offline.db.tgz' })

    drawerButtons()[0].click()
    await flushPromises()
    expect(document.querySelector('[role="dialog"]')).toBeNull()

    await page.find('.vuln-db-update-button').trigger('click')
    expect(drawerButtons()[1].disabled).toBe(true)
    log.mockRestore()
  })
})
