import { getShellApiClient } from '@/core/api/client'

export interface SessionBootstrapPermission {
  tenantId: string
  resourceKind: string
  resourceId?: string
  action: string
}

export interface SessionBootstrapMembership {
  membershipId: string
  tenantId: string
  tenantName: string
}

export interface SessionBootstrapResponse {
  subject: {
    id: string
    type: 'user' | 'workload' | 'service'
    displayName: string
  }
  selectedTenantId: string
  memberships: SessionBootstrapMembership[]
  capabilities: Array<{ id: string; version: string }>
  permissions: SessionBootstrapPermission[]
  policyVersion: string
  permissionVersion: string
}

export function scopedPermissionsToCodes(permissions: SessionBootstrapPermission[]): string[] {
  const result = new Set<string>()
  for (const permission of permissions) {
    if (!permission.resourceKind || !permission.action) continue
    result.add(`${permission.resourceKind}:${permission.action}`)
    if (permission.resourceKind === '*') result.add('*')
  }
  return Array.from(result)
}

export async function fetchSessionBootstrap(): Promise<SessionBootstrapResponse> {
  return getShellApiClient().get<SessionBootstrapResponse>('/api/v1/session/bootstrap')
}
