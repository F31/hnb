import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '@hnb/types'

const TOKEN_KEY = 'hnb_token'
const REFRESH_TOKEN_KEY = 'hnb_refresh_token'
const USER_KEY = 'hnb_user'

function safeReadUser(): User | null {
  try {
    const raw = localStorage.getItem(USER_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (
      parsed &&
      typeof parsed.id === 'string' &&
      typeof parsed.username === 'string'
    ) {
      return parsed as User
    }
    return null
  } catch {
    return null
  }
}

function decodeJwtPayload(tokenValue: string | null): any {
  if (!tokenValue) return null
  try {
    const payload = tokenValue.split('.')[1]
    if (!payload) return null
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(normalized.length + ((4 - normalized.length % 4) % 4), '=')
    return JSON.parse(atob(padded))
  } catch {
    return null
  }
}

function permissionsFromToken(tokenValue: string | null): string[] {
  const claims = decodeJwtPayload(tokenValue)
  const scoped = Array.isArray(claims?.scopedPermissions) ? claims.scopedPermissions : []
  const permissions = new Set<string>()
  for (const item of scoped) {
    const resource = item?.resourceKind
    const action = item?.action
    if (typeof resource !== 'string' || typeof action !== 'string') continue
    permissions.add(`${resource}:${action}`)
    if (resource === '*') {
      permissions.add('*')
    }
  }
  return Array.from(permissions)
}

function isTokenExpired(tokenValue: string | null, skewSeconds = 10): boolean {
  const claims = decodeJwtPayload(tokenValue)
  if (typeof claims?.exp !== 'number') return true
  return claims.exp * 1000 <= Date.now() + skewSeconds * 1000
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(null)
  const refreshTokenRef = ref<string | null>(null)
  const user = ref<User | null>(null)
  const loading = ref(false)
  const sessionExpired = ref(false)
  let refreshInFlight: Promise<void> | null = null

  const isAuthenticated = computed(() => !!token.value)

  async function login(username: string, password: string): Promise<void> {
    loading.value = true
    try {
      const res = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })
      if (!res.ok) {
        const errBody = await res.json().catch(() => ({}))
        throw new Error(errBody.message || `登录失败: ${res.status}`)
      }
      const body = await res.json()
      const data = body.data ?? body
      token.value = data.access_token
      refreshTokenRef.value = data.refresh_token
      const u: User = {
        id: data.user_id ?? data.subject_id,
        username: data.username,
        displayName: data.displayName || data.username,
        email: data.email,
        roles: data.roles || [],
        permissions: data.permissions || permissionsFromToken(data.access_token),
      }
      user.value = u
      localStorage.setItem(TOKEN_KEY, data.access_token)
      if (data.refresh_token) {
        localStorage.setItem(REFRESH_TOKEN_KEY, data.refresh_token)
      }
      localStorage.setItem(USER_KEY, JSON.stringify(u))
    } finally {
      loading.value = false
    }
  }

  async function logout(): Promise<void> {
    stopSlidingSession()
    try {
      if (token.value) {
        await fetch('/api/v1/auth/logout', {
          method: 'POST',
          headers: { Authorization: `Bearer ${token.value}` },
        })
      }
    } catch (e) {
      console.warn('[auth] logout request failed:', e)
    } finally {
      token.value = null
      refreshTokenRef.value = null
      user.value = null
      sessionExpired.value = false
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(REFRESH_TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
    }
  }

  async function refreshToken(): Promise<void> {
    if (!refreshTokenRef.value) throw new Error('refresh token is missing')
    if (refreshInFlight) return refreshInFlight
    refreshInFlight = doRefreshToken().finally(() => {
      refreshInFlight = null
    })
    return refreshInFlight
  }

  async function doRefreshToken(): Promise<void> {
    try {
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshTokenRef.value }),
      })
      if (!res.ok) throw new Error(`refresh failed: ${res.status}`)
      sessionExpired.value = false
      const body = await res.json()
      const data = body.data ?? body
      token.value = data.access_token
      if (user.value && !Array.isArray(data.permissions)) {
        user.value = { ...user.value, permissions: permissionsFromToken(data.access_token) }
        localStorage.setItem(USER_KEY, JSON.stringify(user.value))
      }
      if (data.refresh_token) {
        refreshTokenRef.value = data.refresh_token
        localStorage.setItem(REFRESH_TOKEN_KEY, data.refresh_token)
      }
      localStorage.setItem(TOKEN_KEY, data.access_token)
    } catch (e) {
      console.warn('[auth] token refresh failed:', e)
      sessionExpired.value = true
      throw e
    }
  }

  async function ensureFreshToken(minRemainingSeconds = 10): Promise<void> {
    if (!token.value) return
    if (isTokenExpired(token.value, minRemainingSeconds)) {
      await refreshToken()
    }
  }

  let slidingTimer: ReturnType<typeof setTimeout> | null = null
  function onUserActivity(): void {
    if (slidingTimer) return
    slidingTimer = setTimeout(() => {
      slidingTimer = null
    }, 30_000)
    ensureFreshToken(30)
  }

  function startSlidingSession(): void {
    window.addEventListener('mousedown', onUserActivity, { passive: true })
    window.addEventListener('keydown', onUserActivity, { passive: true })
    window.addEventListener('touchstart', onUserActivity, { passive: true })
  }

  function stopSlidingSession(): void {
    window.removeEventListener('mousedown', onUserActivity)
    window.removeEventListener('keydown', onUserActivity)
    window.removeEventListener('touchstart', onUserActivity)
    if (slidingTimer) {
      clearTimeout(slidingTimer)
      slidingTimer = null
    }
  }

  function restoreSession(): void {
    token.value = localStorage.getItem(TOKEN_KEY)
    refreshTokenRef.value = localStorage.getItem(REFRESH_TOKEN_KEY)
    user.value = safeReadUser()
  }

  function setPermissions(permissions: string[]): void {
    if (!user.value) return
    user.value = { ...user.value, permissions }
    localStorage.setItem(USER_KEY, JSON.stringify(user.value))
  }

  return {
    token,
    refreshToken: refreshTokenRef,
    user,
    loading,
    sessionExpired,
    isAuthenticated,
    login,
    logout,
    refreshTokenAction: refreshToken,
    restoreSession,
    ensureFreshToken,
    setPermissions,
    startSlidingSession,
    stopSlidingSession,
  }
})
