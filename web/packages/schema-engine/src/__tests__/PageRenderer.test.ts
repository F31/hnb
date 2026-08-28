/**
 * PageRenderer 组件测试（V2.6 §7.2 / §4.4 / §16.4）：
 *  - per-region runtimeProps 返回稳定引用：数据未变化时重算不产生新对象；
 *  - ConditionEvaluator 以 getter 注入上下文：权限变化后条件可见性重算；
 *  - schema 变更后已移除 region 的状态被清理。
 */
import { describe, it, expect, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import PageRenderer from '../components/PageRenderer.vue'
import { createComponentRegistry } from '../ComponentRegistry'
import { createDataSourceManager } from '../DataSourceManager'
import type { PageSchema } from '../types'

const FakeTable = defineComponent({
  name: 'FakeTable',
  props: { data: Array, loading: Boolean },
  setup(props) {
    return () => h('div', {
      'data-test': 'fake-table',
      'data-count': String(props.data?.length ?? -1),
    })
  },
})

function makeSchema(regions: unknown[]): PageSchema {
  return {
    apiVersion: 'ui.hnb.io/v1',
    kind: 'PageSchema',
    metadata: { id: 'test.page', revision: 1 },
    spec: { template: 'list', regions: regions as any },
  } as unknown as PageSchema
}

function makeDataSources(items: unknown[] = []) {
  const apiClient = {
    get: vi.fn(async () => items),
    post: vi.fn(),
    put: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  } as any
  const dataSources = createDataSourceManager(apiClient)
  dataSources.allowEndpoint('/api/v1/test')
  dataSources.registerEndpoint({ id: 'test.ds.endpoint', path: '/api/v1/test' })
  dataSources.registerDataSource({ id: 'test.ds', type: 'query', endpointId: 'test.ds.endpoint' })
  return { dataSources, apiClient }
}

describe('PageRenderer', () => {
  it('region runtimeProps 的 data 引用在重渲染/更新时保持稳定（V2.6 §7.2）', async () => {
    const registry = createComponentRegistry()
    registry.register({ type: 'FakeTable', component: FakeTable })
    const { dataSources } = makeDataSources([{ uid: 'a' }, { uid: 'b' }])
    const schema = makeSchema([
      { id: 'r1', componentType: 'FakeTable', props: { dataSource: 'test.ds' } },
    ])

    const wrapper = mount(PageRenderer, {
      props: { schema, registry, dataSources },
    })
    await flushPromises()

    const table = wrapper.findComponent(FakeTable)
    expect(table.exists()).toBe(true)
    const data1 = table.props('data') as unknown[]
    expect(data1).toHaveLength(2)

    // 与 region 数据无关的页面重渲染（texts 更新）不得更换 data 引用
    await wrapper.setProps({ texts: { 'page.title': '标题' } })
    await nextTick()
    expect(wrapper.findComponent(FakeTable).props('data')).toBe(data1)

    // schema 更新（同一 region id、同一 props 的新对象）复用已加载数据，
    // runtimeProps 中 data 引用保持稳定（V2.6 §7.4 未变化 region 复用状态）
    await wrapper.setProps({
      schema: makeSchema([
        { id: 'r1', componentType: 'FakeTable', props: { dataSource: 'test.ds' } },
      ]),
    })
    await flushPromises()
    expect(wrapper.findComponent(FakeTable).props('data')).toBe(data1)
  })

  it('条件 region 随权限上下文变化动态显隐（V2.6 §4.4 getter 上下文）', async () => {
    const registry = createComponentRegistry()
    registry.register({ type: 'FakeTable', component: FakeTable })
    const schema = makeSchema([
      {
        id: 'r2',
        componentType: 'FakeTable',
        condition: { all: [{ permission: 'cluster:read' }] },
      },
    ])

    const wrapper = mount(PageRenderer, {
      props: {
        schema,
        registry,
        conditionContext: { permissions: [] },
      },
    })
    await nextTick()
    // 默认拒绝：无权限时 region 不渲染
    expect(wrapper.findComponent(FakeTable).exists()).toBe(false)

    // 权限就绪后（新对象传入），同一渲染器实例应重算可见性
    await wrapper.setProps({ conditionContext: { permissions: ['cluster:read'] } })
    await nextTick()
    expect(wrapper.findComponent(FakeTable).exists()).toBe(true)

    // 权限收回后再次隐藏
    await wrapper.setProps({ conditionContext: { permissions: [] } })
    await nextTick()
    expect(wrapper.findComponent(FakeTable).exists()).toBe(false)
  })

  it('schema 变更后已移除 region 的数据状态被清理（V2.6 §7.4）', async () => {
    const registry = createComponentRegistry()
    registry.register({ type: 'FakeTable', component: FakeTable })
    const { dataSources } = makeDataSources([{ uid: 'a' }])
    const schema1 = makeSchema([
      { id: 'keep', componentType: 'FakeTable', props: { dataSource: 'test.ds' } },
      { id: 'gone', componentType: 'FakeTable', props: { dataSource: 'test.ds' } },
    ])

    const wrapper = mount(PageRenderer, {
      props: { schema: schema1, registry, dataSources },
    })
    await flushPromises()
    expect(wrapper.findAllComponents(FakeTable)).toHaveLength(2)

    // 移除 'gone' region：仅保留 'keep'
    const schema2 = makeSchema([
      { id: 'keep', componentType: 'FakeTable', props: { dataSource: 'test.ds' } },
    ])
    await wrapper.setProps({ schema: schema2 })
    await flushPromises()
    expect(wrapper.findAllComponents(FakeTable)).toHaveLength(1)

    // 再次切回含 'gone' 的 schema：'gone' 应能重新加载渲染，
    // 证明其旧状态已被清理而非残留错误。
    await wrapper.setProps({ schema: schema1 })
    await flushPromises()
    expect(wrapper.findAllComponents(FakeTable)).toHaveLength(2)
  })
})
