import { useMutation, useQueries, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  ArrowLeft,
  Boxes,
  CheckCircle2,
  CircleDollarSign,
  Code2,
  Database,
  FileCheck2,
  FileJson2,
  KeyRound,
  Lightbulb,
  Link2,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Send,
  ShieldAlert,
  Sparkles,
  Users,
} from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, NavLink, useParams } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { LifecycleStepper } from '@/features/lifecycle/lifecycle-stepper'
import { PublishGateCard } from '@/features/publish-gate/publish-gate-card'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { ConfirmDialog, Dialog } from '@/components/ui/dialog'
import { EmptyState, ErrorState, PageLoader } from '@/components/ui/query-state'
import { approvalLabel, sensitivityLabel, statusLabel } from '@/lib/labels.ko'
import { cn, formatDate } from '@/lib/utils'
import { canWriteDataWorks, useAuthStore } from '@/stores/auth-store'
import type {
  ApprovalTrace,
  ApprovalTraceInput,
  ContractVersion,
  ContractVersionInput,
  DataProduct,
  EvidencePack,
  ProductCanvas,
  ProductCanvasInput,
  ProductLifecycleAction,
  PublishGate,
} from '@/types/dataworks'

const tabs = [
  'overview', 'assets', 'canvas', 'api', 'customers', 'risk', 'approvals', 'evidence', 'contracts', 'usage', 'revenue', 'versions', 'activity',
] as const

const tabLabels: Record<(typeof tabs)[number], string> = {
  overview: '개요',
  assets: '데이터 자산',
  canvas: '블루프린트',
  api: 'API',
  customers: '고객',
  risk: '위험',
  approvals: '승인',
  evidence: '증적',
  contracts: '계약',
  usage: '사용량',
  revenue: '수익',
  versions: '버전',
  activity: '활동 이력',
}

const lifecycleActionMeta: Record<ProductLifecycleAction, { label: string; description: string; variant: 'accent' | 'secondary' | 'danger' }> = {
  submit: { label: '검토 요청', description: '초안을 검토 단계로 보내 승인 작업을 시작합니다.', variant: 'accent' },
  approve: { label: '상품 승인', description: '현재 검토를 승인하고 상품을 출시 준비 상태로 전환합니다.', variant: 'accent' },
  reject: { label: '초안으로 반려', description: '현재 결정을 반려하고 수정 가능한 초안 상태로 되돌립니다.', variant: 'danger' },
  archive: { label: '상품 보관', description: '출시된 상품을 보관 상태로 전환하고 신규 이용을 중단합니다.', variant: 'danger' },
}

function lifecycleActionsFor(status: string) {
  const actions: ProductLifecycleAction[] = status === 'draft'
    ? ['submit']
    : status === 'review' || status === 'risk_review'
      ? ['approve', 'reject']
      : status === 'approved'
        ? ['reject']
        : status === 'published'
          ? ['archive']
          : []
  return actions.map((action) => ({ action, ...lifecycleActionMeta[action] }))
}

export function ProductWorkspacePage() {
  const { productKey = '', tab = 'overview' } = useParams()
  // React Router has already decoded path params. Decoding again would corrupt
  // legitimate keys containing a literal percent sequence (for example `%20`).
  const decodedKey = productKey
  const queryClient = useQueryClient()
  const mode = useAuthStore((state) => state.mode)
  const user = useAuthStore((state) => state.user)
  const canWrite = canWriteDataWorks(mode, user)
  const [transitionTarget, setTransitionTarget] = useState<ProductLifecycleAction | null>(null)
  const [publishConfirm, setPublishConfirm] = useState(false)
  const [products, canvas, approvals, evidence, gate, contract] = useQueries({
    queries: [
      { queryKey: ['dataworks', 'products'], queryFn: dataworksApi.products, staleTime: 30_000 },
      { queryKey: ['dataworks', 'product', decodedKey, 'canvas'], queryFn: () => dataworksApi.canvas(decodedKey), enabled: Boolean(decodedKey), staleTime: 30_000 },
      { queryKey: ['dataworks', 'product', decodedKey, 'approvals'], queryFn: () => dataworksApi.approvals(decodedKey), enabled: Boolean(decodedKey), staleTime: 15_000 },
      { queryKey: ['dataworks', 'product', decodedKey, 'evidence'], queryFn: () => dataworksApi.evidencePack(decodedKey), enabled: Boolean(decodedKey), staleTime: 15_000 },
      { queryKey: ['dataworks', 'product', decodedKey, 'gate'], queryFn: () => dataworksApi.publishGate(decodedKey), enabled: Boolean(decodedKey), staleTime: 10_000 },
      { queryKey: ['dataworks', 'product', decodedKey, 'contract'], queryFn: () => dataworksApi.contractVersions(decodedKey), enabled: Boolean(decodedKey), staleTime: 30_000 },
    ],
  })
  const publish = useMutation({
    mutationFn: () => dataworksApi.publish(decodedKey),
    onSuccess: async () => {
      setPublishConfirm(false)
      await queryClient.invalidateQueries({ queryKey: ['dataworks'] })
    },
  })
  const transition = useMutation({
    mutationFn: (action: ProductLifecycleAction) => dataworksApi.transitionProduct(decodedKey, action),
    onSuccess: async () => {
      setTransitionTarget(null)
      await queryClient.invalidateQueries({ queryKey: ['dataworks'] })
    },
  })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['dataworks'] })

  const product = products.data?.products.find((item) => item.product_key === decodedKey)
  const pending = products.isPending || canvas.isPending || approvals.isPending || evidence.isPending || gate.isPending || contract.isPending
  const error = products.error || canvas.error || approvals.error || evidence.error || gate.error || contract.error
  if (pending) return <PageLoader label="상품 작업 공간을 구성하는 중" />
  if (error) return <ErrorState error={error} retry={refresh} />
  if (!product) return <Card><EmptyState title="상품을 찾을 수 없습니다" description={`카탈로그에 ${decodedKey} 상품이 없습니다.`} /></Card>
  const lifecycleActions = lifecycleActionsFor(product.status)

  const workspaceData = {
    product,
    canvas: canvas.data?.canvas,
    canvasDraft: canvas.data?.draft,
    approvals: approvals.data?.approvals,
    evidence: evidence.data?.evidence_pack,
    gate: gate.data?.publish_gate,
    contract: contract.data?.contract_version,
  }

  return (
    <div>
      <div className="mb-5 flex items-center justify-between gap-4">
        <Link to="/products" className="inline-flex items-center gap-1.5 text-xs font-bold text-[var(--muted)] no-underline hover:text-[var(--ink)]"><ArrowLeft className="size-3.5" /> 전체 상품</Link>
        <Button variant="secondary" size="sm" onClick={refresh}><RefreshCw className="size-3.5" /> 새로고침</Button>
      </div>

      <header className="mb-5 flex flex-col justify-between gap-5 lg:flex-row lg:items-start">
        <div className="flex min-w-0 gap-4">
          <span className="grid size-12 shrink-0 place-items-center rounded-xl bg-[var(--product)] text-white"><Boxes className="size-5" /></span>
          <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="font-mono text-[11px] font-bold uppercase tracking-[.12em] text-[var(--muted-soft)]">완성 상품 · {product.product_key}</p><StatusBadge status={product.status} /><Badge tone={product.sensitivity.includes('restricted') || product.sensitivity.includes('personal') ? 'danger' : 'neutral'}>{sensitivityLabel(product.sensitivity)}</Badge></div><h1 className="mt-2 text-[clamp(1.7rem,3vw,2.45rem)] font-[750] leading-tight tracking-[-.05em] text-[var(--ink)]">{product.name_ko || product.name_en || product.product_key}</h1><p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--muted)]">{product.description || product.executive_summary || '상품 설명이 아직 작성되지 않았습니다.'}</p></div>
        </div>
        <div className="flex shrink-0 flex-wrap justify-end gap-2"><Button asChild variant="secondary"><Link to={`/factory?copilot=1&product=${encodeURIComponent(decodedKey)}`}><Sparkles className="size-4" /> 코파일럿에게 묻기</Link></Button>{canWrite ? lifecycleActions.map((action) => <Button key={action.action} onClick={() => { transition.reset(); setTransitionTarget(action.action) }} variant={action.variant}><Send className="size-4" /> {action.label}</Button>) : null}{canWrite && gate.data?.publish_gate.allowed && product.status === 'approved' ? <Button variant="accent" onClick={() => { publish.reset(); setPublishConfirm(true) }} disabled={publish.isPending}>{publish.isPending ? <LoaderCircle className="size-4 animate-spin" /> : null}{publish.isPending ? '출시 중…' : '상품 출시'}</Button> : null}</div>
      </header>

      <Card className="mb-5 px-4 pt-4"><div className="mb-1 flex items-center justify-between gap-3"><p className="text-[11px] font-extrabold uppercase tracking-[.12em] text-[var(--muted)]">상품 생명주기</p><p className="text-[11px] text-[var(--muted-soft)]">실시간 상품 증적을 기준으로 계산한 상태입니다</p></div><LifecycleStepper data={workspaceData} /></Card>

      <nav className="workspace-tabs mb-6" aria-label="상품 작업 공간 메뉴">
        {tabs.map((item) => <NavLink key={item} end={item === 'overview'} to={item === 'overview' ? `/products/${encodeURIComponent(decodedKey)}` : `/products/${encodeURIComponent(decodedKey)}/${item}`} className={({ isActive }) => cn('workspace-tab', isActive && 'is-active')}>{tabLabels[item]}</NavLink>)}
      </nav>

      <WorkspaceTab
        tab={tabs.includes(tab as (typeof tabs)[number]) ? tab : 'overview'}
        product={product}
        canvas={canvas.data?.canvas}
        canvasDraft={canvas.data?.draft}
        approvals={approvals.data?.approvals ?? []}
        evidence={evidence.data?.evidence_pack ?? null}
        gate={gate.data!.publish_gate}
        contract={contract.data?.contract_version ?? null}
        onPublish={() => { publish.reset(); setPublishConfirm(true) }}
        publishing={publish.isPending}
        canWrite={canWrite}
      />
      <ConfirmDialog
        confirmLabel={transitionTarget ? lifecycleActionMeta[transitionTarget].label : '상태 변경'}
        confirmVariant={transitionTarget === 'archive' || transitionTarget === 'reject' ? 'danger' : 'accent'}
        description={transitionTarget ? lifecycleActionMeta[transitionTarget].description : ''}
        error={transition.error}
        onClose={() => setTransitionTarget(null)}
        onConfirm={() => transitionTarget && transition.mutate(transitionTarget)}
        open={Boolean(transitionTarget)}
        pending={transition.isPending}
        title="상품 생명주기를 변경할까요?"
        warning="상태 변경은 감사 이력에 기록되며, 다음 단계의 작업과 출시 게이트에 즉시 반영됩니다."
      />
      <ConfirmDialog
        confirmLabel="상품 출시"
        confirmVariant="accent"
        description={`‘${product.name_ko || product.product_key}’ 상품을 마켓플레이스에 출시합니다.`}
        error={publish.error}
        onClose={() => setPublishConfirm(false)}
        onConfirm={() => publish.mutate()}
        open={publishConfirm}
        pending={publish.isPending}
        title="출시 게이트를 통과한 상품을 출시할까요?"
        warning="출시 즉시 마켓플레이스의 이용자에게 상품이 노출됩니다. 출시 후 종료하려면 먼저 보관 처리하세요."
      />
    </div>
  )
}

function WorkspaceTab({ tab, product, canvas, canvasDraft, approvals, evidence, gate, contract, onPublish, publishing, canWrite }: {
  tab: string
  product: DataProduct
  canvas?: ProductCanvas
  canvasDraft?: boolean
  approvals: ApprovalTrace[]
  evidence: EvidencePack | null
  gate: PublishGate
  contract: ContractVersion | null
  onPublish: () => void
  publishing: boolean
  canWrite: boolean
}) {
  if (tab === 'overview') return <OverviewTab canWrite={canWrite} product={product} gate={gate} evidence={evidence} canvas={canvas} onPublish={onPublish} publishing={publishing} />
  if (tab === 'canvas') return <CanvasTab canWrite={canWrite} canvas={canvas} draft={canvasDraft} productKey={product.product_key} />
  if (tab === 'approvals') return <ApprovalsTab approvals={approvals} canWrite={canWrite} gate={gate} />
  if (tab === 'evidence') return <EvidenceTab evidence={evidence} />
  if (tab === 'risk') return <RiskTab product={product} gate={gate} />
  if (tab === 'assets') return <AssetsTab product={product} gate={gate} />
  if (tab === 'contracts') return <ContractTab canWrite={canWrite} contract={contract} productKey={product.product_key} />
  return <FutureTab tab={tab} product={product} />
}

function OverviewTab({ product, gate, evidence, canvas, onPublish, publishing, canWrite }: { product: DataProduct; gate: PublishGate; evidence: EvidencePack | null; canvas?: ProductCanvas; onPublish: () => void; publishing: boolean; canWrite: boolean }) {
  return <div className="grid gap-5 xl:grid-cols-[1.35fr_.65fr]">
    <PublishGateCard gate={gate} onPublish={!canWrite || product.status !== 'approved' ? undefined : onPublish} publishing={publishing} />
    <div className="space-y-5">
      <Card><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">상품 신호</h2><Badge tone={product.revenue_score >= 75 ? 'success' : 'warning'}>{recommendation(product)}</Badge></CardHeader><CardContent><div className="grid grid-cols-2 gap-3"><Signal label="수익 잠재력" value={product.revenue_score} icon={CircleDollarSign} tone="success" /><Signal label="위험" value={product.risk_score} icon={ShieldAlert} tone={product.risk_score >= 70 ? 'danger' : 'warning'} /></div><div className="mt-4 space-y-3"><KeyValue label="상품 오너" value={product.owner || '미지정'} /><KeyValue label="가격 정책" value={product.pricing_model || '미설정'} /><KeyValue label="원천" value={`${product.source_type} · ${product.source_ref || '연결되지 않음'}`} /><KeyValue label="버전" value={`v${product.version}`} /></div></CardContent></Card>
      <Card><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">작업 공간 완성도</h2></CardHeader><CardContent className="space-y-3"><CompletionRow label="상품 블루프린트" complete={Boolean(canvas)} /><CompletionRow label="증적 패키지" complete={Boolean(evidence)} /><CompletionRow label="OpenAPI 계약" complete={gate.api_contract_configured} /><CompletionRow label="SLA 목표" complete={gate.sla_configured} /><CompletionRow label="가격 및 비용" complete={gate.pricing_model_configured} /></CardContent></Card>
    </div>
  </div>
}

function CanvasTab({ canvas, draft, productKey, canWrite }: { canvas?: ProductCanvas; draft?: boolean; productKey: string; canWrite: boolean }) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const save = useMutation({
    mutationFn: (payload: ProductCanvasInput) => dataworksApi.saveCanvas(productKey, payload),
    onSuccess: async () => {
      setEditing(false)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['dataworks', 'product', productKey, 'canvas'] }),
        queryClient.invalidateQueries({ queryKey: ['dataworks', 'product', productKey, 'gate'] }),
      ])
    },
  })
  const fields: Array<[string, string]> = canvas ? [
    ['고객 문제', canvas.customer_problem], ['구매 담당자', canvas.buyer], ['사용 사례', canvas.use_cases],
    ['제공 데이터', canvas.provided_data], ['차별점', canvas.differentiation], ['가격 정책', canvas.pricing_model],
    ['위험 참고 사항', canvas.risk_notes], ['PoC 성공 기준', canvas.poc_success_criteria], ['예상 수익', canvas.expected_revenue],
  ] : []
  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div><h2 className="text-lg font-bold tracking-[-.03em] text-[var(--ink)]">상품 블루프린트</h2><p className="mt-1 text-xs text-[var(--muted)]">고객 문제와 상품 가설을 하나의 설계도에서 검토합니다.</p></div>
        <div className="flex items-center gap-2">{canvas ? <Badge tone={draft ? 'warning' : 'success'}>{draft ? '초안 기본값' : '저장됨'}</Badge> : null}{canWrite ? <Button onClick={() => { save.reset(); setEditing(true) }} size="sm" variant="accent">{canvas ? <Pencil className="size-3.5" /> : <Plus className="size-3.5" />}{canvas ? '블루프린트 편집' : '블루프린트 만들기'}</Button> : null}</div>
      </div>
      {canvas ? <Card className="overflow-hidden"><div className="grid md:grid-cols-2 xl:grid-cols-3">{fields.map(([label, value]) => <section className="min-h-36 border-b border-r border-[var(--line)] p-5" key={label}><p className="text-[11px] font-extrabold uppercase tracking-[.12em] text-[var(--accent)]">{label}</p><p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-[var(--ink)]">{value || '—'}</p></section>)}</div></Card> : <Card><EmptyState description="상품 공장에서 생성하거나 직접 블루프린트를 작성해 주세요." title="블루프린트가 없습니다" /></Card>}
      <CanvasEditor
        canvas={canvas}
        error={save.error}
        key={editing ? canvas?.updated_at || 'new' : 'closed'}
        onClose={() => setEditing(false)}
        onSave={(payload) => save.mutate(payload)}
        open={editing}
        pending={save.isPending}
      />
    </div>
  )
}

const emptyCanvas: ProductCanvasInput = {
  customer_problem: '', buyer: '', use_cases: '', provided_data: '', differentiation: '', pricing_model: '',
  risk_notes: '', poc_success_criteria: '', expected_revenue: '', owner: '',
}

function CanvasEditor({ canvas, open, pending, error, onClose, onSave }: {
  canvas?: ProductCanvas
  open: boolean
  pending: boolean
  error: Error | null
  onClose: () => void
  onSave: (payload: ProductCanvasInput) => void
}) {
  const [form, setForm] = useState<ProductCanvasInput>(() => canvas ? {
    customer_problem: canvas.customer_problem,
    buyer: canvas.buyer,
    use_cases: canvas.use_cases,
    provided_data: canvas.provided_data,
    differentiation: canvas.differentiation,
    pricing_model: canvas.pricing_model,
    risk_notes: canvas.risk_notes,
    poc_success_criteria: canvas.poc_success_criteria,
    expected_revenue: canvas.expected_revenue,
    owner: canvas.owner,
  } : emptyCanvas)
  const submit = (event: FormEvent) => { event.preventDefault(); onSave(form) }
  const textFields: Array<{ key: keyof ProductCanvasInput; label: string; placeholder: string }> = [
    { key: 'customer_problem', label: '고객 문제', placeholder: '누가 어떤 문제를 해결해야 하나요?' },
    { key: 'buyer', label: '구매 담당자', placeholder: '예: 리스크 관리 책임자' },
    { key: 'use_cases', label: '사용 사례', placeholder: '핵심 사용 시나리오' },
    { key: 'provided_data', label: '제공 데이터', placeholder: '필드와 제공 범위' },
    { key: 'differentiation', label: '차별점', placeholder: '대안 대비 강점' },
    { key: 'pricing_model', label: '가격 정책', placeholder: '과금 단위와 가격 가설' },
    { key: 'risk_notes', label: '위험 참고 사항', placeholder: '개인정보·보안·규제 위험' },
    { key: 'poc_success_criteria', label: 'PoC 성공 기준', placeholder: '측정 가능한 성공 기준' },
    { key: 'expected_revenue', label: '예상 수익', placeholder: '기간과 가정을 포함한 수익' },
  ]
  return (
    <Dialog
      actions={<><Button disabled={pending} onClick={onClose} type="button" variant="secondary">취소</Button><Button disabled={pending} form="canvas-editor-form" type="submit" variant="accent">{pending ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}{pending ? '저장 중…' : '블루프린트 저장'}</Button></>}
      busy={pending}
      className="!w-[min(100%,900px)]"
      description="출시 검토에 필요한 고객 가치, 데이터 범위, 가격과 위험 가설을 함께 관리합니다."
      onClose={onClose}
      open={open}
      title="상품 블루프린트 편집"
    >
      <form className="grid gap-4 sm:grid-cols-2" id="canvas-editor-form" onSubmit={submit}>
        {textFields.map((field, index) => <FormField key={field.key} label={field.label}><textarea autoFocus={index === 0} className="field-input h-24 py-3" onChange={(event) => setForm({ ...form, [field.key]: event.target.value })} placeholder={field.placeholder} value={form[field.key]} /></FormField>)}
        <FormField label="블루프린트 오너"><input className="field-input" onChange={(event) => setForm({ ...form, owner: event.target.value })} placeholder="product-owner" value={form.owner} /></FormField>
        {error ? <p className="field-error sm:col-span-2" role="alert">{error.message}</p> : null}
      </form>
    </Dialog>
  )
}

function ApprovalsTab({ approvals, gate, canWrite }: { approvals: ApprovalTrace[]; gate: PublishGate; canWrite: boolean }) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<ApprovalTrace | 'new' | null>(null)
  const save = useMutation({
    mutationFn: (payload: ApprovalTraceInput) => dataworksApi.saveApproval(gate.product_key, payload),
    onSuccess: async () => {
      setEditing(null)
      await queryClient.invalidateQueries({ queryKey: ['dataworks', 'product', gate.product_key] })
    },
  })
  const required = gate.required_approvals.map((step) => approvals.find((item) => item.step === step) ?? {
    id: '', product_key: gate.product_key, step, status: gate.approval_status[step] || 'pending', required: true,
    evidence_ref: '', notes: '', decided_by: '', expires_at: '', created_at: '', updated_at: '',
  } as ApprovalTrace)
  const requiredSteps = new Set(gate.required_approvals)
  const rows = [...required, ...approvals.filter((item) => !requiredSteps.has(item.step))]
  return (
    <Card>
      <CardHeader>
        <div><h2 className="text-sm font-bold text-[var(--ink)]">승인 추적</h2><p className="mt-1 text-xs text-[var(--muted)]">데이터 오너·법무·준법 결정과 만료 상태</p></div>
        <div className="flex items-center gap-2"><Badge tone={gate.missing_approvals.length ? 'warning' : 'success'}>{gate.missing_approvals.length ? `${gate.missing_approvals.length}건 대기` : '완료'}</Badge>{canWrite ? <Button onClick={() => { save.reset(); setEditing('new') }} size="sm" variant="secondary"><Plus className="size-3.5" /> 승인 항목 추가</Button> : null}</div>
      </CardHeader>
      <CardContent>
        {rows.length ? <div className="space-y-2">{rows.map((approval) => <div className="flex flex-col justify-between gap-3 rounded-xl border border-[var(--line)] p-4 sm:flex-row sm:items-center" key={`${approval.id}:${approval.step}`}><div className="flex items-center gap-3"><span className={`grid size-9 place-items-center rounded-xl ${['approved', 'waived'].includes(approval.status) ? 'bg-[var(--success-soft)] text-[var(--success)]' : 'bg-[var(--warning-soft)] text-[var(--warning)]'}`}>{['approved', 'waived'].includes(approval.status) ? <CheckCircle2 className="size-4" /> : <FileCheck2 className="size-4" />}</span><div><p className="text-xs font-bold text-[var(--ink)]">{approvalLabel(approval.step)}</p><p className="mt-1 text-[11px] text-[var(--muted)]">{approval.decided_by || '검토자 미지정'}{approval.expires_at ? ` · ${formatDate(approval.expires_at)} 만료` : ''}</p>{approval.notes ? <p className="mt-1 text-[11px] text-[var(--muted)]">{approval.notes}</p> : null}</div></div><div className="flex items-center gap-2"><StatusBadge status={approval.status} />{canWrite ? <Button onClick={() => { save.reset(); setEditing(approval) }} size="sm" variant="secondary"><Pencil className="size-3.5" /> 결정 기록</Button> : null}</div></div>)}</div> : <EmptyState description="승인 단계를 추가하고 검토 결정을 기록해 주세요." title="승인 항목이 없습니다" />}
      </CardContent>
      <ApprovalEditor
        approval={editing === 'new' ? undefined : editing ?? undefined}
        error={save.error}
        key={editing === 'new' ? 'new' : editing ? `${editing.id}:${editing.step}` : 'closed'}
        onClose={() => setEditing(null)}
        onSave={(payload) => save.mutate(payload)}
        open={editing !== null}
        pending={save.isPending}
        strictGate={gate.strict_gate}
      />
    </Card>
  )
}

const emptyApproval: ApprovalTraceInput = {
  step: '', status: 'pending', required: true, evidence_ref: '', notes: '', decided_by: '', expires_at: '',
}

function ApprovalEditor({ approval, open, pending, error, strictGate, onClose, onSave }: {
  approval?: ApprovalTrace
  open: boolean
  pending: boolean
  error: Error | null
  strictGate: boolean
  onClose: () => void
  onSave: (payload: ApprovalTraceInput) => void
}) {
  const [form, setForm] = useState<ApprovalTraceInput>(() => approval ? {
    id: approval.id || undefined,
    step: approval.step,
    status: approval.status,
    required: approval.required,
    evidence_ref: approval.evidence_ref,
    notes: approval.notes,
    decided_by: approval.decided_by,
    expires_at: approval.expires_at,
  } : emptyApproval)
  const policyRequired = strictGate && ['data_owner', 'legal', 'compliance'].includes(form.step.trim().toLowerCase())
  const submit = (event: FormEvent) => { event.preventDefault(); onSave({ ...form, step: form.step.trim(), required: policyRequired || form.required }) }
  return (
    <Dialog
      actions={<><Button disabled={pending} onClick={onClose} type="button" variant="secondary">취소</Button><Button disabled={pending || !form.step.trim()} form="approval-editor-form" type="submit" variant="accent">{pending ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}{pending ? '저장 중…' : '승인 기록 저장'}</Button></>}
      busy={pending}
      className="!w-[min(100%,680px)]"
      description="필수 승인 여부, 결정 상태와 연결 증적을 저장하면 출시 게이트가 다시 계산됩니다."
      onClose={onClose}
      open={open}
      title={approval ? '승인 결정 수정' : '승인 항목 추가'}
    >
      <form className="grid gap-4 sm:grid-cols-2" id="approval-editor-form" onSubmit={submit}>
        <FormField label="승인 단계"><input autoFocus className="field-input" disabled={Boolean(approval?.id)} onChange={(event) => setForm({ ...form, step: event.target.value })} placeholder="legal" required value={form.step} /></FormField>
        <FormField label="결정 상태"><select className="field-input" onChange={(event) => setForm({ ...form, status: event.target.value })} value={form.status}><option value="pending">대기</option><option value="approved">승인</option><option value="rejected">거절</option><option value="waived">면제</option><option value="expired">만료</option></select></FormField>
        <FormField label="결정자"><input className="field-input" onChange={(event) => setForm({ ...form, decided_by: event.target.value })} placeholder="비우면 현재 관리자" value={form.decided_by} /></FormField>
        <FormField label="만료 시각"><input className="field-input" onChange={(event) => setForm({ ...form, expires_at: event.target.value })} placeholder="2026-12-31T23:59:59Z" value={form.expires_at} /></FormField>
        <FormField className="sm:col-span-2" label="증적 참조"><input className="field-input" onChange={(event) => setForm({ ...form, evidence_ref: event.target.value })} placeholder="문서 또는 티켓 참조" value={form.evidence_ref} /></FormField>
        <FormField className="sm:col-span-2" label="결정 메모"><textarea className="field-input h-24 py-3" onChange={(event) => setForm({ ...form, notes: event.target.value })} placeholder="결정 근거와 후속 조건" value={form.notes} /></FormField>
        <label className="toggle-row sm:col-span-2"><span><strong>필수 승인</strong><small>{policyRequired ? '엄격 게이트 정책상 필수 단계입니다.' : '누락되거나 만료되면 출시 게이트를 차단합니다.'}</small></span><input checked={policyRequired || form.required} disabled={policyRequired} onChange={(event) => setForm({ ...form, required: event.target.checked })} type="checkbox" /></label>
        {error ? <p className="field-error sm:col-span-2" role="alert">{error.message}</p> : null}
      </form>
    </Dialog>
  )
}

function EvidenceTab({ evidence }: { evidence: EvidencePack | null }) {
  if (!evidence) return <Card><EmptyState title="증적 패키지가 없습니다" description="현재 상품 상태와 승인 증적을 모아 증적 패키지를 생성해야 출시 게이트를 통과할 수 있습니다." /></Card>
  let parsed: unknown = evidence.pack_json
  try { parsed = JSON.parse(evidence.pack_json) } catch { /* show raw value */ }
  return <div className="grid gap-5 xl:grid-cols-[.6fr_1.4fr]"><Card><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">증적 아티팩트</h2><Badge tone="success">검증됨</Badge></CardHeader><CardContent className="space-y-3"><KeyValue label="상품" value={evidence.product_key} /><KeyValue label="아티팩트 참조" value={evidence.artifact_ref || '내장 패키지'} /><KeyValue label="생성자" value={evidence.created_by || '시스템'} /><KeyValue label="수정 시각" value={formatDate(evidence.updated_at || evidence.created_at)} /></CardContent></Card><Card><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">증적 데이터</h2><FileJson2 className="size-4 text-[var(--violet)]" /></CardHeader><CardContent><pre className="max-h-[560px] overflow-auto rounded-xl bg-[var(--ink)] p-4 text-[11px] leading-5 text-[var(--surface)]">{typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2)}</pre></CardContent></Card></div>
}

function RiskTab({ product, gate }: { product: DataProduct; gate: PublishGate }) {
  return <div className="grid gap-5 lg:grid-cols-3"><SignalCard title="위험 점수" value={product.risk_score} description={product.risk_score >= 70 ? '엄격한 출시 게이트가 적용됩니다' : '표준 위험 프로필입니다'} icon={<ShieldAlert className="size-5" />} tone={product.risk_score >= 70 ? 'danger' : 'warning'} /><SignalCard title="위험 검토" value={gate.risk_reviewed ? '완료' : '누락'} description="최신 상품 위험 검토 상태" icon={<FileCheck2 className="size-5" />} tone={gate.risk_reviewed ? 'success' : 'warning'} /><SignalCard title="마스킹" value={gate.masking_configured ? '준비됨' : '누락'} description="민감 응답 보호 정책" icon={<KeyRound className="size-5" />} tone={gate.masking_configured ? 'success' : 'danger'} /></div>
}

function AssetsTab({ product, gate }: { product: DataProduct; gate: PublishGate }) {
  return <Card><CardHeader><div><h2 className="text-sm font-bold text-[var(--ink)]">원천 데이터 자산</h2><p className="mt-1 text-xs text-[var(--muted)]">상품 원천 참조에 연결된 자산입니다</p></div><Database className="size-4 text-[var(--accent)]" /></CardHeader><CardContent>{gate.asset_readiness.length ? <div className="space-y-3">{gate.asset_readiness.map((asset) => <div key={asset.asset_key} className="flex items-center gap-4 rounded-xl border border-[var(--line)] p-4"><span className="grid size-10 place-items-center rounded-xl bg-[var(--info-soft)] text-[var(--accent)]"><Database className="size-4" /></span><div className="min-w-0 flex-1"><p className="truncate font-mono text-xs font-bold text-[var(--ink)]">{asset.asset_key}</p><p className="mt-1 text-[11px] text-[var(--muted)]">최신성 {asset.freshness_score} · 스키마 {asset.schema_score} · API 준비도 {asset.api_readiness_score}</p></div><Badge tone={asset.overall_score >= gate.minimum_readiness ? 'success' : 'danger'}>{asset.overall_score}/100</Badge></div>)}</div> : <EmptyState title="준비도 증적이 없습니다" description={`${product.source_ref || '연결된 자산'}에 대한 준비도 점수를 실행해 주세요.`} />}</CardContent></Card>
}

function ContractTab({ contract, productKey, canWrite }: { contract: ContractVersion | null; productKey: string; canWrite: boolean }) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const create = useMutation({
    mutationFn: (payload: ContractVersionInput) => {
      let parsed: unknown
      try { parsed = JSON.parse(payload.contract_json) } catch { throw new Error('계약 정의는 올바른 JSON이어야 합니다.') }
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error('계약 정의는 JSON 객체여야 합니다.')
      return dataworksApi.createContractVersion(productKey, payload)
    },
    onSuccess: async () => {
      setEditing(false)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['dataworks', 'product', productKey, 'contract'] }),
        queryClient.invalidateQueries({ queryKey: ['dataworks', 'product', productKey, 'gate'] }),
      ])
    },
  })
  let formattedContract = contract?.contract_json ?? '{}'
  try { formattedContract = JSON.stringify(JSON.parse(formattedContract), null, 2) } catch { /* preserve stored value */ }
  return (
    <Card>
      <CardHeader>
        <div><h2 className="text-sm font-bold text-[var(--ink)]">최신 계약 버전</h2><p className="mt-1 text-xs text-[var(--muted)]">계약은 감사 가능성을 위해 수정·삭제하지 않고 새 버전으로 추가합니다.</p></div>
        <div className="flex items-center gap-2">{contract ? <StatusBadge status={contract.status} /> : null}{canWrite ? <Button onClick={() => { create.reset(); setEditing(true) }} size="sm" variant="accent"><Plus className="size-3.5" /> {contract ? '새 버전' : '계약 생성'}</Button> : null}</div>
      </CardHeader>
      <CardContent>
        {contract ? <><div className="grid gap-4 sm:grid-cols-3"><KeyValue label="버전" value={`v${contract.version}`} /><KeyValue label="상태" value={statusLabel(contract.status)} /><KeyValue label="생성 시각" value={formatDate(contract.created_at)} /></div><pre className="mt-5 max-h-96 overflow-auto rounded-xl bg-[var(--ink)] p-4 text-[11px] leading-5 text-[var(--surface)]">{formattedContract}</pre></> : <EmptyState description="고객 범위, 허용 필드, 호출 한도와 사용 목적을 정의해 계약을 생성하세요." title="계약 버전이 없습니다" />}
      </CardContent>
      <ContractEditor
        contract={contract}
        error={create.error}
        key={editing ? contract?.version ?? 'new' : 'closed'}
        onClose={() => setEditing(false)}
        onSave={(payload) => create.mutate(payload)}
        open={editing}
        pending={create.isPending}
      />
    </Card>
  )
}

function ContractEditor({ contract, open, pending, error, onClose, onSave }: {
  contract: ContractVersion | null
  open: boolean
  pending: boolean
  error: Error | null
  onClose: () => void
  onSave: (payload: ContractVersionInput) => void
}) {
  const [form, setForm] = useState<ContractVersionInput>(() => ({
    status: contract?.status || 'draft',
    contract_json: contract?.contract_json || JSON.stringify({ allowed_fields: [], purpose: '', rate_limit: 1000 }, null, 2),
  }))
  const submit = (event: FormEvent) => { event.preventDefault(); onSave(form) }
  return (
    <Dialog
      actions={<><Button disabled={pending} onClick={onClose} type="button" variant="secondary">취소</Button><Button disabled={pending} form="contract-editor-form" type="submit" variant="accent">{pending ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}{pending ? '생성 중…' : '계약 버전 생성'}</Button></>}
      busy={pending}
      className="!w-[min(100%,760px)]"
      description={contract ? `v${contract.version} 내용을 바탕으로 변경된 계약을 새 버전으로 저장합니다.` : '허용 필드, 이용 목적과 호출 한도를 JSON 객체로 정의합니다.'}
      onClose={onClose}
      open={open}
      title={contract ? '새 계약 버전 만들기' : '첫 계약 버전 만들기'}
    >
      <form className="grid gap-4" id="contract-editor-form" onSubmit={submit}>
        <FormField label="계약 상태"><select className="field-input" onChange={(event) => setForm({ ...form, status: event.target.value })} value={form.status}><option value="draft">초안</option><option value="active">활성</option><option value="approved">승인</option><option value="expired">만료</option></select></FormField>
        <FormField label="계약 정의(JSON)"><textarea autoFocus className="field-input h-80 py-3 font-mono text-xs leading-5" onChange={(event) => setForm({ ...form, contract_json: event.target.value })} spellCheck={false} value={form.contract_json} /></FormField>
        {error ? <p className="field-error" role="alert">{error.message}</p> : null}
      </form>
    </Dialog>
  )
}

function FutureTab({ tab, product }: { tab: string; product: DataProduct }) {
  const config: Record<string, { icon: ReactNode; title: string; description: string }> = {
    api: { icon: <Code2 className="size-5" />, title: 'API 시험실', description: 'OpenAPI 문서, 요청 샘플과 실제 응답 미리보기를 확인합니다.' },
    customers: { icon: <Users className="size-5" />, title: '고객', description: '고객군, 적합도 점수, PoC와 제안 이력을 상품 중심으로 연결합니다.' },
    usage: { icon: <Activity className="size-5" />, title: '운영 사용량', description: '호출량, 지연 시간, 할당량과 활성 접근 권한을 추적합니다.' },
    revenue: { icon: <CircleDollarSign className="size-5" />, title: '수익 및 마진', description: '상품 비용 구조와 고객별 예상 수익·마진을 비교합니다.' },
    versions: { icon: <Link2 className="size-5" />, title: '버전', description: '상품 정의 스냅샷과 버전 간 변경점을 검토합니다.' },
    activity: { icon: <Activity className="size-5" />, title: '활동 이력', description: '승인, AI 실행, 계약, 출시 관련 감사 이벤트를 시간순으로 표시합니다.' },
  }
  const item = config[tab] ?? { icon: <Lightbulb className="size-5" />, title: '상품 모듈', description: '이 상품 작업 모듈은 준비 중입니다.' }
  return <Card><CardContent className="flex min-h-72 flex-col items-center justify-center text-center"><span className="grid size-12 place-items-center rounded-xl bg-[var(--info-soft)] text-[var(--accent)]">{item.icon}</span><h2 className="mt-4 text-base font-bold tracking-[-.03em] text-[var(--ink)]">{item.title}</h2><p className="mt-2 max-w-md text-sm leading-6 text-[var(--muted)]">{item.description}</p><p className="mt-5 font-mono text-[11px] text-[var(--muted-soft)]">{product.product_key} / {tabLabels[tab as keyof typeof tabLabels] ?? '모듈'}</p></CardContent></Card>
}

function Signal({ label, value, icon: Icon, tone }: { label: string; value: number; icon: typeof CircleDollarSign; tone: 'success' | 'warning' | 'danger' }) { return <div className="rounded-xl bg-[var(--surface-muted)] p-3"><p className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-[var(--muted)]"><Icon className="size-3" />{label}</p><p className="mt-2 text-2xl font-[750] tracking-[-.05em]" style={{ color: `var(--${tone})` }}>{value}</p></div> }
function KeyValue({ label, value }: { label: string; value: string }) { return <div className="flex items-start justify-between gap-4 border-b border-[var(--line)] pb-3 last:border-0 last:pb-0"><span className="text-xs font-semibold text-[var(--muted)]">{label}</span><span className="max-w-[65%] text-right text-xs font-bold text-[var(--ink)]">{value}</span></div> }
function CompletionRow({ label, complete }: { label: string; complete: boolean }) { return <div className="flex items-center justify-between gap-3"><span className="text-xs font-semibold text-[var(--muted)]">{label}</span><Badge tone={complete ? 'success' : 'warning'}>{complete ? '완료' : '누락'}</Badge></div> }
function SignalCard({ title, value, description, icon, tone }: { title: string; value: string | number; description: string; icon: ReactNode; tone: 'success' | 'warning' | 'danger' }) { return <Card><CardContent><span className="grid size-10 place-items-center rounded-xl" style={{ background: `var(--${tone}-soft)`, color: `var(--${tone})` }}>{icon}</span><p className="mt-5 text-[11px] font-bold uppercase tracking-wider text-[var(--muted)]">{title}</p><p className="mt-2 text-3xl font-[750] tracking-[-.05em] text-[var(--ink)]">{value}</p><p className="mt-2 text-xs text-[var(--muted)]">{description}</p></CardContent></Card> }
function FormField({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) { return <label className={className}><span className="field-label">{label}</span>{children}</label> }
function recommendation(product: DataProduct) { if (product.status === 'archived') return '종료 검토'; if (product.risk_score >= 75) return '재검토'; if (product.revenue_score >= 80 && product.status !== 'published') return '출시'; if (product.revenue_score >= 70) return '투자'; return '개선' }
