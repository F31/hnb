import { defineStore } from 'pinia'
import { ref, computed, shallowRef } from 'vue'
import type { HNBContext } from '@hnb/types'
import { fetchWorkspaces } from '@/core/api/workspaces'

export function normalizeWorkspace(workspace: any): any {
  return {
    ...workspace,
    tenantId: workspace.tenantId ?? workspace.tenant_id,
    displayName: workspace.displayName ?? workspace.display_name,
    isActive: workspace.isActive ?? workspace.is_active,
  }
}

export const useContextStore = defineStore('context', () => {
  const current = ref<HNBContext>({})
  const workspaces = ref<any[]>([])

  const switchGeneration = shallowRef(0)
  const abortController = shallowRef<AbortController | null>(null)

  const spaceId = computed(() => current.value.spaceId)
  const tenantId = computed(() => current.value.tenantId)

  /**
   * Load the workspaces accessible to the currently authenticated user.
   * The generation argument is used to discard responses from stale requests.
   */
  async function loadWorkspaces(gen: number): Promise<any[]> {
    if (switchGeneration.value > 0 && gen !== switchGeneration.value) return []

    if (abortController.value) {
      abortController.value.abort()
    }
    const controller = new AbortController()
    abortController.value = controller

    try {
      const list: any[] = (await fetchWorkspaces(controller.signal)).map(normalizeWorkspace)
      if (switchGeneration.value === 0 || gen === switchGeneration.value) {
        workspaces.value = list
      }
      return list
    } catch (err: any) {
      if (err?.name === 'AbortError') return []
      console.error('[context] failed to load workspaces:', err)
      throw err
    }
  }

  /**
   * Switch the active space, preserving the tenantId (HNBContext semantics:
   * a workspace belongs to exactly one tenant; selecting a workspace should
   * set both spaceId and tenantId atomically).
   */
  async function setSpace(spaceId: string, gen: number): Promise<void> {
    if (gen !== switchGeneration.value) return
    const ws = workspaces.value.find((w) => w.id === spaceId)
    current.value = {
      ...current.value,
      spaceId,
      tenantId: ws?.tenantId ?? current.value.tenantId,
    }
  }

  /**
   * Bulk replace the context. Used during tenant switching.
   */
  function setFullContext(ctx: HNBContext, gen?: number): void {
    if (gen !== undefined && gen !== switchGeneration.value) return
    current.value = { ...ctx }
  }

  /**
   * Atomically switch tenant — aborts in-flight requests, increments
   * the generation counter, clears the current context and loads workspaces.
   * Returns the new generation number.
   */
  async function switchTenant(newTenantId: string): Promise<number> {
    const gen = ++switchGeneration.value
    if (abortController.value) {
      abortController.value.abort()
    }
    current.value = { tenantId: newTenantId }
    await loadWorkspaces(gen)
    return gen
  }

  function reset(): void {
    if (abortController.value) {
      abortController.value.abort()
      abortController.value = null
    }
    current.value = {}
    workspaces.value = []
    switchGeneration.value = 0
  }

  function matches(ctx: HNBContext): boolean {
    return (
      !!current.value.tenantId &&
      !!ctx.tenantId &&
      current.value.tenantId === ctx.tenantId &&
      current.value.spaceId === ctx.spaceId
    )
  }

  return {
    current,
    workspaces,
    spaceId,
    tenantId,
    switchGeneration,
    loadWorkspaces,
    setSpace,
    setFullContext,
    switchTenant,
    reset,
    matches,
  }
})
