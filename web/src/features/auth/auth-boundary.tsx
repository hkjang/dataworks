import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowRight, Database, KeyRound, LoaderCircle, ShieldCheck, X } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'
import { useUIStore } from '@/stores/ui-store'

const loginSchema = z.object({
  email: z.email('올바른 이메일을 입력해 주세요.'),
  password: z.string().min(1, '비밀번호를 입력해 주세요.'),
})

type LoginValues = z.infer<typeof loginSchema>

export function AuthBoundary({ children }: { children: ReactNode }) {
  const { mode, initialize, markUnauthorized } = useAuthStore()
  const setAccessOpen = useUIStore((state) => state.setAccessOpen)

  useEffect(() => {
    void initialize()
  }, [initialize])

  useEffect(() => {
    const handleUnauthorized = () => {
      if (useAuthStore.getState().mode === 'legacy') setAccessOpen(true)
      else markUnauthorized()
    }
    window.addEventListener('dataworks:unauthorized', handleUnauthorized)
    return () => window.removeEventListener('dataworks:unauthorized', handleUnauthorized)
  }, [markUnauthorized, setAccessOpen])

  if (mode === 'checking') {
    return (
      <div className="grid min-h-screen place-items-center bg-[var(--app-bg)]">
        <div className="flex items-center gap-3 text-sm font-semibold text-[var(--muted)]">
          <LoaderCircle className="size-5 animate-spin text-[var(--accent)]" /> Data Works 준비 중
        </div>
      </div>
    )
  }

  if (mode === 'unauthenticated') return <LoginScreen />
  return (
    <>
      {children}
      <LegacyAccessDialog />
    </>
  )
}

function LoginScreen() {
  const login = useAuthStore((state) => state.login)
  const serviceVersion = useAuthStore((state) => state.version)
  const ssoError = useAuthStore((state) => state.ssoError)
  const [serverError, setServerError] = useState('')
  const [sso, setSSO] = useState<{ enabled: boolean; url: string; version: string } | null>(null)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginValues>({ resolver: zodResolver(loginSchema) })

  useEffect(() => {
    fetch('/auth/sso/status')
      .then((response) => response.json())
      .then((value: { keycloak_enabled?: boolean; login_url?: string; version?: string }) =>
        setSSO({ enabled: Boolean(value.keycloak_enabled), url: value.login_url ?? '/auth/keycloak/login', version: value.version ?? '' }),
      )
      .catch(() => setSSO(null))
  }, [])

  const onSubmit = handleSubmit(async (values) => {
    setServerError('')
    try {
      await login(values.email, values.password)
    } catch (error) {
      setServerError(error instanceof Error ? error.message : '로그인에 실패했습니다.')
    }
  })

  return (
    <main className="login-canvas">
      <section className="login-story" aria-label="Data Works 소개">
        <div className="flex items-center gap-3">
          <span className="brand-mark"><Database className="size-5" /></span>
          <span className="text-sm font-extrabold tracking-[-.02em]">DATA WORKS</span>
        </div>
        <div className="max-w-xl">
          <p className="mb-4 text-sm font-extrabold tracking-[.18em] text-white/55">데이터 상품 운영 체계</p>
          <h1 className="text-[clamp(2.6rem,6vw,5.6rem)] font-[750] leading-[.94] tracking-[-.07em]">
            신뢰할 수 있는 데이터를<br />작동하는 상품으로.
          </h1>
          <p className="mt-6 max-w-lg text-base leading-7 text-white/65">
            자산 발견부터 승인, 증적, 계약, 출시와 수익성까지 하나의 운영 흐름에서 관리합니다.
          </p>
        </div>
        <div className="grid max-w-xl grid-cols-3 gap-3 text-xs text-white/60">
          {['준비도', '거버넌스', '수익'].map((item, index) => (
            <div key={item} className="border-t border-white/20 pt-3">
              <span className="mr-2 text-white/35">0{index + 1}</span>{item}
            </div>
          ))}
        </div>
      </section>

      <section className="login-form-panel">
        <form className="w-full max-w-sm" onSubmit={onSubmit}>
          <div className="mb-8 grid size-12 place-items-center rounded-2xl bg-[var(--info-soft)] text-[var(--accent)] lg:hidden">
            <Database className="size-5" />
          </div>
          <p className="text-sm font-extrabold tracking-[.16em] text-[var(--accent)]">보안 작업 공간</p>
          <h2 className="mt-3 text-3xl font-[750] tracking-[-.045em] text-[var(--ink)]">다시 만나 반갑습니다</h2>
          <p className="mt-2 text-sm leading-6 text-[var(--muted)]">계정으로 로그인해 Data Works를 계속 사용하세요.</p>

          <label className="field-label mt-8" htmlFor="email">이메일</label>
          <input id="email" className="field-input" autoComplete="username" placeholder="admin@company.com" {...register('email')} />
          {errors.email ? <p className="field-error">{errors.email.message}</p> : null}

          <label className="field-label mt-5" htmlFor="password">비밀번호</label>
          <input id="password" className="field-input" type="password" autoComplete="current-password" placeholder="••••••••" {...register('password')} />
          {errors.password ? <p className="field-error">{errors.password.message}</p> : null}
          {ssoError ? <p className="mt-4 rounded-xl bg-[var(--danger-soft)] px-3 py-2 text-sm font-medium text-[var(--danger)]">Keycloak 로그인에 실패했습니다: {ssoError}</p> : null}
          {serverError ? <p className="mt-4 rounded-xl bg-[var(--danger-soft)] px-3 py-2 text-sm font-medium text-[var(--danger)]">{serverError}</p> : null}

          <Button className="mt-6 w-full" size="lg" variant="accent" disabled={isSubmitting}>
            {isSubmitting ? <LoaderCircle className="size-4 animate-spin" /> : <ShieldCheck className="size-4" />}
            로그인 <ArrowRight className="ml-auto size-4" />
          </Button>
          {sso?.enabled ? (
            <Button className="mt-3 w-full" size="lg" variant="secondary" type="button" onClick={() => { window.location.href = sso.url }}>
              Keycloak SSO로 계속
            </Button>
          ) : null}
          <p className="mt-8 flex items-center justify-center gap-2 text-xs text-[var(--muted)]">
            <KeyRound className="size-3.5" /> 세션과 접근 범위는 서버 정책으로 보호됩니다.
          </p>
          <p className="mt-3 text-center font-mono text-xs text-[var(--muted-soft)]">서비스 버전 {serviceVersion || sso?.version || '확인 중'}</p>
        </form>
      </section>
    </main>
  )
}

function LegacyAccessDialog() {
  const open = useUIStore((state) => state.accessOpen)
  const setOpen = useUIStore((state) => state.setAccessOpen)
  const setLegacyToken = useAuthStore((state) => state.setLegacyToken)
  const [token, setToken] = useState(() => sessionStorage.getItem('adminToken') ?? '')

  if (!open) return null
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setOpen(false) }}>
      <section className="dialog-card" role="dialog" aria-modal="true" aria-labelledby="access-title">
        <div className="flex items-start justify-between gap-4">
          <div>
            <span className="mb-4 grid size-10 place-items-center rounded-xl bg-[var(--info-soft)] text-[var(--accent)]"><KeyRound className="size-4" /></span>
            <h2 id="access-title" className="text-xl font-bold tracking-[-.03em] text-[var(--ink)]">관리자 토큰 연결</h2>
            <p className="mt-2 text-sm leading-6 text-[var(--muted)]">기존 ADMIN_TOKEN 모드에서 API 요청에 사용할 토큰입니다. 브라우저 세션에만 저장됩니다.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={() => setOpen(false)} aria-label="닫기"><X className="size-4" /></Button>
        </div>
        <label className="field-label mt-6" htmlFor="admin-token">관리자 Bearer 토큰</label>
        <input id="admin-token" className="field-input font-mono text-xs" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder="ADMIN_TOKEN" />
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="secondary" onClick={() => setOpen(false)}>취소</Button>
          <Button variant="accent" onClick={() => { setLegacyToken(token); setOpen(false); window.location.reload() }}>연결하고 새로고침</Button>
        </div>
      </section>
    </div>
  )
}
