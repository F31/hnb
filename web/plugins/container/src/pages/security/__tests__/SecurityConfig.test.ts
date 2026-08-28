import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import messages from '../../../locales'
import SecurityConfig from '../SecurityConfig.vue'

const securityApi = vi.hoisted(() => ({
  getVulnerabilityDatabaseRecords: vi.fn(),
  getVulnerabilityScanProjects: vi.fn(),
  saveVulnerabilityScanRules: vi.fn(),
  uploadVulnerabilityDatabase: vi.fn(),
}))

vi.mock('../../../api/securityApi', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../api/securityApi')>(),
  ...securityApi,
}))

let wrapper: VueWrapper | undefined

async function mountPage(): Promise<VueWrapper> {
  const pack = messages['zh-CN'] as Record<string, unknown>
  wrapper = mount(SecurityConfig, {
    attachTo: document.body,
    global: {
      plugins: [createI18n({
        legacy: false,
        locale: 'zh-CN',
        messages: { 'zh-CN': { container: pack } },
      })],
    },
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  securityApi.getVulnerabilityDatabaseRecords.mockResolvedValue([])
  securityApi.getVulnerabilityScanProjects.mockResolvedValue([{
    id: 'project-a',
    name: 'Project A',
    autoScan: true,
    scheduledScan: true,
    frequency: 'daily',
    scanTime: '02:00',
  }])
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

describe('Container security configuration drawers', () => {
  it('opens vulnerability database update in a drawer with submit disabled until a file is selected', async () => {
    const page = await mountPage()
    const update = page.findAll('button').find((button) => button.text().includes('漏洞库更新'))!
    await update.trigger('click')

    expect(document.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('漏洞库更新')
    const buttons = document.querySelectorAll<HTMLButtonElement>('.hnb-drawer__footer button')
    expect(buttons[1].disabled).toBe(true)
  })

  it('opens an individual project scan setting in a drawer', async () => {
    const page = await mountPage()
    const scanTab = page.findAll('[role="tab"]').find((tab) => tab.text().includes('漏洞扫描设置'))!
    await scanTab.trigger('click')
    await flushPromises()
    const settings = page.findAll('button').filter((button) => button.text().includes('扫描设置'))
    await settings.at(-1)!.trigger('click')

    expect(document.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('扫描设置')
    expect(document.querySelector('.rule-form')).not.toBeNull()
  })
})
