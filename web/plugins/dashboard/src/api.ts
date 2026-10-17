import type { ApiClient } from '@hnb/types'

let client: ApiClient | null = null

export function setDashboardApiClient(value: ApiClient): void {
  client = value
}

export function dashboardApiClient(): ApiClient {
  if (!client) throw new Error('dashboard api client is not initialized')
  return client
}
