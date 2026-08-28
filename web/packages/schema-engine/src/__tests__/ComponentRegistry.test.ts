import { describe, it, expect, beforeEach } from 'vitest'
import { ComponentRegistry } from '../ComponentRegistry'

const FakeComponent = { render: () => {} }

describe('ComponentRegistry', () => {
  let registry: ComponentRegistry

  beforeEach(() => {
    registry = new ComponentRegistry()
  })

  it('register + resolve + has', () => {
    registry.register({ type: 'DataTable', component: FakeComponent as any })
    expect(registry.has('DataTable')).toBe(true)
    expect(registry.resolve('DataTable')).toBe(FakeComponent)
    expect(registry.resolve('Unknown')).toBeNull()
  })

  it('缺 type 或 component 拒绝注册', () => {
    expect(() => registry.register({ type: '', component: FakeComponent as any })).toThrow()
    expect(() => registry.register({ type: 'X' } as any)).toThrow()
  })

  it('unregisterPlugin 只移除该插件的组件', () => {
    registry.register({ type: 'A', component: FakeComponent as any, pluginId: 'p1' })
    registry.register({ type: 'B', component: FakeComponent as any, pluginId: 'p2' })
    registry.unregisterPlugin('p1')
    expect(registry.has('A')).toBe(false)
    expect(registry.has('B')).toBe(true)
  })

  it('props 校验：required 缺失不通过', () => {
    registry.register({
      type: 'StatusBadge',
      component: FakeComponent as any,
      propsSchema: {
        type: 'object',
        required: ['label'],
        properties: { label: { type: 'string' } },
      },
    })
    expect(registry.validateProps('StatusBadge', {}).valid).toBe(false)
    expect(registry.validateProps('StatusBadge', { label: '运行中' }).valid).toBe(true)
  })

  it('props 校验：additionalProperties:false 拒绝未声明属性', () => {
    registry.register({
      type: 'MetricCard',
      component: FakeComponent as any,
      propsSchema: {
        type: 'object',
        properties: { title: { type: 'string' } },
        additionalProperties: false,
      },
    })
    const result = registry.validateProps('MetricCard', {
      title: 'CPU',
      onClick: 'evil()',
    } as any)
    expect(result.valid).toBe(false)
    expect(result.errors[0]).toMatch(/unknown property/)
  })

  it('props 校验：enum 与类型检查', () => {
    registry.register({
      type: 'Badge',
      component: FakeComponent as any,
      propsSchema: {
        type: 'object',
        properties: {
          semantic: { enum: ['success', 'error'] },
          count: { type: 'number' },
        },
      },
    })
    expect(registry.validateProps('Badge', { semantic: 'warning' }).valid).toBe(false)
    expect(registry.validateProps('Badge', { count: '3' as any }).valid).toBe(false)
    expect(registry.validateProps('Badge', { semantic: 'success', count: 3 }).valid).toBe(true)
  })

  it('未注册类型或无 propsSchema 的组件校验直接通过', () => {
    expect(registry.validateProps('Nope', { any: 1 }).valid).toBe(true)
    registry.register({ type: 'Plain', component: FakeComponent as any })
    expect(registry.validateProps('Plain', { any: 1 }).valid).toBe(true)
  })

  it('validateProps 缓存键对嵌套 props 不碰撞（V2.6 §8.2）', () => {
    registry.register({
      type: 'Nested',
      component: FakeComponent as any,
      propsSchema: {
        type: 'object',
        properties: {
          a: {
            type: 'object',
            properties: { x: { type: 'number' }, y: { type: 'number' } },
            additionalProperties: false,
          },
          b: { type: 'number' },
        },
      },
    })
    expect(registry.validateProps('Nested', { a: { x: 1 }, b: 1 }).valid).toBe(true)
    // 旧实现 `JSON.stringify(props, Object.keys(props).sort())` 会丢弃嵌套键，
    // 使 {a:{y:'bad'}} 与 {a:{x:1}} 产生相同缓存键，错误返回缓存中的 true。
    expect(registry.validateProps('Nested', { a: { y: 'bad' }, b: 1 }).valid).toBe(false)
    expect(registry.validateProps('Nested', { a: { y: 2 }, b: 1 }).valid).toBe(true)
    expect(registry.validateProps('Nested', { a: { x: 1 }, b: 'bad' } as any).valid).toBe(false)
  })
})
