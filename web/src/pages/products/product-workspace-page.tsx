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
  RefreshCw,
  ShieldAlert,
  Sparkles,
  Users,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { Link, NavLink, useParams } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { LifecycleStepper } from '@/features/lifecycle/lifecycle-stepper'
import { PublishGateCard } from '@/features/publish-gate/publish-gate-card'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { EmptyState, ErrorState, PageLoader } from '@/components/ui/query-state'
import { approvalLabel, sensitivityLabel, statusLabel } from '@/lib/labels.ko'
import { cn, formatDate } from '@/lib/utils'
import type { ApprovalTrace, DataProduct, EvidencePack, ProductCanvas, PublishGate } from '@/types/dataworks'

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

export function ProductWorkspacePage() {
  const { productKey = '', tab = 'overview' } = useParams()
  const decodedKey = decodeURIComponent(productKey)
  const queryClient = useQueryClient()
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
      await queryClient.invalidateQueries({ queryKey: ['dataworks'] })
    },
  })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['dataworks', 'product', decodedKey] })

  const product = products.data?.products.find((item) => item.product_key === decodedKey)
  const pending = products.isPending || canvas.isPending || approvals.isPending || evidence.isPending || gate.isPending || contract.isPending
  const error = products.error || canvas.error || approvals.error || evidence.error || gate.error || contract.error
  if (pending) return <PageLoader label="상품 작업 공간을 구성하는 중" />
  if (error) return <ErrorState error={error} retry={refresh} />
  if (!product) return <Card><EmptyState title="상품을 찾을 수 없습니다" description={`카탈로그에 ${decodedKey} 상품이 없습니다.`} /></Card>

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
        <div className="flex shrink-0 gap-2"><Button asChild variant="secondary"><Link to={`/factory?copilot=1&product=${encodeURIComponent(decodedKey)}`}><Sparkles className="size-4" /> 코파일럿에게 묻기</Link></Button>{gate.data?.publish_gate.allowed && product.status !== 'published' ? <Button variant="accent" onClick={() => publish.mutate()} disabled={publish.isPending}>{publish.isPending ? '출시 중…' : '상품 출시'}</Button> : null}</div>
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
        onPublish={() => publish.mutate()}
        publishing={publish.isPending}
      />
    </div>
  )
}

function WorkspaceTab({ tab, product, canvas, canvasDraft, approvals, evidence, gate, contract, onPublish, publishing }: {
  tab: string
  product: DataProduct
  canvas?: ProductCanvas
  canvasDraft?: boolean
  approvals: ApprovalTrace[]
  evidence: EvidencePack | null
  gate: PublishGate
  contract: { version: number; status: string; created_at: string } | null
  onPublish: () => void
  publishing: boolean
}) {
  if (tab === 'overview') return <OverviewTab product={product} gate={gate} evidence={evidence} canvas={canvas} onPublish={onPublish} publishing={publishing} />
  if (tab === 'canvas') return <CanvasTab canvas={canvas} draft={canvasDraft} />
  if (tab === 'approvals') return <ApprovalsTab approvals={approvals} gate={gate} />
  if (tab === 'evidence') return <EvidenceTab evidence={evidence} />
  if (tab === 'risk') return <RiskTab product={product} gate={gate} />
  if (tab === 'assets') return <AssetsTab product={product} gate={gate} />
  if (tab === 'contracts') return <ContractTab contract={contract} />
  return <FutureTab tab={tab} product={product} />
}

function OverviewTab({ product, gate, evidence, canvas, onPublish, publishing }: { product: DataProduct; gate: PublishGate; evidence: EvidencePack | null; canvas?: ProductCanvas; onPublish: () => void; publishing: boolean }) {
  return <div className="grid gap-5 xl:grid-cols-[1.35fr_.65fr]">
    <PublishGateCard gate={gate} onPublish={product.status === 'published' ? undefined : onPublish} publishing={publishing} />
    <div className="space-y-5">
      <Card><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">상품 신호</h2><Badge tone={product.revenue_score >= 75 ? 'success' : 'warning'}>{recommendation(product)}</Badge></CardHeader><CardContent><div className="grid grid-cols-2 gap-3"><Signal label="수익 잠재력" value={product.revenue_score} icon={CircleDollarSign} tone="success" /><Signal label="위험" value={product.risk_score} icon={ShieldAlert} tone={product.risk_score >= 70 ? 'danger' : 'warning'} /></div><div className="mt-4 space-y-3"><KeyValue label="상품 오너" value={product.owner || '미지정'} /><KeyValue label="가격 정책" value={product.pricing_model || '미설정'} /><KeyValue label="원천" value={`${product.source_type} · ${product.source_ref || '연결되지 않음'}`} /><KeyValue label="버전" value={`v${product.version}`} /></div></CardContent></Card>
      <Card><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">작업 공간 완성도</h2></CardHeader><CardContent className="space-y-3"><CompletionRow label="상품 블루프린트" complete={Boolean(canvas)} /><CompletionRow label="증적 패키지" complete={Boolean(evidence)} /><CompletionRow label="OpenAPI 계약" complete={gate.api_contract_configured} /><CompletionRow label="SLA 목표" complete={gate.sla_configured} /><CompletionRow label="가격 및 비용" complete={gate.pricing_model_configured} /></CardContent></Card>
    </div>
  </div>
}

function CanvasTab({ canvas, draft }: { canvas?: ProductCanvas; draft?: boolean }) {
  if (!canvas) return <Card><EmptyState title="블루프린트가 없습니다" description="AI 상품 공장에서 상품 블루프린트를 생성해 주세요." /></Card>
  const fields: Array<[string, string]> = [['고객 문제', canvas.customer_problem], ['구매 담당자', canvas.buyer], ['사용 사례', canvas.use_cases], ['제공 데이터', canvas.provided_data], ['차별점', canvas.differentiation], ['가격 정책', canvas.pricing_model], ['위험 참고 사항', canvas.risk_notes], ['PoC 성공 기준', canvas.poc_success_criteria], ['예상 수익', canvas.expected_revenue]]
  return <div><div className="mb-4 flex items-center justify-between"><div><h2 className="text-lg font-bold tracking-[-.03em] text-[var(--ink)]">상품 블루프린트</h2><p className="mt-1 text-xs text-[var(--muted)]">고객 문제와 상품 가설을 하나의 설계도에서 검토합니다.</p></div><Badge tone={draft ? 'warning' : 'success'}>{draft ? '초안 기본값' : '저장됨'}</Badge></div><Card className="overflow-hidden"><div className="grid md:grid-cols-2 xl:grid-cols-3">{fields.map(([label, value]) => <section key={label} className="min-h-36 border-b border-r border-[var(--line)] p-5"><p className="text-[11px] font-extrabold uppercase tracking-[.12em] text-[var(--accent)]">{label}</p><p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-[var(--ink)]">{value || '—'}</p></section>)}</div></Card></div>
}

function ApprovalsTab({ approvals, gate }: { approvals: ApprovalTrace[]; gate: PublishGate }) {
  const required = gate.required_approvals.map((step) => approvals.find((item) => item.step === step) ?? { step, status: gate.approval_status[step] || 'pending', required: true } as ApprovalTrace)
  return <Card><CardHeader><div><h2 className="text-sm font-bold text-[var(--ink)]">승인 추적</h2><p className="mt-1 text-xs text-[var(--muted)]">데이터 오너·법무·준법 결정과 만료 상태</p></div><Badge tone={gate.missing_approvals.length ? 'warning' : 'success'}>{gate.missing_approvals.length ? `${gate.missing_approvals.length}건 대기` : '완료'}</Badge></CardHeader><CardContent><div className="space-y-2">{required.map((approval) => <div key={approval.step} className="flex flex-col justify-between gap-3 rounded-xl border border-[var(--line)] p-4 sm:flex-row sm:items-center"><div className="flex items-center gap-3"><span className={`grid size-9 place-items-center rounded-xl ${['approved', 'waived'].includes(approval.status) ? 'bg-[var(--success-soft)] text-[var(--success)]' : 'bg-[var(--warning-soft)] text-[var(--warning)]'}`}>{['approved', 'waived'].includes(approval.status) ? <CheckCircle2 className="size-4" /> : <FileCheck2 className="size-4" />}</span><div><p className="text-xs font-bold text-[var(--ink)]">{approvalLabel(approval.step)}</p><p className="mt-1 text-[11px] text-[var(--muted)]">{approval.decided_by || '검토자 미지정'}{approval.expires_at ? ` · ${formatDate(approval.expires_at)} 만료` : ''}</p></div></div><div className="flex items-center gap-2"><StatusBadge status={approval.status} /><Button asChild size="sm" variant="secondary"><Link to={`/review?product=${encodeURIComponent(gate.product_key)}`}>검토 열기</Link></Button></div></div>)}</div></CardContent></Card>
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

function ContractTab({ contract }: { contract: { version: number; status: string; created_at: string } | null }) {
  return contract ? <Card><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">최신 계약 버전</h2><StatusBadge status={contract.status} /></CardHeader><CardContent><div className="grid gap-4 sm:grid-cols-3"><KeyValue label="버전" value={`v${contract.version}`} /><KeyValue label="상태" value={statusLabel(contract.status)} /><KeyValue label="생성 시각" value={formatDate(contract.created_at)} /></div></CardContent></Card> : <Card><EmptyState title="계약 버전이 없습니다" description="고객 범위, 허용 필드, 호출 한도와 사용 목적을 정의해 계약을 생성하세요." /></Card>
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
function recommendation(product: DataProduct) { if (product.status === 'archived') return '종료 검토'; if (product.risk_score >= 75) return '재검토'; if (product.revenue_score >= 80 && product.status !== 'published') return '출시'; if (product.revenue_score >= 70) return '투자'; return '개선' }
