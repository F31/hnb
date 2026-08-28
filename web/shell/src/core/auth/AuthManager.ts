import type { AuthStore } from '@hnb/types'
import { useAuthStore } from '@/stores/authStore'

export function createAuthManager(): AuthStore {
  return useAuthStore()
}