import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import * as uiKit from '../../index'
import HNBOperationProgress from '../HNBOperationProgress.vue'
import HNBPageState from '../HNBPageState.vue'
import HNBSkeleton from '../HNBSkeleton.vue'
import HNBStatusGroup from '../HNBStatusGroup.vue'

describe('page state matrix', () => {
  for (const state of ['loading', 'empty', 'error', 'no-permission', 'offline', 'incompatible'] as const) {
    it(`renders ${state} with accessible state semantics`, () => {
      const wrapper = mount(HNBPageState, { props: { state, title: `${state} title`, description: 'State details' } })
      expect(wrapper.attributes('data-state')).toBe(state)
      expect(wrapper.attributes('aria-label')).toBe(`${state} title`)
      if (state !== 'loading') expect(wrapper.text()).toContain(`${state} title`)
      if (state === 'loading') expect(wrapper.get('[role="status"]').attributes('aria-label')).toBe('loading title')
      if (state === 'offline') expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    })
  }

  it('emits the shared recovery action', async () => {
    const wrapper = mount(HNBPageState, { props: { state: 'error', title: 'Failed', actionText: 'Retry' } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('action')).toHaveLength(1)
  })
})

describe('status and progress', () => {
  it('keeps dictionary statuses separate and retains last-known context', () => {
    const wrapper = mount(HNBStatusGroup, {
      props: {
        ariaLabel: 'Target states',
        lastKnownLabel: 'Last known yesterday',
        items: [
          { key: 'lifecycle', label: 'Lifecycle', valueLabel: 'Active', semantic: 'success' },
          { key: 'freshness', label: 'Freshness', valueLabel: 'Stale', semantic: 'warning' },
        ],
      },
    })
    expect(wrapper.get('[role="group"]').attributes('aria-label')).toBe('Target states')
    expect(wrapper.findAll('.status-badge')).toHaveLength(2)
    expect(wrapper.text()).toContain('Last known yesterday')
  })

  it('exposes determinate progress and current step semantics', () => {
    const wrapper = mount(HNBOperationProgress, {
      props: {
        label: 'Deployment progress',
        value: 150,
        statusMessage: 'Running validation',
        steps: [
          { id: 'one', label: 'Queued', status: 'success' },
          { id: 'two', label: 'Validate', status: 'running' },
        ],
      },
    })
    expect(wrapper.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('100')
    expect(wrapper.get('[aria-current="step"]').text()).toContain('Validate')
    expect(wrapper.get('[aria-live="polite"]').text()).toBe('Running validation')
  })

  it('supports localized skeleton labels and shapes', () => {
    const wrapper = mount(HNBSkeleton, { props: { label: 'Loading nodes', variant: 'table' } })
    expect(wrapper.attributes('role')).toBe('status')
    expect(wrapper.attributes('aria-label')).toBe('Loading nodes')
    expect(wrapper.classes()).toContain('hnb-skeleton--table')
  })
})

describe('public API compatibility', () => {
  it('retains old exports and publishes the reusable primitives', () => {
    const expected = [
      'HNBTable', 'HNBButton', 'EmptyState', 'ErrorState', 'HNBSkeleton',
      'HNBDialog', 'HNBConfirmation', 'HNBAlert', 'HNBLiveRegion', 'HNBTabs',
      'HNBPagination', 'HNBStatusGroup', 'HNBPageState', 'HNBOperationProgress',
    ]
    expect(expected.every(name => name in uiKit)).toBe(true)
  })
})
