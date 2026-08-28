import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { PluginManager } from '../PluginManager'
import type { PluginLoader } from '@/core/plugin-loader/PluginLoader'
import { getPluginRegistry } from '@/core/plugin-loader/PluginRegistry'

const FakeComponent = { render: () => {} }

function makeManifest(name: string): any {
  return { name, enabled: true, mode: 'local', tier: 'T1' }
}

function makeInstance(name: string) {
  return {
    name,
    onActivate: vi.fn(async () => {}),
    onDeactivate: vi.fn(async () => {}),
  }
}

function makeLoader(overrides: Partial<Record<string, any>> = {}) {
  return {
    loadLocalPlugin: vi.fn(),
    loadRemotePlugin: vi.fn(),
    getModule: vi.fn(),
    getPlugin: vi.fn(),
    clear: vi.fn(),
    ...overrides,
  } as unknown as PluginLoader & {
    loadLocalPlugin: ReturnType<typeof vi.fn>
    getModule: ReturnType<typeof vi.fn>
    getPlugin: ReturnType<typeof vi.fn>
  }
}

describe('PluginManager', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getPluginRegistry().clear()
  })

  it('registers bundle components into the registry after load', async () => {
    const instance = makeInstance('container')
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async () => instance),
      getModule: vi.fn(() => ({ components: { Workloads: FakeComponent } })),
    })
    const pm = new PluginManager(loader)

    await pm.loadPlugin(makeManifest('container'))

    expect(getPluginRegistry().hasComponent('container', 'Workloads')).toBe(true)
  })

  it('activatePlugin calls onActivate once and is idempotent', async () => {
    const instance = makeInstance('container')
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async () => instance),
      getModule: vi.fn(() => ({ components: {} })),
      getPlugin: vi.fn(() => instance),
    })
    const pm = new PluginManager(loader)

    await pm.loadPlugin(makeManifest('container'))
    await pm.activatePlugin('container')
    await pm.activatePlugin('container')

    expect(instance.onActivate).toHaveBeenCalledTimes(1)
  })

  it('deactivatePlugin calls onDeactivate and unregisters components', async () => {
    const instance = makeInstance('container')
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async () => instance),
      getModule: vi.fn(() => ({ components: { Workloads: FakeComponent } })),
      getPlugin: vi.fn(() => instance),
    })
    const pm = new PluginManager(loader)

    await pm.loadPlugin(makeManifest('container'))
    await pm.activatePlugin('container')
    await pm.deactivatePlugin('container')

    expect(instance.onDeactivate).toHaveBeenCalledTimes(1)
    expect(getPluginRegistry().hasComponent('container', 'Workloads')).toBe(false)
  })

  it('activateRequiredPlugins isolates failures: one broken plugin does not block others', async () => {
    const goodInstance = makeInstance('good')
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async (manifest: any) => {
        if (manifest.name === 'bad') throw new Error('bundle corrupt')
        return goodInstance
      }),
      getModule: vi.fn(() => ({ components: {} })),
      getPlugin: vi.fn((id: string) => (id === 'good' ? goodInstance : undefined)),
    })
    const pm = new PluginManager(loader)

    const activated = await pm.activateRequiredPlugins([
      makeManifest('bad'),
      makeManifest('good'),
    ])

    expect(activated.map((p) => p.name)).toEqual(['good'])
    expect(goodInstance.onActivate).toHaveBeenCalledTimes(1)
  })

  it('loadPlugin failure records error state and rethrows', async () => {
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async () => {
        throw new Error('network')
      }),
    })
    const pm = new PluginManager(loader)

    await expect(pm.loadPlugin(makeManifest('broken'))).rejects.toThrow('network')
    expect(pm.getState('broken').error?.message).toBe('network')
  })

  it('reset deactivates plugins and clears the registry', async () => {
    const instance = makeInstance('container')
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async () => instance),
      getModule: vi.fn(() => ({ components: { Workloads: FakeComponent } })),
      getPlugin: vi.fn(() => instance),
      clear: vi.fn(),
    })
    const pm = new PluginManager(loader)

    await pm.loadPlugin(makeManifest('container'))
    await pm.activatePlugin('container')
    pm.reset()

    expect(getPluginRegistry().hasComponent('container', 'Workloads')).toBe(false)
    expect(pm.getActivatedPlugins()).toHaveLength(0)
  })

  it('远程插件在 Remote Bundle 关闭时拒绝加载', async () => {
    const loader = makeLoader({
      loadRemotePlugin: vi.fn(async () => makeInstance('ext')),
    })
    const pm = new PluginManager(loader)
    pm.setRemoteBundlesEnabled(false)

    await expect(
      pm.loadPlugin({ ...makeManifest('ext'), mode: 'remote' }),
    ).rejects.toThrow(/Remote bundles are disabled/)
    expect(loader.loadRemotePlugin).not.toHaveBeenCalled()
  })

  it('mode=remote 且开启后分发到 loadRemotePlugin', async () => {
    const instance = makeInstance('ext')
    const loader = makeLoader({
      loadRemotePlugin: vi.fn(async () => instance),
      getModule: vi.fn(() => ({ components: {} })),
    })
    const pm = new PluginManager(loader)
    pm.setRemoteBundlesEnabled(true)

    await pm.loadPlugin({ ...makeManifest('ext'), mode: 'remote' })
    expect(loader.loadRemotePlugin).toHaveBeenCalledTimes(1)
  })

  it('激活门槛：缺少必需权限时拒绝激活', async () => {
    const instance = makeInstance('secure')
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async () => instance),
      getModule: vi.fn(() => ({ components: {} })),
      getPlugin: vi.fn(() => instance),
    })
    const pm = new PluginManager(loader)
    const manifest = {
      ...makeManifest('secure'),
      permissions: { required: ['secure:manage'] },
    }

    await pm.loadPlugin(manifest)
    await expect(pm.activatePlugin('secure')).rejects.toThrow(/missing required permissions/)
    expect(instance.onActivate).not.toHaveBeenCalled()
  })

  it('notifyContextChange 调用激活插件的 onContextChange 并隔离失败', async () => {
    const good: any = makeInstance('good')
    const bad: any = makeInstance('bad')
    bad.onContextChange = vi.fn(async () => {
      throw new Error('hook failed')
    })
    good.onContextChange = vi.fn(async () => {})
    const instances: Record<string, any> = { good, bad }
    const loader = makeLoader({
      loadLocalPlugin: vi.fn(async (m: any) => instances[m.name]),
      getModule: vi.fn(() => ({ components: {} })),
      getPlugin: vi.fn((id: string) => instances[id]),
    })
    const pm = new PluginManager(loader)
    await pm.activateRequiredPlugins([makeManifest('good'), makeManifest('bad')])

    const ctx = { tenantId: 't1', spaceId: 's2' }
    await pm.notifyContextChange(ctx)

    expect(good.onContextChange).toHaveBeenCalledWith(ctx)
    expect(bad.onContextChange).toHaveBeenCalledWith(ctx)
  })
})
