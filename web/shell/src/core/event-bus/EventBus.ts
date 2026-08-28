import type { EventBus } from '@hnb/types'

type Handler = (...args: any[]) => void

function createEventBusImpl(): EventBus {
  const listeners = new Map<string, Set<Handler>>()

  function on(event: string, handler: Handler): void {
    if (!listeners.has(event)) {
      listeners.set(event, new Set())
    }
    listeners.get(event)!.add(handler)
  }

  function off(event: string, handler: Handler): void {
    listeners.get(event)?.delete(handler)
  }

  function emit(event: string, ...args: any[]): void {
    listeners.get(event)?.forEach((handler) => {
      try {
        handler(...args)
      } catch (e) {
        console.error(`[EventBus] handler error for event "${event}":`, e)
      }
    })
  }

  return { on, off, emit }
}

let _eventBus: EventBus | null = null

export function getEventBus(): EventBus {
  if (!_eventBus) {
    _eventBus = createEventBusImpl()
  }
  return _eventBus
}

export function createEventBus(): EventBus {
  return getEventBus()
}

export { getEventBus as default }
