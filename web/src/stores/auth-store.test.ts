import { describe, expect, it } from 'vitest'

import { canAccessDataWorks, canManageDataWorksSettings, canWriteDataWorks } from './auth-store'

describe('Data Works session permissions', () => {
  it('admin:read scope가 있는 JWT 세션만 운영 화면을 조회한다', () => {
    expect(canAccessDataWorks('jwt', { role: 'admin', scopes: ['admin:read'] })).toBe(true)
    expect(canAccessDataWorks('jwt', { role: 'developer', scopes: ['models:read'] })).toBe(false)
  })

  it('쓰기 UI는 admin:write scope와 레거시 관리자 세션에만 허용한다', () => {
    expect(canWriteDataWorks('jwt', { role: 'viewer', scopes: ['admin:read'] })).toBe(false)
    expect(canWriteDataWorks('jwt', { role: 'admin', scopes: ['admin:read', 'admin:write'] })).toBe(true)
    expect(canAccessDataWorks('legacy', null)).toBe(true)
    expect(canWriteDataWorks('legacy', null)).toBe(true)
  })

  it('설정 화면은 부분 관리자와 admin:write 사용자에게 유지한다', () => {
    expect(canManageDataWorksSettings('jwt', { role: 'ops_admin', scopes: ['admin:read'] })).toBe(true)
    expect(canManageDataWorksSettings('jwt', { role: 'custom_operator', scopes: ['admin:read', 'admin:write'] })).toBe(true)
    expect(canManageDataWorksSettings('jwt', { role: 'viewer', scopes: ['admin:read'] })).toBe(false)
  })
})
