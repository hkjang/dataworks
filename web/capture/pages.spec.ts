import { expect, test, type Page } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { installDemoRoutes, PRIMARY_PRODUCT } from './dataworks-demo-fixtures'

const docsRoot = fileURLToPath(new URL('../../docs/assets/', import.meta.url))
const screenshotRoot = path.join(docsRoot, 'screenshots')

const corePages = [
  { path: '/', file: '01-control-room.jpg', heading: '팩토리 관제실' },
  { path: '/assets', file: '02-data-assets.jpg', heading: '데이터 자산' },
  { path: '/factory', file: '03-factory-floor.jpg', heading: '데이터 상품 공장' },
  { path: '/products', file: '04-products.jpg', heading: '데이터 상품' },
  { path: '/review', file: '05-review-center.jpg', heading: '검토 센터' },
  { path: '/portfolio', file: '06-supply-chain-map.jpg', heading: '공급망 지도', waitFor: '.react-flow__node' },
  { path: '/marketplace', file: '07-marketplace.jpg', heading: '데이터 마켓플레이스' },
  { path: '/analytics', file: '08-analytics.jpg', heading: '성과 분석', waitFor: '.recharts-surface' },
  { path: '/governance', file: '09-governance.jpg', heading: '거버넌스' },
  { path: '/personal', file: '10-personal-workspace.jpg', heading: '데모 관리자님의 작업 공간' },
  { path: '/personal/keys', file: '11-personal-api-keys.jpg', heading: '내 API 키' },
  { path: '/settings', file: '12-admin-ai-mcp.jpg', heading: '관리자 설정' },
] as const

const productTabs = [
  { tab: '', file: '00-overview-release-gate.jpg' },
  { tab: 'assets', file: '01-assets.jpg' },
  { tab: 'canvas', file: '02-blueprint.jpg' },
  { tab: 'api', file: '03-api.jpg' },
  { tab: 'customers', file: '04-customers.jpg' },
  { tab: 'risk', file: '05-risk.jpg' },
  { tab: 'approvals', file: '06-approvals.jpg' },
  { tab: 'evidence', file: '07-evidence.jpg' },
  { tab: 'contracts', file: '08-contracts.jpg' },
  { tab: 'usage', file: '09-usage.jpg' },
  { tab: 'revenue', file: '10-revenue.jpg' },
  { tab: 'versions', file: '11-versions.jpg' },
  { tab: 'activity', file: '12-activity.jpg' },
] as const

test('문서용 전체 화면을 가상 데이터로 캡처한다', async ({ page }, testInfo) => {
  test.setTimeout(600_000)
  const project = testInfo.project.name
  const outputDir = path.join(screenshotRoot, project)
  const productDir = path.join(outputDir, 'product')
  await mkdir(productDir, { recursive: true })

  const routeState = await installDemoRoutes(page)
  const consoleErrors: string[] = []
  const pageErrors: string[] = []
  page.on('console', (message) => { if (message.type() === 'error') consoleErrors.push(message.text()) })
  page.on('pageerror', (error) => pageErrors.push(error.message))

  await page.goto('./')
  await expect(page.getByRole('heading', { name: '다시 만나 반갑습니다' })).toBeVisible()
  await expect(page.getByText('서비스 버전 v0.9.38')).toBeVisible()
  await capture(page, path.join(outputDir, '00-login.jpg'), false)

  routeState.setAuthenticated(true)
  // 로그인 화면의 인증 여부 확인에서 발생하는 401은 의도된 상태이므로
  // 이후 실제 서비스 화면에서 발생하는 콘솔 오류만 검증합니다.
  consoleErrors.length = 0

  for (const item of corePages) {
    await gotoApp(page, item.path, item.heading, item.waitFor)
    await capture(page, path.join(outputDir, item.file))

    if (project === 'desktop' && item.path === '/') {
      await page.setViewportSize({ width: 1200, height: 630 })
      await settle(page)
      await capture(page, path.join(docsRoot, 'dataworks-social-card.jpg'), false, 92)
      await page.setViewportSize({ width: 1440, height: 960 })
    }
  }

  await gotoApp(page, '/settings', '관리자 설정')
  await page.getByRole('tab', { name: /Keycloak SSO/ }).click()
  await expect(page.getByRole('heading', { name: 'Keycloak OIDC 간편 연동' })).toBeVisible()
  await capture(page, path.join(outputDir, '13-admin-keycloak-sso.jpg'))

  await page.getByRole('tab', { name: /전체 설정/ }).click()
  await expect(page.getByPlaceholder('설정 키 또는 설명 검색')).toBeVisible()
  await capture(page, path.join(outputDir, '14-admin-runtime-settings.jpg'))

  await gotoApp(page, '/', '팩토리 관제실')
  if (project === 'mobile') {
    await page.getByRole('button', { name: '메뉴 열기' }).click()
    await expect(page.getByRole('navigation', { name: '주요 메뉴' })).toBeVisible()
    await capture(page, path.join(outputDir, '19-mobile-navigation.jpg'), false)
  }
  await page.locator('button[aria-haspopup="menu"]').click({ force: project === 'mobile' })
  await expect(page.getByRole('menu', { name: '사용자 메뉴' })).toBeVisible()
  await expect(page.getByText('서비스 버전').last()).toBeVisible()
  await capture(page, path.join(outputDir, '15-profile-menu.jpg'), false)

  await gotoApp(page, '/', '팩토리 관제실')
  await page.locator('.command-trigger').click()
  const searchDialog = page.getByRole('dialog', { name: '통합 검색' })
  await expect(searchDialog).toBeVisible()
  await searchDialog.getByPlaceholder('상품, 자산, 고객, 계약 검색…').fill('신용')
  await expect(searchDialog.getByText('중소기업 신용 인사이트 API')).toBeVisible()
  await capture(page, path.join(outputDir, '16-command-palette.jpg'), false)
  await searchDialog.getByRole('button', { name: '통합 검색 닫기' }).click()
  await expect(searchDialog).toBeHidden()

  await page.getByRole('button', { name: '알림' }).click()
  await expect(page.getByRole('region', { name: '알림 센터' })).toBeVisible()
  await capture(page, path.join(outputDir, '17-notification-center.jpg'), false)

  await gotoApp(page, `/factory?copilot=1&product=${PRIMARY_PRODUCT}`, '데이터 상품 공장')
  const copilot = page.getByRole('complementary', { name: 'Data Works 코파일럿' })
  await expect(copilot).toBeVisible()
  await copilot.getByRole('button', { name: '보내기' }).click()
  await expect(copilot.getByText(/법무 승인이 대기 중/)).toBeVisible()
  await capture(page, path.join(outputDir, '18-dataworks-copilot.jpg'), false)

  for (const item of productTabs) {
    const suffix = item.tab ? `/${item.tab}` : ''
    await gotoApp(page, `/products/${PRIMARY_PRODUCT}${suffix}`, '중소기업 신용 인사이트 API')
    await capture(page, path.join(productDir, item.file))
  }

  expect(routeState.unexpected, '예상하지 않은 API 또는 외부 네트워크 요청').toEqual([])
  expect(pageErrors, '브라우저 페이지 오류').toEqual([])
  expect(consoleErrors, '브라우저 콘솔 오류').toEqual([])
})

async function gotoApp(page: Page, route: string, heading: string, waitFor?: string) {
  await page.goto(`.${route}`)
  await expect(page.locator('.page-canvas')).toBeVisible()
  await expect(page.getByRole('heading', { name: heading, exact: true }).first()).toBeVisible()
  if (waitFor) await expect(page.locator(waitFor).first()).toBeVisible()
  await settle(page)
}

async function settle(page: Page) {
  await page.waitForLoadState('networkidle')
  await page.evaluate(async () => { await document.fonts.ready })
  await page.waitForTimeout(120)
}

async function capture(page: Page, file: string, fullPage = true, quality = 88) {
  await settle(page)
  await page.screenshot({
    path: file,
    type: 'jpeg',
    quality,
    fullPage,
    animations: 'disabled',
    caret: 'hide',
  })
}
