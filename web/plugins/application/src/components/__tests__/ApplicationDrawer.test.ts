import { mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import ApplicationDrawer from '../ApplicationDrawer.vue'

const wrappers: VueWrapper[] = []
const i18nMessages = {
  'en-US': { application: { common: { cancel: 'Cancel', confirm: 'Confirm', close: 'Close' } } },
}

function mountDrawer(props: Record<string, unknown> = {}, slots: Record<string, string> = {}) {
  const wrapper = mount(ApplicationDrawer, {
    attachTo: document.body,
    props: { modelValue: true, title: 'Create application', ...props },
    slots: { default: '<input aria-label="Application name">', ...slots },
    global: {
      plugins: [createI18n({
        legacy: false,
        locale: 'en-US',
        messages: i18nMessages,
      })],
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('ApplicationDrawer', () => {
  it('teleports a right-side labelled dialog and emits confirm', async () => {
    const wrapper = mountDrawer({ width: 640, error: 'Deployment failed' })
    await nextTick()

    const layer = document.querySelector<HTMLElement>('.application-drawer-layer')!
    const dialog = layer.querySelector<HTMLElement>('[role="dialog"]')!
    const title = document.getElementById(dialog.getAttribute('aria-labelledby')!)
    expect(layer.parentElement).toBe(document.body)
    expect(dialog.style.width).toBe('640px')
    expect(title?.textContent).toBe('Create application')
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(document.querySelector('[role="alert"]')?.textContent).toBe('Deployment failed')

    document.querySelectorAll<HTMLButtonElement>('.application-drawer__footer button')[1].click()
    await nextTick()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
  })

  it('supports hidden and disabled confirm actions', () => {
    const hidden = mountDrawer({ hideConfirm: true })
    expect(document.querySelectorAll('.application-drawer__footer button')).toHaveLength(1)
    hidden.unmount()
    document.body.innerHTML = ''

    mountDrawer({ confirmDisabled: true })
    expect(document.querySelectorAll<HTMLButtonElement>('.application-drawer__footer button')[1].disabled).toBe(true)
  })

  it('keeps backdrop close opt-in and blocks all close paths while busy', async () => {
    const locked = mountDrawer()
    document.querySelector<HTMLElement>('.application-drawer-layer')!.click()
    await nextTick()
    expect(locked.emitted('cancel')).toBeUndefined()
    locked.unmount()
    document.body.innerHTML = ''

    const closable = mountDrawer({ closeOnBackdrop: true })
    document.querySelector<HTMLElement>('.application-drawer-layer')!.click()
    await nextTick()
    expect(closable.emitted('update:modelValue')).toEqual([[false]])
    expect(closable.emitted('cancel')).toHaveLength(1)
    closable.unmount()
    document.body.innerHTML = ''

    const busy = mountDrawer({ busy: true, closeOnBackdrop: true })
    expect(document.querySelector('[role="dialog"]')?.getAttribute('aria-busy')).toBe('true')
    document.querySelector<HTMLElement>('.application-drawer-layer')!.click()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    document.querySelector<HTMLButtonElement>('.application-drawer__close')!.click()
    await nextTick()
    expect(busy.emitted('cancel')).toBeUndefined()
  })

  it('closes on Escape and traps Tab within the dialog', async () => {
    const wrapper = mountDrawer()
    await nextTick()
    const buttons = document.querySelectorAll<HTMLButtonElement>('.application-drawer button')
    const last = buttons[buttons.length - 1]
    last.focus()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(buttons[0])

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })

  it('lets only the topmost nested drawer handle Escape', async () => {
    const outer = mountDrawer({ title: 'Outer drawer' })
    const inner = mountDrawer({ title: 'Inner drawer' })
    await nextTick()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()

    expect(inner.emitted('cancel')).toHaveLength(1)
    expect(outer.emitted('cancel')).toBeUndefined()
  })

  it('focuses the first control, locks scrolling, and restores both after close', async () => {
    const opener = document.createElement('button')
    opener.textContent = 'Open'
    document.body.appendChild(opener)
    opener.focus()
    document.body.style.overflow = 'scroll'

    const Host = defineComponent({
      components: { ApplicationDrawer },
      setup() {
        return { visible: ref(true) }
      },
      template: '<ApplicationDrawer v-model="visible" title="Drawer"><input aria-label="Field"></ApplicationDrawer>',
    })
    const wrapper = mount(Host, {
      attachTo: document.body,
      global: { plugins: [createI18n({ legacy: false, locale: 'en-US', messages: i18nMessages })] },
    })
    wrappers.push(wrapper)
    await nextTick()

    expect(document.body.style.overflow).toBe('hidden')
    expect(document.activeElement).toBe(document.querySelector('.application-drawer__close'))
    document.querySelector<HTMLButtonElement>('.application-drawer__close')!.click()
    await nextTick()
    expect(document.querySelector('.application-drawer')).toBeNull()
    expect(document.body.style.overflow).toBe('scroll')
    expect(document.activeElement).toBe(opener)
  })
})
