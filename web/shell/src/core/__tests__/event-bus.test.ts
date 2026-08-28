import { describe, it, expect, beforeEach, vi } from 'vitest'
import { getEventBus, createEventBus } from '../event-bus'

beforeEach(() => {
  // Reset singleton by re-creating
  vi.resetModules()
})

describe('EventBus', () => {
  it('on and emit fire handlers', () => {
    const bus = createEventBus()
    const handler = vi.fn()
    bus.on('test:event', handler)
    bus.emit('test:event', 'arg1', 42)
    expect(handler).toHaveBeenCalledWith('arg1', 42)
  })

  it('off removes a handler', () => {
    const bus = createEventBus()
    const handler = vi.fn()
    bus.on('test:event', handler)
    bus.off('test:event', handler)
    bus.emit('test:event')
    expect(handler).not.toHaveBeenCalled()
  })

  it('emit does not throw for events with no listeners', () => {
    const bus = createEventBus()
    expect(() => bus.emit('nonexistent')).not.toThrow()
  })

  it('off does not throw for missing event', () => {
    const bus = createEventBus()
    const handler = vi.fn()
    expect(() => bus.off('nonexistent', handler)).not.toThrow()
  })

  it('handler errors are caught (do not propagate)', () => {
    const bus = createEventBus()
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    bus.on('test:event', () => { throw new Error('handler error') })
    expect(() => bus.emit('test:event')).not.toThrow()
    expect(consoleSpy).toHaveBeenCalled()
    consoleSpy.mockRestore()
  })

  it('getEventBus returns a singleton', () => {
    const bus1 = getEventBus()
    const bus2 = getEventBus()
    expect(bus1).toBe(bus2)
  })

  it('multiple handlers for same event all fire', () => {
    const bus = createEventBus()
    const h1 = vi.fn()
    const h2 = vi.fn()
    bus.on('test:event', h1)
    bus.on('test:event', h2)
    bus.emit('test:event')
    expect(h1).toHaveBeenCalledOnce()
    expect(h2).toHaveBeenCalledOnce()
  })
})