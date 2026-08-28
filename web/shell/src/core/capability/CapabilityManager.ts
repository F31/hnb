import type { CapabilityManager } from '@hnb/types'
import { getShellApiClient } from '@/core/api/client'

export function createCapabilityManager(): CapabilityManager {
  const cache = new Map<string, boolean>()

  async function hasCapability(name: string): Promise<boolean> {
    if (cache.has(name)) return cache.get(name)!
    try {
      const data = await getShellApiClient().get<{ available?: boolean }>(
        `/api/v1/capabilities/${encodeURIComponent(name)}`,
      )
      const result = data.available === true
      cache.set(name, result)
      return result
    } catch {
      return false
    }
  }

  async function hasAllCapabilities(names: string[]): Promise<boolean> {
    const results = await Promise.all(names.map(hasCapability))
    return results.every(Boolean)
  }

  async function checkRequired(required: string[]): Promise<boolean> {
    return hasAllCapabilities(required)
  }

  async function refresh(): Promise<void> {
    cache.clear()
  }

  return { hasCapability, hasAllCapabilities, checkRequired, refresh }
}
