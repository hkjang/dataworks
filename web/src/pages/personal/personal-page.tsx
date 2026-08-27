import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, Copy, KeyRound, RefreshCw, ShieldCheck, UserRound, X } from 'lucide-react'
import { useState } from 'react'
import { useForm, type UseFormRegisterReturn } from 'react-hook-form'
import { Link, useLocation } from 'react-router-dom'
import { z } from 'zod'

import { platformApi, type APIKeyPolicyInput, type PersonalAPIKey } from '@/api/platform'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { ErrorState, PageLoader } from '@/components/ui/query-state'
import { formatDate, formatNumber } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

const keySchema = z.object({
  name: z.string().trim().min(2, '키 이름을 2자 이상 입력해 주세요.'),
  expires_at: z.string().optional(),
  allowed_ips: z.string().optional(),
  allowed_models: z.string().optional(),
  denied_models: z.string().optional(),
  allowed_providers: z.string().optional(),
  denied_providers: z.string().optional(),
  budget_limit_krw: z.string().refine((value) => !value || (Number.isFinite(Number(value)) && Number(value) >= 0), '예산은 0 이상의 숫자여야 합니다.'),
})

type KeyForm = z.infer<typeof keySchema>

const scopeDescriptions: Record<string, string> = {
  'chat:completion': '대화형 AI 호출',
  'embeddings:create': '임베딩 생성',
  'models:read': '모델 목록 조회',
  'routing:read': '라우팅 정보 조회',
  'observability:read': '관측 데이터 조회',
  'costs:read': '비용 정보 조회',
  'mcp:use': 'MCP 도구 사용',
  'team:read': '팀 정보 조회',
  'admin:read': '관리자 정보 조회',
  'admin:write': '관리자 설정 변경',
  'mcp:admin': 'MCP 관리',
  'security:read': '보안 정보 조회',
}

export function PersonalPage() {
  const location = useLocation()
  const keysOnly = location.pathname.endsWith('/keys')
  const user = useAuthStore((state) => state.user)
  const mode = useAuthStore((state) => state.mode)
  const dashboard = useQuery({ queryKey: ['personal', 'dashboard'], queryFn: platformApi.personalDashboard, enabled: mode === 'jwt' && !keysOnly, staleTime: 30_000 })

  if (mode !== 'jwt') return <div><PageHeader eyebrow="개인화" title={keysOnly ? '내 API 키' : '내 작업 공간'} description="개인화 기능은 사용자 계정 세션에서 사용할 수 있습니다." /><Card><CardContent className="flex min-h-72 flex-col items-center justify-center text-center"><UserRound className="size-8 text-[var(--muted)]" /><h2 className="mt-4 text-base font-bold text-[var(--ink)]">개인 계정 로그인이 필요합니다</h2><p className="mt-2 text-sm text-[var(--muted)]">Keycloak SSO 또는 로컬 사용자 계정으로 로그인해 주세요.</p></CardContent></Card></div>

  if (!keysOnly && dashboard.isPending) return <PageLoader label="개인 작업 공간을 준비하는 중" />
  if (!keysOnly && dashboard.error) return <ErrorState error={dashboard.error} retry={() => void dashboard.refetch()} />

  return (
    <div>
      <PageHeader
        eyebrow="개인화 · 내 작업 공간"
        title={keysOnly ? '내 API 키' : `${user?.name || user?.email?.split('@')[0] || '사용자'}님의 작업 공간`}
        description={keysOnly ? '개인 키를 발급하고, 발급 후 권한을 조정하며, 안전하게 회전하거나 폐기합니다.' : '내 사용량, 비용, 품질과 개인 보안 상태를 서비스 관리자 화면과 분리해 확인합니다.'}
        actions={keysOnly ? <Button asChild variant="secondary"><Link to="/personal"><UserRound className="size-4" /> 개인화 홈</Link></Button> : <Button asChild variant="secondary"><Link to="/personal/keys"><KeyRound className="size-4" /> 키 관리</Link></Button>}
      />

      {!keysOnly && dashboard.data ? <PersonalOverview dashboard={dashboard.data} /> : null}
      <KeyManager compact={!keysOnly} />
    </div>
  )
}

function PersonalOverview({ dashboard }: { dashboard: Awaited<ReturnType<typeof platformApi.personalDashboard>> }) {
  const profile = dashboard.profile
  return (
    <>
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="개인 사용 지표">
        <PersonalMetric label="오늘 요청" value={formatNumber(dashboard.today.requests)} detail={`오류 ${formatNumber(dashboard.today.errors)}건`} />
        <PersonalMetric label="이번 달 토큰" value={formatNumber(dashboard.month.tokens)} detail={`요청 ${formatNumber(dashboard.month.requests)}건`} />
        <PersonalMetric label="이번 달 비용" value={`₩${formatNumber(Math.round(dashboard.month.cost_krw))}`} detail={`절감 가능 ₩${formatNumber(Math.round(dashboard.potential_savings_krw))}`} />
        <PersonalMetric label="성공률" value={`${Math.round((profile?.success_rate ?? 0) * 100)}%`} detail={`위험 점수 ${profile?.risk_score ?? 0}`} />
      </section>

      <Card className="mt-5">
        <CardHeader>
          <div><h2 className="text-base font-bold text-[var(--ink)]">개인화 신호</h2><p className="mt-1 text-xs text-[var(--muted)]">원문 프롬프트가 아닌 사용 메타데이터에서 계산됩니다.</p></div>
          <Badge tone={(profile?.risk_score ?? 0) >= 35 ? 'danger' : 'success'}>{(profile?.risk_score ?? 0) >= 35 ? '점검 필요' : '정상'}</Badge>
        </CardHeader>
        <CardContent>
          <p className="text-sm leading-6 text-[var(--muted)]">{profile?.summary || '사용 이력이 쌓이면 작업 유형과 모델 선호도를 분석해 맞춤 안내를 제공합니다.'}</p>
          <div className="mt-4 grid gap-3 sm:grid-cols-3">
            <Signal label="평균 응답 시간" value={`${formatNumber(Math.round(profile?.avg_latency_ms ?? 0))}ms`} />
            <Signal label="캐시 활용률" value={`${Math.round((profile?.cache_rate ?? 0) * 100)}%`} />
            <Signal label="MCP 활용률" value={`${Math.round((profile?.mcp_usage_rate ?? 0) * 100)}%`} />
          </div>
        </CardContent>
      </Card>
    </>
  )
}

function KeyManager({ compact }: { compact: boolean }) {
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey: ['personal', 'keys'], queryFn: platformApi.myKeys, staleTime: 10_000 })
  const [selectedScopes, setSelectedScopes] = useState<string[] | null>(null)
  const [revealed, setRevealed] = useState<{ title: string; secret: string } | null>(null)
  const { register, handleSubmit, reset, formState: { errors } } = useForm<KeyForm>({ resolver: zodResolver(keySchema), defaultValues: { name: '', expires_at: '', allowed_ips: '', allowed_models: '', denied_models: '', allowed_providers: '', denied_providers: '', budget_limit_krw: '' } })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['personal', 'keys'] })
  const create = useMutation({
    mutationFn: (values: KeyForm) => platformApi.createMyKey({ name: values.name, ...createPolicy(values, selectedScopes ?? query.data?.grantable_scopes ?? []) }),
    onSuccess: (result) => {
      setRevealed({ title: '새 API 키가 발급되었습니다', secret: result.secret })
      setSelectedScopes(null)
      reset()
      void refresh()
    },
  })
  const update = useMutation({ mutationFn: ({ id, policy }: { id: string; policy: APIKeyPolicyInput }) => platformApi.updateMyKeyPolicy(id, policy), onSuccess: () => void refresh() })
  const rotate = useMutation({ mutationFn: platformApi.rotateMyKey, onSuccess: (result) => { setRevealed({ title: 'API 키 회전이 완료되었습니다', secret: result.secret }); void refresh() } })
  const revoke = useMutation({ mutationFn: platformApi.revokeMyKey, onSuccess: () => void refresh() })

  if (query.isPending) return <div className="mt-5"><PageLoader label="개인 키를 불러오는 중" /></div>
  if (query.error) return <div className="mt-5"><ErrorState error={query.error} retry={() => void query.refetch()} /></div>
  const grantable = query.data.grantable_scopes ?? []
  const keys = query.data.api_keys ?? []

  return (
    <section className={compact ? 'mt-5' : ''} aria-labelledby="my-keys-title">
      <div className="mb-3 flex items-end justify-between gap-3">
        <div><h2 id="my-keys-title" className="text-lg font-bold text-[var(--ink)]">개인 키 관리</h2><p className="mt-1 text-xs text-[var(--muted)]">역할 범위 안에서 키별 권한을 발급 후에도 변경할 수 있습니다.</p></div>
        <Badge tone="info">{query.data.role || '사용자'} · {keys.length}개</Badge>
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(280px,.72fr)_minmax(0,1.28fr)]">
        <Card>
          <CardHeader><h3 className="text-base font-bold text-[var(--ink)]">새 키 발급</h3><KeyRound className="size-5 text-[var(--accent)]" /></CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit((values) => create.mutate(values))}>
              <label className="field-label" htmlFor="key-name">키 이름</label>
              <input id="key-name" className="field-input" placeholder="예: 분석 노트북" {...register('name')} />
              {errors.name ? <p className="field-error">{errors.name.message}</p> : null}
              <label className="field-label mt-4" htmlFor="key-expiry">만료일(선택)</label>
              <input id="key-expiry" className="field-input" type="date" {...register('expires_at')} />
              <fieldset className="mt-5">
                <legend className="field-label">초기 권한</legend>
                <ScopePicker scopes={grantable} selected={selectedScopes ?? grantable} onChange={setSelectedScopes} />
              </fieldset>
              <details className="mt-4 rounded-xl border border-[var(--line)] p-3">
                <summary className="cursor-pointer text-xs font-bold text-[var(--ink)]">고급 권한 제약</summary>
                <div className="mt-4 grid gap-4">
                  <PolicyTextField label="허용 IP·CIDR" placeholder="10.20.0.0/16, 192.168.1.10" registration={register('allowed_ips')} />
                  <PolicyTextField label="허용 모델" placeholder="qwen-*, local-*" registration={register('allowed_models')} />
                  <PolicyTextField label="차단 모델" placeholder="외부-model-*" registration={register('denied_models')} />
                  <PolicyTextField label="허용 공급자" placeholder="internal, vllm" registration={register('allowed_providers')} />
                  <PolicyTextField label="차단 공급자" placeholder="external-*" registration={register('denied_providers')} />
                  <label><span className="field-label">예산 한도(원)</span><input className="field-input" type="number" min="0" step="1" placeholder="0은 무제한" {...register('budget_limit_krw')} />{errors.budget_limit_krw ? <span className="field-error block">{errors.budget_limit_krw.message}</span> : null}</label>
                </div>
              </details>
              {create.error ? <MutationError error={create.error} /> : null}
              <Button className="mt-5 w-full" variant="accent" disabled={create.isPending}><KeyRound className="size-4" /> {create.isPending ? '발급 중…' : '개인 키 발급'}</Button>
            </form>
          </CardContent>
        </Card>

        <Card className="overflow-hidden">
          <CardHeader><div><h3 className="text-base font-bold text-[var(--ink)]">발급된 키</h3><p className="mt-1 text-xs text-[var(--muted)]">비밀값은 발급·회전 직후 한 번만 표시됩니다.</p></div><Button variant="ghost" size="icon" onClick={() => void query.refetch()} aria-label="키 목록 새로고침"><RefreshCw className="size-4" /></Button></CardHeader>
          <CardContent className="pt-3">
            {keys.length ? <div className="space-y-3">{keys.map((key) => (
              <KeyCard
                key={`${key.id}:${JSON.stringify(key)}`}
                apiKey={key}
                grantable={grantable}
                onSave={(policy) => update.mutate({ id: key.id, policy })}
                onRotate={() => rotate.mutate(key.id)}
                onRevoke={() => { if (window.confirm(`${key.name} 키를 폐기하시겠습니까?`)) revoke.mutate(key.id) }}
                busy={update.isPending || rotate.isPending || revoke.isPending}
              />
            ))}{update.error || rotate.error || revoke.error ? <MutationError error={(update.error || rotate.error || revoke.error) as Error} /> : null}</div> : <div className="py-12 text-center"><KeyRound className="mx-auto size-7 text-[var(--muted-soft)]" /><p className="mt-3 text-sm font-bold text-[var(--ink)]">발급된 개인 키가 없습니다</p><p className="mt-1 text-xs text-[var(--muted)]">왼쪽 양식에서 용도별 키를 발급하세요.</p></div>}
          </CardContent>
        </Card>
      </div>
      {revealed ? <SecretDialog value={revealed} onClose={() => setRevealed(null)} /> : null}
    </section>
  )
}

function KeyCard({ apiKey, grantable, onSave, onRotate, onRevoke, busy }: { apiKey: PersonalAPIKey; grantable: string[]; onSave: (policy: APIKeyPolicyInput) => void; onRotate: () => void; onRevoke: () => void; busy: boolean }) {
  const [selected, setSelected] = useState(apiKey.scopes ?? [])
  const [allowedIPs, setAllowedIPs] = useState(listText(apiKey.allowed_ips))
  const [allowedModels, setAllowedModels] = useState(listText(apiKey.allowed_models))
  const [deniedModels, setDeniedModels] = useState(listText(apiKey.denied_models))
  const [allowedProviders, setAllowedProviders] = useState(listText(apiKey.allowed_providers))
  const [deniedProviders, setDeniedProviders] = useState(listText(apiKey.denied_providers))
  const [budget, setBudget] = useState(String(apiKey.budget_limit_krw || ''))
  const [expiry, setExpiry] = useState(apiKey.expires_at ? apiKey.expires_at.slice(0, 10) : '')
  const policy = editPolicy({ selected, allowedIPs, allowedModels, deniedModels, allowedProviders, deniedProviders, budget, expiry })
  const original = editPolicy({ selected: apiKey.scopes ?? [], allowedIPs: listText(apiKey.allowed_ips), allowedModels: listText(apiKey.allowed_models), deniedModels: listText(apiKey.denied_models), allowedProviders: listText(apiKey.allowed_providers), deniedProviders: listText(apiKey.denied_providers), budget: String(apiKey.budget_limit_krw || ''), expiry: apiKey.expires_at ? apiKey.expires_at.slice(0, 10) : '' })
  const changed = JSON.stringify(policy) !== JSON.stringify(original)
  return (
    <article className="rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><div className="flex items-center gap-2"><h4 className="text-sm font-bold text-[var(--ink)]">{apiKey.name}</h4><StatusBadge status={apiKey.status} /></div><p className="mt-1 font-mono text-xs text-[var(--muted)]">{apiKey.id}</p></div>
        <div className="flex flex-wrap gap-2"><Button size="sm" variant="secondary" disabled={busy} onClick={onRotate}><RefreshCw className="size-3.5" /> 회전</Button><Button size="sm" variant="danger" disabled={busy || apiKey.status !== 'active'} onClick={onRevoke}>폐기</Button></div>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-[var(--muted)]"><span>역할 <strong className="text-[var(--ink)]">{apiKey.role || '기본'}</strong></span><span>생성 {formatDate(apiKey.created_at)}</span><span>만료 {apiKey.expires_at ? formatDate(apiKey.expires_at) : '없음'}</span></div>
      <details className="mt-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
        <summary className="cursor-pointer text-xs font-bold text-[var(--ink)]">키 권한 조정 <span className="ml-1 font-normal text-[var(--muted)]">({selected.length}개)</span></summary>
        <div className="mt-3"><ScopePicker scopes={grantable} selected={selected} onChange={setSelected} /></div>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          <PolicyInput label="허용 IP·CIDR" value={allowedIPs} onChange={setAllowedIPs} placeholder="10.20.0.0/16" />
          <PolicyInput label="허용 모델" value={allowedModels} onChange={setAllowedModels} placeholder="qwen-*" />
          <PolicyInput label="차단 모델" value={deniedModels} onChange={setDeniedModels} placeholder="external-*" />
          <PolicyInput label="허용 공급자" value={allowedProviders} onChange={setAllowedProviders} placeholder="internal" />
          <PolicyInput label="차단 공급자" value={deniedProviders} onChange={setDeniedProviders} placeholder="external-*" />
          <PolicyInput label="예산 한도(원)" value={budget} onChange={setBudget} placeholder="0은 무제한" type="number" />
          <label><span className="field-label">만료일</span><input className="field-input" type="date" value={expiry} onChange={(event) => setExpiry(event.target.value)} /></label>
        </div>
        <p className="mt-3 text-xs leading-5 text-[var(--muted)]">목록 값은 쉼표 또는 줄바꿈으로 구분합니다. 빈 허용 목록은 제한 없음, 빈 권한 목록은 호출 권한 없음으로 적용됩니다.</p>
        <div className="mt-3 flex justify-end"><Button size="sm" variant="accent" disabled={!changed || busy || Number(budget || 0) < 0} onClick={() => onSave(policy)}><Check className="size-3.5" /> 권한 정책 저장</Button></div>
      </details>
    </article>
  )
}

function ScopePicker({ scopes, selected, onChange }: { scopes: string[]; selected: string[]; onChange: (value: string[]) => void }) {
  if (!scopes.length) return <p className="text-xs text-[var(--muted)]">현재 역할에서 발급 가능한 권한이 없습니다.</p>
  return <div className="grid gap-2">{scopes.map((scope) => <label key={scope} className="flex cursor-pointer items-start gap-2 rounded-lg border border-[var(--line)] px-3 py-2 hover:bg-[var(--surface-muted)]"><input className="mt-0.5 accent-[var(--accent)]" type="checkbox" checked={selected.includes(scope)} onChange={(event) => onChange(event.target.checked ? [...selected, scope] : selected.filter((item) => item !== scope))} /><span><span className="block font-mono text-xs font-bold text-[var(--ink)]">{scope}</span><span className="mt-0.5 block text-xs text-[var(--muted)]">{scopeDescriptions[scope] || '역할 정책으로 허용된 권한'}</span></span></label>)}</div>
}

function PolicyTextField({ label, placeholder, registration }: { label: string; placeholder: string; registration: UseFormRegisterReturn }) {
  return <label><span className="field-label">{label}</span><input className="field-input" placeholder={placeholder} {...registration} /></label>
}

function PolicyInput({ label, value, onChange, placeholder, type = 'text' }: { label: string; value: string; onChange: (value: string) => void; placeholder: string; type?: 'text' | 'number' }) {
  return <label><span className="field-label">{label}</span><input className="field-input" type={type} min={type === 'number' ? 0 : undefined} value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} /></label>
}

function splitList(value?: string) {
  return [...new Set((value ?? '').split(/[\n,]+/).map((item) => item.trim()).filter(Boolean))]
}

function listText(value?: string[]) {
  return (value ?? []).join(', ')
}

function expiryTimestamp(value?: string) {
  return value ? new Date(`${value}T23:59:59Z`).toISOString() : ''
}

function createPolicy(values: KeyForm, scopes: string[]): APIKeyPolicyInput {
  const policy: APIKeyPolicyInput = { scopes }
  if (values.expires_at) policy.expires_at = expiryTimestamp(values.expires_at)
  if (values.allowed_ips?.trim()) policy.allowed_ips = splitList(values.allowed_ips)
  if (values.allowed_models?.trim()) policy.allowed_models = splitList(values.allowed_models)
  if (values.denied_models?.trim()) policy.denied_models = splitList(values.denied_models)
  if (values.allowed_providers?.trim()) policy.allowed_providers = splitList(values.allowed_providers)
  if (values.denied_providers?.trim()) policy.denied_providers = splitList(values.denied_providers)
  if (values.budget_limit_krw?.trim()) policy.budget_limit_krw = Number(values.budget_limit_krw)
  return policy
}

interface EditablePolicy {
  selected: string[]
  allowedIPs: string
  allowedModels: string
  deniedModels: string
  allowedProviders: string
  deniedProviders: string
  budget: string
  expiry: string
}

function editPolicy(value: EditablePolicy): APIKeyPolicyInput {
  return {
    scopes: [...value.selected].sort(),
    allowed_ips: splitList(value.allowedIPs).sort(),
    allowed_models: splitList(value.allowedModels).sort(),
    denied_models: splitList(value.deniedModels).sort(),
    allowed_providers: splitList(value.allowedProviders).sort(),
    denied_providers: splitList(value.deniedProviders).sort(),
    budget_limit_krw: Number(value.budget || 0),
    expires_at: expiryTimestamp(value.expiry),
  }
}

function SecretDialog({ value, onClose }: { value: { title: string; secret: string }; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => { await navigator.clipboard.writeText(value.secret); setCopied(true) }
  return <div className="dialog-backdrop" role="presentation"><section className="dialog-card" role="dialog" aria-modal="true" aria-labelledby="secret-title"><div className="flex items-start justify-between gap-3"><div><span className="grid size-10 place-items-center rounded-xl bg-[var(--success-soft)] text-[var(--success)]"><ShieldCheck className="size-5" /></span><h2 id="secret-title" className="mt-4 text-xl font-bold text-[var(--ink)]">{value.title}</h2><p className="mt-2 text-sm leading-6 text-[var(--muted)]">이 비밀값은 다시 표시되지 않습니다. 안전한 비밀 저장소에 복사해 주세요.</p></div><Button variant="ghost" size="icon" onClick={onClose} aria-label="닫기"><X className="size-4" /></Button></div><pre className="mt-5 overflow-x-auto rounded-xl border border-[var(--line-strong)] bg-[var(--ink)] p-4 font-mono text-sm text-white">{value.secret}</pre><div className="mt-4 flex justify-end gap-2"><Button variant="secondary" onClick={() => void copy()}>{copied ? <Check className="size-4" /> : <Copy className="size-4" />}{copied ? '복사됨' : '비밀값 복사'}</Button><Button onClick={onClose}>확인</Button></div></section></div>
}

function PersonalMetric({ label, value, detail }: { label: string; value: string; detail: string }) { return <Card><CardContent><p className="text-xs font-bold text-[var(--muted)]">{label}</p><p className="mt-3 text-2xl font-bold tracking-[-.04em] text-[var(--ink)]">{value}</p><p className="mt-2 text-xs text-[var(--muted)]">{detail}</p></CardContent></Card> }
function Signal({ label, value }: { label: string; value: string }) { return <div className="rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-3"><p className="text-xs text-[var(--muted)]">{label}</p><p className="mt-2 text-lg font-bold text-[var(--ink)]">{value}</p></div> }
function MutationError({ error }: { error: Error }) { return <p className="mt-3 flex items-start gap-2 rounded-xl bg-[var(--danger-soft)] p-3 text-xs text-[var(--danger)]"><AlertTriangle className="mt-0.5 size-4 shrink-0" />{error.message}</p> }
