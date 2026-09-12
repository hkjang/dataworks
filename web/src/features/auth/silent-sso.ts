// Silent SSO (OIDC prompt=none): decide whether to try signing in without drawing the login
// screen. prompt=none either answers immediately with a code or comes back with
// login_required; retrying that on every page load would bounce the browser between the
// provider and the app forever, so every rule below exists to make sure an attempt happens
// at most once per tab session — and never when the visitor just signed out.

export interface SilentSSOStatus {
  keycloak_enabled?: boolean
  auto_login?: boolean
  login_url?: string
}

// sessionStorage rather than localStorage: a fresh tab may try again, a reload after a
// refusal must not.
const ATTEMPTED_KEY = 'dataworks.sso.silentAttempted'
const SIGNED_OUT_KEY = 'dataworks.sso.signedOut'

// The callback appends ?sso=none when the provider had no session (and the legacy
// /dataworks/#kc_error= fragment reports an interactive failure). Both mean "do not try".
export const SSO_MARKER_PARAM = 'sso'

interface BrowserLocation {
  pathname: string
  search: string
}

function readFlag(key: string): boolean {
  try {
    return window.sessionStorage.getItem(key) === 'true'
  } catch {
    // Private modes and blocked site data throw. Reading that as "not attempted yet" would
    // start the loop, so a storage failure counts as "already attempted".
    return true
  }
}

function writeFlag(key: string, value: boolean) {
  try {
    if (value) window.sessionStorage.setItem(key, 'true')
    else window.sessionStorage.removeItem(key)
  } catch {
    /* readFlag already fails closed */
  }
}

/** Records that the visitor signed out on purpose, which suppresses auto-login. */
export function markSignedOut() {
  writeFlag(SIGNED_OUT_KEY, true)
  writeFlag(ATTEMPTED_KEY, true)
}

/** Clears the suppression once a session exists again. */
export function clearSilentSSOState() {
  writeFlag(SIGNED_OUT_KEY, false)
  writeFlag(ATTEMPTED_KEY, false)
}

export function silentSSOAttempted(): boolean {
  return readFlag(ATTEMPTED_KEY)
}

/**
 * Paths where a silent attempt must never start: the login/callback legs themselves (the most
 * common loop source) and anything that is not the browser workbench. API, MCP and health
 * paths never load this bundle, but the rule is written down so it stays true if they do.
 */
export function silentSSOAllowedPath(pathname: string): boolean {
  if (pathname.startsWith('/auth/')) return false
  return pathname === '/dataworks' || pathname.startsWith('/dataworks/')
}

/**
 * Decides whether to try signing in without showing a login screen.
 *
 * @param status   the public /auth/sso/status answer (auto_login is only published when the
 *                 administrator switched it on)
 * @param location window.location (injected for tests)
 * @param ssoError an interactive SSO failure carried in the URL fragment, if any
 */
export function shouldAttemptSilentSSO(status: SilentSSOStatus | null | undefined, location: BrowserLocation = window.location, ssoError = ''): boolean {
  if (!status?.keycloak_enabled || !status.auto_login) return false
  if (ssoError) return false
  if (!silentSSOAllowedPath(location.pathname)) return false
  // Address marker first: it survives a wiped sessionStorage.
  const marker = new URLSearchParams(location.search).get(SSO_MARKER_PARAM)
  if (marker === 'none' || marker === 'error') return false
  if (readFlag(SIGNED_OUT_KEY)) return false
  if (readFlag(ATTEMPTED_KEY)) return false
  return true
}

/** Same-origin in-app path only: must start with "/" and not "//". */
export function safeReturnTo(value: string): string {
  return value.startsWith('/') && !value.startsWith('//') && !value.startsWith('/\\') ? value : '/dataworks/'
}

/** Builds the top-level navigation target for a silent attempt. */
export function silentSSOLoginURL(loginURL: string, returnTo: string): string {
  const base = loginURL || '/auth/keycloak/login'
  return `${base}${base.includes('?') ? '&' : '?'}prompt=none&return_to=${encodeURIComponent(safeReturnTo(returnTo))}`
}

/**
 * The "continue with Keycloak" button: an ordinary interactive login that still carries the
 * deep link so the visitor lands where they were heading. The ?sso=none marker is stripped
 * from the carried path — it belongs to the refused attempt, not to the destination.
 */
export function interactiveSSOLoginURL(loginURL: string, location: BrowserLocation = window.location): string {
  const base = loginURL || '/auth/keycloak/login'
  const params = new URLSearchParams(location.search)
  params.delete(SSO_MARKER_PARAM)
  const search = params.toString()
  const returnTo = safeReturnTo(location.pathname + (search ? `?${search}` : ''))
  return `${base}${base.includes('?') ? '&' : '?'}return_to=${encodeURIComponent(returnTo)}`
}

/**
 * Sends the browser to the provider for a silent attempt. A top-level navigation rather than a
 * hidden iframe: it works with third-party cookies blocked and does not depend on the provider
 * allowing frames. The attempted flag is written before leaving so a refusal cannot retry.
 */
export function beginSilentSSO(loginURL: string, returnTo: string, navigate: (url: string) => void = (url) => window.location.assign(url)) {
  writeFlag(ATTEMPTED_KEY, true)
  navigate(silentSSOLoginURL(loginURL, returnTo))
}

/** Drops the ?sso=… marker from the address once a session exists, so it does not linger. */
export function stripSSOMarker(location: BrowserLocation = window.location) {
  const params = new URLSearchParams(location.search)
  if (!params.has(SSO_MARKER_PARAM)) return
  params.delete(SSO_MARKER_PARAM)
  const search = params.toString()
  history.replaceState(null, '', location.pathname + (search ? `?${search}` : '') + window.location.hash)
}
