import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import AppCreateWizard from '../AppCreateWizard.vue'

const labels: Record<string, string> = {
  'application.createWizard.title': 'Create application',
  'application.createWizard.breadcrumbList': 'Applications',
  'application.createWizard.steps.basic': 'Basic information',
  'application.createWizard.steps.runtime': 'Runtime settings',
  'application.createWizard.steps.confirm': 'Confirm',
  'application.createWizard.basic.choosePackage': 'Choose package',
  'application.createWizard.packageModal.title': 'Select package',
  'application.common.cancel': 'Cancel',
  'application.common.prev': 'Previous',
  'application.common.next': 'Next',
  'application.common.confirm': 'Confirm',
  'application.createWizard.confirm.deploy': 'Create',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => labels[key] || key }),
}))

const wrappers: VueWrapper[] = []

function mountWizard() {
  const wrapper = mount(AppCreateWizard, {
    attachTo: document.body,
    props: { mode: 'monolith', appKindLabel: 'Monolith' },
  })
  wrappers.push(wrapper)
  return wrapper
}

function button(text: string, root: ParentNode = document) {
  const match = Array.from(root.querySelectorAll<HTMLButtonElement>('button'))
    .find(item => item.textContent?.trim() === text)
  if (!match) throw new Error(`Button not found: ${text}`)
  return match
}

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('AppCreateWizard', () => {
  it('opens as a wide right-side drawer', async () => {
    mountWizard()
    await nextTick()

    const layers = document.querySelectorAll('.application-drawer-layer')
    const dialog = document.querySelector<HTMLElement>('.application-drawer')!
    expect(layers).toHaveLength(1)
    expect(dialog.style.width).toBe('960px')
    expect(dialog.getAttribute('role')).toBe('dialog')
    expect(document.querySelector('.application-drawer__title')?.textContent).toBe('Create application')
    expect(document.querySelector('.stepper li.active')?.textContent).toContain('Basic information')
  })

  it('preserves all steps and the custom navigation footer', async () => {
    mountWizard()
    await nextTick()

    expect(button('Next')).toBeTruthy()
    expect(document.querySelector('.application-drawer__footer')?.textContent).not.toContain('Previous')

    button('Next').click()
    await nextTick()
    expect(document.querySelector('.stepper li.active')?.textContent).toContain('Runtime settings')
    expect(button('Previous')).toBeTruthy()

    button('Next').click()
    await nextTick()
    expect(document.querySelector('.stepper li.active')?.textContent).toContain('Confirm')
    expect(button('Create')).toBeTruthy()

    button('Previous').click()
    await nextTick()
    expect(document.querySelector('.stepper li.active')?.textContent).toContain('Runtime settings')
  })

  it('opens a stacked package selector drawer and keeps package selection behavior', async () => {
    mountWizard()
    await nextTick()

    button('Choose package').click()
    await nextTick()

    const layers = document.querySelectorAll<HTMLElement>('.application-drawer-layer')
    const dialogs = document.querySelectorAll<HTMLElement>('.application-drawer')
    expect(layers).toHaveLength(2)
    expect(layers[1].compareDocumentPosition(layers[0]) & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy()
    expect(dialogs[1].style.width).toBe('700px')
    expect(dialogs[1].querySelector('.application-drawer__title')?.textContent).toBe('Select package')

    button('nginx', dialogs[1]).click()
    const versions = dialogs[1].querySelectorAll<HTMLInputElement>('input[type="radio"]')
    versions[1].click()
    button('Confirm', dialogs[1]).click()
    await nextTick()

    expect(document.querySelectorAll('.application-drawer-layer')).toHaveLength(1)
    expect(document.querySelector<HTMLInputElement>('.package-picker input')?.value)
      .toBe('nginx:v1-1776073488551')
  })

  it('emits close from drawer controls and submit from the final step', async () => {
    const wrapper = mountWizard()
    await nextTick()

    document.querySelector<HTMLButtonElement>('.application-drawer__close')!.click()
    await nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)

    button('Next').click()
    await nextTick()
    button('Next').click()
    await nextTick()
    button('Create').click()
    await nextTick()
    expect(wrapper.emitted('submit')).toHaveLength(1)
  })
})
