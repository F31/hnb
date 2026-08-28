import { describe, it, expect, vi } from 'vitest'
import { definePlugin, definePluginInstance, createPluginLogger } from '../index'

describe('plugin-sdk', () => {
  it('definePlugin 返回传入的插件对象', () => {
    const plugin = {
      name: 'demo',
      version: '1.0.0',
      displayName: 'Demo',
      tier: 'T1',
      enabled: true,
      mode: 'local',
      components: {},
      create: async () => ({}),
    } as const
    expect(definePlugin(plugin as any)).toBe(plugin)
  })

  it('definePluginInstance 返回传入的实例对象', () => {
    const instance = { name: 'demo' }
    expect(definePluginInstance(instance)).toBe(instance)
  })

  it('createPluginLogger 输出带插件命名空间前缀', () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const logger = createPluginLogger('container')
    logger.warn('something')
    expect(spy).toHaveBeenCalledWith('[hnb-plugin:container]', 'something')
    spy.mockRestore()
  })
})
