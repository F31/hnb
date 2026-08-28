import { createApiClient } from '@hnb/api-client'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore } from '@/stores/contextStore'

let client: ReturnType<typeof createApiClient> | null = null

export function getShellApiClient(): ReturnType<typeof createApiClient> {
  if (!client) {
    client = createApiClient({
      getToken: () => useAuthStore().token,
      refreshToken: () => useAuthStore().refreshTokenAction(),
      beforeRequest: () => useAuthStore().ensureFreshToken(30),
      onUnauthorized: () => { useAuthStore().sessionExpired = true },
      getContext: () => useContextStore().current,
    })
  }
  return client
}
