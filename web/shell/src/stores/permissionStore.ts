import { defineStore } from 'pinia'
import { ref } from 'vue'

export const usePermissionStore = defineStore('permission', () => {
  const permissions = ref<string[]>([])
  /** 服务端权限版本（来自 NavigationResponse.versions.permission） */
  const version = ref<string>('')

  function setPermissions(list: string[]): void {
    permissions.value = Array.isArray(list) ? list : []
  }

  function setVersion(v: string): void {
    version.value = v
  }

  function hasPermission(permission: string): boolean {
    return permissions.value.includes(permission) || permissions.value.includes('*')
  }

  function hasAllPermissions(list: string[]): boolean {
    if (!Array.isArray(list) || list.length === 0) return true
    return list.every((p) => hasPermission(p))
  }

  function hasAnyPermission(list: string[]): boolean {
    if (!Array.isArray(list) || list.length === 0) return false
    return list.some((p) => hasPermission(p))
  }

  function clear(): void {
    permissions.value = []
    version.value = ''
  }

  return {
    permissions,
    version,
    setPermissions,
    setVersion,
    hasPermission,
    hasAllPermissions,
    hasAnyPermission,
    clear,
  }
})
