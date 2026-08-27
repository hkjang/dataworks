import { useQueries, useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle,
  ArrowRight,
  Boxes,
  CheckCircle2,
  CircleDollarSign,
  Clock3,
  Database,
  Factory,
  FileClock,
  RefreshCw,
  ShieldAlert,
  Sparkles,
  TrendingUp,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { CSSProperties } from 'react'

import { dataworksApi } from '@/api/dataworks'
import { ProductionFlow } from '@/components/refinery/production-flow'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { ErrorState, PageLoader } from '@/components/ui/query-state'
import { actionMessageLabel, actionTypeLabel, runTypeLabel } from '@/lib/labels.ko'
import { formatDate, formatNumber } from '@/lib/utils'
import type { ActionItem, ActionSummary } from '@/types/dataworks'

const attentionCards: Array<{
  key: keyof ActionSummary
  label: string
  description: string
  icon: typeof ShieldAlert
  tone: 'danger' | 'warning' | 'info' | 'violet'
}> = [
  { key: 'blocked_launches', label: '출시 차단', description: '출시 조건을 충족하지 못한 상품', icon: ShieldAlert, tone: 'danger' },
  { key: 'approval_pending', label: '승인 대기', description: '검토 또는 승인 처리가 필요한 상품', icon: FileClock, tone: 'warning' },
  { key: 'expiring_contracts', label: '계약 만료 예정', description: '30일 안에 만료되는 계약', icon: Clock3, tone: 'violet' },
  { key: 'stale_watermarks', label: '최신성 지연 자산', description: '최신성 기준을 벗어난 데이터', icon: Database, tone: 'info' },
  { key: 'negative_margin', label: '마진 경고', description: '예상 마진이 음수인 상품', icon: CircleDollarSign, tone: 'warning' },
]

export function HomePage() {
  const queryClient = useQueryClient()
  const [home, actions, products, runs, readiness] = useQueries({
    queries: [
      { queryKey: ['dataworks', 'home'], queryFn: dataworksApi.home, staleTime: 30_000 },
      { queryKey: ['dataworks', 'action-center'], queryFn: dataworksApi.actionCenter, staleTime: 30_000 },
      { queryKey: ['dataworks', 'products'], queryFn: dataworksApi.products, staleTime: 60_000 },
      { queryKey: ['dataworks', 'factory-runs'], queryFn: dataworksApi.factoryRuns, staleTime: 30_000 },
      { queryKey: ['dataworks', 'readiness'], queryFn: () => dataworksApi.readiness(), staleTime: 60_000 },
    ],
  })

  const isPending = home.isPending || actions.isPending || products.isPending
  const error = home.error || actions.error || products.error
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['dataworks'] })
  if (isPending) return <PageLoader label="Workbench를 구성하는 중" />
  if (error) return <ErrorState error={error} retry={refresh} />

  const dashboard = home.data!.dashboard
  const summary = actions.data!.summary
  const recentProducts = [...(products.data?.products ?? [])]
    .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    .slice(0, 5)
  const chartData = home.data!.top_products.map((item) => ({
    name: item.name || item.product_key,
    revenue: item.revenue_score,
    risk: item.risk_score,
  }))
  const latestRuns = runs.data?.runs.slice(0, 4) ?? []
  const readyAssets = readiness.data?.readiness?.filter((item) => item.overall_score >= 70).length ?? 0
  const buildingProducts = (products.data?.products ?? []).filter((item) =>
    ['draft', 'review', 'risk_review', 'approved'].includes(item.status),
  ).length

  return (
    <div>
      <PageHeader
        eyebrow="오늘의 운영 브리핑"
        title="팩토리 관제실"
        description="오늘 출시를 막고 있는 일부터 처리하고, 최근 상품화 흐름을 한눈에 확인하세요."
        actions={<Button variant="secondary" onClick={refresh}><RefreshCw className="size-4" /> 새로고침</Button>}
      />

      <ProductionFlow
        raw={dashboard.total_assets}
        ready={readyAssets}
        building={buildingProducts}
        live={dashboard.published_products}
      />

      <section className="mt-5 grid grid-cols-2 gap-3 lg:grid-cols-4" aria-label="핵심 지표">
        <MetricCard label="데이터 자산" value={dashboard.total_assets} delta={`상품 ${dashboard.total_products}개`} icon={Database} tone="info" />
        <MetricCard label="출시 상품" value={dashboard.published_products} delta={`검토 중 ${dashboard.review_pending}개`} icon={CheckCircle2} tone="success" />
        <MetricCard label="평균 수익 점수" value={dashboard.avg_revenue_score} suffix="/100" delta={`아이디어 ${dashboard.ideas_total}개`} icon={TrendingUp} tone="violet" />
        <MetricCard label="고위험" value={dashboard.high_risk} delta={`PoC 대기 ${dashboard.poc_pending}개`} icon={AlertTriangle} tone="danger" />
      </section>

      <section className="mt-7">
        <div className="mb-3 flex items-end justify-between gap-4">
          <div><h2 className="text-lg font-bold tracking-[-.035em] text-[var(--ink)]">확인이 필요한 작업</h2><p className="mt-1 text-xs text-[var(--muted)]">우선순위가 높은 운영 작업입니다.</p></div>
          <Link to="/review" className="flex items-center gap-1 text-xs font-bold text-[var(--accent)]">액션 센터 열기 <ArrowRight className="size-3.5" /></Link>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          {attentionCards.map((item) => (
            <Link key={item.key} to={`/review?type=${item.key}`} className="no-underline">
              <Card className="group h-full p-4 transition hover:-translate-y-0.5 hover:border-[var(--line-strong)] hover:shadow-[0_16px_36px_rgba(17,31,42,.07)]">
                <div className="flex items-start justify-between gap-3">
                  <span className="grid size-9 place-items-center rounded-xl" style={{ background: `var(--${item.tone}-soft)`, color: `var(--${item.tone})` }}><item.icon className="size-4" /></span>
                  <span className="text-2xl font-[750] tracking-[-.05em] text-[var(--ink)]">{summary[item.key]}</span>
                </div>
                <p className="mt-4 text-xs font-bold text-[var(--ink)]">{item.label}</p>
                <p className="mt-1 text-xs leading-5 text-[var(--muted)]">{item.description}</p>
              </Card>
            </Link>
          ))}
        </div>
      </section>

      <section className="mt-7 grid gap-5 xl:grid-cols-[1.35fr_.85fr]">
        <Card>
          <CardHeader>
            <div><h2 className="text-sm font-bold text-[var(--ink)]">포트폴리오 지표</h2><p className="mt-1 text-xs text-[var(--muted)]">최근 상품의 수익 잠재력과 위험</p></div>
            <Badge tone="info">실시간 카탈로그</Badge>
          </CardHeader>
          <CardContent className="h-[300px] pt-4">
            {chartData.length ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chartData} margin={{ left: -22, right: 8, top: 12, bottom: 0 }}>
                  <defs>
                    <linearGradient id="revenueGradient" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="var(--accent)" stopOpacity={0.28} /><stop offset="100%" stopColor="var(--accent)" stopOpacity={0.01} /></linearGradient>
                  </defs>
                  <CartesianGrid stroke="var(--line)" vertical={false} strokeDasharray="3 5" />
                  <XAxis dataKey="name" tick={{ fill: 'var(--muted)', fontSize: 11 }} tickLine={false} axisLine={false} interval="preserveStartEnd" />
                  <YAxis domain={[0, 100]} tick={{ fill: 'var(--muted-soft)', fontSize: 11 }} tickLine={false} axisLine={false} />
                  <Tooltip contentStyle={{ background: 'var(--surface)', border: '1px solid var(--line)', borderRadius: 12, fontSize: 11 }} />
                  <Area type="monotone" dataKey="revenue" stroke="var(--accent)" fill="url(#revenueGradient)" strokeWidth={2} name="수익" />
                  <Area type="monotone" dataKey="risk" stroke="var(--danger)" fill="transparent" strokeWidth={1.5} strokeDasharray="5 4" name="위험" />
                </AreaChart>
              </ResponsiveContainer>
            ) : <div className="grid h-full place-items-center text-xs text-[var(--muted)]">상품 데이터가 쌓이면 포트폴리오 추세가 표시됩니다.</div>}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><div><h2 className="text-sm font-bold text-[var(--ink)]">권장 조치</h2><p className="mt-1 text-xs text-[var(--muted)]">영향도와 위험 기준 상위 5개</p></div><Sparkles className="size-4 text-[var(--accent)]" /></CardHeader>
          <CardContent className="pt-3">
            {actions.data!.actions.slice(0, 5).map((action, index) => <ActionRow key={`${action.type}-${action.product_key}-${index}`} action={action} />)}
            {!actions.data!.actions.length ? <div className="py-12 text-center"><CheckCircle2 className="mx-auto size-6 text-[var(--success)]" /><p className="mt-2 text-xs font-bold text-[var(--ink)]">모두 정상</p><p className="mt-1 text-xs text-[var(--muted)]">현재 처리할 운영 작업이 없습니다.</p></div> : null}
          </CardContent>
        </Card>
      </section>

      <section className="mt-5 grid gap-5 xl:grid-cols-[1fr_1fr]">
        <Card>
          <CardHeader><div><h2 className="text-sm font-bold text-[var(--ink)]">최근 작업한 상품</h2><p className="mt-1 text-xs text-[var(--muted)]">상품 메타데이터 수정 시각 기준</p></div><Link to="/products" className="text-xs font-bold text-[var(--accent)]">모두 보기</Link></CardHeader>
          <CardContent className="pt-2">
            {recentProducts.map((product) => (
              <Link key={product.product_key} to={`/products/${encodeURIComponent(product.product_key)}`} className="flex items-center gap-3 border-b border-[var(--line)] py-3 no-underline last:border-0">
                <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-[var(--info-soft)] text-[var(--accent)]"><Boxes className="size-4" /></span>
                <span className="min-w-0 flex-1"><span className="block truncate text-xs font-bold text-[var(--ink)]">{product.name_ko || product.product_key}</span><span className="mt-0.5 block truncate text-xs text-[var(--muted)]">{product.product_key} · {formatDate(product.updated_at)}</span></span>
                <StatusBadge status={product.status} />
              </Link>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><div><h2 className="text-sm font-bold text-[var(--ink)]">공장 실행 현황</h2><p className="mt-1 text-xs text-[var(--muted)]">AI 상품화 실행의 최근 상태</p></div><Factory className="size-4 text-[var(--violet)]" /></CardHeader>
          <CardContent className="pt-2">
            {latestRuns.length ? latestRuns.map((run) => (
              <div key={run.id} className="flex items-center gap-3 border-b border-[var(--line)] py-3 last:border-0">
                <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-[var(--violet-soft)] text-[var(--violet)]"><Factory className="size-4" /></span>
                <div className="min-w-0 flex-1"><p className="truncate text-xs font-bold text-[var(--ink)]">{runTypeLabel(run.run_type)}</p><p className="mt-0.5 truncate text-xs text-[var(--muted)]">{run.model || '기본 모델'} · {formatNumber(run.latency_ms)}ms · {formatDate(run.created_at)}</p></div>
                <StatusBadge status={run.status} />
              </div>
            )) : <div className="py-12 text-center text-xs text-[var(--muted)]">최근 상품 공장 실행이 없습니다.</div>}
          </CardContent>
        </Card>
      </section>
    </div>
  )
}

function MetricCard({ label, value, suffix, delta, icon: Icon, tone }: { label: string; value: number; suffix?: string; delta: string; icon: typeof Database; tone: 'info' | 'success' | 'violet' | 'danger' }) {
  return (
    <Card className="metric-card" style={{ '--metric-glow': `var(--${tone}-soft)` } as CSSProperties}>
      <div className="relative z-10 flex items-start justify-between gap-3"><div><p className="text-xs font-bold tracking-[.08em] text-[var(--muted)]">{label}</p><p className="metric-value mt-3 text-3xl font-[750] text-[var(--ink)]">{formatNumber(value)}<span className="ml-1 text-xs font-semibold tracking-normal text-[var(--muted)]">{suffix}</span></p><p className="mt-3 text-xs text-[var(--muted)]">{delta}</p></div><span className="grid size-9 place-items-center rounded-xl" style={{ background: `var(--${tone}-soft)`, color: `var(--${tone})` }}><Icon className="size-4" /></span></div>
    </Card>
  )
}

function ActionRow({ action }: { action: ActionItem }) {
  return (
    <Link className="action-row no-underline" to={action.product_key ? `/products/${encodeURIComponent(action.product_key)}` : '/review'}>
      <span className={`grid size-8 place-items-center rounded-xl ${action.severity === 'high' ? 'bg-[var(--danger-soft)] text-[var(--danger)]' : 'bg-[var(--warning-soft)] text-[var(--warning)]'}`}><AlertTriangle className="size-3.5" /></span>
      <span className="min-w-0"><span className="block truncate text-xs font-bold text-[var(--ink)]">{action.product_key || actionTypeLabel(action.type)}</span><span className="mt-1 line-clamp-2 block text-xs leading-5 text-[var(--muted)]">{actionMessageLabel(action.next_action || action.blocked_reasons?.[0] || action.reason || action.title)}</span></span>
      <ArrowRight className="mt-2 size-3.5 text-[var(--muted-soft)]" />
    </Link>
  )
}
