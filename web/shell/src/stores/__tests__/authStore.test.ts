import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '../authStore'

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
})

const mockUser = {
  id: 'u1',
  username: 'admin',
  displayName: 'Admin User',
  email: 'admin@hnb.io',
  roles: ['admin'],
  permissions: ['*'],
}

describe('authStore', () => {
  it('starts unauthenticated', () => {
    const store = useAuthStore()
    expect(store.isAuthenticated).toBe(false)
    expect(store.user).toBeNull()
    expect(store.token).toBeNull()
  })

  it('restoreSession reads from localStorage', () => {
    localStorage.setItem('hnb_token', 'test-token')
    localStorage.setItem('hnb_user', JSON.stringify(mockUser))
    const store = useAuthStore()
    store.restoreSession()
    expect(store.isAuthenticated).toBe(true)
    expect(store.token).toBe('test-token')
    expect(store.user?.username).toBe('admin')
  })

  it('setPermissions updates user permissions', () => {
    localStorage.setItem('hnb_token', 'test-token')
    localStorage.setItem('hnb_user', JSON.stringify(mockUser))
    const store = useAuthStore()
    store.restoreSession()
    store.setPermissions(['read:project', 'write:project'])
    expect(store.user?.permissions).toEqual(['read:project', 'write:project'])
  })

  it('login stores token and user on success', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ access_token: 'tok', refresh_token: 'rtok', user_id: 'u1', username: 'admin' }), { status: 200 }),
    )
    const store = useAuthStore()
    await store.login('admin', 'pass')
    expect(store.isAuthenticated).toBe(true)
    expect(store.token).toBe('tok')
    expect(store.user?.username).toBe('admin')
    expect(localStorage.getItem('hnb_token')).toBe('tok')
  })

  it('login throws on failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ message: 'bad credentials' }), { status: 401 }),
    )
    const store = useAuthStore()
    await expect(store.login('admin', 'wrong')).rejects.toThrow(/bad credentials/)
    expect(store.isAuthenticated).toBe(false)
  })

  it('logout clears all state', async () => {
    localStorage.setItem('hnb_token', 'tok')
    localStorage.setItem('hnb_refresh_token', 'rtok')
    localStorage.setItem('hnb_user', JSON.stringify(mockUser))
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(new Response(null, { status: 200 }))
    const store = useAuthStore()
    store.restoreSession()
    await store.logout()
    expect(store.isAuthenticated).toBe(false)
    expect(store.user).toBeNull()
    expect(localStorage.getItem('hnb_token')).toBeNull()
  })
})