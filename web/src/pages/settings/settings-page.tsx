import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Bot, CheckCircle2, DatabaseZap, KeyRound, LoaderCircle, LockKeyhole, RotateCcw, Save, ShieldCheck, TestTube2, Trash2 } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'

import { platformApi, type KeycloakConfig, type ProviderConfig, type RuntimeSetting, type TrackingViolation } from '@/api/platform'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { ErrorState, PageLoader } from '@/components/ui/query-state'
import { formatDate } from '@/lib/utils'
import { canManageDataWorksSettings, useAuthStore } from '@/stores/auth-store'
import { RoleManagement } from './role-management'

type SettingsTab = 'ai' | 'sso' | 'roles' | 'tracking' | 'runtime'

const tabItems: Array<{ key: SettingsTab; label: string; description: string }> = [
  { key: 'ai', label: 'AI 및 MCP', description: '공급자·스트리밍·토큰' },
  { key: 'sso', label: 'Keycloak SSO', description: 'OIDC 로그인 연동' },
  { key: 'roles', label: '역할 및 권한', description: 'RBAC 설계·사용자 할당' },
  { key: 'tracking', label: '방문 추적', description: '추적 스크립트·CSP' },
  { key: 'runtime', label: '전체 설정', description: '런타임 설정 관리' },
]

// The order administrators fill the form in: turn it on last, after the
// provider and its address are saved, so a half-filled setup never goes live.
const trackingKeys = ['tracking.provider', 'tracking.momento_url', 'tracking.momento_site_id', 'tracking.momento_proxy', 'tracking.momento_environment', 'tracking.measurement_id', 'tracking.matomo_url', 'tracking.matomo_site_id', 'tracking.custom_snippet', 'tracking.allowed_hosts', 'tracking.placement', 'tracking.include_admin', 'tracking.enabled']

export function SettingsPage() {
  const [tab, setTab] = useState<SettingsTab>('ai')
  const version = useAuthStore((state) => state.version)
  const mode = useAuthStore((state) => state.mode)
  const user = useAuthStore((state) => state.user)
  if (!canManageDataWorksSettings(mode, user)) return <div><PageHeader eyebrow="서비스 관리" title="관리자 설정" description="서비스 관리자만 운영 설정을 변경할 수 있습니다." /><Card><CardContent className="flex min-h-72 flex-col items-center justify-center text-center"><span className="grid size-12 place-items-center rounded-xl bg-[var(--danger-soft)] text-[var(--danger)]"><LockKeyhole className="size-5" /></span><h2 className="mt-4 text-base font-bold text-[var(--ink)]">접근 권한이 없습니다</h2><p className="mt-2 text-sm text-[var(--muted)]">서비스 관리자에게 설정 권한을 요청해 주세요.</p></CardContent></Card></div>
  return (
    <div>
      <PageHeader eyebrow="서비스 관리 · 관리자" title="관리자 설정" description="AI, 인증, 역할, MCP와 운영 정책을 환경변수 추가 없이 안전하게 적용합니다." actions={<Badge tone="info">서비스 {version || '버전 확인 중'}</Badge>} />
      <div className="mb-5 grid gap-2 md:grid-cols-5" role="tablist" aria-label="설정 영역">
        {tabItems.map((item) => <button key={item.key} type="button" role="tab" aria-selected={tab === item.key} className={`settings-tab ${tab === item.key ? 'is-active' : ''}`} onClick={() => setTab(item.key)}><strong>{item.label}</strong><span>{item.description}</span></button>)}
      </div>
      {tab === 'ai' ? <AISettings /> : tab === 'sso' ? <KeycloakSettings /> : tab === 'roles' ? <RoleManagement /> : tab === 'tracking' ? <TrackingSettings /> : <RuntimeSettings />}
    </div>
  )
}

function AISettings() {
  const queryClient = useQueryClient()
  const providers = useQuery({ queryKey: ['admin', 'providers'], queryFn: platformApi.providers, staleTime: 15_000 })
  const settings = useQuery({ queryKey: ['admin', 'settings'], queryFn: platformApi.settings, staleTime: 15_000 })
  const [form, setForm] = useState({ name: 'openai', base_url: '', api_key: '', timeout_ms: 600000, enabled: true, model_patterns: '*' })
  const saveProvider = useMutation({ mutationFn: platformApi.saveProvider, onSuccess: () => { setForm((current) => ({ ...current, api_key: '' })); void queryClient.invalidateQueries({ queryKey: ['admin', 'providers'] }) } })

  if (providers.isPending || settings.isPending) return <PageLoader label="AI 설정을 불러오는 중" />
  const error = providers.error || settings.error
  if (error) return <ErrorState error={error} retry={() => { void providers.refetch(); void settings.refetch() }} />
  const aiKeys = ['ai.default_stream', 'limits.max_output_tokens', 'limits.agent_max_tokens', 'mcp.agentic_model', 'mcp.max_tokens', 'mcp.max_agent_steps', 'mcp.max_tools', 'mcp.force_tool_first']
  const aiSettings = settings.data.settings.filter((item) => aiKeys.includes(item.key))

  const selectProvider = (provider: ProviderConfig) => setForm({ name: provider.name, base_url: provider.base_url, api_key: '', timeout_ms: provider.timeout_ms, enabled: provider.enabled, model_patterns: provider.model_patterns })
  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(360px,.8fr)]">
      <div className="space-y-5">
        <Card>
          <CardHeader><div><h2 className="text-base font-bold text-[var(--ink)]">AI 공급자</h2><p className="mt-1 text-xs text-[var(--muted)]">오프라인망 내부의 OpenAI 호환 엔드포인트도 연결할 수 있습니다.</p></div><DatabaseZap className="size-5 text-[var(--ai)]" /></CardHeader>
          <CardContent>
            <div className="grid gap-3 sm:grid-cols-2">
              {(providers.data.providers ?? []).map((provider) => <button type="button" className="provider-card" key={provider.name} onClick={() => selectProvider(provider)}><span className="flex items-center justify-between gap-3"><strong>{provider.name}</strong><StatusBadge status={provider.enabled ? 'active' : 'inactive'} /></span><span>{provider.base_url}</span><span className="mt-2 flex gap-2"><Badge tone={provider.api_key_configured ? 'success' : 'warning'}>{provider.api_key_configured ? 'API 키 설정됨' : 'API 키 필요'}</Badge><Badge>{provider.model_patterns || '패턴 없음'}</Badge></span></button>)}
              {!providers.data.providers?.length ? <p className="col-span-full rounded-xl border border-dashed border-[var(--line-strong)] p-6 text-center text-sm text-[var(--muted)]">등록된 공급자가 없습니다. 오른쪽에서 첫 공급자를 추가하세요.</p> : null}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><div><h2 className="text-base font-bold text-[var(--ink)]">AI 호출 정책</h2><p className="mt-1 text-xs text-[var(--muted)]">스트리밍을 기본으로 사용하고 최대 256Ki 토큰까지 안전하게 설정합니다.</p></div><Bot className="size-5 text-[var(--ai)]" /></CardHeader>
          <CardContent className="space-y-3">
            {aiSettings.map((setting) => <QuickSetting key={`${setting.key}:${setting.source}:${setting.value}`} setting={setting} />)}
          </CardContent>
        </Card>
      </div>

      <Card className="h-fit xl:sticky xl:top-24">
        <CardHeader><div><h2 className="text-base font-bold text-[var(--ink)]">공급자 연결</h2><p className="mt-1 text-xs text-[var(--muted)]">기존 이름을 선택하면 해당 설정을 갱신합니다.</p></div><KeyRound className="size-5 text-[var(--accent)]" /></CardHeader>
        <CardContent>
          <div className="grid gap-4">
            <FormField label="공급자 이름"><input className="field-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="openai" /></FormField>
            <FormField label="Base URL"><input className="field-input" value={form.base_url} onChange={(event) => setForm({ ...form, base_url: event.target.value })} placeholder="http://ai-gateway.internal:8000" /></FormField>
            <FormField label={providers.data.providers?.find((item) => item.name === form.name)?.api_key_configured ? 'API 키 · 비우면 기존 키 유지' : 'API 키'}><input className="field-input" type="password" value={form.api_key} onChange={(event) => setForm({ ...form, api_key: event.target.value })} autoComplete="new-password" placeholder="sk-…" /></FormField>
            <FormField label="모델 패턴"><input className="field-input" value={form.model_patterns} onChange={(event) => setForm({ ...form, model_patterns: event.target.value })} placeholder="gpt-*,qwen-*,local-*" /></FormField>
            <FormField label="호출 제한 시간(ms)"><input className="field-input" type="number" min={1000} max={3600000} value={form.timeout_ms} onChange={(event) => setForm({ ...form, timeout_ms: Number(event.target.value) })} /></FormField>
            <label className="toggle-row"><span><strong>공급자 활성화</strong><small>비활성 공급자는 라우팅에서 제외됩니다.</small></span><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} /></label>
            {saveProvider.error ? <p className="field-error">{saveProvider.error.message}</p> : null}
            <Button variant="accent" disabled={saveProvider.isPending || !form.name.trim() || !form.base_url.trim()} onClick={() => saveProvider.mutate(form)}>{saveProvider.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />} 공급자 저장</Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function KeycloakSettings() {
  const query = useQuery({ queryKey: ['admin', 'keycloak'], queryFn: platformApi.keycloakConfig, staleTime: 15_000 })
  if (query.isPending) return <PageLoader label="Keycloak 설정을 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />
  const current = query.data as KeycloakConfig
  return <KeycloakSettingsEditor key={`${current.source}:${current.updated_at}:${current.enabled}:${current.client_id}`} current={current} />
}

function KeycloakSettingsEditor({ current }: { current: KeycloakConfig }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState(() => ({ enabled: current.enabled, issuer_url: current.issuer_url, client_id: current.client_id, client_secret: '', redirect_uri: current.redirect_uri || `${window.location.origin}/auth/keycloak/callback`, scopes: (current.scopes ?? ['openid', 'profile', 'email']).join(' '), default_role: current.default_role || 'developer', role_claim: current.role_claim || 'realm_access.roles', group_claim: current.group_claim || 'groups', role_map_json: JSON.stringify(current.role_map ?? {}, null, 2), allow_local_login: current.allow_local_login, auto_login: Boolean(current.auto_login) }))
  const [testResult, setTestResult] = useState<{ ok: boolean; reason?: string; issuer?: string; rsa_signing_keys?: number } | null>(null)
  const save = useMutation({ mutationFn: () => {
    let roleMap: unknown
    try { roleMap = JSON.parse(form.role_map_json || '{}') } catch { throw new Error('역할 매핑은 올바른 JSON 객체여야 합니다.') }
    if (!roleMap || Array.isArray(roleMap) || typeof roleMap !== 'object') throw new Error('역할 매핑은 JSON 객체여야 합니다.')
    const { role_map_json: _roleMapJSON, ...formValue } = form
    void _roleMapJSON
    const payload: Record<string, unknown> = { ...formValue, role_map: roleMap, scopes: form.scopes.split(/[ ,]+/).filter(Boolean) }
    if (!form.client_secret) delete payload.client_secret
    return platformApi.saveKeycloakConfig(payload)
  }, onSuccess: () => { setForm((value) => ({ ...value, client_secret: '' })); void queryClient.invalidateQueries({ queryKey: ['admin', 'keycloak'] }) } })
  const test = useMutation({ mutationFn: platformApi.testKeycloak, onSuccess: setTestResult })

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Card>
        <CardHeader><div><h2 className="text-base font-bold text-[var(--ink)]">Keycloak OIDC 간편 연동</h2><p className="mt-1 text-xs text-[var(--muted)]">Issuer URL, Client ID와 Secret만 입력하면 Discovery와 PKCE 흐름을 자동 구성합니다.</p></div><ShieldCheck className="size-5 text-[var(--success)]" /></CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-2">
            <FormField label="Issuer URL" className="md:col-span-2"><input className="field-input" value={form.issuer_url} onChange={(event) => setForm({ ...form, issuer_url: event.target.value })} placeholder="https://keycloak.internal/realms/dataworks" /></FormField>
            <FormField label="Client ID"><input className="field-input" value={form.client_id} onChange={(event) => setForm({ ...form, client_id: event.target.value })} placeholder="dataworks" /></FormField>
            <FormField label={current.client_secret_set ? 'Client Secret · 비우면 기존 값 유지' : 'Client Secret'}><input className="field-input" type="password" value={form.client_secret} onChange={(event) => setForm({ ...form, client_secret: event.target.value })} autoComplete="new-password" /></FormField>
            <FormField label="Redirect URI" className="md:col-span-2"><input className="field-input" value={form.redirect_uri} onChange={(event) => setForm({ ...form, redirect_uri: event.target.value })} /></FormField>
            <label className="toggle-row"><span><strong>Keycloak SSO 활성화</strong><small>저장 후 로그인 화면에 SSO 버튼이 표시됩니다.</small></span><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} /></label>
            <label className="toggle-row"><span><strong>로컬 로그인 허용</strong><small>비상 접근을 위해 관리자 계정 로그인을 유지합니다.</small></span><input type="checkbox" checked={form.allow_local_login} onChange={(event) => setForm({ ...form, allow_local_login: event.target.checked })} /></label>
            <label className="toggle-row md:col-span-2"><span><strong>자동 로그인 (Silent SSO)</strong><small>Keycloak에 이미 로그인한 사용자는 로그인 화면 없이 바로 들어옵니다(OIDC prompt=none). 세션이 없으면 로그인 화면이 한 번만 표시되고 반복 이동하지 않습니다.</small></span><input type="checkbox" checked={form.auto_login} disabled={!form.enabled} onChange={(event) => setForm({ ...form, auto_login: event.target.checked })} /></label>
          </div>
          <details className="mt-5 rounded-xl border border-[var(--line)] p-4">
            <summary className="cursor-pointer text-sm font-bold text-[var(--ink)]">고급 클레임 및 역할 설정</summary>
            <div className="mt-4 grid gap-4 md:grid-cols-2">
              <FormField label="OIDC Scope"><input className="field-input" value={form.scopes} onChange={(event) => setForm({ ...form, scopes: event.target.value })} /></FormField>
              <FormField label="기본 내부 역할"><input className="field-input" value={form.default_role} onChange={(event) => setForm({ ...form, default_role: event.target.value })} /></FormField>
              <FormField label="역할 Claim"><input className="field-input" value={form.role_claim} onChange={(event) => setForm({ ...form, role_claim: event.target.value })} /></FormField>
              <FormField label="그룹 Claim"><input className="field-input" value={form.group_claim} onChange={(event) => setForm({ ...form, group_claim: event.target.value })} /></FormField>
              <FormField label="Keycloak 역할 → 내부 역할 매핑(JSON)" className="md:col-span-2"><textarea className="field-input h-32 py-3 font-mono leading-5" value={form.role_map_json} onChange={(event) => setForm({ ...form, role_map_json: event.target.value })} spellCheck={false} /></FormField>
            </div>
          </details>
          {save.error ? <p className="field-error mt-4">{save.error.message}</p> : null}
          <div className="mt-5 flex flex-wrap justify-end gap-2"><Button variant="secondary" disabled={test.isPending || !current.enabled} onClick={() => test.mutate()}>{test.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <TestTube2 className="size-4" />} 연결 진단</Button><Button variant="accent" disabled={save.isPending} onClick={() => save.mutate()}>{save.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />} SSO 설정 저장</Button></div>
        </CardContent>
      </Card>

      <div className="space-y-5">
        <Card><CardContent><p className="text-xs font-bold text-[var(--muted)]">현재 상태</p><div className="mt-3 flex items-center justify-between"><StatusBadge status={current.enabled ? 'active' : 'inactive'} /><Badge>{current.source === 'db' ? '관리자 설정' : '초기 설정'}</Badge></div><dl className="mt-5 space-y-3 text-xs"><Info label="Client Secret" value={current.client_secret_set ? '암호화 저장됨' : '미설정'} /><Info label="자동 로그인" value={current.auto_login ? '켜짐' : '꺼짐'} /><Info label="역할 매핑" value={`${Object.keys(current.role_map ?? {}).length}개`} /><Info label="마지막 수정" value={current.updated_at ? formatDate(current.updated_at) : '없음'} /></dl></CardContent></Card>
        {testResult ? <Card className={testResult.ok ? 'border-[var(--success)]' : 'border-[var(--danger)]'}><CardContent><div className="flex items-center gap-2">{testResult.ok ? <CheckCircle2 className="size-5 text-[var(--success)]" /> : <ShieldCheck className="size-5 text-[var(--danger)]" />}<p className="font-bold text-[var(--ink)]">{testResult.ok ? 'OIDC 연결 정상' : '연결 점검 필요'}</p></div><p className="mt-3 break-words text-xs leading-5 text-[var(--muted)]">{testResult.ok ? `${testResult.issuer} · RSA 키 ${testResult.rsa_signing_keys ?? 0}개` : testResult.reason}</p></CardContent></Card> : null}
      </div>
    </div>
  )
}

function TrackingSettings() {
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['admin', 'settings'], queryFn: platformApi.settings, staleTime: 15_000 })
  const status = useQuery({ queryKey: ['admin', 'tracking', 'status'], queryFn: platformApi.trackingStatus, staleTime: 5_000 })
  const violations = useQuery({ queryKey: ['admin', 'tracking', 'violations'], queryFn: platformApi.trackingViolations, staleTime: 5_000, refetchInterval: 15_000 })
  const refreshTracking = () => { void queryClient.invalidateQueries({ queryKey: ['admin', 'tracking'] }); void queryClient.invalidateQueries({ queryKey: ['admin', 'settings'] }) }
  const allow = useMutation({ mutationFn: platformApi.allowTrackingOrigin, onSuccess: refreshTracking })
  const clear = useMutation({ mutationFn: platformApi.clearTrackingViolations, onSuccess: refreshTracking })

  if (settings.isPending || status.isPending) return <PageLoader label="방문 추적 설정을 불러오는 중" />
  const error = settings.error || status.error
  if (error) return <ErrorState error={error} retry={() => { void settings.refetch(); void status.refetch() }} />
  const current = status.data
  const provider = current.provider
  const visible = (key: string) => {
    if (key.startsWith('tracking.momento_')) return provider === 'momento'
    if (key === 'tracking.measurement_id') return provider === 'ga4' || provider === 'gtm'
    if (key.startsWith('tracking.matomo_')) return provider === 'matomo'
    if (key === 'tracking.custom_snippet') return provider === 'custom'
    return true
  }
  const rows = trackingKeys.map((key) => settings.data.settings.find((item) => item.key === key)).filter((item): item is RuntimeSetting => Boolean(item) && visible(item!.key))
  const items: TrackingViolation[] = violations.data?.items ?? []
  const sources = Array.from(new Set([...current.script_sources, ...current.connect_sources, ...current.image_sources]))
  const state = !current.enabled ? '꺼짐' : current.problem ? '설정 미완료' : current.active ? '추적 중' : '비활성'
  const tone = !current.enabled ? 'inactive' : current.problem ? 'pending' : 'active'

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(360px,.8fr)]">
      <Card>
        <CardHeader><div><h2 className="text-base font-bold text-[var(--ink)]">추적 스크립트</h2><p className="mt-1 text-xs text-[var(--muted)]">provider 를 고르고 주소를 저장한 뒤 마지막에 켭니다. 스니펫은 요청마다 nonce 를 달고 들어가므로 CSP 를 느슨하게 풀지 않습니다.</p></div><Activity className="size-5 text-[var(--accent)]" /></CardHeader>
        <CardContent className="space-y-3">
          {rows.map((setting) => <QuickSetting key={`${setting.key}:${setting.source}:${setting.value}`} setting={setting} />)}
        </CardContent>
      </Card>

      <div className="space-y-5">
        <Card>
          <CardContent>
            <p className="text-xs font-bold text-[var(--muted)]">현재 상태</p>
            <div className="mt-3 flex items-center justify-between"><StatusBadge status={tone} /><Badge>{state}</Badge></div>
            <dl className="mt-5 space-y-3 text-xs">
              <Info label="provider" value={provider} />
              <Info label="관리 화면 포함" value={current.include_admin ? '예' : '아니오'} />
              <Info label="삽입 위치" value={current.placement} />
              {provider === 'momento' ? <Info label="같은 오리진 프록시" value={current.momento_proxy ? `${current.proxy_path}/* → ${current.proxy_target}` : '사용 안 함'} /> : null}
            </dl>
            {current.problem ? <p className="field-error mt-4">{current.problem}</p> : null}
            {sources.length ? <div className="mt-4"><p className="text-xs font-bold text-[var(--muted)]">CSP 에 더해지는 출처</p><ul className="mt-2 space-y-1 text-xs">{sources.map((source) => <li key={source}><code>{source}</code></li>)}</ul></div> : current.active ? <p className="mt-4 text-xs text-[var(--muted)]">외부 출처가 정책에 등장하지 않습니다.</p> : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><div><h2 className="text-base font-bold text-[var(--ink)]">차단된 출처</h2><p className="mt-1 text-xs text-[var(--muted)]">추적이 켜진 동안 브라우저가 정책 위반으로 신고한 주소입니다. 허용을 누르면 <code>tracking.allowed_hosts</code> 에 추가됩니다.</p></div><Button size="sm" variant="secondary" aria-label="차단 기록 지우기" disabled={clear.isPending || !items.length} onClick={() => clear.mutate()}><Trash2 className="size-3.5" /></Button></CardHeader>
          <CardContent>
            {!items.length ? <p className="text-sm text-[var(--muted)]">{current.active ? '차단된 출처가 없습니다.' : '추적이 켜지면 정책 위반 신고가 여기에 모입니다.'}</p> : null}
            <ul className="divide-y divide-[var(--line)]">
              {items.map((item) => <li key={`${item.directive}:${item.origin}`} className="flex items-center justify-between gap-3 py-2 text-xs"><div className="min-w-0"><code className="break-all">{item.origin}</code><p className="mt-1 text-[var(--muted)]">{item.directive} · {item.count}회 · {formatDate(item.last_seen)}</p></div>{item.allowed ? <Badge tone="success">허용됨</Badge> : <Button size="sm" variant="accent" disabled={allow.isPending} onClick={() => allow.mutate(item.origin)}>허용</Button>}</li>)}
            </ul>
            {allow.error || clear.error ? <p className="field-error mt-3">{(allow.error || clear.error)?.message}</p> : null}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function RuntimeSettings() {
  const query = useQuery({ queryKey: ['admin', 'settings'], queryFn: platformApi.settings, staleTime: 15_000 })
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState('전체')
  const categories = useMemo(() => ['전체', ...Array.from(new Set(query.data?.settings.map((item) => item.category.split('.')[0]) ?? []))], [query.data])
  if (query.isPending) return <PageLoader label="전체 설정을 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />
  const filtered = query.data.settings.filter((item) => (category === '전체' || item.category.startsWith(category)) && (!search.trim() || `${item.key} ${item.description} ${item.category}`.toLowerCase().includes(search.toLowerCase())))
  return <Card className="overflow-hidden"><div className="flex flex-col gap-3 border-b border-[var(--line)] p-4 md:flex-row"><input className="field-input md:max-w-sm" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="설정 키 또는 설명 검색" /><select className="field-input md:ml-auto md:max-w-48" value={category} onChange={(event) => setCategory(event.target.value)}>{categories.map((item) => <option key={item}>{item}</option>)}</select></div><div className="divide-y divide-[var(--line)]">{filtered.map((setting) => <RuntimeSettingRow setting={setting} key={`${setting.key}:${setting.source}:${setting.value}`} />)}</div>{!filtered.length ? <p className="p-10 text-center text-sm text-[var(--muted)]">조건에 맞는 설정이 없습니다.</p> : null}</Card>
}

function QuickSetting({ setting }: { setting: RuntimeSetting }) { return <RuntimeSettingRow setting={setting} compact /> }

function RuntimeSettingRow({ setting, compact = false }: { setting: RuntimeSetting; compact?: boolean }) {
  const queryClient = useQueryClient()
  const [value, setValue] = useState(setting.is_secret ? '' : setting.value)
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['admin', 'settings'] })
  const save = useMutation({ mutationFn: () => platformApi.saveSetting(setting.key, value), onSuccess: () => void refresh() })
  const revert = useMutation({ mutationFn: () => platformApi.revertSetting(setting.key), onSuccess: () => void refresh() })
  const control = setting.type === 'bool' ? <select className="field-input" value={value} onChange={(event) => setValue(event.target.value)}><option value="true">활성화</option><option value="false">비활성화</option></select> : <input className="field-input" type={setting.is_secret ? 'password' : setting.type === 'int' || setting.type === 'float' ? 'number' : 'text'} min={setting.key.includes('tokens') ? 0 : undefined} max={setting.key.includes('tokens') ? 262144 : undefined} value={value} onChange={(event) => setValue(event.target.value)} placeholder={setting.is_secret && setting.is_set ? '새 값을 입력할 때만 변경' : ''} />
  return <div className={`runtime-setting-row ${compact ? 'is-compact' : ''}`}><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><code>{setting.key}</code><Badge>{setting.category}</Badge>{setting.restart_required ? <Badge tone="warning">재시작 필요</Badge> : null}{setting.is_secret ? <Badge tone="violet">암호화</Badge> : null}</div><p>{setting.description || '관리자 런타임 설정'}</p></div><div className="runtime-setting-control">{control}<div className="flex gap-2"><Button size="sm" variant="secondary" aria-label={`${setting.key} 기본값 복원`} disabled={setting.read_only || setting.source !== 'admin' || revert.isPending} onClick={() => revert.mutate()}><RotateCcw className="size-3.5" /></Button><Button size="sm" variant="accent" disabled={!setting.can_write || setting.read_only || save.isPending || (setting.is_secret && !value)} onClick={() => save.mutate()}>{save.isPending ? <LoaderCircle className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} 저장</Button></div></div>{save.error || revert.error ? <p className="field-error col-span-full">{(save.error || revert.error)?.message}</p> : null}</div>
}

function FormField({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) { return <label className={className}><span className="field-label">{label}</span>{children}</label> }
function Info({ label, value }: { label: string; value: string }) { return <div className="flex items-center justify-between gap-3"><dt className="text-[var(--muted)]">{label}</dt><dd className="m-0 text-right font-bold text-[var(--ink)]">{value}</dd></div> }
