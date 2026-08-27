import { useQueries } from '@tanstack/react-query'
import { createColumnHelper, tableFeatures, useTable } from '@tanstack/react-table'
import { ArrowUpDown, Database, Filter, Search, Shield, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { useSearchParams } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/page-header'
import { EmptyState, ErrorState, PageLoader } from '@/components/ui/query-state'
import { refreshCycleLabel, sensitivityLabel } from '@/lib/labels.ko'
import { cn, formatDate } from '@/lib/utils'
import type { AssetReadiness, DataAsset } from '@/types/dataworks'

interface AssetRow extends DataAsset {
  readiness?: AssetReadiness
}

const features = tableFeatures({})
const columnHelper = createColumnHelper<typeof features, AssetRow>()

export function AssetsPage() {
  const [params] = useSearchParams()
  const [search, setSearch] = useState(params.get('asset') ?? '')
  const [domain, setDomain] = useState('all')
  const [sensitivity, setSensitivity] = useState('all')
  const [assets, readiness] = useQueries({
    queries: [
      { queryKey: ['dataworks', 'assets'], queryFn: dataworksApi.assets, staleTime: 60_000 },
      { queryKey: ['dataworks', 'readiness'], queryFn: () => dataworksApi.readiness(), staleTime: 30_000 },
    ],
  })

  const readinessByKey = useMemo(
    () => new Map((readiness.data?.readiness ?? []).map((item) => [item.asset_key.toLowerCase(), item])),
    [readiness.data],
  )
  const rows = useMemo(() => {
    const query = search.trim().toLowerCase()
    return (assets.data?.assets ?? [])
      .map((asset) => ({ ...asset, readiness: readinessByKey.get(asset.asset_key.toLowerCase()) }))
      .filter((asset) => {
        const matchesSearch = !query || `${asset.asset_key} ${asset.name} ${asset.domain} ${asset.owner}`.toLowerCase().includes(query)
        return matchesSearch && (domain === 'all' || asset.domain === domain) && (sensitivity === 'all' || asset.sensitivity === sensitivity)
      })
  }, [assets.data, readinessByKey, search, domain, sensitivity])

  const domains = useMemo(() => Array.from(new Set((assets.data?.assets ?? []).map((item) => item.domain).filter(Boolean))).sort(), [assets.data])
  const sensitivities = useMemo(() => Array.from(new Set((assets.data?.assets ?? []).map((item) => item.sensitivity).filter(Boolean))).sort(), [assets.data])

  const columns = useMemo(() => columnHelper.columns([
    columnHelper.accessor('name', {
      header: '자산',
      cell: ({ row }) => <div className="flex min-w-[220px] items-center gap-3"><span className="grid size-9 shrink-0 place-items-center rounded-xl bg-[var(--info-soft)] text-[var(--accent)]"><Database className="size-4" /></span><div className="min-w-0"><p className="truncate font-bold text-[var(--ink)]">{row.original.name || row.original.asset_key}</p><p className="mt-0.5 truncate font-mono text-[11px] text-[var(--muted)]">{row.original.asset_key}</p></div></div>,
    }),
    columnHelper.accessor('domain', { header: '도메인', cell: (info) => <Badge tone="neutral">{info.getValue() || '미지정'}</Badge> }),
    columnHelper.accessor('owner', { header: '담당자', cell: (info) => <span className={cn('font-medium', !info.getValue() && 'text-[var(--danger)]')}>{info.getValue() || '담당자 지정 필요'}</span> }),
    columnHelper.accessor('sensitivity', { header: '민감도', cell: (info) => <SensitivityBadge value={info.getValue()} /> }),
    columnHelper.accessor('refresh_cycle', { header: '갱신 주기', cell: (info) => refreshCycleLabel(info.getValue()) }),
    columnHelper.accessor((row) => row.readiness?.overall_score ?? -1, {
      id: 'readiness',
      header: '준비도',
      cell: ({ row }) => row.original.readiness ? <ScoreRing value={row.original.readiness.overall_score} /> : <Badge tone="warning">미평가</Badge>,
    }),
    columnHelper.accessor('updated_at', { header: '수정일', cell: (info) => <span className="whitespace-nowrap text-[11px]">{formatDate(info.getValue())}</span> }),
  ]), [])

  const table = useTable({ data: rows, columns, features })

  if (assets.isPending || readiness.isPending) return <PageLoader label="자산 카탈로그를 불러오는 중" />
  const error = assets.error || readiness.error
  if (error) return <ErrorState error={error} retry={() => { void assets.refetch(); void readiness.refetch() }} />

  const scored = readiness.data?.readiness.length ?? 0
  const ready = readiness.data?.readiness.filter((item) => item.overall_score >= 70).length ?? 0
  return (
    <div>
      <PageHeader
        eyebrow="카탈로그 · 기준 정보"
        title="데이터 자산"
        description="상품으로 전환할 수 있는 데이터 자산의 소유권, 민감도, 최신성과 준비도를 관리합니다."
        actions={<Button variant="accent"><Sparkles className="size-4" /> 준비도 점검 실행</Button>}
      />

      <div className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <AssetStat label="전체 자산" value={assets.data?.assets.length ?? 0} />
        <AssetStat label="평가 완료" value={scored} />
        <AssetStat label="준비 완료 ≥ 70" value={ready} tone="success" />
        <AssetStat label="개선 필요" value={Math.max(0, scored - ready)} tone="warning" />
      </div>

      <Card className="overflow-hidden">
        <div className="flex flex-col gap-3 border-b border-[var(--line)] p-4 lg:flex-row lg:items-center">
          <div className="relative min-w-0 flex-1">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--muted-soft)]" />
            <input className="field-input pl-10" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="자산 이름, 키, 도메인 또는 담당자 검색" aria-label="자산 검색" />
          </div>
          <div className="flex items-center gap-2 overflow-x-auto">
            <Filter className="size-4 shrink-0 text-[var(--muted)]" />
            <select className="field-input h-10 min-w-36" value={domain} onChange={(event) => setDomain(event.target.value)} aria-label="도메인 필터"><option value="all">모든 도메인</option>{domains.map((item) => <option key={item}>{item}</option>)}</select>
            <select className="field-input h-10 min-w-40" value={sensitivity} onChange={(event) => setSensitivity(event.target.value)} aria-label="민감도 필터"><option value="all">모든 민감도</option>{sensitivities.map((item) => <option key={item} value={item}>{sensitivityLabel(item)}</option>)}</select>
          </div>
        </div>
        {rows.length ? (
          <div className="table-scroll">
            <table className="data-table">
              <thead>{table.getHeaderGroups().map((headerGroup) => <tr key={headerGroup.id}>{headerGroup.headers.map((header) => <th key={header.id}><span className="inline-flex items-center gap-1">{header.isPlaceholder ? null : <table.FlexRender header={header} />}<ArrowUpDown className="size-2.5 opacity-40" /></span></th>)}</tr>)}</thead>
              <tbody>{table.getRowModel().rows.map((row) => <tr key={row.id}>{row.getAllCells().map((cell) => <td key={cell.id}><table.FlexRender cell={cell} /></td>)}</tr>)}</tbody>
            </table>
          </div>
        ) : <EmptyState title="조건에 맞는 자산이 없습니다" description="검색어나 필터를 바꾸거나 새로운 데이터 자산을 등록해 보세요." />}
      </Card>
    </div>
  )
}

function AssetStat({ label, value, tone = 'info' }: { label: string; value: number; tone?: 'info' | 'success' | 'warning' }) {
  return <Card className="flex items-center justify-between p-4"><div><p className="text-[11px] font-bold tracking-[.08em] text-[var(--muted)]">{label}</p><p className="mt-2 text-2xl font-[750] tracking-[-.05em] text-[var(--ink)]">{value}</p></div><span className="grid size-8 place-items-center rounded-xl" style={{ background: `var(--${tone}-soft)`, color: `var(--${tone})` }}>{tone === 'info' ? <Database className="size-3.5" /> : <Shield className="size-3.5" />}</span></Card>
}

function SensitivityBadge({ value }: { value: string }) {
  const normalized = value.trim().toLowerCase()
  const tone = ['restricted', 'personal_credit'].includes(normalized) ? 'danger' : normalized === 'public' ? 'success' : normalized === 'pseudonymized' ? 'warning' : 'neutral'
  return <Badge tone={tone}>{sensitivityLabel(value)}</Badge>
}

function ScoreRing({ value }: { value: number }) {
  const color = value >= 80 ? 'var(--success)' : value >= 70 ? 'var(--accent)' : value >= 50 ? 'var(--warning)' : 'var(--danger)'
  return <div className="score-ring" style={{ '--score': value, '--ring-color': color } as CSSProperties}><span>{value}</span></div>
}
