import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  beginSilentSSO,
  clearSilentSSOState,
  interactiveSSOLoginURL,
  markSignedOut,
  safeReturnTo,
  shouldAttemptSilentSSO,
  silentSSOAllowedPath,
  silentSSOAttempted,
  silentSSOLoginURL,
  stripSSOMarker,
} from './silent-sso'

const enabled = { keycloak_enabled: true, auto_login: true, login_url: '/auth/keycloak/login' }
const at = (pathname: string, search = '') => ({ pathname, search })

describe('silent SSO 시도 규칙', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('auto_login 이 꺼져 있거나 SSO 가 꺼져 있으면 시도하지 않는다', () => {
    expect(shouldAttemptSilentSSO({ keycloak_enabled: true }, at('/dataworks/'))).toBe(false)
    expect(shouldAttemptSilentSSO({ keycloak_enabled: true, auto_login: false }, at('/dataworks/'))).toBe(false)
    expect(shouldAttemptSilentSSO({ keycloak_enabled: false, auto_login: true }, at('/dataworks/'))).toBe(false)
    expect(shouldAttemptSilentSSO(null, at('/dataworks/'))).toBe(false)
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'))).toBe(true)
  })

  it('한 탭 세션에 한 번만 시도한다', () => {
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'))).toBe(true)
    beginSilentSSO(enabled.login_url, '/dataworks/products/x', () => {})
    expect(silentSSOAttempted()).toBe(true)
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'))).toBe(false)
    // 새 세션이 생기면 억제가 풀린다.
    clearSilentSSOState()
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'))).toBe(true)
  })

  it('스스로 로그아웃했으면 시도하지 않고, 다시 로그인하면 풀린다', () => {
    markSignedOut()
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'))).toBe(false)
    clearSilentSSOState()
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'))).toBe(true)
  })

  it('콜백이 남긴 ?sso=none 표시가 있으면 저장소가 비어 있어도 시도하지 않는다', () => {
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/products/x', '?sso=none&tab=contracts'))).toBe(false)
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/', '?sso=error'))).toBe(false)
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/', '?tab=contracts'))).toBe(true)
  })

  it('대화형 SSO 실패(kc_error)를 안고 돌아온 페이지에서는 시도하지 않는다', () => {
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'), 'access_denied')).toBe(false)
  })

  it('콜백·로그인 경로와 워크벤치 밖 경로에서는 시도하지 않는다', () => {
    expect(silentSSOAllowedPath('/auth/keycloak/callback')).toBe(false)
    expect(silentSSOAllowedPath('/auth/keycloak/login')).toBe(false)
    expect(silentSSOAllowedPath('/v1/models')).toBe(false)
    expect(silentSSOAllowedPath('/mcp')).toBe(false)
    expect(silentSSOAllowedPath('/healthz')).toBe(false)
    expect(silentSSOAllowedPath('/dataworks')).toBe(true)
    expect(silentSSOAllowedPath('/dataworks/products/x')).toBe(true)
    expect(shouldAttemptSilentSSO(enabled, at('/auth/keycloak/callback'))).toBe(false)
  })

  it('저장소를 읽지 못하면 "이미 시도했다"로 쳐서 막히는 쪽으로 실패한다', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError')
    })
    expect(shouldAttemptSilentSSO(enabled, at('/dataworks/'))).toBe(false)
    expect(silentSSOAttempted()).toBe(true)
  })
})

describe('silent SSO 이동 주소', () => {
  it('최상위 이동으로 prompt=none 과 return_to 를 싣는다', () => {
    expect(silentSSOLoginURL('/auth/keycloak/login', '/dataworks/products/x?tab=contracts')).toBe(
      '/auth/keycloak/login?prompt=none&return_to=%2Fdataworks%2Fproducts%2Fx%3Ftab%3Dcontracts',
    )
    const navigate = vi.fn()
    beginSilentSSO('', '/dataworks/', navigate)
    expect(navigate).toHaveBeenCalledWith('/auth/keycloak/login?prompt=none&return_to=%2Fdataworks%2F')
  })

  it('return_to 는 / 로 시작하고 // 로 시작하지 않는 값만 받는다', () => {
    expect(safeReturnTo('/dataworks/products/x')).toBe('/dataworks/products/x')
    expect(safeReturnTo('//evil.example/')).toBe('/dataworks/')
    expect(safeReturnTo('/\\evil.example/')).toBe('/dataworks/')
    expect(safeReturnTo('https://evil.example/')).toBe('/dataworks/')
    expect(safeReturnTo('')).toBe('/dataworks/')
  })

  it('SSO 버튼은 깊은 링크를 싣되 거절 표시(sso=none)는 떼어 낸다', () => {
    expect(interactiveSSOLoginURL('/auth/keycloak/login', at('/dataworks/products/x', '?sso=none&tab=contracts'))).toBe(
      '/auth/keycloak/login?return_to=%2Fdataworks%2Fproducts%2Fx%3Ftab%3Dcontracts',
    )
    expect(interactiveSSOLoginURL('/auth/keycloak/login', at('/dataworks/', '?sso=none'))).toBe(
      '/auth/keycloak/login?return_to=%2Fdataworks%2F',
    )
  })

  it('세션이 생기면 주소의 sso 표시만 지운다', () => {
    const replaceState = vi.spyOn(history, 'replaceState').mockImplementation(() => {})
    stripSSOMarker(at('/dataworks/products/x', '?sso=none&tab=contracts'))
    expect(replaceState).toHaveBeenCalledWith(null, '', '/dataworks/products/x?tab=contracts')
    replaceState.mockClear()
    stripSSOMarker(at('/dataworks/products/x', '?tab=contracts'))
    expect(replaceState).not.toHaveBeenCalled()
    vi.restoreAllMocks()
  })
})
