import type { PermissionStore } from '@hnb/types'
import { usePermissionStore } from '@/stores/permissionStore'

export function createPermissionManager(): PermissionStore {
  return usePermissionStore()
}