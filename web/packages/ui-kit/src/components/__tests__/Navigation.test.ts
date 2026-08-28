import { describe, expect, it } from 'vitest'
import { mount, shallowMount } from '@vue/test-utils'
import HNBPagination from '../HNBPagination.vue'
import HNBTabs from '../HNBTabs.vue'
import HNBTable from '../HNBTable.vue'

describe('controlled tabs', () => {
  it('links tabs and panels and supports roving keyboard selection', async () => {
    const wrapper = mount(HNBTabs, {
      props: {
        modelValue: 'overview',
        ariaLabel: 'Details',
        tabs: [
          { id: 'overview', label: 'Overview' },
          { id: 'disabled', label: 'Disabled', disabled: true, disabledReason: 'Unavailable' },
          { id: 'nodes', label: 'Nodes' },
        ],
      },
      slots: { 'panel-overview': 'Overview panel', 'panel-nodes': 'Nodes panel' },
    })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs[0].attributes('aria-selected')).toBe('true')
    expect(tabs[0].attributes('aria-controls')).toBe(wrapper.findAll('[role="tabpanel"]')[0].attributes('id'))
    expect(tabs[1].attributes('aria-describedby')).toBeTruthy()
    await tabs[0].trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['nodes'])
  })
})

describe('controlled pagination', () => {
  it('announces exact page state and emits page and page-size changes', async () => {
    const wrapper = mount(HNBPagination, { props: { page: 2, pageSize: 20, total: 45 } })
    expect(wrapper.get('nav').attributes('aria-label')).toBe('Pagination')
    expect(wrapper.text()).toContain('Page 2 of 3, 45 items')
    const buttons = wrapper.findAll('button')
    await buttons[0].trigger('click')
    await buttons[1].trigger('click')
    await wrapper.get('select').setValue('50')
    expect(wrapper.emitted('update:page')?.map(event => event[0])).toEqual([1, 3])
    expect(wrapper.emitted('update:pageSize')?.[0]).toEqual([50])
    expect(wrapper.get('[aria-live="polite"]').text()).toContain('Page 2 of 3')
  })
})

describe('responsive table compatibility', () => {
  it('adds a named horizontal scroll region without changing table props', () => {
    const columns = [{ key: 'name', title: 'Name' }]
    const data = [{ name: 'one' }]
    const wrapper = shallowMount(HNBTable, { props: { columns, data, ariaLabel: 'Targets', minWidth: '720px' } })
    const region = wrapper.get('[role="region"]')
    expect(region.attributes('aria-label')).toBe('Targets')
    expect(region.attributes('tabindex')).toBe('0')
    expect(wrapper.props('columns')).toEqual(columns)
    expect(wrapper.props('data')).toEqual(data)
  })
})
