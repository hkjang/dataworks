const ACCESS_KEY = 'authAccess'
const REFRESH_KEY = 'authRefresh'
const ADMIN_TOKEN_KEY = 'adminToken'

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly payload?: unknown,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

let refreshRequest: Promise<boolean> | null = null

function authHeaders() {
  const headers = new Headers({ Accept: 'application/json' })
  const access = sessionStorage.getItem(ACCESS_KEY)
  const legacyToken = sessionStorage.getItem(ADMIN_TOKEN_KEY)
  const token = access || legacyToken
  if (token) headers.set('Authorization', `Bearer ${token}`)
  return headers
}

async function refreshAccessToken() {
  const refreshToken = sessionStorage.getItem(REFRESH_KEY)
  if (!refreshToken) return false

  if (!refreshRequest) {
    refreshRequest = fetch('/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
      .then(async (response) => {
        if (!response.ok) return false
        const tokens = (await response.json()) as {
          access_token?: string
          refresh_token?: string
        }
        if (tokens.access_token) sessionStorage.setItem(ACCESS_KEY, tokens.access_token)
        if (tokens.refresh_token) sessionStorage.setItem(REFRESH_KEY, tokens.refresh_token)
        return Boolean(tokens.access_token)
      })
      .catch(() => false)
      .finally(() => {
        refreshRequest = null
      })
  }
  return refreshRequest
}

async function parseResponse<T>(response: Response): Promise<T> {
  if (response.status === 204) return undefined as T
  const contentType = response.headers.get('content-type') ?? ''
  const payload = contentType.includes('application/json')
    ? await response.json()
    : await response.text()
  if (!response.ok) {
    const message =
      typeof payload === 'string'
        ? payload
        : ((payload as { error?: { message?: string }; message?: string })?.error?.message ??
          (payload as { message?: string })?.message ??
          response.statusText)
    throw new ApiError(message || '요청을 완료하지 못했습니다.', response.status, payload)
  }
  return payload as T
}

export async function authenticatedFetch(path: string, init: RequestInit = {}) {
  const request = () => {
    const headers = authHeaders()
    new Headers(init.headers).forEach((value, key) => headers.set(key, value))
    if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
    return fetch(path, { ...init, headers })
  }

  let response = await request()
  if (response.status === 401 && (await refreshAccessToken())) response = await request()
  if (response.status === 401) window.dispatchEvent(new CustomEvent('dataworks:unauthorized'))
  return response
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  return parseResponse<T>(await authenticatedFetch(path, init))
}

export const authStorage = {
  accessKey: ACCESS_KEY,
  refreshKey: REFRESH_KEY,
  adminTokenKey: ADMIN_TOKEN_KEY,
  clearJWT() {
    sessionStorage.removeItem(ACCESS_KEY)
    sessionStorage.removeItem(REFRESH_KEY)
    sessionStorage.removeItem('authUser')
  },
}
