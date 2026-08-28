import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { usePermissionStore } from '../permissionStore'

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('permissionStore', () => {
  it('starts with no permissions', () => {
    const store = usePermissionStore()
    expect(store.permissions).toEqual([])
    expect(store.hasPermission('anything')).toBe(false)
  })

  it('hasPermission returns true for wildcard', () => {
    const store = usePermissionStore()
    store.setPermissions(['*'])
    expect(store.hasPermission('read:project')).toBe(true)
  })

  it('hasPermission checks exact match', () => {
    const store = usePermissionStore()
    store.setPermissions(['read:project', 'write:project'])
    expect(store.hasPermission('read:project')).toBe(true)
    expect(store.hasPermission('delete:project')).toBe(false)
  })

  it('hasAllPermissions returns true when all present', () => {
    const store = usePermissionStore()
    store.setPermissions(['a', 'b', 'c'])
    expect(store.hasAllPermissions(['a', 'b'])).toBe(true)
    expect(store.hasAllPermissions(['a', 'd'])).toBe(false)
  })

  it('hasAllPermissions returns true for empty list', () => {
    const store = usePermissionStore()
    expect(store.hasAllPermissions([])).toBe(true)
  })

  it('hasAnyPermission returns true when at least one present', () => {
    const store = usePermissionStore()
    store.setPermissions(['a', 'b'])
    expect(store.hasAnyPermission(['c', 'a'])).toBe(true)
    expect(store.hasAnyPermission(['c', 'd'])).toBe(false)
  })

  it('hasAnyPermission returns false for empty list', () => {
    const store = usePermissionStore()
    expect(store.hasAnyPermission([])).toBe(false)
  })

  it('clear removes all permissions', () => {
    const store = usePermissionStore()
    store.setPermissions(['read:project'])
    store.clear()
    expect(store.permissions).toEqual([])
  })

  it('version 跟踪与 clear 重置', () => {
    const store = usePermissionStore()
    expect(store.version).toBe('')
    store.setVersion('p18')
    expect(store.version).toBe('p18')
    store.clear()
    expect(store.version).toBe('')
  })
})