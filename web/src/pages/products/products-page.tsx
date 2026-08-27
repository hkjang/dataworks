import { useQuery } from '@tanstack/react-query'
import { ArrowRight, Boxes, CircleDollarSign, Filter, Search, ShieldAlert } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { StatusBadge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { EmptyState, ErrorState, PageLoader } from '@/components/ui/query-state'
import { statusLabel } from '@/lib/labels.ko'

export function ProductsPage() {
  const query = useQuery({ queryKey: ['dataworks', 'products'], queryFn: dataworksApi.products, staleTime: 30_000 })
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('all')
  const products = useMemo(() => {
    const value = search.trim().toLowerCase()
    return (query.data?.products ?? []).filter((product) =>
      (!value || `${product.product_key} ${product.name_ko} ${product.name_en} ${product.owner}`.toLowerCase().includes(value)) &&
      (status === 'all' || product.status === status),
    )
  }, [query.data, search, status])

  if (query.isPending) return <PageLoader label="상품 포트폴리오를 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />
  return (
    <div>
      <PageHeader eyebrow="포트폴리오 · 생명주기" title="데이터 상품" description="아이디어부터 운영 단계까지 모든 데이터 상품을 하나의 생명주기로 관리합니다." />
      <div className="mb-5 flex flex-col gap-3 rounded-[18px] border border-[var(--line)] bg-[var(--surface)] p-3 sm:flex-row sm:items-center">
        <div className="relative flex-1"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--muted-soft)]" /><input className="field-input pl-10" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="상품 이름, 키 또는 담당자 검색" aria-label="상품 검색" /></div>
        <div className="flex items-center gap-2"><Filter className="size-4 text-[var(--muted)]" /><select className="field-input h-11 min-w-40" value={status} onChange={(event) => setStatus(event.target.value)} aria-label="상품 상태 필터"><option value="all">모든 상태</option>{['draft', 'review', 'risk_review', 'approved', 'published', 'archived'].map((item) => <option key={item} value={item}>{statusLabel(item)}</option>)}</select></div>
      </div>
      {products.length ? <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">{products.map((product) => (
        <Link key={product.product_key} to={`/products/${encodeURIComponent(product.product_key)}`} className="no-underline">
          <Card className="product-card">
            <div className="flex items-start justify-between gap-4"><span className="grid size-10 place-items-center rounded-2xl bg-[var(--info-soft)] text-[var(--accent)]"><Boxes className="size-[18px]" /></span><StatusBadge status={product.status} /></div>
            <div className="mt-5"><p className="font-mono text-[11px] font-bold uppercase tracking-[.11em] text-[var(--muted-soft)]">{product.product_key}</p><h2 className="mt-2 text-base font-bold tracking-[-.03em] text-[var(--ink)]">{product.name_ko || product.name_en || product.product_key}</h2><p className="mt-2 line-clamp-2 text-[11px] leading-5 text-[var(--muted)]">{product.description || product.executive_summary || '상품 설명이 아직 없습니다.'}</p></div>
            <div className="mt-auto flex items-end justify-between gap-4 pt-5"><div className="flex gap-2"><ScorePill icon={CircleDollarSign} label="수익" value={product.revenue_score} tone="success" /><ScorePill icon={ShieldAlert} label="위험" value={product.risk_score} tone={product.risk_score >= 70 ? 'danger' : 'warning'} /></div><ArrowRight className="size-4 text-[var(--muted-soft)]" /></div>
          </Card>
        </Link>
      ))}</div> : <Card><EmptyState title="상품이 없습니다" description="검색 조건을 바꾸거나 AI 팩토리에서 새로운 상품 후보를 생성해 보세요." /></Card>}
    </div>
  )
}

function ScorePill({ icon: Icon, label, value, tone }: { icon: typeof CircleDollarSign; label: string; value: number; tone: 'success' | 'warning' | 'danger' }) {
  return <div className="rounded-xl bg-[var(--surface-muted)] px-2.5 py-2"><p className="flex items-center gap-1 text-[11px] font-bold tracking-wider text-[var(--muted)]"><Icon className="size-2.5" />{label}</p><p className="mt-1 text-xs font-extrabold" style={{ color: `var(--${tone})` }}>{value}</p></div>
}
