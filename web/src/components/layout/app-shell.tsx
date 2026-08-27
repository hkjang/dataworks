import { useQuery } from '@tanstack/react-query'
import {
  Bell,
  Blocks,
  Bot,
  Boxes,
  ChartNoAxesCombined,
  ChevronDown,
  CircleGauge,
  Database,
  Factory,
  FileCheck2,
  KeyRound,
  LogOut,
  Menu,
  Moon,
  PanelTop,
  Search,
  Settings,
  ShieldCheck,
  ShoppingBag,
  Sun,
  UserRound,
  X,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { Button } from '@/components/ui/button'
import { CopilotPanel } from '@/features/copilot/copilot-panel'
import { cn, initials } from '@/lib/utils'
import { roleLabel } from '@/lib/labels.ko'
import { useAuthStore } from '@/stores/auth-store'
import { useUIStore } from '@/stores/ui-store'
import { CommandPalette } from './command-palette'
import { NotificationPanel } from './notification-panel'

const serviceNavGroups = [
  {
    label: '작업 공간',
    items: [
      { to: '/', label: '관제실', icon: CircleGauge },
      { to: '/assets', label: '데이터 자산', icon: Database },
      { to: '/factory', label: '상품 공장', icon: Factory },
      { to: '/products', label: '데이터 상품', icon: Boxes },
      { to: '/review', label: '검토 센터', icon: FileCheck2 },
    ],
  },
  {
    label: '서비스 관리',
    items: [
      { to: '/portfolio', label: '공급망 지도', icon: Blocks },
      { to: '/marketplace', label: '마켓플레이스', icon: ShoppingBag },
      { to: '/analytics', label: '성과 분석', icon: ChartNoAxesCombined },
      { to: '/governance', label: '거버넌스', icon: ShieldCheck },
    ],
  },
]

const pageNames: Record<string, string> = {
  '/': '관제실',
  '/assets': '데이터 자산',
  '/factory': 'AI 상품 공장',
  '/products': '데이터 상품',
  '/review': '검토 센터',
  '/portfolio': '공급망 지도',
  '/marketplace': '마켓플레이스',
  '/analytics': '성과 분석',
  '/governance': '거버넌스',
  '/personal': '내 작업 공간',
  '/personal/keys': '내 API 키',
  '/settings': '관리자 설정',
}

export function AppShell() {
  const location = useLocation()
  const sidebarOpen = useUIStore((state) => state.sidebarOpen)
  const setSidebarOpen = useUIStore((state) => state.setSidebarOpen)
  const setCommandOpen = useUIStore((state) => state.setCommandOpen)
  const notificationsOpen = useUIStore((state) => state.notificationsOpen)
  const setNotificationsOpen = useUIStore((state) => state.setNotificationsOpen)
  const theme = useUIStore((state) => state.theme)
  const toggleTheme = useUIStore((state) => state.toggleTheme)
  const user = useAuthStore((state) => state.user)
  const version = useAuthStore((state) => state.version)
  const mode = useAuthStore((state) => state.mode)
  const logout = useAuthStore((state) => state.logout)
  const setAccessOpen = useUIStore((state) => state.setAccessOpen)
  const [profileOpen, setProfileOpen] = useState(false)
  const profileRef = useRef<HTMLDivElement>(null)
  const actionQuery = useQuery({
    queryKey: ['dataworks', 'action-center'],
    queryFn: dataworksApi.actionCenter,
    staleTime: 30_000,
  })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
  }, [theme])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setCommandOpen(true)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [setCommandOpen])

  useEffect(() => setSidebarOpen(false), [location.pathname, setSidebarOpen])

  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (!profileRef.current?.contains(event.target as Node)) setProfileOpen(false)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setProfileOpen(false)
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', escape)
    }
  }, [])

  const actionCount = actionQuery.data?.actions.length ?? 0
  const canManageService = mode === 'legacy' || Boolean(user?.role?.toLowerCase().includes('admin'))
  const productMatch = location.pathname.match(/^\/products\/([^/]+)/)
  const currentPage = productMatch
    ? decodeURIComponent(productMatch[1])
    : (pageNames[location.pathname] ?? 'Data Works')

  return (
    <div className="app-frame">
      <button
        aria-label="메뉴 닫기"
        className={cn('sidebar-scrim', sidebarOpen && 'is-open')}
        onClick={() => setSidebarOpen(false)}
      />
      <aside className={cn('app-sidebar', sidebarOpen && 'is-open')}>
        <div className="flex h-[76px] items-center justify-between px-5">
          <NavLink to="/" className="flex items-center gap-3" aria-label="Data Works 홈">
            <span className="brand-mark"><Database className="size-[18px]" /></span>
            <span>
              <span className="block text-[13px] font-black tracking-[-.03em] text-[var(--ink)]">DATA WORKS</span>
              <span className="block text-[11px] font-bold tracking-[.12em] text-[var(--muted)]">상품 운영 체계</span>
            </span>
          </NavLink>
          <Button className="lg:hidden" variant="ghost" size="icon" onClick={() => setSidebarOpen(false)}><X className="size-4" /></Button>
        </div>

        <nav className="flex-1 overflow-y-auto px-3 pb-5" aria-label="주요 메뉴">
          {serviceNavGroups.map((group) => (
            <div key={group.label} className="mb-6">
              <p className="px-3 pb-2 text-[11px] font-black tracking-[.14em] text-[var(--muted-soft)]">{group.label}</p>
              <div className="space-y-1">
                {group.items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.to === '/'}
                    className={({ isActive }) => cn('nav-item', isActive && 'is-active')}
                  >
                    <item.icon className="size-[17px]" strokeWidth={1.8} />
                    <span>{item.label}</span>
                    {item.to === '/review' && actionCount ? <span className="nav-count">{actionCount}</span> : null}
                  </NavLink>
                ))}
              </div>
            </div>
          ))}
          {mode === 'jwt' ? (
            <div className="mb-6">
              <p className="px-3 pb-2 text-[11px] font-black tracking-[.14em] text-[var(--muted-soft)]">개인화</p>
              <div className="space-y-1">
                <NavLink to="/personal" end className={({ isActive }) => cn('nav-item', isActive && 'is-active')}>
                  <UserRound className="size-[17px]" strokeWidth={1.8} /> 내 작업 공간
                </NavLink>
                <NavLink to="/personal/keys" className={({ isActive }) => cn('nav-item', isActive && 'is-active')}>
                  <KeyRound className="size-[17px]" strokeWidth={1.8} /> 내 API 키
                </NavLink>
              </div>
            </div>
          ) : null}
        </nav>

        <div className="relative border-t border-[var(--line)] p-3" ref={profileRef}>
          {canManageService ? <NavLink to="/settings" className={({ isActive }) => cn('nav-item', isActive && 'is-active')}>
            <Settings className="size-[17px]" strokeWidth={1.8} /> 관리자 설정
          </NavLink> : null}
          {profileOpen ? (
            <div className="profile-context-menu" role="menu" aria-label="사용자 메뉴">
              <div className="border-b border-[var(--line)] px-3 pb-3">
                <p className="truncate text-sm font-bold text-[var(--ink)]">{user?.name || user?.email || '관리자'}</p>
                <p className="mt-1 truncate text-xs text-[var(--muted)]">{user?.email || '로컬 관리자 세션'}</p>
              </div>
              <div className="profile-menu-scroll">
                {mode === 'jwt' ? (
                  <>
                    <Link role="menuitem" to="/personal" onClick={() => setProfileOpen(false)}><UserRound /> 내 작업 공간</Link>
                    <Link role="menuitem" to="/personal/keys" onClick={() => setProfileOpen(false)}><KeyRound /> 내 API 키와 회전</Link>
                  </>
                ) : null}
                {canManageService ? <Link role="menuitem" to="/settings" onClick={() => setProfileOpen(false)}><Settings /> 서비스 관리자 설정</Link> : null}
                <a role="menuitem" href="/openapi.json" target="_blank" rel="noreferrer"><PanelTop /> OpenAPI 명세</a>
                <a role="menuitem" href="/swagger" target="_blank" rel="noreferrer"><Blocks /> API 문서</a>
              </div>
              <div className="profile-version">
                <span>서비스 버전</span><strong>{version || '확인 중'}</strong>
              </div>
              {mode === 'jwt' ? (
                <button role="menuitem" className="profile-logout" onClick={() => void logout()}><LogOut /> 로그아웃</button>
              ) : (
                <button role="menuitem" className="profile-logout" onClick={() => { setProfileOpen(false); setAccessOpen(true) }}><KeyRound /> 관리자 토큰 설정</button>
              )}
            </div>
          ) : null}
          <button
            type="button"
            aria-haspopup="menu"
            aria-expanded={profileOpen}
            className="mt-2 flex w-full items-center gap-3 rounded-xl bg-[var(--surface-muted)] p-3 text-left"
            onClick={() => setProfileOpen((open) => !open)}
          >
            <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-[var(--ink)] text-[11px] font-bold text-[var(--surface)]">
              {initials(user?.name || user?.email)}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-bold text-[var(--ink)]">{user?.name || user?.email?.split('@')[0] || '관리자'}</p>
              <p className="truncate text-xs text-[var(--muted)]">{user?.role ? roleLabel(user.role) : (mode === 'legacy' ? '토큰 모드' : '운영자')}</p>
            </div>
            <ChevronDown className={cn('size-4 text-[var(--muted)] transition-transform', profileOpen && 'rotate-180')} />
          </button>
        </div>
      </aside>

      <div className="app-main">
        <header className="app-topbar">
          <div className="flex min-w-0 items-center gap-3">
            <Button className="lg:hidden" variant="ghost" size="icon" onClick={() => setSidebarOpen(true)} aria-label="메뉴 열기"><Menu className="size-5" /></Button>
            <div className="min-w-0">
              <p className="text-[11px] font-extrabold tracking-[.14em] text-[var(--muted-soft)]">데이터 상품 작업 공간</p>
              <p className="truncate text-sm font-bold tracking-[-.02em] text-[var(--ink)]">{currentPage}</p>
            </div>
          </div>
          <div className="flex items-center gap-1.5">
            <button className="command-trigger" onClick={() => setCommandOpen(true)} aria-label="통합 검색 열기">
              <Search className="size-4" /><span className="hidden sm:inline">통합 검색</span><kbd>⌘K</kbd>
            </button>
            <Button variant="ghost" size="icon" onClick={toggleTheme} aria-label="테마 변경">
              {theme === 'light' ? <Moon className="size-[18px]" /> : <Sun className="size-[18px]" />}
            </Button>
            <div className="relative">
              <Button variant="ghost" size="icon" onClick={() => setNotificationsOpen(!notificationsOpen)} aria-label="알림">
                <Bell className="size-[18px]" />
                {actionCount ? <span className="absolute right-2 top-2 size-2 rounded-full bg-[var(--danger)] ring-2 ring-[var(--surface)]" /> : null}
              </Button>
              <NotificationPanel query={actionQuery} />
            </div>
            <Button className="ml-1 hidden sm:inline-flex" variant="secondary" size="sm" asChild>
              <Link to="/factory?copilot=1"><Bot className="size-3.5" /> 코파일럿에게 묻기</Link>
            </Button>
          </div>
        </header>

        <main className="page-canvas"><Outlet /></main>
      </div>
      <CommandPalette />
      <CopilotPanel />
    </div>
  )
}
