import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { usePluginStore } from '../pluginStore'

const mockPlugin = { name: 'dashboard', version: '1.0.0', module: '/plugins/dashboard.js' }

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('pluginStore', () => {
  it('starts empty', () => {
    const store = usePluginStore()
    expect(store.getAll()).toEqual([])
    expect(store.loadingCount).toBe(0)
  })

  it('add and get a plugin', () => {
    const store = usePluginStore()
    store.add(mockPlugin as any)
    expect(store.get('dashboard')?.name).toBe('dashboard')
    expect(store.getAll()).toHaveLength(1)
  })

  it('activate and deactivate a plugin', () => {
    const store = usePluginStore()
    store.add(mockPlugin as any)
    expect(store.isActivated('dashboard')).toBe(false)
    store.activate('dashboard')
    expect(store.isActivated('dashboard')).toBe(true)
    store.deactivate('dashboard')
    expect(store.isActivated('dashboard')).toBe(false)
  })

  it('getAllActive returns only activated plugins', () => {
    const store = usePluginStore()
    store.add(mockPlugin as any)
    store.add({ name: 'monitor', version: '1.0.0', module: '/plugins/monitor.js' } as any)
    store.activate('dashboard')
    const active = store.getAllActive
    expect(active).toHaveLength(1)
    expect(active[0].name).toBe('dashboard')
  })

  it('setError and hasError', () => {
    const store = usePluginStore()
    store.setError('dashboard', new Error('load failed'))
    expect(store.hasError('dashboard')).toBe(true)
    expect(store.getError('dashboard')?.message).toBe('load failed')
    store.clearErrors()
    expect(store.hasError('dashboard')).toBe(false)
  })

  it('setLoading and isLoading', () => {
    const store = usePluginStore()
    store.setLoading('dashboard', true)
    expect(store.isLoading('dashboard')).toBe(true)
    expect(store.loadingCount).toBe(1)
    store.setLoading('dashboard', false)
    expect(store.isLoading('dashboard')).toBe(false)
    expect(store.loadingCount).toBe(0)
  })

  it('clear resets all state', () => {
    const store = usePluginStore()
    store.add(mockPlugin as any)
    store.activate('dashboard')
    store.setError('dashboard', new Error('err'))
    store.clear()
    expect(store.getAll()).toEqual([])
    expect(store.isActivated('dashboard')).toBe(false)
    expect(store.hasError('dashboard')).toBe(false)
  })
})