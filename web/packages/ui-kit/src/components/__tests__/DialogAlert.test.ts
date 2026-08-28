import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HNBAlert from '../HNBAlert.vue'
import HNBConfirmation from '../HNBConfirmation.vue'
import HNBDialog from '../HNBDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('dialog and confirmation', () => {
  it('labels the dialog, traps focus, closes with Escape, and restores focus', async () => {
    const trigger = document.createElement('button')
    trigger.textContent = 'Open'
    document.body.append(trigger)
    trigger.focus()
    const wrapper = mount(HNBDialog, {
      attachTo: document.body,
      props: { modelValue: true, title: 'Confirm change', description: 'Review the impact' },
      slots: {
        default: '<button id="first">First</button><button id="last">Last</button>',
      },
    })
    await nextTick()

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toBe('Confirm change')
    const first = document.querySelector<HTMLElement>('#first')!
    const last = document.querySelector<HTMLElement>('#last')!
    last.focus()
    dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    expect(document.activeElement).toBe(dialog.querySelector('button'))

    dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([false])
    await wrapper.setProps({ modelValue: false })
    expect(document.activeElement).toBe(trigger)
    wrapper.unmount()
  })

  it('keeps a busy dialog open and associates async errors', async () => {
    const wrapper = mount(HNBDialog, {
      attachTo: document.body,
      props: { modelValue: true, title: 'Working', busy: true, error: 'Request failed' },
    })
    await nextTick()
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(dialog.getAttribute('aria-busy')).toBe('true')
    expect(document.getElementById(dialog.getAttribute('aria-errormessage')!)?.textContent).toBe('Request failed')
    wrapper.unmount()
  })

  it('requires explicit acknowledgement for dangerous confirmation', async () => {
    const wrapper = mount(HNBConfirmation, {
      attachTo: document.body,
      props: { modelValue: true, title: 'Remove item', danger: true, requireAcknowledgement: true },
    })
    await nextTick()
    const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>('.hnb-dialog__footer button'))
    expect(buttons[1].disabled).toBe(true)
    document.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click()
    await nextTick()
    expect(buttons[1].disabled).toBe(false)
    buttons[1].click()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
    wrapper.unmount()
  })
})

describe('alert', () => {
  it('uses assertive semantics only when requested and exposes dismiss naming', async () => {
    const wrapper = mount(HNBAlert, {
      props: { title: 'Failure', semantic: 'error', live: 'assertive', dismissLabel: 'Dismiss failure', dismissible: true },
      slots: { default: 'Try again later' },
    })
    expect(wrapper.attributes('role')).toBe('alert')
    expect(wrapper.attributes('aria-live')).toBe('assertive')
    expect(wrapper.text()).toContain('Try again later')
    const dismiss = wrapper.get('button')
    expect(dismiss.attributes('aria-label')).toBe('Dismiss failure')
    await dismiss.trigger('click')
    expect(wrapper.emitted('dismiss')).toHaveLength(1)
  })
})
