import { useQueries, useQuery } from '@tanstack/react-query'
import { Background, Controls, MarkerType, ReactFlow, type Edge, type Node } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Blocks,
  Bot,
  Boxes,
  CircleDollarSign,
  Code2,
  Database,
  Factory,
  FileClock,
  KeyRound,
  Landmark,
  PackageCheck,
  RefreshCw,
  Settings,
  ShieldCheck,
  ShoppingBag,
  Sparkles,
  Users,
  type LucideIcon,
} from 'lucide-react'
import { useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'

import { dataworksApi } from '@/api/dataworks'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { EmptyState, ErrorState, PageLoader } from '@/components/ui/query-state'
import {
  actionMessageLabel,
  actionTypeLabel,
  gateMessageLabel,
  graphRelationLabel,
  policyDecisionLabel,
  runTypeLabel,
  severityLabel,
  statusLabel,
} from '@/lib/labels.ko'
import { formatDate, formatNumber } from '@/lib/utils'
import type { ActionItem, PortfolioGraphNode } from '@/types/dataworks'

const completedRunStatuses = new Set(['completed', 'success', 'succeeded'])
const activeRunStatuses = new Set(['pending', 'processing', 'queued', 'running'])
const passedPolicyDecisions = new Set(['allow', 'allowed', 'approved'])
const blockedPolicyDecisions = new Set(['block', 'blocked', 'deny', 'denied'])

export function FactoryPage() {
  const query = useQuery({
    queryKey: ['dataworks', 'factory-runs'],
    queryFn: dataworksApi.factoryRuns,
    staleTime: 20_000,
  })
  if (query.isPending) return <PageLoader label="AI 팩토리 실행 이력을 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />

  const runs = query.data.runs
  const successful = runs.filter((run) => completedRunStatuses.has(run.status.toLowerCase())).length
  const active = runs.filter((run) => activeRunStatuses.has(run.status.toLowerCase())).length
  const averageLatency = runs.length
    ? Math.round(runs.reduce((sum, run) => sum + run.latency_ms, 0) / runs.length)
    : 0
  const totalCost = runs.reduce((sum, run) => sum + run.token_cost, 0)
  const inputCount = new Set(runs.map((run) => run.input_hash).filter(Boolean)).size
  const outputCount = new Set(runs.map((run) => run.output_ref).filter(Boolean)).size
  const policyEvaluated = runs.filter((run) => run.policy_decision.trim()).length
  const policyPassed = runs.filter((run) => passedPolicyDecisions.has(run.policy_decision.toLowerCase())).length
  const policyBlocked = runs.filter((run) => blockedPolicyDecisions.has(run.policy_decision.toLowerCase())).length
  const latestRun = runs[0]

  return (
    <div>
      <PageHeader
        eyebrow="AI 팩토리 · 실행 관찰"
        title="데이터 상품 공장"
        description="입력 데이터가 AI 공정과 정책 게이트를 거쳐 재사용 가능한 데이터 상품 산출물로 정제되는 흐름을 추적합니다."
        actions={(
          <Button variant="secondary" onClick={() => void query.refetch()} disabled={query.isFetching}>
            <RefreshCw className={query.isFetching ? 'size-4 animate-spin' : 'size-4'} /> 실행 이력 새로고침
          </Button>
        )}
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <MiniMetric label="최근 30일 실행" value={runs.length} icon={Factory} tone="violet" />
        <MiniMetric label="완료된 실행" value={successful} icon={ShieldCheck} tone="success" />
        <MiniMetric label="평균 응답 시간" value={`${formatNumber(averageLatency)}ms`} icon={Activity} tone="info" />
        <MiniMetric label="누적 토큰 비용" value={totalCost.toFixed(2)} icon={CircleDollarSign} tone="warning" />
      </div>

      <Card className="mb-5 overflow-hidden rounded-xl shadow-none">
        <CardHeader>
          <div>
            <h2 className="text-sm font-bold text-[var(--ink)]">정제 공정 흐름</h2>
            <p className="mt-1 text-[11px] leading-5 text-[var(--muted)]">노드 → 스테이션 → 게이트 → 패키지 순서로 현재 공정 상태를 표시합니다.</p>
          </div>
          <Badge tone={active ? 'violet' : 'neutral'}>{active ? `${active}건 처리 중` : '현재 대기 중'}</Badge>
        </CardHeader>
        <CardContent className="overflow-x-auto pt-4">
          <div className="grid min-w-[920px] grid-cols-[minmax(185px,1fr)_46px_minmax(185px,1fr)_46px_minmax(185px,1fr)_46px_minmax(185px,1fr)] items-center gap-2">
            <FactoryStage number="N-01" kind="raw" icon={Database} title="입력 노드" value={`${inputCount || runs.length}건`} detail="고유 입력과 실행 요청" state={runs.length ? '원료 유입' : '입력 대기'} />
            <FactoryPipe flowing={active > 0} />
            <FactoryStage number="S-02" kind="ai" icon={Sparkles} title="AI 정제 스테이션" value={active ? `${active}건` : `${runs.length}건`} detail={latestRun?.model || '기본 모델'} state={active ? '처리 중' : '공정 대기'} active={active > 0} />
            <FactoryPipe flowing={active > 0} />
            <FactoryStage number="G-03" kind="gate" icon={ShieldCheck} title="정책 출시 게이트" value={policyEvaluated ? `${policyPassed}/${policyEvaluated}` : '미판정'} detail={policyBlocked ? `${policyBlocked}건 차단` : '차단된 정책 없음'} state={policyBlocked ? '검토 필요' : policyEvaluated ? '게이트 통과' : '판정 대기'} blocked={policyBlocked > 0} />
            <FactoryPipe flowing={active > 0 && policyBlocked === 0} />
            <FactoryStage number="P-04" kind="product" icon={PackageCheck} title="상품 패키지" value={`${successful}건`} detail={`${outputCount}개 산출물 참조`} state={successful ? '정제 완료' : '패키지 대기'} />
          </div>
        </CardContent>
      </Card>

      <Card className="overflow-hidden rounded-xl shadow-none">
        <CardHeader>
          <div>
            <h2 className="text-sm font-bold text-[var(--ink)]">최근 공정 실행</h2>
            <p className="mt-1 text-[11px] text-[var(--muted)]">모델, 프롬프트, 정책 판정과 실행 성능을 시간순으로 확인합니다.</p>
          </div>
          <Badge tone="info">{runs.length}건</Badge>
        </CardHeader>
        {runs.length ? (
          <div className="table-scroll">
            <table className="data-table">
              <thead><tr><th>실행</th><th>모델</th><th>프롬프트</th><th>응답 시간</th><th>비용</th><th>정책</th><th>상태</th><th>생성일</th></tr></thead>
              <tbody>
                {runs.map((run) => {
                  const decision = run.policy_decision.toLowerCase()
                  const policyTone = passedPolicyDecisions.has(decision) ? 'success' : blockedPolicyDecisions.has(decision) ? 'danger' : 'neutral'
                  return (
                    <tr key={run.id}>
                      <td><div className="min-w-44"><p className="font-bold text-[var(--ink)]">{runTypeLabel(run.run_type)}</p><p className="mt-1 font-mono text-[11px] text-[var(--muted)]">{run.id}</p></div></td>
                      <td>{run.model || '기본 모델'}</td>
                      <td>{run.prompt_version || '—'}</td>
                      <td>{formatNumber(run.latency_ms)}ms</td>
                      <td>{run.token_cost.toFixed(3)}</td>
                      <td><Badge tone={policyTone}>{policyDecisionLabel(run.policy_decision)}</Badge></td>
                      <td><StatusBadge status={run.status} /></td>
                      <td>{formatDate(run.created_at)}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : <EmptyState title="팩토리 실행이 없습니다" description="새 상품 아이디어나 정의서 생성 작업을 시작해 보세요." />}
      </Card>
    </div>
  )
}

function FactoryStage({ number, kind, icon: Icon, title, value, detail, state, active, blocked }: {
  number: string
  kind: 'raw' | 'ai' | 'gate' | 'product'
  icon: LucideIcon
  title: string
  value: string
  detail: string
  state: string
  active?: boolean
  blocked?: boolean
}) {
  const className = kind === 'gate' ? 'factory-station is-active border-2' : `factory-station is-${kind}${active ? ' is-active' : ''}`
  const gateStyle = kind === 'gate' ? {
    '--station-color': blocked ? 'var(--danger)' : 'var(--warning)',
    '--station-bg': blocked ? 'var(--danger-soft)' : 'var(--warning-soft)',
  } as CSSProperties : undefined
  return (
    <section className={className} style={gateStyle} aria-label={`${title}: ${state}`}>
      <div className="factory-station-topline"><span>{title}</span><span>{number}</span></div>
      <div className="factory-station-body">
        <span className="factory-station-icon"><Icon /></span>
        <div className="min-w-0"><strong className="block truncate">{value}</strong><p className="truncate">{detail}</p></div>
      </div>
      <div className="mt-4 border-t border-[color-mix(in_srgb,var(--station-color)_24%,transparent)] pt-2 text-[11px] font-bold text-[var(--station-color)]">{state}</div>
    </section>
  )
}

function FactoryPipe({ flowing }: { flowing: boolean }) {
  return <div className={`flow-pipe w-full ${flowing ? 'is-flowing' : ''}`} aria-hidden="true"><span className="flow-pipe-line" /><span className="flow-pipe-pulse" /></div>
}

export function ReviewPage() {
  const [params, setParams] = useSearchParams()
  const query = useQuery({ queryKey: ['dataworks', 'action-center'], queryFn: dataworksApi.actionCenter, staleTime: 15_000 })
  const [severity, setSeverity] = useState('all')
  if (query.isPending) return <PageLoader label="액션 센터를 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />

  const requested = params.get('type') ?? 'all'
  // Every summary counter becomes a filter button, so each key needs the action type it counts;
  // a missing entry filters on the counter name and shows an empty list instead.
  const typeMap: Record<string, string> = { approval_pending: 'approval_pending', blocked_launches: 'launch_blocked', expiring_access: 'entitlement_expiring', expiring_contracts: 'contract_expiring', inactive_access: 'entitlement_inactive', low_fit_scores: 'low_customer_fit', negative_margin: 'negative_margin', retirement_candidates: 'retirement_candidate', stale_watermarks: 'stale_watermark' }
  const type = typeMap[requested] ?? requested
  const actions = query.data.actions.filter((item) => (type === 'all' || item.type === type) && (severity === 'all' || item.severity === severity))

  return (
    <div>
      <PageHeader eyebrow="검토 · 액션 센터" title="검토 센터" description="출시 차단, 승인 대기, 계약 만료와 운영 경고를 영향도 순서로 처리합니다." />
      <div className="mb-5 flex flex-wrap gap-2">
        <FilterButton active={requested === 'all'} onClick={() => setParams({})}>전체 <span>{query.data.actions.length}</span></FilterButton>
        {Object.entries(query.data.summary).filter(([, count]) => count > 0).map(([key, count]) => (
          <FilterButton key={key} active={requested === key} onClick={() => setParams({ type: key })}>{actionTypeLabel(key)} <span>{count}</span></FilterButton>
        ))}
        <select className="field-input ml-auto h-9 w-36" value={severity} onChange={(event) => setSeverity(event.target.value)} aria-label="심각도 필터">
          <option value="all">모든 심각도</option><option value="high">높음</option><option value="medium">보통</option><option value="low">낮음</option>
        </select>
      </div>
      <Card className="rounded-xl shadow-none"><CardContent>{actions.length ? <div>{actions.map((action, index) => <ReviewRow key={`${action.type}-${action.product_key}-${index}`} action={action} />)}</div> : <EmptyState title="해당 액션이 없습니다" description="현재 필터 조건에 해당하는 운영 작업이 없습니다." />}</CardContent></Card>
    </div>
  )
}

type SupplyKind = 'asset' | 'product' | 'api' | 'customer'

interface SupplyGraphNode extends PortfolioGraphNode {
  supplyKind: SupplyKind
  productKey?: string
  sourceType?: string
}

interface SupplyEdge {
  from: string
  to: string
  relation: string
}

const supplyKindOrder: SupplyKind[] = ['asset', 'product', 'api', 'customer']
const supplyKindLabels: Record<SupplyKind, string> = { api: 'API 제공', asset: '데이터 자산', customer: '고객 성과', product: '데이터 상품' }

export function ProductGraphPage() {
  const query = useQuery({ queryKey: ['dataworks', 'portfolio-graph'], queryFn: dataworksApi.portfolioGraph, staleTime: 30_000 })
  const [filter, setFilter] = useState<'all' | SupplyKind>('all')

  const graph = useMemo(() => {
    const raw = query.data?.graph
    if (!raw) return { counts: { api: 0, asset: 0, customer: 0, product: 0 } as Record<SupplyKind, number>, edges: [] as Edge[], nodes: [] as Node[] }

    const originalNodes: SupplyGraphNode[] = raw.nodes.map((node) => ({ ...node, supplyKind: supplyKindFor(node.type), productKey: node.type === 'product' ? productKeyFromID(node.id) : undefined, sourceType: node.type }))
    const productNodes = originalNodes.filter((node) => node.supplyKind === 'product')
    const apiNodes: SupplyGraphNode[] = productNodes.map((product) => ({ id: apiNodeID(product.id), label: /api/i.test(product.label) ? `${product.label} 제공 채널` : `${product.label} API`, productKey: product.productKey, sourceType: 'api', status: product.status, supplyKind: 'api', type: 'api' }))
    const allNodes = [...originalNodes, ...apiNodes]
    const kindByID = new Map(allNodes.map((node) => [node.id, node.supplyKind]))
    const supplyEdges: SupplyEdge[] = raw.edges.map((edge) => {
      let from = edge.from
      let to = edge.to
      if (kindByID.get(from) === 'product' && kindByID.get(to) === 'customer') from = apiNodeID(from)
      if (kindByID.get(from) === 'customer' && kindByID.get(to) === 'product') to = apiNodeID(to)
      return { from, to, relation: edge.relation_type || 'connected' }
    })
    productNodes.forEach((product) => supplyEdges.push({ from: product.id, to: apiNodeID(product.id), relation: 'provides_api' }))

    const counts = allNodes.reduce<Record<SupplyKind, number>>((result, node) => ({ ...result, [node.supplyKind]: result[node.supplyKind] + 1 }), { api: 0, asset: 0, customer: 0, product: 0 })
    const visible = new Set(allNodes.filter((node) => filter === 'all' || node.supplyKind === filter).map((node) => node.id))
    if (filter !== 'all') supplyEdges.forEach((edge) => { if (visible.has(edge.from)) visible.add(edge.to); if (visible.has(edge.to)) visible.add(edge.from) })
    const selected = allNodes.filter((node) => filter === 'all' || visible.has(node.id))
    const counters: Record<SupplyKind, number> = { api: 0, asset: 0, customer: 0, product: 0 }
    const nodes = selected.map((node): Node => {
      const column = supplyKindOrder.indexOf(node.supplyKind)
      const row = counters[node.supplyKind]
      counters[node.supplyKind] += 1
      return { id: node.id, position: { x: column * 300, y: row * 132 + (column % 2 ? 28 : 0) }, data: { label: <GraphNode node={node} /> }, style: { width: 220, padding: 0, border: 0, borderRadius: 12, background: 'transparent' } }
    })
    const nodeIDs = new Set(nodes.map((node) => node.id))
    const edges = supplyEdges.filter((edge) => nodeIDs.has(edge.from) && nodeIDs.has(edge.to)).map((edge, index): Edge => {
      const color = edgeColor(edge.relation)
      return { id: `${edge.from}-${edge.to}-${index}`, source: edge.from, target: edge.to, label: relationLabel(edge.relation), type: 'smoothstep', animated: edge.relation === 'provides_api', markerEnd: { type: MarkerType.ArrowClosed, color }, style: { stroke: color, strokeWidth: edge.relation === 'provides_api' ? 2 : 1.4 }, labelStyle: { fill: 'var(--muted)', fontSize: 11, fontWeight: 700 }, labelBgStyle: { fill: 'var(--surface-raised)', fillOpacity: 0.92 }, labelBgPadding: [5, 3], labelBgBorderRadius: 6 }
    })
    return { counts, edges, nodes }
  }, [query.data, filter])

  if (query.isPending) return <PageLoader label="데이터 상품 공급망 지도를 구성하는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />
  const totalNodeCount = supplyKindOrder.reduce((sum, kind) => sum + graph.counts[kind], 0)

  return (
    <div>
      <PageHeader eyebrow="포트폴리오 · 데이터 계보" title="공급망 지도" description="데이터 자산이 상품으로 정제되고 API 채널을 거쳐 고객 성과로 이어지는 관계와 영향 범위를 탐색합니다." actions={<Button variant="secondary" onClick={() => void query.refetch()} disabled={query.isFetching}><RefreshCw className={query.isFetching ? 'size-4 animate-spin' : 'size-4'} /> 지도 새로고침</Button>} />
      <Card className="overflow-hidden rounded-xl shadow-none">
        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--line)] p-3">
          <Blocks className="mr-1 size-4 text-[var(--accent)]" />
          <FilterButton active={filter === 'all'} onClick={() => setFilter('all')}>전체 노드 <span>{totalNodeCount}</span></FilterButton>
          {supplyKindOrder.map((kind) => <FilterButton key={kind} active={filter === kind} onClick={() => setFilter(kind)}>{supplyKindLabels[kind]} <span>{graph.counts[kind]}</span></FilterButton>)}
        </div>
        <div className="grid grid-cols-2 border-b border-[var(--line)] bg-[var(--surface-muted)] px-4 py-3 md:grid-cols-4">
          {supplyKindOrder.map((kind, index) => <div key={kind} className="flex items-center gap-2 text-[11px] font-bold text-[var(--muted)]"><span className="size-2 rounded-full" style={{ background: supplyKindColor(kind) }} /><span>0{index + 1} · {supplyKindLabels[kind]}</span></div>)}
        </div>
        <div className="h-[680px] bg-[var(--surface-raised)]">
          {graph.nodes.length ? <ReactFlow nodes={graph.nodes} edges={graph.edges} fitView minZoom={0.25} maxZoom={1.6}><Background color="var(--line-strong)" gap={24} size={1} /><Controls /></ReactFlow> : <EmptyState title="그래프 데이터가 없습니다" description="자산과 상품 관계가 등록되면 여기에 표시됩니다." />}
        </div>
      </Card>
    </div>
  )
}

export function MarketplacePage() {
  const query = useQuery({ queryKey: ['dataworks', 'products'], queryFn: dataworksApi.products, staleTime: 30_000 })
  if (query.isPending) return <PageLoader label="마켓플레이스를 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />
  const products = query.data.products.filter((item) => item.status === 'published')

  return (
    <div>
      <PageHeader eyebrow="탐색 · 이용" title="데이터 마켓플레이스" description="승인된 데이터 상품을 탐색하고 API 접근 또는 PoC를 요청합니다." />
      <div className="mb-6 rounded-xl bg-[var(--ink)] p-6 text-[var(--surface)] md:p-8"><div className="flex flex-col justify-between gap-5 md:flex-row md:items-center"><div><p className="text-[11px] font-bold tracking-[.14em] opacity-60">엄선된 상품 포트폴리오</p><h2 className="mt-3 text-2xl font-[750] tracking-[-.045em]">신뢰할 수 있는 데이터를 바로 활용하세요.</h2><p className="mt-2 max-w-xl text-xs leading-6 opacity-70">거버넌스 검증을 통과한 상품의 계약, SLA, 문서와 샘플을 한곳에서 확인하세요.</p></div><ShoppingBag className="size-12 opacity-25" /></div></div>
      {products.length ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {products.map((product) => (
            <Link key={product.product_key} to={`/products/${encodeURIComponent(product.product_key)}`} className="no-underline">
              <Card className="product-card rounded-xl shadow-none hover:shadow-none" style={{ borderRadius: 12, boxShadow: 'none' }}>
                <div className="flex justify-between"><span className="grid size-10 place-items-center rounded-xl bg-[var(--success-soft)] text-[var(--success)]"><ShoppingBag className="size-4" /></span><Badge tone="success">이용 가능</Badge></div>
                <p className="mt-5 font-mono text-[11px] font-bold tracking-wider text-[var(--muted-soft)]">{product.product_key}</p>
                <h2 className="mt-2 text-base font-bold text-[var(--ink)]">{product.name_ko || product.name_en}</h2>
                <p className="mt-2 line-clamp-2 text-[11px] leading-5 text-[var(--muted)]">{product.sales_pitch || product.description}</p>
                <div className="mt-auto flex items-center justify-between pt-5"><div className="flex gap-2"><Badge>{sourceTypeLabel(product.source_type)}</Badge><Badge tone="info">{pricingModelLabel(product.pricing_model)}</Badge></div><ArrowRight className="size-4 text-[var(--muted-soft)]" /></div>
              </Card>
            </Link>
          ))}
        </div>
      ) : <Card className="rounded-xl shadow-none"><EmptyState title="출시된 상품이 없습니다" description="출시 게이트를 통과한 상품이 마켓플레이스에 표시됩니다." /></Card>}
    </div>
  )
}

export function AnalyticsPage() {
  const [home, products] = useQueries({ queries: [{ queryKey: ['dataworks', 'home'], queryFn: dataworksApi.home }, { queryKey: ['dataworks', 'products'], queryFn: dataworksApi.products }] })
  if (home.isPending || products.isPending) return <PageLoader label="성과 분석을 준비하는 중" />
  const error = home.error || products.error
  if (error) return <ErrorState error={error} retry={() => { void home.refetch(); void products.refetch() }} />
  const statusData = ['draft', 'review', 'risk_review', 'approved', 'published', 'archived'].map((status) => ({ status: statusLabel(status), count: products.data!.products.filter((item) => item.status === status).length }))

  return (
    <div>
      <PageHeader eyebrow="성과 · 포트폴리오" title="성과 분석" description="상품 생명주기, 수익 잠재력과 위험 분포를 포트폴리오 관점에서 봅니다." />
      <div className="grid gap-5 xl:grid-cols-[.9fr_1.1fr]">
        <Card className="rounded-xl shadow-none"><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">생명주기 분포</h2><Badge tone="info">상품 {products.data!.products.length}개</Badge></CardHeader><CardContent className="h-[340px]"><ResponsiveContainer width="100%" height="100%"><BarChart data={statusData} margin={{ left: -24, right: 8, top: 15 }}><CartesianGrid stroke="var(--line)" vertical={false} strokeDasharray="3 5" /><XAxis dataKey="status" tick={{ fill: 'var(--muted)', fontSize: 11 }} tickLine={false} axisLine={false} /><YAxis tick={{ fill: 'var(--muted-soft)', fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} /><Tooltip contentStyle={{ background: 'var(--surface)', border: '1px solid var(--line)', borderRadius: 10, fontSize: 12 }} /><Bar dataKey="count" name="상품 수" radius={[6, 6, 0, 0]}>{statusData.map((entry, index) => <Cell key={entry.status} fill={index === 4 ? 'var(--success)' : index === 2 ? 'var(--danger)' : 'var(--accent)'} fillOpacity={index === 4 || index === 2 ? 1 : .62} />)}</Bar></BarChart></ResponsiveContainer></CardContent></Card>
        <Card className="rounded-xl shadow-none"><CardHeader><h2 className="text-sm font-bold text-[var(--ink)]">상위 상품 지표</h2></CardHeader><CardContent className="pt-2">{home.data!.top_products.map((product) => <div key={product.product_key} className="flex items-center gap-4 border-b border-[var(--line)] py-3 last:border-0"><span className="grid size-9 place-items-center rounded-xl bg-[var(--info-soft)] text-[var(--accent)]"><Boxes className="size-4" /></span><div className="min-w-0 flex-1"><p className="truncate text-xs font-bold text-[var(--ink)]">{product.name || product.product_key}</p><p className="mt-1 font-mono text-[11px] text-[var(--muted)]">{product.product_key}</p></div><div className="text-right"><p className="text-[11px] text-[var(--muted)]">수익</p><p className="text-xs font-bold text-[var(--success)]">{product.revenue_score}</p></div><div className="text-right"><p className="text-[11px] text-[var(--muted)]">위험</p><p className="text-xs font-bold text-[var(--danger)]">{product.risk_score}</p></div></div>)}</CardContent></Card>
      </div>
    </div>
  )
}

export function GovernancePage() {
  const query = useQuery({ queryKey: ['dataworks', 'action-center'], queryFn: dataworksApi.actionCenter, staleTime: 30_000 })
  if (query.isPending) return <PageLoader label="거버넌스 상태를 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />
  const modules = [
    { icon: ShieldCheck, title: '승인 체계', text: '데이터 오너, 법무, 준법 승인과 만료 관리' },
    { icon: Landmark, title: '계약 범위', text: '허용 필드, 목적, 기간과 호출 한도 정책' },
    { icon: KeyRound, title: '사용 권한', text: 'API 키와 고객 계약 범위 연결' },
    { icon: FileClock, title: '감사 이력', text: '상품 변경과 의사결정 증적의 시간순 기록' },
    { icon: Code2, title: 'API 계약', text: 'OpenAPI 3.1 기반 제공 계약과 운영 상태' },
    { icon: AlertTriangle, title: '정책 영향', text: '민감도·품질·스키마 변경의 영향 분석' },
  ]
  return (
    <div>
      <PageHeader eyebrow="통제 · 신뢰" title="거버넌스" description="승인, 계약, 사용 권한과 감사 증적을 상품 생명주기에 맞춰 관리합니다." />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4"><GovernanceCard title="승인" value={query.data.summary.approval_pending} description="검토 대기 상품" icon={FileClock} tone="warning" /><GovernanceCard title="계약" value={query.data.summary.expiring_contracts} description="30일 내 만료" icon={Landmark} tone="violet" /><GovernanceCard title="사용 권한" value={query.data.summary.inactive_access} description="비활성 또는 만료" icon={KeyRound} tone="danger" /><GovernanceCard title="출시 통제" value={query.data.summary.blocked_launches} description="출시 게이트 차단 상품" icon={ShieldCheck} tone="info" /></div>
      <Card className="mt-5 rounded-xl shadow-none"><CardHeader><div><h2 className="text-sm font-bold text-[var(--ink)]">거버넌스 모듈</h2><p className="mt-1 text-[11px] text-[var(--muted)]">기존 API를 상품 작업공간 중심으로 재구성했습니다.</p></div></CardHeader><CardContent><div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">{modules.map((item) => <div key={item.title} className="rounded-xl border border-[var(--line)] p-4"><item.icon className="size-4 text-[var(--accent)]" /><p className="mt-4 text-xs font-bold text-[var(--ink)]">{item.title}</p><p className="mt-1 text-[11px] leading-5 text-[var(--muted)]">{item.text}</p></div>)}</div></CardContent></Card>
    </div>
  )
}

export function SettingsPage() {
  return (
    <div>
      <PageHeader eyebrow="관리" title="설정" description="React 전환 기간 동안 플랫폼 전역 설정은 기존 관리자 콘솔에서 계속 관리합니다." />
      <div className="grid gap-5 md:grid-cols-2">
        <Card className="rounded-xl shadow-none"><CardContent className="flex min-h-56 flex-col"><span className="grid size-10 place-items-center rounded-xl bg-[var(--info-soft)] text-[var(--accent)]"><Settings className="size-4" /></span><h2 className="mt-5 text-base font-bold text-[var(--ink)]">기존 관리자 콘솔</h2><p className="mt-2 text-xs leading-6 text-[var(--muted)]">인증, 공급자, 모델, 저장소 및 기존 운영 기능은 마이그레이션 중에도 그대로 사용할 수 있습니다.</p><Button className="mt-auto self-start" variant="secondary" asChild><a href="/admin">관리자 콘솔 열기 <ArrowRight className="size-4" /></a></Button></CardContent></Card>
        <Card className="rounded-xl shadow-none"><CardContent className="flex min-h-56 flex-col"><span className="grid size-10 place-items-center rounded-xl bg-[var(--violet-soft)] text-[var(--violet)]"><Bot className="size-4" /></span><h2 className="mt-5 text-base font-bold text-[var(--ink)]">사용 환경 설정</h2><p className="mt-2 text-xs leading-6 text-[var(--muted)]">테마와 브라우저 세션 토큰은 현재 브라우저 세션 범위에서 저장됩니다.</p><div className="mt-auto flex gap-2 pt-5"><Badge tone="success">현재 세션에만 적용</Badge><Badge tone="neutral">서버 변경 없음</Badge></div></CardContent></Card>
      </div>
    </div>
  )
}

function MiniMetric({ label, value, icon: Icon, tone }: { label: string; value: string | number; icon: LucideIcon; tone: 'violet' | 'success' | 'info' | 'warning' }) {
  return <Card className="flex items-center gap-3 rounded-xl p-4 shadow-none"><span className="grid size-9 place-items-center rounded-xl" style={{ background: `var(--${tone}-soft)`, color: `var(--${tone})` }}><Icon className="size-4" /></span><div><p className="text-[11px] font-bold tracking-wider text-[var(--muted)]">{label}</p><p className="mt-1 text-xl font-[750] tracking-[-.04em] text-[var(--ink)]">{value}</p></div></Card>
}

function FilterButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return <button type="button" className={`inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-[11px] font-bold ${active ? 'border-[var(--ink)] bg-[var(--ink)] text-[var(--surface)]' : 'border-[var(--line)] bg-[var(--surface)] text-[var(--muted)] hover:border-[var(--line-strong)]'}`} onClick={onClick}>{children}</button>
}

function ReviewRow({ action }: { action: ActionItem }) {
  const details = action.next_action ? actionMessageLabel(action.next_action) : action.blocked_reasons?.map((reason) => gateMessageLabel(reason)).join(' · ') || action.reason || ''
  return (
    <Link to={action.product_key ? `/products/${encodeURIComponent(action.product_key)}` : '/review'} className="action-row no-underline">
      <span className={`grid size-8 place-items-center rounded-xl ${action.severity === 'high' ? 'bg-[var(--danger-soft)] text-[var(--danger)]' : 'bg-[var(--warning-soft)] text-[var(--warning)]'}`}><AlertTriangle className="size-3.5" /></span>
      <span className="min-w-0"><span className="flex flex-wrap items-center gap-2"><strong className="truncate text-xs text-[var(--ink)]">{action.title || action.product_key || actionTypeLabel(action.type)}</strong><Badge tone={action.severity === 'high' ? 'danger' : 'warning'}>{severityLabel(action.severity)}</Badge></span><span className="mt-1 block text-[11px] text-[var(--muted)]">{actionTypeLabel(action.type)}</span>{details ? <span className="mt-2 line-clamp-2 block text-[11px] leading-5 text-[var(--muted)]">{details}</span> : null}</span>
      <ArrowRight className="mt-2 size-4 text-[var(--muted-soft)]" />
    </Link>
  )
}

function GraphNode({ node }: { node: SupplyGraphNode }) {
  const config: Record<SupplyKind, { icon: LucideIcon; detail: string }> = {
    api: { icon: Code2, detail: 'API 제공 채널' },
    asset: { icon: Database, detail: '원천 데이터 자산' },
    customer: { icon: Users, detail: node.sourceType === 'poc_outcome' ? 'PoC 고객 성과' : '고객 제안 피드백' },
    product: { icon: PackageCheck, detail: '정제된 데이터 상품' },
  }
  const item = config[node.supplyKind]
  const Icon = item.icon
  const link = node.supplyKind === 'asset' ? `/assets?asset=${encodeURIComponent(node.id.replace(/^asset:/, ''))}` : node.productKey ? `/products/${encodeURIComponent(node.productKey)}${node.supplyKind === 'api' ? '/api' : ''}` : ''
  const label = node.supplyKind === 'customer' ? customerOutcomeLabel(node.label) : node.label
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-3 text-left">
      <div className="flex items-center gap-3"><span className="grid size-8 place-items-center rounded-lg" style={{ background: `color-mix(in srgb, ${supplyKindColor(node.supplyKind)} 12%, var(--surface))`, color: supplyKindColor(node.supplyKind) }}><Icon className="size-3.5" /></span><div className="min-w-0"><p className="truncate text-xs font-bold text-[var(--ink)]">{label}</p><p className="mt-0.5 text-[11px] font-semibold text-[var(--muted)]">{item.detail}</p></div></div>
      <div className="mt-3 flex items-center justify-between gap-2 border-t border-[var(--line)] pt-2">{node.status ? <StatusBadge status={node.status} /> : node.success !== undefined ? <Badge tone={node.success ? 'success' : 'warning'}>{node.success ? '성과 확인' : '후속 검토'}</Badge> : <Badge>{supplyKindLabels[node.supplyKind]}</Badge>}{link ? <Link className="nodrag text-[11px] font-bold text-[var(--accent)] no-underline" to={link}>{node.supplyKind === 'asset' ? '자산 열기' : node.supplyKind === 'api' ? 'API 열기' : '상품 열기'}</Link> : null}</div>
    </div>
  )
}

function GovernanceCard({ title, value, description, icon: Icon, tone }: { title: string; value: number; description: string; icon: LucideIcon; tone: 'warning' | 'violet' | 'danger' | 'info' }) {
  return <Card className="rounded-xl shadow-none"><CardContent><span className="grid size-10 place-items-center rounded-xl" style={{ background: `var(--${tone}-soft)`, color: `var(--${tone})` }}><Icon className="size-4" /></span><p className="mt-5 text-[11px] font-bold tracking-wider text-[var(--muted)]">{title}</p><p className="mt-2 text-3xl font-[750] tracking-[-.05em] text-[var(--ink)]">{value}</p><p className="mt-2 text-[11px] text-[var(--muted)]">{description}</p></CardContent></Card>
}

function supplyKindFor(type: string): SupplyKind {
  if (type === 'asset') return 'asset'
  if (type === 'product') return 'product'
  if (type === 'api') return 'api'
  return 'customer'
}

function supplyKindColor(kind: SupplyKind) {
  return ({ asset: 'var(--data)', product: 'var(--product)', api: 'var(--ai)', customer: 'var(--success)' } as Record<SupplyKind, string>)[kind]
}

function apiNodeID(productNodeID: string) { return `api:${productNodeID}` }
function productKeyFromID(id: string) { return id.replace(/^product:/, '') }
function relationLabel(relation: string) { if (relation === 'provides_api') return 'API 제공'; if (relation === 'connected') return '연결'; return graphRelationLabel(relation) }
function edgeColor(relation: string) { if (relation === 'provides_api') return 'var(--ai)'; if (relation === 'feeds' || relation === 'uses_asset') return 'var(--data)'; return 'var(--success)' }
function sourceTypeLabel(value: string) { return ({ api: 'API', dataset: '데이터셋', query: '쿼리', report: '보고서', table: '테이블', view: '뷰' } as Record<string, string>)[value.toLowerCase()] ?? '기타' }
function pricingModelLabel(value: string) { if (!value) return '가격 문의'; return ({ contact: '가격 문의', fixed: '정액제', subscription: '구독형', tier: '등급형', usage: '사용량 기반' } as Record<string, string>)[value.toLowerCase()] ?? '별도 협의' }
function customerOutcomeLabel(value: string) { return ({ accepted: '제안 수락', contract_candidate: '계약 후보', lost: '미성사', poc: 'PoC 진행', recorded: '결과 기록', rejected: '제안 거절', won: '계약 성사' } as Record<string, string>)[value.toLowerCase()] ?? value }
