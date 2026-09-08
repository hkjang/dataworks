import { expect, test, type Page } from '@playwright/test'

async function mockPlatform(page: Page) {
  await page.route('**/auth/me', (route) => route.fulfill({ json: { auth_enabled: true, version: 'v0.9.48', user: { id: 'tester', email: 'tester@dataworks.local', name: '테스트 관리자', role: 'super_admin', scopes: ['admin:read', 'admin:write'], default_home: '#/dataworks/home' } } }))
  await page.route('**/auth/sso/status', (route) => route.fulfill({ json: { keycloak_enabled: false, version: 'v0.9.48' } }))
  await page.route('**/me/dashboard', (route) => route.fulfill({ json: {
    user_id: 'tester', today: { requests: 2, tokens: 100, cost_krw: 10, errors: 0 }, month: { requests: 20, tokens: 1000, cost_krw: 100, errors: 1 },
    profile: { requests: 20, total_cost_krw: 100, avg_cost_per_request: 5, avg_latency_ms: 120, success_rate: .95, error_rate: .05, cache_rate: .2, text2sql_usage_rate: .1, mcp_usage_rate: .3, risk_score: 4, summary: '정상 사용 패턴입니다.' },
    potential_savings_krw: 15, potential_savings_model: 'local', key_alerts: [], recent_failures: [],
  } }))
  await page.route('**/me/keys', (route) => route.fulfill({ json: { api_keys: [{ id: 'key-1', name: '분석 노트북', role: 'operator', status: 'active', scopes: ['chat:completion'], allowed_ips: [], allowed_models: ['qwen-*'], denied_models: [], allowed_providers: ['internal'], denied_providers: [], budget_limit_krw: 50000, expires_at: '2027-08-31T23:59:59Z', created_at: '2026-08-27T00:00:00Z' }], role: '운영자', grantable_scopes: ['chat:completion', 'mcp:use'] } }))
  await page.route('**/admin/providers', (route) => route.fulfill({ json: { providers: [] } }))
  await page.route('**/admin/settings/effective', (route) => route.fulfill({ json: { settings: [] } }))
  await page.route('**/admin/sso/keycloak/config', (route) => route.fulfill({ json: {
    enabled: false, issuer_url: '', client_id: '', client_secret_set: false, redirect_uri: '', scopes: ['openid', 'profile', 'email'],
    default_role: 'developer', role_claim: 'realm_access.roles', group_claim: 'groups', allow_local_login: true, role_map: {}, source: 'default', updated_at: '',
  } }))
  await page.route('**/admin/roles', (route) => route.fulfill({ json: {
    all_scopes: ['models:read', 'admin:read', 'admin:write', 'costs:read'],
    roles: [
      { role: 'super_admin', description: '최고 관리자', scopes: ['models:read', 'admin:read', 'admin:write'], default_home: '#/dataworks/home', is_admin: true, is_system: true, rank: 5, user_count: 1, active_user_count: 1, can_assign: true },
      { role: 'developer', description: '개발자', scopes: ['models:read'], default_home: '#/factory', is_admin: false, is_system: true, rank: 2, user_count: 0, active_user_count: 0, can_assign: true },
      { role: 'cost_analyst', description: '비용 분석 담당', scopes: ['models:read', 'costs:read'], default_home: '#/factory', is_admin: false, is_system: false, rank: 1, user_count: 0, active_user_count: 0, can_assign: true },
    ],
  } }))
  await page.route('**/admin/users', (route) => route.fulfill({ json: { auth_users: [{ id: 'tester', email: 'tester@dataworks.local', name: '테스트 관리자', role: 'super_admin', status: 'active', team_id: 'team_data', created_at: '2026-08-01T00:00:00Z' }] } }))
  await page.route('**/admin/dataworks/**', (route) => {
    const url = route.request().url()
    if (url.includes('/action-center')) return route.fulfill({ json: { summary: { approval_pending: 0, blocked_launches: 0, low_fit_scores: 0, expiring_contracts: 0, inactive_access: 0, stale_watermarks: 0, negative_margin: 0, retirement_candidates: 0 }, actions: [] } })
    if (url.includes('/factory/runs')) return route.fulfill({ json: { runs: [], evaluation_summaries: {} } })
    if (url.includes('/assets/readiness')) return route.fulfill({ json: { readiness: [] } })
    if (url.endsWith('/assets')) return route.fulfill({ json: { assets: [] } })
    if (url.endsWith('/products')) return route.fulfill({ json: { products: [] } })
    if (url.endsWith('/home')) return route.fulfill({ json: { dashboard: { total_assets: 0, total_products: 0, published_products: 0, review_pending: 0, high_risk: 0, poc_pending: 0, ideas_total: 0, avg_revenue_score: 0 }, top_products: [] } })
    if (url.includes('/portfolio/graph')) return route.fulfill({ json: { graph: { nodes: [], edges: [] } } })
    return route.fulfill({ json: {} })
  })
}

test.beforeEach(async ({ page }) => {
  await mockPlatform(page)
})

test('developer SSO 세션은 action-center를 호출하지 않고 개인 작업 공간으로 이동한다', async ({ page }) => {
  await page.unroute('**/auth/me')
  let meAuthorization = ''
  await page.route('**/auth/me', (route) => {
    meAuthorization = route.request().headers().authorization ?? ''
    return route.fulfill({ json: {
      auth_enabled: true,
      version: 'v0.9.48',
      user: {
        id: 'sso-developer',
        email: 'developer@dataworks.local',
        name: 'SSO 개발자',
        role: 'developer',
        scopes: ['models:read', 'mcp:use'],
        default_home: '#/factory',
      },
    } })
  })
  let actionCenterRequests = 0
  page.on('request', (request) => {
    if (request.url().includes('/admin/dataworks/action-center')) actionCenterRequests += 1
  })

  await page.goto('./#kc_access=sso-access-token&kc_refresh=sso-refresh-token')

  await expect(page).toHaveURL(/\/dataworks\/personal$/)
  await expect(page.getByRole('heading', { name: /작업 공간/ })).toBeVisible()
  await expect.poll(() => meAuthorization).toBe('Bearer sso-access-token')
  await expect.poll(() => actionCenterRequests).toBe(0)
  await expect(page.getByRole('navigation', { name: '주요 메뉴' })).not.toContainText('검토 센터')
})

test('한국어 관제실과 주요 메뉴를 연다', async ({ page }) => {
  await page.goto('./')

  await expect(page.getByRole('heading', { name: '팩토리 관제실' })).toBeVisible()
  await expect(page.getByRole('navigation', { name: '주요 메뉴' })).toContainText('데이터 상품')
  await expect(page.getByRole('link', { name: 'Data Works 홈' })).toBeVisible()
  await expect(page.getByText('생산 흐름')).toBeVisible()
})

test('직접 URL과 새로고침 뒤에도 선택한 메뉴를 유지한다', async ({ page }) => {
  await page.goto('./products')
  await expect(page.getByRole('heading', { name: '데이터 상품' })).toBeVisible()
  await page.reload()
  await expect(page).toHaveURL(/\/dataworks\/products$/)
  await expect(page.getByRole('heading', { name: '데이터 상품' })).toBeVisible()
  await expect(page.getByRole('link', { name: '데이터 상품', exact: true })).toHaveClass(/is-active/)
})

test('쓰기 권한으로 데이터 자산을 등록한다', async ({ page }) => {
  let created: Record<string, unknown> | undefined
  await page.route('**/admin/dataworks/assets', async (route) => {
    if (route.request().method() === 'POST') {
      created = route.request().postDataJSON() as Record<string, unknown>
      return route.fulfill({ json: { ok: true, asset_key: created.asset_key } })
    }
    return route.fulfill({ json: { assets: [] } })
  })

  await page.goto('./assets')
  await page.getByRole('button', { name: '자산 등록' }).click()
  const dialog = page.getByRole('dialog', { name: '새 데이터 자산 등록' })
  await dialog.getByLabel('자산 키').fill('risk.events')
  await dialog.getByLabel('자산 이름').fill('위험 이벤트')
  await dialog.getByLabel('담당자').fill('risk-data')
  await dialog.getByRole('button', { name: '자산 등록' }).click()

  await expect(dialog).toBeHidden()
  await expect.poll(() => created).toMatchObject({
    asset_key: 'risk.events',
    name: '위험 이벤트',
    owner: 'risk-data',
  })
})

test('통합 검색을 Escape 키로 닫는다', async ({ page }) => {
  await page.goto('./')
  await page.getByRole('button', { name: /통합 검색/ }).click()
  await expect(page.getByRole('dialog', { name: '통합 검색' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: '통합 검색' })).toBeHidden()
})

test('사용자 정의 역할을 설계하고 저장 전 권한 영향을 확인한다', async ({ page }) => {
  let created: Record<string, unknown> | undefined
  await page.unroute('**/admin/roles')
  await page.route('**/admin/roles', async (route) => {
    if (route.request().method() === 'POST') {
      created = route.request().postDataJSON() as Record<string, unknown>
      return route.fulfill({ status: 201, json: { role: { ...created, is_system: false, rank: 1, user_count: 0, active_user_count: 0, can_assign: true } } })
    }
    return route.fulfill({ json: {
      all_scopes: ['models:read', 'admin:read', 'admin:write', 'costs:read'],
      roles: [{ role: 'super_admin', description: '최고 관리자', scopes: ['models:read', 'admin:read', 'admin:write'], default_home: '#/dataworks/home', is_admin: true, is_system: true, rank: 5, user_count: 1, active_user_count: 1, can_assign: true }],
    } })
  })

  await page.goto('./settings')
  await page.getByRole('tab', { name: /역할 및 권한/ }).click()
  await expect(page.getByRole('heading', { name: '역할 카탈로그' })).toBeVisible()
  await page.getByRole('button', { name: '역할 만들기' }).click()
  const dialog = page.getByRole('dialog', { name: '사용자 정의 역할 만들기' })
  await dialog.getByLabel('역할 식별자').fill('product_analyst')
  await dialog.getByLabel('설명').fill('상품 비용 분석 담당')
  await expect(dialog.getByText('권한 영향 미리보기')).toBeVisible()
  await dialog.getByRole('button', { name: '역할 저장' }).click()

  await expect(dialog).toBeHidden()
  await expect.poll(() => created).toMatchObject({ role: 'product_analyst', description: '상품 비용 분석 담당', scopes: ['models:read'] })
})

test('핵심 화면을 콘솔 오류 없이 탐색한다', async ({ page }) => {
  const errors: string[] = []
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
  page.on('pageerror', (error) => errors.push(error.message))

  const routes = [
    ['/assets', '데이터 자산'],
    ['/factory', '데이터 상품 공장'],
    ['/products', '데이터 상품'],
    ['/review', '검토 센터'],
    ['/portfolio', '공급망 지도'],
    ['/marketplace', '데이터 마켓플레이스'],
    ['/analytics', '성과 분석'],
    ['/governance', '거버넌스'],
    ['/personal', '작업 공간'],
    ['/personal/keys', '내 API 키'],
    ['/settings', '관리자 설정'],
  ] as const

  for (const [path, heading] of routes) {
    await page.goto(`.${path}`)
    await expect(page.getByRole('heading', { name: new RegExp(heading) }).first()).toBeVisible()
  }
  expect(errors).toEqual([])
})

test('발급된 키의 권한 정책을 조정한다', async ({ page }) => {
  let update: Record<string, unknown> | undefined
  await page.route('**/me/keys/key-1', async (route) => {
    update = route.request().postDataJSON() as Record<string, unknown>
    await route.fulfill({ json: { id: 'key-1', status: 'active', ...update } })
  })
  await page.goto('./personal/keys')
  const keyCard = page.locator('article').filter({ hasText: '분석 노트북' })
  await keyCard.getByText('키 권한 조정').click()
  await keyCard.getByLabel('허용 IP·CIDR').fill('10.20.0.0/16')
  await keyCard.getByRole('button', { name: '권한 정책 저장' }).click()
  await expect.poll(() => update?.allowed_ips).toEqual(['10.20.0.0/16'])
  expect(update?.scopes).toEqual(['chat:completion'])
  expect(update?.allowed_models).toEqual(['qwen-*'])
})
