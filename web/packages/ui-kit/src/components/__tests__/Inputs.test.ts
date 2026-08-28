import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import HNBButton from '../HNBButton.vue'
import HNBSelectInput from '../HNBSelectInput.vue'
import HNBDateInput from '../HNBDateInput.vue'
import HNBDetailPanel from '../HNBDetailPanel.vue'
import HNBFormField from '../HNBFormField.vue'

describe('UI Kit baseline controls', () => {
  it('renders button variants through tokens-backed classes', () => {
    const wrapper = mount(HNBButton, { props: { variant: 'primary', size: 'large' }, slots: { default: 'Create' } })
    expect(wrapper.text()).toBe('Create')
    expect(wrapper.classes()).toContain('hnb-button--primary')
    expect(wrapper.classes()).toContain('hnb-button--large')
  })

  it('shows spinner and disables button when loading', () => {
    const wrapper = mount(HNBButton, { props: { loading: true }, slots: { default: 'Save' } })
    expect(wrapper.classes()).toContain('hnb-button--loading')
    expect(wrapper.attributes('disabled')).toBeDefined()
    expect(wrapper.attributes('aria-busy')).toBe('true')
    expect(wrapper.find('.hnb-button__spinner').exists()).toBe(true)
    expect(wrapper.find('.hnb-button__content--hidden').exists()).toBe(true)
    expect(wrapper.find('.hnb-button__content--hidden').text()).toBe('Save')
  })

  it('emits select value updates', async () => {
    const wrapper = mount(HNBSelectInput, {
      props: { options: [{ label: 'Prod', value: 'prod' }] },
    })
    await wrapper.find('select').setValue('prod')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['prod'])
  })

  it('emits date value updates', async () => {
    const wrapper = mount(HNBDateInput)
    await wrapper.find('input').setValue('2026-07-29')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['2026-07-29'])
  })

  it('renders detail values with empty fallback', () => {
    const wrapper = mount(HNBDetailPanel, { props: { items: [{ label: 'Status', value: null }] } })
    expect(wrapper.text()).toContain('Status')
    expect(wrapper.text()).toContain('-')
  })

  it('renders form field label, help, and error', () => {
    const wrapper = mount(HNBFormField, { props: { label: 'Name', help: 'Enter your name' }, slots: { default: '<input />' } })
    expect(wrapper.text()).toContain('Name')
    expect(wrapper.text()).toContain('Enter your name')
    expect(wrapper.find('.hnb-form-field__error').exists()).toBe(false)
  })

  it('shows error over help text', () => {
    const wrapper = mount(HNBFormField, { props: { label: 'Name', help: 'help text', error: 'error text' } })
    expect(wrapper.text()).toContain('error text')
    expect(wrapper.text()).not.toContain('help text')
  })

  it('shows required asterisk', () => {
    const wrapper = mount(HNBFormField, { props: { label: 'Name', required: true } })
    expect(wrapper.find('.hnb-form-field__label').text()).toContain('*')
  })

  it('sets aria-describedby on select when wrapped in form field', () => {
    const wrapper = mount(HNBFormField, {
      props: { label: 'Env', inputId: 'env', help: 'select environment' },
      slots: { default: h(HNBSelectInput, { options: [{ label: 'Prod', value: 'prod' }] }) },
    })
    const select = wrapper.find('select')
    expect(select.attributes('aria-describedby')).toBe('env-help')
  })

  it('sets aria-describedby on date input when wrapped in form field', () => {
    const wrapper = mount(HNBFormField, {
      props: { label: 'Date', inputId: 'date', help: 'pick a date' },
      slots: { default: h(HNBDateInput) },
    })
    const input = wrapper.find('input')
    expect(input.attributes('aria-describedby')).toBe('date-help')
  })
})
