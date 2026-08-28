import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import messages from '../../../locales'
import NetworkDrawer from '../NetworkDrawer.vue'

function mountDrawer(props: Record<string, unknown> = {}) {
  const pack = messages['en-US'] as Record<string, unknown>
  return mount(NetworkDrawer, {
    attachTo: document.body,
    props: { modelValue: true, title: 'Create resource', ...props },
    slots: { default: '<form><input aria-label="Name"></form>' },
    global: {
      plugins: [createI18n({
        legacy: false,
        locale: 'en-US',
        messages: { 'en-US': { container: pack } },
      })],
    },
  })
}

afterEach(() => { document.body.innerHTML = '' })

describe('NetworkDrawer', () => {
  it('renders a right-side dialog and emits confirm', async () => {
    const wrapper = mountDrawer()
    expect(document.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('Create resource')
    document.querySelectorAll<HTMLButtonElement>('.hnb-drawer__footer button')[1].click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
  })

  it('supports read-only and disabled-confirm states', () => {
    const readOnly = mountDrawer({ hideConfirm: true })
    expect(document.querySelectorAll('.hnb-drawer__footer button')).toHaveLength(1)
    readOnly.unmount()
    document.body.innerHTML = ''

    mountDrawer({ confirmDisabled: true })
    expect(document.querySelectorAll<HTMLButtonElement>('.hnb-drawer__footer button')[1].disabled).toBe(true)
  })

  it('can prevent backdrop close and blocks close while busy', async () => {
    const backdropLocked = mountDrawer({ closeOnBackdrop: false })
    document.querySelector<HTMLElement>('.hnb-drawer-layer')!.click()
    await backdropLocked.vm.$nextTick()
    expect(backdropLocked.emitted('cancel')).toBeUndefined()
    backdropLocked.unmount()
    document.body.innerHTML = ''

    const busy = mountDrawer({ busy: true })
    document.querySelector<HTMLButtonElement>('.hnb-drawer__close')!.click()
    await busy.vm.$nextTick()
    expect(busy.emitted('cancel')).toBeUndefined()
  })
})
