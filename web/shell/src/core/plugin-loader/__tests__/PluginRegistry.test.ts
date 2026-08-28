import { describe, it, expect, beforeEach } from 'vitest'
import { PluginRegistry } from '../PluginRegistry'

const FakeComponent = { render: () => {} }

describe('PluginRegistry', () => {
  let registry: PluginRegistry

  beforeEach(() => {
    registry = new PluginRegistry()
  })

  it('register + hasComponent + resolveComponent', async () => {
    registry.register('container', { Workloads: FakeComponent as any })
    expect(registry.hasComponent('container', 'Workloads')).toBe(true)
    const component = await registry.resolveComponent('container', 'Workloads')
    expect(component).toBe(FakeComponent)
  })

  it('resolveComponent throws for unknown component', async () => {
    await expect(registry.resolveComponent('container', 'Nope')).rejects.toThrow(
      /not found/,
    )
  })

  it('components of different plugins are isolated', () => {
    registry.register('p1', { Page: FakeComponent as any })
    expect(registry.hasComponent('p2', 'Page')).toBe(false)
  })

  it('unregister removes only the target plugin components', () => {
    registry.register('p1', { A: FakeComponent as any })
    registry.register('p2', { B: FakeComponent as any })
    registry.unregister('p1')
    expect(registry.hasComponent('p1', 'A')).toBe(false)
    expect(registry.hasComponent('p2', 'B')).toBe(true)
  })

  it('clear removes all components and plugins', () => {
    registry.register('p1', { A: FakeComponent as any })
    registry.clear()
    expect(registry.hasComponent('p1', 'A')).toBe(false)
    expect(registry.getAllPlugins()).toHaveLength(0)
  })

  it('re-registering a component replaces the previous one', async () => {
    const Other = { render: () => {} }
    registry.register('p1', { A: FakeComponent as any })
    registry.register('p1', { A: Other as any })
    expect(await registry.resolveComponent('p1', 'A')).toBe(Other)
  })

  it('远程 Bundle 白名单：相对路径与同源放行，跨域拒绝', () => {
    expect(registry.isEntryAllowed('/modules/a/index.js')).toBe(true)
    expect(registry.isEntryAllowed(`${window.location.origin}/modules/a/index.js`)).toBe(true)
    expect(registry.isEntryAllowed('https://evil.example.com/x.js')).toBe(false)
    expect(registry.isEntryAllowed('not a url')).toBe(false)
  })

  it('远程 Bundle 白名单：显式添加的域名放行', () => {
    registry.setAllowedDomain('https://plugins.hnb.local')
    expect(registry.isEntryAllowed('https://plugins.hnb.local/a/index.js')).toBe(true)
    expect(registry.isEntryAllowed('https://other.hnb.local/a/index.js')).toBe(false)
  })
})
