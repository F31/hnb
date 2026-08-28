import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HNBVirtualList from '../HNBVirtualList.vue'

class ResizeObserverStub {
  observe() {}
  disconnect() {}
}

describe('HNBVirtualList', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', ResizeObserverStub)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('emits endReached once per data length', async () => {
    const data = Array.from({ length: 10 }, (_, id) => ({ id }))
    const wrapper = mount(HNBVirtualList, {
      props: { data, itemHeight: 20, height: 100, rowKey: 'id' },
      slots: { item: ({ item }: { item: unknown }) => String((item as { id: number }).id) },
    })
    const container = wrapper.get('.hnb-virtual-list')

    Object.defineProperty(container.element, 'scrollTop', { configurable: true, value: 160 })
    await container.trigger('scroll')
    expect(wrapper.emitted('endReached')).toHaveLength(1)

    Object.defineProperty(container.element, 'scrollTop', { configurable: true, value: 0 })
    await container.trigger('scroll')
    Object.defineProperty(container.element, 'scrollTop', { configurable: true, value: 160 })
    await container.trigger('scroll')
    expect(wrapper.emitted('endReached')).toHaveLength(1)

    await wrapper.setProps({ data: [...data, { id: 10 }] })
    Object.defineProperty(container.element, 'scrollTop', { configurable: true, value: 180 })
    await container.trigger('scroll')
    await nextTick()
    expect(wrapper.emitted('endReached')).toHaveLength(2)
  })

  it('does not request more data while loading or empty', async () => {
    const wrapper = mount(HNBVirtualList, {
      props: { data: [], itemHeight: 20, height: 100, loading: true },
    })

    expect(wrapper.text()).toContain('加载中...')
    expect(wrapper.emitted('endReached')).toBeUndefined()

    await wrapper.setProps({ loading: false })
    expect(wrapper.text()).toContain('暂无数据')
    expect(wrapper.emitted('endReached')).toBeUndefined()
  })
})
