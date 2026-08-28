import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { NavigationCacheEntry } from '@hnb/types'

interface NavigationState {
  current: NavigationCacheEntry | null
  etag: string
  versions: Record<string, string>
}

export const useNavigationStore = defineStore('navigation', {
  state: (): NavigationState => ({
    current: null,
    etag: '',
    versions: {},
  }),

  getters: {
    tenantId: (state) => state.current?.context?.tenantId,
    spaceId: (state) => state.current?.context?.spaceId,
    items: (state) => state.current?.payload?.menus ?? [],
    getCurrent: (state) => state.current,
    getEtag: (state) => state.etag,
    getVersions: (state) => state.versions,
  },

  actions: {
    replace(data: {
      current: NavigationCacheEntry
      etag?: string
      versions?: Record<string, string>
    }): void {
      this.current = {
        ...data.current,
        cacheKeyHash: data.current.cacheKeyHash ?? '',
        userIdHash: data.current.userIdHash ?? '',
        expiresAt:
          data.current.expiresAt ??
          new Date(Date.now() + 10 * 60 * 1000).toISOString(),
      }
      this.etag = data.etag ?? ''
      this.versions = data.versions ?? data.current.versions ?? {}
    },

    clear(): void {
      this.current = null
      this.etag = ''
      this.versions = {}
    },
  },
})
