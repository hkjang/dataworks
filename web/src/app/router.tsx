/* eslint-disable react-refresh/only-export-components */
import { lazy, Suspense, type ComponentType } from 'react'
import { createBrowserRouter, Link, Navigate } from 'react-router-dom'

import { AppShell } from '@/components/layout/app-shell'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { PageLoader } from '@/components/ui/query-state'

const HomePage = lazy(() => import('@/pages/home/home-page').then((module) => ({ default: module.HomePage })))
const AssetsPage = lazy(() => import('@/pages/assets/assets-page').then((module) => ({ default: module.AssetsPage })))
const ProductsPage = lazy(() => import('@/pages/products/products-page').then((module) => ({ default: module.ProductsPage })))
const ProductWorkspacePage = lazy(() => import('@/pages/products/product-workspace-page').then((module) => ({ default: module.ProductWorkspacePage })))
const PersonalPage = lazy(() => import('@/pages/personal/personal-page').then((module) => ({ default: module.PersonalPage })))
const SettingsPage = lazy(() => import('@/pages/settings/settings-page').then((module) => ({ default: module.SettingsPage })))
const secondaryPage = <T extends keyof typeof import('@/pages/secondary/secondary-pages')>(name: T) =>
  lazy(() => import('@/pages/secondary/secondary-pages').then((module) => ({ default: module[name] as ComponentType })))
const FactoryPage = secondaryPage('FactoryPage')
const ReviewPage = secondaryPage('ReviewPage')
const ProductGraphPage = secondaryPage('ProductGraphPage')
const MarketplacePage = secondaryPage('MarketplacePage')
const AnalyticsPage = secondaryPage('AnalyticsPage')
const GovernancePage = secondaryPage('GovernancePage')

const render = (Component: ComponentType) => <Suspense fallback={<PageLoader />}><Component /></Suspense>

export const router = createBrowserRouter(
  [
    {
      path: '/',
      element: <AppShell />,
      children: [
        { index: true, element: render(HomePage) },
        { path: 'assets', element: render(AssetsPage) },
        { path: 'factory', element: render(FactoryPage) },
        { path: 'products', element: render(ProductsPage) },
        { path: 'products/:productKey', element: render(ProductWorkspacePage) },
        { path: 'products/:productKey/:tab', element: render(ProductWorkspacePage) },
        { path: 'review', element: render(ReviewPage) },
        { path: 'portfolio', element: render(ProductGraphPage) },
        { path: 'graph', element: <Navigate to="/portfolio" replace /> },
        { path: 'marketplace', element: render(MarketplacePage) },
        { path: 'analytics', element: render(AnalyticsPage) },
        { path: 'governance', element: render(GovernancePage) },
        { path: 'personal', element: render(PersonalPage) },
        { path: 'personal/keys', element: render(PersonalPage) },
        { path: 'settings', element: render(SettingsPage) },
        { path: '*', element: <NotFoundPage /> },
      ],
    },
  ],
  { basename: '/dataworks' },
)

function NotFoundPage() {
  return (
    <Card>
      <CardContent className="flex min-h-[60vh] flex-col items-center justify-center text-center">
        <p className="text-xs font-extrabold uppercase tracking-[.18em] text-[var(--accent)]">404</p>
        <h1 className="mt-4 text-3xl font-[750] tracking-[-.05em] text-[var(--ink)]">페이지를 찾을 수 없습니다</h1>
        <p className="mt-2 text-sm text-[var(--muted)]">요청한 Data Works 화면이 존재하지 않습니다.</p>
        <Button className="mt-6" asChild><Link to="/">관제실로 이동</Link></Button>
      </CardContent>
    </Card>
  )
}
