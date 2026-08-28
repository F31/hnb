import { describe, expect, it, vi } from 'vitest'
import {
  configMapResource,
  createSecret,
  deleteSecret,
  isProtectedSecret,
  listConfigMaps,
  mapConfigMap,
  mapSecret,
  saveConfigMap,
  secretResource,
  setContainerConfigClient,
} from '../configApi'

function mockClient(overrides: Record<string, unknown> = {}) {
  return {
    get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn(), ...overrides,
  } as any
}

describe('configApi', () => {
  it('maps ConfigMaps and Secrets without exposing Secret values', () => {
    expect(mapConfigMap({ metadata: { name: 'settings', namespace: 'default', creationTimestamp: 'now' }, data: { mode: 'prod' } }))
      .toEqual({ name: 'settings', namespace: 'default', createdAt: 'now', data: { mode: 'prod' } })
    expect(mapSecret({ metadata: { name: 'argocd-secret', namespace: 'argocd' }, type: 'Opaque', data: { password: 'c2VjcmV0' } }))
      .toEqual({ name: 'argocd-secret', namespace: 'argocd', type: 'Opaque', dataKeys: ['password'], createdAt: '', protected: true })
  })

  it('builds ConfigMap data and Secret stringData resources', () => {
    expect(configMapResource({ name: 'settings', namespace: 'default', data: { mode: 'prod' } })).toMatchObject({ apiVersion: 'v1', kind: 'ConfigMap', data: { mode: 'prod' } })
    expect(secretResource({ name: 'credentials', namespace: 'default', type: 'Opaque', stringData: { password: '密码' } })).toMatchObject({ kind: 'Secret', stringData: { password: '密码' } })
  })

  it('lists ConfigMaps through the Kubernetes proxy', async () => {
    const get = vi.fn().mockResolvedValue({ items: [{ metadata: { name: 'settings', namespace: 'default' }, data: {} }] })
    setContainerConfigClient(mockClient({ get }))
    await expect(listConfigMaps('cluster-a', 'default')).resolves.toHaveLength(1)
    expect(get).toHaveBeenCalledWith('/api/v1/proxy/cluster-a/api/v1/namespaces/default/configmaps')
  })

  it('creates Secrets with plaintext stringData', async () => {
    const post = vi.fn().mockResolvedValue(undefined)
    setContainerConfigClient(mockClient({ post }))
    await createSecret('cluster-a', { name: 'credentials', namespace: 'default', type: 'Opaque', stringData: { password: 'secret' } })
    expect(post).toHaveBeenCalledWith('/api/v1/proxy/cluster-a/api/v1/namespaces/default/secrets', expect.objectContaining({ stringData: { password: 'secret' } }))
  })

  it('removes deleted ConfigMap keys with a merge patch', async () => {
    const patch = vi.fn().mockResolvedValue(undefined)
    setContainerConfigClient(mockClient({ patch }))
    await saveConfigMap('cluster-a', { name: 'settings', namespace: 'default', data: { retained: 'new' } }, 'settings', { retained: 'old', removed: 'old' })
    expect(patch).toHaveBeenCalledWith(
      '/api/v1/proxy/cluster-a/api/v1/namespaces/default/configmaps/settings',
      expect.objectContaining({ data: { retained: 'new', removed: null } }),
      { headers: { 'Content-Type': 'application/merge-patch+json' } },
    )
  })

  it('protects system Secrets before issuing delete requests', async () => {
    const remove = vi.fn()
    setContainerConfigClient(mockClient({ delete: remove }))
    expect(isProtectedSecret('argocd-secret')).toBe(true)
    expect(isProtectedSecret('argocd-secret-copy')).toBe(false)
    await expect(deleteSecret('cluster-a', 'argocd', 'argocd-secret')).rejects.toThrow('protected')
    expect(remove).not.toHaveBeenCalled()
  })
})
