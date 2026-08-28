import { createApiClient } from '@hnb/api-client'
import { useAuthStore } from '@/stores/authStore'

interface WorkspaceEnvelope {
  data?: unknown[]
  items?: unknown[]
}

function workspaceClient() {
  return createApiClient({
    getToken: () => useAuthStore().token,
    refreshToken: () => useAuthStore().refreshTokenAction(),
    beforeRequest: () => useAuthStore().ensureFreshToken(30),
  })
}

export async function fetchWorkspaces(signal?: AbortSignal): Promise<unknown[]> {
  const data = await workspaceClient().get<WorkspaceEnvelope | unknown[]>('/api/v1/workspaces', { signal })
  if (Array.isArray(data)) return data
  return data.data ?? data.items ?? []
}
