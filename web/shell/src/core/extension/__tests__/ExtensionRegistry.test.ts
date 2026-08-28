import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { getExtensionRegistry } from '../ExtensionRegistry'

describe('ExtensionRegistry', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getExtensionRegistry().clear()
  })

  it('registers points and accepts contributions', () => {
    const reg = getExtensionRegistry()
    reg.registerPoint('dashboard.widgets')
    reg.contribute('dashboard.widgets', { pluginId: 'ai', payload: { key: 'gpu' } })
    const items = reg.getContributions<{ key: string }>('dashboard.widgets')
    expect(items).toHaveLength(1)
    expect(items[0].payload.key).toBe('gpu')
  })

  it('rejects duplicate point registration but ensurePoint is idempotent', () => {
    const reg = getExtensionRegistry()
    reg.registerPoint('p1')
    expect(() => reg.registerPoint('p1')).toThrow(/already registered/)
    expect(() => reg.ensurePoint('p1')).not.toThrow()
    expect(reg.hasPoint('p1')).toBe(true)
  })

  it('rejects contributions to unknown points', () => {
    const reg = getExtensionRegistry()
    expect(() =>
      reg.contribute('no.such.point', { pluginId: 'x', payload: null }),
    ).toThrow(/unknown extension point/)
  })

  it('sorts contributions by priority descending, stable for ties', () => {
    const reg = getExtensionRegistry()
    reg.registerPoint('p')
    reg.contribute('p', { pluginId: 'a', payload: 'a', priority: 1 })
    reg.contribute('p', { pluginId: 'b', payload: 'b', priority: 5 })
    reg.contribute('p', { pluginId: 'c', payload: 'c' })
    expect(reg.getContributions<string>('p').map((c) => c.payload)).toEqual(['b', 'a', 'c'])
  })

  it('removeByPlugin reclaims all contributions of that plugin', () => {
    const reg = getExtensionRegistry()
    reg.registerPoint('p1')
    reg.registerPoint('p2')
    reg.contribute('p1', { pluginId: 'ai', payload: 1 })
    reg.contribute('p1', { pluginId: 'system', payload: 2 })
    reg.contribute('p2', { pluginId: 'ai', payload: 3 })
    reg.removeByPlugin('ai')
    expect(reg.getContributions('p1').map((c) => c.pluginId)).toEqual(['system'])
    expect(reg.getContributions('p2')).toEqual([])
  })
})
