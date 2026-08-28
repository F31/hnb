import { describe, expect, it } from 'vitest'
import { scopedPermissionsToCodes } from '../session'

describe('scopedPermissionsToCodes', () => {
  it('uses canonical cluster permissions without the legacy view alias', () => {
    expect(scopedPermissionsToCodes([
      { tenantId: 'tenant-a', resourceKind: 'cluster', action: 'list' },
      { tenantId: 'tenant-a', resourceKind: 'cluster', action: 'read' },
      { tenantId: 'tenant-a', resourceKind: 'cluster', action: 'create' },
      { tenantId: 'tenant-a', resourceKind: 'cluster', action: 'update' },
      { tenantId: 'tenant-a', resourceKind: 'cluster', action: 'delete' },
    ])).toEqual([
      'cluster:list',
      'cluster:read',
      'cluster:create',
      'cluster:update',
      'cluster:delete',
    ])
  })
})
