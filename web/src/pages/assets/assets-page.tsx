import { useMutation, useQueries, useQueryClient } from '@tanstack/react-query'
import { createColumnHelper, tableFeatures, useTable } from '@tanstack/react-table'
import { ArrowUpDown, Database, Filter, LoaderCircle, Pencil, Plus, Search, Shield, Sparkles, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { CSSProperties, FormEvent, ReactNode } from 'react'
import { useSearchParams } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { ConfirmDialog, Dialog } from '@/components/ui/dialog'
import { PageHeader } from '@/components/ui/page-header'
import { EmptyState, ErrorState, PageLoader } from '@/components/ui/query-state'
import { refreshCycleLabel, sensitivityLabel } from '@/lib/labels.ko'
import { cn, formatDate } from '@/lib/utils'
import { canWriteDataWorks, useAuthStore } from '@/stores/auth-store'
import type { AssetReadiness, DataAsset, DataAssetInput } from '@/types/dataworks'

interface AssetRow extends DataAsset {
  readiness?: AssetReadiness
}

const features = tableFeatures({})
const columnHelper = createColumnHelper<typeof features, AssetRow>()

export function AssetsPage() {
  const queryClient = useQueryClient()
  const mode = useAuthStore((state) => state.mode)
  const user = useAuthStore((state) => state.user)
  const canWrite = canWriteDataWorks(mode, user)
  const [params] = useSearchParams()
  const [search, setSearch] = useState(params.get('asset') ?? '')
  const [domain, setDomain] = useState('all')
  const [sensitivity, setSensitivity] = useState('all')
  const [editing, setEditing] = useState<DataAsset | 'new' | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DataAsset | null>(null)
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

  const saveAsset = useMutation({
    mutationFn: dataworksApi.saveAsset,
    onSuccess: async () => {
      setEditing(null)
      await queryClient.invalidateQueries({ queryKey: ['dataworks', 'assets'] })
    },
  })
  const deleteAsset = useMutation({
    mutationFn: (assetKey: string) => dataworksApi.deleteAsset(assetKey),
    onSuccess: async () => {
      setDeleteTarget(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['dataworks', 'assets'] }),
        queryClient.invalidateQueries({ queryKey: ['dataworks', 'readiness'] }),
      ])
    },
  })
  const checkReadiness = useMutation({
    mutationFn: async (assetKeys: string[]) => Promise.all(assetKeys.map((assetKey) => dataworksApi.checkAssetReadiness(assetKey))),
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey: ['dataworks', 'readiness'] })
    },
  })

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
    ...(canWrite ? [columnHelper.accessor((row) => row.asset_key, {
      id: 'actions',
      header: '관리',
      cell: ({ row }) => <div className="flex items-center justify-end gap-1"><Button aria-label={`${row.original.name || row.original.asset_key} 준비도 점검`} disabled={checkReadiness.isPending} onClick={() => checkReadiness.mutate([row.original.asset_key])} size="icon" title="준비도 점검" variant="ghost"><Sparkles className="size-3.5" /></Button><Button aria-label={`${row.original.name || row.original.asset_key} 수정`} onClick={() => { saveAsset.reset(); setEditing(row.original) }} size="icon" title="수정" variant="ghost"><Pencil className="size-3.5" /></Button><Button aria-label={`${row.original.name || row.original.asset_key} 삭제`} onClick={() => { deleteAsset.reset(); setDeleteTarget(row.original) }} size="icon" title="삭제" variant="ghost"><Trash2 className="size-3.5 text-[var(--danger)]" /></Button></div>,
    })] : []),
  ]), [canWrite, checkReadiness, deleteAsset, saveAsset])

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
        actions={canWrite ? <><Button disabled={checkReadiness.isPending || !(assets.data?.assets.length)} onClick={() => checkReadiness.mutate((assets.data?.assets ?? []).map((asset) => asset.asset_key))} variant="secondary">{checkReadiness.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <Sparkles className="size-4" />} 전체 준비도 점검</Button><Button onClick={() => { saveAsset.reset(); setEditing('new') }} variant="accent"><Plus className="size-4" /> 자산 등록</Button></> : undefined}
      />

      {checkReadiness.error ? <p className="field-error mb-4 rounded-xl bg-[var(--danger-soft)] p-3" role="alert">준비도 점검에 실패했습니다. {checkReadiness.error.message}</p> : null}

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

      <AssetEditor
        asset={editing === 'new' ? undefined : editing ?? undefined}
        error={saveAsset.error}
        key={editing === 'new' ? 'new' : editing?.id ?? 'closed'}
        onClose={() => setEditing(null)}
        onSave={(payload) => saveAsset.mutate(payload)}
        open={editing !== null}
        pending={saveAsset.isPending}
      />
      <ConfirmDialog
        confirmLabel="자산 삭제"
        description={`‘${deleteTarget?.name || deleteTarget?.asset_key || ''}’ 자산과 준비도·품질·드리프트 운영 정보를 삭제합니다. 상품 원천으로 사용 중이면 삭제가 차단됩니다.`}
        error={deleteAsset.error}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteAsset.mutate(deleteTarget.asset_key)}
        open={Boolean(deleteTarget)}
        pending={deleteAsset.isPending}
        title="데이터 자산을 삭제할까요?"
      />
    </div>
  )
}

const emptyAsset: DataAssetInput = {
  asset_key: '', name: '', domain: '', owner: '', columns_summary: '', sensitivity: 'internal', refresh_cycle: 'daily',
}

function AssetEditor({ asset, open, pending, error, onClose, onSave }: {
  asset?: DataAsset
  open: boolean
  pending: boolean
  error: Error | null
  onClose: () => void
  onSave: (payload: DataAssetInput) => void
}) {
  const [form, setForm] = useState<DataAssetInput>(() => asset ? {
    id: asset.id,
    asset_key: asset.asset_key,
    name: asset.name,
    domain: asset.domain,
    owner: asset.owner,
    columns_summary: asset.columns_summary,
    sensitivity: asset.sensitivity || 'internal',
    refresh_cycle: asset.refresh_cycle || 'daily',
  } : emptyAsset)
  const submit = (event: FormEvent) => {
    event.preventDefault()
    onSave({ ...form, asset_key: form.asset_key.trim(), name: form.name.trim(), domain: form.domain.trim(), owner: form.owner.trim() })
  }
  return (
    <Dialog
      actions={<><Button disabled={pending} onClick={onClose} type="button" variant="secondary">취소</Button><Button disabled={pending || !form.asset_key.trim() || !form.name.trim()} form="asset-editor-form" type="submit" variant="accent">{pending ? <LoaderCircle className="size-4 animate-spin" /> : null}{pending ? '저장 중…' : asset ? '변경 저장' : '자산 등록'}</Button></>}
      busy={pending}
      className="!w-[min(100%,720px)]"
      description="상품 원천으로 재사용할 데이터의 식별자, 소유권과 분류 정보를 관리합니다."
      onClose={onClose}
      open={open}
      title={asset ? '데이터 자산 수정' : '새 데이터 자산 등록'}
    >
      <form className="grid gap-4 sm:grid-cols-2" id="asset-editor-form" onSubmit={submit}>
        <Field label="자산 키"><input autoFocus={!asset} className="field-input" disabled={Boolean(asset)} onChange={(event) => setForm({ ...form, asset_key: event.target.value })} placeholder="finance.transactions" required value={form.asset_key} /></Field>
        <Field label="자산 이름"><input autoFocus={Boolean(asset)} className="field-input" onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="금융 거래 원장" required value={form.name} /></Field>
        <Field label="도메인"><input className="field-input" onChange={(event) => setForm({ ...form, domain: event.target.value })} placeholder="finance" value={form.domain} /></Field>
        <Field label="담당자"><input className="field-input" onChange={(event) => setForm({ ...form, owner: event.target.value })} placeholder="data-platform" value={form.owner} /></Field>
        <Field label="민감도"><select className="field-input" onChange={(event) => setForm({ ...form, sensitivity: event.target.value })} value={form.sensitivity}><option value="public">공개</option><option value="internal">내부</option><option value="restricted">제한</option><option value="personal_credit">개인신용정보</option><option value="pseudonymized">가명처리</option><option value="aggregated">집계</option></select></Field>
        <Field label="갱신 주기"><select className="field-input" onChange={(event) => setForm({ ...form, refresh_cycle: event.target.value })} value={form.refresh_cycle}><option value="realtime">실시간</option><option value="hourly">매시간</option><option value="daily">매일</option><option value="weekly">매주</option><option value="monthly">매월</option><option value="manual">수동</option></select></Field>
        <Field className="sm:col-span-2" label="컬럼 요약"><textarea className="field-input h-24 py-3" onChange={(event) => setForm({ ...form, columns_summary: event.target.value })} placeholder="transaction_id, customer_id, amount, occurred_at" value={form.columns_summary} /></Field>
        {error ? <p className="field-error sm:col-span-2" role="alert">{error.message}</p> : null}
      </form>
    </Dialog>
  )
}

function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return <label className={className}><span className="field-label">{label}</span>{children}</label>
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
