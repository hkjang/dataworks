import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Edit3, LoaderCircle, Plus, Search, ShieldCheck, Trash2, UserCog, Users } from 'lucide-react'
import { useMemo, useState } from 'react'

import { platformApi, type AdminUserSummary, type RoleInfo } from '@/api/platform'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { ConfirmDialog, Dialog } from '@/components/ui/dialog'
import { ErrorState, PageLoader } from '@/components/ui/query-state'
import { canWriteDataWorks, useAuthStore } from '@/stores/auth-store'

const scopeGroups = [
  { label: 'AI 사용', scopes: ['chat:completion', 'embeddings:create', 'models:read'] },
  { label: '운영 관리', scopes: ['admin:read', 'admin:write', 'observability:read', 'costs:read', 'security:read'] },
  { label: '라우팅·MCP', scopes: ['routing:read', 'routing:write', 'mcp:use', 'mcp:admin'] },
  { label: '팀', scopes: ['team:read'] },
]

const scopeLabels: Record<string, string> = {
  'chat:completion': '채팅 실행',
  'embeddings:create': '임베딩 실행',
  'models:read': '모델 조회',
  'admin:read': '운영 화면 조회',
  'admin:write': '운영 설정 변경',
  'routing:read': '라우팅 조회',
  'routing:write': '라우팅 변경',
  'observability:read': '관측 데이터 조회',
  'costs:read': '비용 조회',
  'security:read': '보안 데이터 조회',
  'mcp:use': 'MCP 도구 사용',
  'mcp:admin': 'MCP 관리',
  'team:read': '팀 데이터 조회',
}

const scopeDependencies: Record<string, string[]> = {
  'admin:write': ['admin:read'],
  'routing:write': ['routing:read'],
  'mcp:admin': ['mcp:use'],
}

const homeOptions = [
  { value: '', label: '권한에 따라 자동 선택' },
  { value: '#/dataworks/home', label: 'Data Works 홈' },
  { value: '#/dataworks/risk', label: '리스크 검토' },
  { value: '#/factory', label: 'Product Factory' },
  { value: '#/team', label: '팀 대시보드' },
  { value: '#/settings', label: '관리자 설정' },
]

type RoleDraft = { role: string; description: string; scopes: string[]; default_home: string }
type UserChange = { user: AdminUserSummary; role?: string; status?: 'active' | 'disabled' }

const emptyRole: RoleDraft = { role: '', description: '', scopes: ['models:read'], default_home: '' }

function rankLabel(rank: number) {
  if (rank >= 5) return '최고 관리자'
  if (rank >= 4) return '전체 관리자'
  if (rank >= 3) return '운영 관리자'
  if (rank >= 2) return '팀 관리자'
  return '사용자'
}

function roleLabel(role: string) {
  const labels: Record<string, string> = {
    super_admin: '최고 관리자', admin: '관리자', team_admin: '팀 관리자', team_manager: '팀 매니저',
    developer: '개발자', viewer: '뷰어', service_account: '서비스 계정', ops_admin: '운영 설정 관리자',
    ai_admin: 'AI 설정 관리자', security_admin: '보안 관리자', billing_admin: '비용 관리자', readonly_admin: '읽기전용 관리자',
  }
  return labels[role] ?? role.replaceAll('_', ' ')
}

export function RoleManagement() {
  const queryClient = useQueryClient()
  const mode = useAuthStore((state) => state.mode)
  const currentUser = useAuthStore((state) => state.user)
  const writable = canWriteDataWorks(mode, currentUser)
  const roles = useQuery({ queryKey: ['admin', 'roles'], queryFn: platformApi.roles, staleTime: 10_000 })
  const users = useQuery({ queryKey: ['admin', 'users'], queryFn: platformApi.adminUsers, staleTime: 10_000 })
  const [search, setSearch] = useState('')
  const [editor, setEditor] = useState<RoleDraft | null>(null)
  const [editingName, setEditingName] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<RoleInfo | null>(null)
  const [userChange, setUserChange] = useState<UserChange | null>(null)

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['admin', 'roles'] }),
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] }),
    ])
  }
  const saveRole = useMutation({ mutationFn: platformApi.saveRole, onSuccess: async () => { setEditor(null); setEditingName(''); await refresh() } })
  const deleteRole = useMutation({ mutationFn: platformApi.deleteRole, onSuccess: async () => { setDeleteTarget(null); await refresh() } })
  const updateUser = useMutation({
    mutationFn: ({ user, ...payload }: UserChange) => platformApi.updateAdminUser(user.id, payload),
    onSuccess: async () => { setUserChange(null); await refresh() },
  })

  const filteredRoles = useMemo(() => {
    const needle = search.trim().toLowerCase()
    return (roles.data?.roles ?? []).filter((role) => !needle || `${role.role} ${role.description} ${role.scopes.join(' ')}`.toLowerCase().includes(needle))
  }, [roles.data, search])

  if (roles.isPending || users.isPending) return <PageLoader label="역할과 사용자를 불러오는 중" />
  const error = roles.error || users.error
  if (error) return <ErrorState error={error} retry={() => { void roles.refetch(); void users.refetch() }} />

  const catalog = roles.data.roles
  const authUsers = users.data.auth_users ?? []
  const customCount = catalog.filter((role) => !role.is_system).length
  const openCreate = () => { setEditingName(''); setEditor({ ...emptyRole }) }
  const openEdit = (role: RoleInfo) => {
    setEditingName(role.role)
    setEditor({ role: role.role, description: role.description, scopes: [...role.scopes], default_home: role.default_home })
  }

  return (
    <div className="space-y-5">
      <div className="grid gap-3 sm:grid-cols-3">
        <Metric label="전체 역할" value={`${catalog.length}개`} detail={`기본 ${catalog.length - customCount} · 사용자 정의 ${customCount}`} />
        <Metric label="관리 계정" value={`${authUsers.length}명`} detail={`활성 ${authUsers.filter((user) => user.status === 'active').length}명`} />
        <Metric label="권한 정책" value="최소 권한" detail="상위 역할 위임·종속 스코프 자동 보호" />
      </div>

      {!writable ? <div className="rounded-xl border border-[var(--warning)] bg-[var(--warning-soft)] px-4 py-3 text-xs leading-5 text-[var(--warning)]">현재 계정은 역할을 조회할 수 있지만 변경 권한은 없습니다.</div> : null}

      <div className="grid gap-5 2xl:grid-cols-[minmax(0,.95fr)_minmax(560px,1.35fr)]">
        <Card className="overflow-hidden">
          <CardHeader>
            <div><h2 className="text-base font-bold text-[var(--ink)]">역할 카탈로그</h2><p className="mt-1 text-xs text-[var(--muted)]">기본 역할은 보호되며 사용자 정의 역할만 수정·삭제할 수 있습니다.</p></div>
            <Button variant="accent" size="sm" disabled={!writable} onClick={openCreate}><Plus className="size-4" /> 역할 만들기</Button>
          </CardHeader>
          <CardContent className="p-0">
            <label className="relative block border-b border-[var(--line)] px-4 py-3"><Search className="absolute left-7 top-1/2 size-4 -translate-y-1/2 text-[var(--muted)]" /><input aria-label="역할 검색" className="field-input pl-10" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="역할, 설명 또는 스코프 검색" /></label>
            <div className="max-h-[680px] divide-y divide-[var(--line)] overflow-y-auto">
              {filteredRoles.map((role) => (
                <article className="p-4" key={role.role}>
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2"><strong className="text-sm text-[var(--ink)]">{roleLabel(role.role)}</strong><code className="text-[11px] text-[var(--muted)]">{role.role}</code>{role.is_system ? <Badge>기본</Badge> : <Badge tone="violet">사용자 정의</Badge>}</div>
                      <p className="mt-1 text-xs leading-5 text-[var(--muted)]">{role.description || '설명이 없습니다.'}</p>
                    </div>
                    {!role.is_system ? <div className="flex shrink-0 gap-1"><Button aria-label={`${role.role} 수정`} size="icon" variant="ghost" disabled={!writable || !role.can_assign} onClick={() => openEdit(role)}><Edit3 className="size-4" /></Button><Button aria-label={`${role.role} 삭제`} size="icon" variant="ghost" disabled={!writable || !role.can_assign || role.user_count > 0} onClick={() => setDeleteTarget(role)}><Trash2 className="size-4 text-[var(--danger)]" /></Button></div> : null}
                  </div>
                  <div className="mt-3 flex flex-wrap gap-1.5">{role.scopes.map((scope) => <Badge key={scope} tone={scope.endsWith(':write') || scope === 'mcp:admin' ? 'warning' : 'neutral'}>{scopeLabels[scope] ?? scope}</Badge>)}</div>
                  <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-[11px] text-[var(--muted)]"><span>{rankLabel(role.rank)} · 시작 화면 {homeOptions.find((home) => home.value === role.default_home)?.label ?? role.default_home}</span><span className="font-semibold text-[var(--ink)]"><Users className="mr-1 inline size-3.5" />{role.user_count}명 · 활성 {role.active_user_count}</span></div>
                  {!role.is_system && role.user_count > 0 ? <p className="mt-2 text-[11px] text-[var(--warning)]">사용자를 다른 역할로 이동해야 삭제할 수 있습니다.</p> : null}
                </article>
              ))}
              {!filteredRoles.length ? <p className="p-10 text-center text-sm text-[var(--muted)]">조건에 맞는 역할이 없습니다.</p> : null}
            </div>
          </CardContent>
        </Card>

        <Card className="overflow-hidden">
          <CardHeader><div><h2 className="text-base font-bold text-[var(--ink)]">사용자 역할 할당</h2><p className="mt-1 text-xs text-[var(--muted)]">역할 변경 시 기존 로그인 세션이 종료되고 새 권한은 다음 로그인부터 적용됩니다.</p></div><UserCog className="size-5 text-[var(--accent)]" /></CardHeader>
          <CardContent className="overflow-x-auto p-0">
            <table className="w-full min-w-[700px] text-left text-xs">
              <thead className="bg-[var(--surface-muted)] text-[var(--muted)]"><tr><th className="px-4 py-3">사용자</th><th className="px-4 py-3">현재 역할</th><th className="px-4 py-3">상태</th><th className="px-4 py-3">새 역할</th><th className="px-4 py-3 text-right">계정 제어</th></tr></thead>
              <tbody className="divide-y divide-[var(--line)]">
                {authUsers.map((user) => {
                  const currentRole = catalog.find((role) => role.role === user.role)
                  const manageable = writable && Boolean(currentRole?.can_assign)
                  return <UserRoleRow key={user.id} user={user} roles={catalog} manageable={manageable} onChange={setUserChange} />
                })}
              </tbody>
            </table>
            {!authUsers.length ? <p className="p-10 text-center text-sm text-[var(--muted)]">관리할 사용자가 없습니다.</p> : null}
          </CardContent>
        </Card>
      </div>

      <RoleEditor open={Boolean(editor)} draft={editor} immutableName={Boolean(editingName)} allScopes={roles.data.all_scopes} pending={saveRole.isPending} error={saveRole.error} onChange={setEditor} onClose={() => { if (!saveRole.isPending) { setEditor(null); setEditingName('') } }} onSave={() => editor && saveRole.mutate(editor)} />
      <ConfirmDialog open={Boolean(deleteTarget)} title="사용자 정의 역할 삭제" description={`${deleteTarget?.role ?? ''} 역할을 삭제합니다.`} confirmLabel="역할 삭제" pending={deleteRole.isPending} error={deleteRole.error} onClose={() => { if (!deleteRole.isPending) setDeleteTarget(null) }} onConfirm={() => deleteTarget && deleteRole.mutate(deleteTarget.role)} />
      <ConfirmDialog
        open={Boolean(userChange)}
        title={userChange?.role ? '사용자 역할 변경' : userChange?.status === 'disabled' ? '사용자 비활성화' : '사용자 활성화'}
        description={userChange?.role ? `${userChange.user.email} 계정의 역할을 ${roleLabel(userChange.role)}(으)로 변경합니다.` : `${userChange?.user.email ?? ''} 계정을 ${userChange?.status === 'disabled' ? '비활성화' : '활성화'}합니다.`}
        confirmLabel={userChange?.role ? '역할 변경' : userChange?.status === 'disabled' ? '비활성화' : '활성화'}
        confirmVariant={userChange?.status === 'disabled' ? 'danger' : 'accent'}
        warning="현재 로그인 세션이 즉시 종료될 수 있습니다. 마지막 활성 최고관리자 계정은 보호됩니다."
        pending={updateUser.isPending}
        error={updateUser.error}
        onClose={() => { if (!updateUser.isPending) setUserChange(null) }}
        onConfirm={() => userChange && updateUser.mutate(userChange)}
      />
    </div>
  )
}

function Metric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <Card><CardContent><p className="text-xs font-bold text-[var(--muted)]">{label}</p><p className="mt-2 text-xl font-extrabold tracking-[-.04em] text-[var(--ink)]">{value}</p><p className="mt-1 text-[11px] text-[var(--muted)]">{detail}</p></CardContent></Card>
}

function UserRoleRow({ user, roles, manageable, onChange }: { user: AdminUserSummary; roles: RoleInfo[]; manageable: boolean; onChange: (change: UserChange) => void }) {
  const [nextRole, setNextRole] = useState(user.role)
  const assignable = roles.filter((role) => role.can_assign || role.role === user.role)
  return (
    <tr>
      <td className="px-4 py-3"><strong className="block text-[var(--ink)]">{user.name || user.email.split('@')[0]}</strong><span className="mt-0.5 block text-[11px] text-[var(--muted)]">{user.email}{user.team_id ? ` · ${user.team_id}` : ''}</span></td>
      <td className="px-4 py-3"><span className="font-semibold text-[var(--ink)]">{roleLabel(user.role)}</span><code className="mt-0.5 block text-[10px] text-[var(--muted)]">{user.role}</code></td>
      <td className="px-4 py-3"><StatusBadge status={user.status} /></td>
      <td className="px-4 py-3"><div className="flex items-center gap-2"><select aria-label={`${user.email} 새 역할`} className="field-input min-w-40" disabled={!manageable} value={nextRole} onChange={(event) => setNextRole(event.target.value)}>{assignable.map((role) => <option key={role.role} value={role.role}>{roleLabel(role.role)}</option>)}</select><Button size="sm" variant="secondary" disabled={!manageable || nextRole === user.role} onClick={() => onChange({ user, role: nextRole })}>적용</Button></div></td>
      <td className="px-4 py-3 text-right"><Button size="sm" variant={user.status === 'active' ? 'danger' : 'secondary'} disabled={!manageable} onClick={() => onChange({ user, status: user.status === 'active' ? 'disabled' : 'active' })}>{user.status === 'active' ? '비활성화' : '활성화'}</Button></td>
    </tr>
  )
}

function RoleEditor({ open, draft, immutableName, allScopes, pending, error, onChange, onClose, onSave }: {
  open: boolean
  draft: RoleDraft | null
  immutableName: boolean
  allScopes: string[]
  pending: boolean
  error: Error | null
  onChange: (draft: RoleDraft) => void
  onClose: () => void
  onSave: () => void
}) {
  if (!draft) return null
  const toggleScope = (scope: string) => {
    const selected = draft.scopes.includes(scope)
    if (!selected) {
      onChange({ ...draft, scopes: Array.from(new Set([...draft.scopes, scope, ...(scopeDependencies[scope] ?? [])])) })
      return
    }
    const dependents = Object.entries(scopeDependencies).filter(([, dependencies]) => dependencies.includes(scope)).map(([dependent]) => dependent)
    onChange({ ...draft, scopes: draft.scopes.filter((item) => item !== scope && !dependents.includes(item)) })
  }
  const validName = /^[a-z][a-z0-9_]{2,63}$/.test(draft.role)
  return (
    <Dialog
      open={open}
      onClose={onClose}
      busy={pending}
      className="w-[min(900px,calc(100vw-32px))]"
      title={immutableName ? '사용자 정의 역할 수정' : '사용자 정의 역할 만들기'}
      description="업무에 필요한 권한만 조합하세요. 쓰기 권한의 필수 조회 권한은 자동으로 함께 선택됩니다."
      actions={<><Button variant="secondary" disabled={pending} onClick={onClose}>취소</Button><Button variant="accent" disabled={pending || !validName || !draft.description.trim() || draft.scopes.length === 0} onClick={onSave}>{pending ? <LoaderCircle className="size-4 animate-spin" /> : <ShieldCheck className="size-4" />} 역할 저장</Button></>}
    >
      <div className="grid gap-5 md:grid-cols-2">
        <label><span className="field-label">역할 식별자</span><input className="field-input font-mono" disabled={immutableName} value={draft.role} onChange={(event) => onChange({ ...draft, role: event.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '') })} placeholder="cost_analyst" />{!validName && draft.role ? <span className="mt-1 block text-[11px] text-[var(--danger)]">영문 소문자로 시작하고 영문·숫자·밑줄 3~64자를 사용하세요.</span> : null}</label>
        <label><span className="field-label">로그인 시작 화면</span><select className="field-input" value={draft.default_home} onChange={(event) => onChange({ ...draft, default_home: event.target.value })}>{homeOptions.map((home) => <option key={home.value} value={home.value}>{home.label}</option>)}</select></label>
        <label className="md:col-span-2"><span className="field-label">설명</span><input className="field-input" value={draft.description} onChange={(event) => onChange({ ...draft, description: event.target.value })} placeholder="비용과 사용량을 분석하는 담당자" /></label>
      </div>
      <div className="mt-5 grid gap-3 md:grid-cols-2">
        {scopeGroups.map((group) => <fieldset className="rounded-xl border border-[var(--line)] p-4" key={group.label}><legend className="px-1 text-xs font-bold text-[var(--ink)]">{group.label}</legend><div className="mt-1 space-y-2">{group.scopes.filter((scope) => allScopes.includes(scope)).map((scope) => <label className="flex cursor-pointer items-start gap-3 rounded-lg p-2 hover:bg-[var(--surface-muted)]" key={scope}><input className="mt-0.5 size-4" type="checkbox" checked={draft.scopes.includes(scope)} onChange={() => toggleScope(scope)} /><span><strong className="block text-xs text-[var(--ink)]">{scopeLabels[scope] ?? scope}</strong><code className="text-[10px] text-[var(--muted)]">{scope}</code>{scopeDependencies[scope] ? <small className="mt-0.5 block text-[10px] text-[var(--warning)]">{scopeDependencies[scope].map((item) => scopeLabels[item]).join(', ')} 자동 포함</small> : null}</span></label>)}</div></fieldset>)}
      </div>
      <div className="mt-4 rounded-xl bg-[var(--info-soft)] p-4 text-xs leading-5 text-[var(--info)]"><strong>권한 영향 미리보기</strong><p className="mt-1">선택 {draft.scopes.length}개 · {draft.scopes.includes('admin:write') ? '전체 운영 변경 가능' : draft.scopes.includes('admin:read') ? '운영 화면 접근 가능' : '운영 화면 접근 없음'} · {draft.scopes.includes('security:read') ? '보안 데이터 포함' : '보안 데이터 제외'}</p></div>
      {error ? <p className="field-error mt-4" role="alert">{error.message}</p> : null}
    </Dialog>
  )
}
