import { create } from 'zustand'

import { authStorage } from '@/api/client'

export interface AuthUser {
  id?: string
  email?: string
  name?: string
  role?: string
  team_id?: string
}

type AuthMode = 'checking' | 'jwt' | 'legacy' | 'unauthenticated'

interface TokenResponse {
  access_token: string
  refresh_token: string
  user?: AuthUser
}

interface AuthState {
  mode: AuthMode
  user: AuthUser | null
  version: string
  ssoError: string
  initialize: () => Promise<void>
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
  setLegacyToken: (token: string) => void
  markUnauthorized: () => void
}

function savedUser() {
  try {
    return JSON.parse(sessionStorage.getItem('authUser') ?? 'null') as AuthUser | null
  } catch {
    return null
  }
}

function saveTokens(tokens: TokenResponse) {
  sessionStorage.setItem(authStorage.accessKey, tokens.access_token)
  sessionStorage.setItem(authStorage.refreshKey, tokens.refresh_token)
  if (tokens.user) sessionStorage.setItem('authUser', JSON.stringify(tokens.user))
}

async function refreshSession() {
  const refreshToken = sessionStorage.getItem(authStorage.refreshKey)
  if (!refreshToken) return false
  const response = await fetch('/auth/refresh', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })
  if (!response.ok) return false
  saveTokens((await response.json()) as TokenResponse)
  return true
}

function captureSSOFragment() {
  const params = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const access = params.get('kc_access')
  const refresh = params.get('kc_refresh')
  const error = params.get('kc_error') ?? ''
  if (access && refresh) saveTokens({ access_token: access, refresh_token: refresh })
  if (access || refresh || error) history.replaceState(null, '', window.location.pathname + window.location.search)
  return error
}

async function getMe() {
  const access = sessionStorage.getItem(authStorage.accessKey)
  return fetch('/auth/me', {
    headers: access ? { Authorization: `Bearer ${access}` } : undefined,
  })
}

async function getPublicVersion() {
  try {
    const response = await fetch('/auth/sso/status')
    if (!response.ok) return ''
    const status = (await response.json()) as { version?: string }
    return status.version ?? ''
  } catch {
    return ''
  }
}

export const useAuthStore = create<AuthState>((set, get) => ({
  mode: 'checking',
  user: savedUser(),
  version: '',
  ssoError: '',
  initialize: async () => {
    const ssoError = captureSSOFragment()
    try {
      let response = await getMe()
      if (response.status === 401 && (await refreshSession())) response = await getMe()
      if (response.ok) {
        const me = (await response.json()) as {
          auth_enabled: boolean
          version?: string
          user?: AuthUser
        }
        if (!me.auth_enabled) authStorage.clearJWT()
        if (me.user) sessionStorage.setItem('authUser', JSON.stringify(me.user))
        set({
          mode: me.auth_enabled ? 'jwt' : 'legacy',
          user: me.auth_enabled ? (me.user ?? get().user) : null,
          version: me.version ?? '',
          ssoError,
        })
        return
      }
      authStorage.clearJWT()
      set({ mode: 'unauthenticated', user: null, version: await getPublicVersion(), ssoError })
    } catch {
      set({ mode: 'legacy', ssoError })
    }
  },
  login: async (email, password) => {
    const response = await fetch('/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    })
    if (!response.ok) {
      throw new Error(response.status === 401 ? '이메일 또는 비밀번호가 올바르지 않습니다.' : '로그인에 실패했습니다.')
    }
    const tokens = (await response.json()) as TokenResponse
    saveTokens(tokens)
    let user = tokens.user ?? null
    let version = get().version
    const meResponse = await getMe().catch(() => null)
    if (meResponse?.ok) {
      const me = (await meResponse.json()) as { version?: string; user?: AuthUser }
      user = me.user ?? user
      version = me.version ?? version
    }
    if (user) sessionStorage.setItem('authUser', JSON.stringify(user))
    set({ mode: 'jwt', user, version, ssoError: '' })
  },
  logout: async () => {
    const access = sessionStorage.getItem(authStorage.accessKey)
    const refresh = sessionStorage.getItem(authStorage.refreshKey)
    try {
      await fetch('/auth/logout', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(access ? { Authorization: `Bearer ${access}` } : {}),
        },
        body: JSON.stringify({ refresh_token: refresh }),
      })
    } finally {
      authStorage.clearJWT()
      set({ mode: 'unauthenticated', user: null })
    }
  },
  setLegacyToken: (token) => {
    if (token.trim()) sessionStorage.setItem(authStorage.adminTokenKey, token.trim())
    else sessionStorage.removeItem(authStorage.adminTokenKey)
    set({ mode: 'legacy' })
  },
  markUnauthorized: () => {
    if (get().mode === 'jwt') {
      authStorage.clearJWT()
      set({ mode: 'unauthenticated', user: null })
    }
  },
}))
