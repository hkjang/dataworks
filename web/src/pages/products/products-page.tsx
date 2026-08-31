import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, Boxes, CircleDollarSign, Filter, LoaderCircle, Pencil, Plus, Search, ShieldAlert, Trash2 } from 'lucide-react'
import { useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { Link } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { ConfirmDialog, Dialog } from '@/components/ui/dialog'
import { PageHeader } from '@/components/ui/page-header'
import { EmptyState, ErrorState, PageLoader } from '@/components/ui/query-state'
import { statusLabel } from '@/lib/labels.ko'
import { canWriteDataWorks, useAuthStore } from '@/stores/auth-store'
import type { DataProduct, DataProductInput, ProductStatus } from '@/types/dataworks'

const productStatuses: ProductStatus[] = ['draft', 'review', 'risk_review', 'approved', 'published', 'archived']

export function ProductsPage() {
  const queryClient = useQueryClient()
  const mode = useAuthStore((state) => state.mode)
  const user = useAuthStore((state) => state.user)
  const canWrite = canWriteDataWorks(mode, user)
  const query = useQuery({ queryKey: ['dataworks', 'products'], queryFn: dataworksApi.products, staleTime: 30_000 })
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('all')
  const [editing, setEditing] = useState<DataProduct | 'new' | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DataProduct | null>(null)
  const products = useMemo(() => {
    const value = search.trim().toLowerCase()
    return (query.data?.products ?? []).filter((product) =>
      (!value || `${product.product_key} ${product.name_ko} ${product.name_en} ${product.owner}`.toLowerCase().includes(value)) &&
      (status === 'all' || product.status === status),
    )
  }, [query.data, search, status])

  const saveProduct = useMutation({
    mutationFn: dataworksApi.saveProduct,
    onSuccess: async () => {
      setEditing(null)
      await queryClient.invalidateQueries({ queryKey: ['dataworks'] })
    },
  })
  const deleteProduct = useMutation({
    mutationFn: (id: string) => dataworksApi.deleteProduct(id),
    onSuccess: async () => {
      setDeleteTarget(null)
      await queryClient.invalidateQueries({ queryKey: ['dataworks'] })
    },
  })

  if (query.isPending) return <PageLoader label="상품 포트폴리오를 불러오는 중" />
  if (query.error) return <ErrorState error={query.error} retry={() => void query.refetch()} />
  return (
    <div>
      <PageHeader
        actions={canWrite ? <Button onClick={() => { saveProduct.reset(); setEditing('new') }} variant="accent"><Plus className="size-4" /> 상품 등록</Button> : undefined}
        description="아이디어부터 운영 단계까지 모든 데이터 상품을 하나의 생명주기로 관리합니다."
        eyebrow="포트폴리오 · 생명주기"
        title="데이터 상품"
      />
      <div className="mb-5 flex flex-col gap-3 rounded-[18px] border border-[var(--line)] bg-[var(--surface)] p-3 sm:flex-row sm:items-center">
        <div className="relative flex-1"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--muted-soft)]" /><input aria-label="상품 검색" className="field-input pl-10" onChange={(event) => setSearch(event.target.value)} placeholder="상품 이름, 키 또는 담당자 검색" value={search} /></div>
        <div className="flex items-center gap-2"><Filter className="size-4 text-[var(--muted)]" /><select aria-label="상품 상태 필터" className="field-input h-11 min-w-40" onChange={(event) => setStatus(event.target.value)} value={status}><option value="all">모든 상태</option>{productStatuses.map((item) => <option key={item} value={item}>{statusLabel(item)}</option>)}</select></div>
      </div>
      {products.length ? <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">{products.map((product) => (
        <Card className="flex min-h-[220px] flex-col p-5" key={product.product_key}>
          <div className="flex items-start justify-between gap-4">
            <span className="grid size-10 place-items-center rounded-2xl bg-[var(--info-soft)] text-[var(--accent)]"><Boxes className="size-[18px]" /></span>
            <div className="flex items-center gap-1"><StatusBadge status={product.status} />{canWrite ? <><Button aria-label={`${product.name_ko || product.product_key} 수정`} onClick={() => { saveProduct.reset(); setEditing(product) }} size="icon" title="수정" variant="ghost"><Pencil className="size-3.5" /></Button>{product.status === 'draft' || product.status === 'archived' ? <Button aria-label={`${product.name_ko || product.product_key} 삭제`} onClick={() => { deleteProduct.reset(); setDeleteTarget(product) }} size="icon" title="삭제" variant="ghost"><Trash2 className="size-3.5 text-[var(--danger)]" /></Button> : null}</> : null}</div>
          </div>
          <Link className="group mt-5 no-underline" to={`/products/${encodeURIComponent(product.product_key)}`}>
            <p className="font-mono text-[11px] font-bold uppercase tracking-[.11em] text-[var(--muted-soft)]">{product.product_key}</p>
            <h2 className="mt-2 flex items-center gap-2 text-base font-bold tracking-[-.03em] text-[var(--ink)]">{product.name_ko || product.name_en || product.product_key}<ArrowRight className="size-3.5 text-[var(--muted-soft)] transition-transform group-hover:translate-x-1" /></h2>
            <p className="mt-2 line-clamp-2 text-[11px] leading-5 text-[var(--muted)]">{product.description || product.executive_summary || '상품 설명이 아직 없습니다.'}</p>
          </Link>
          <div className="mt-auto flex gap-2 pt-5"><ScorePill icon={CircleDollarSign} label="수익" tone="success" value={product.revenue_score} /><ScorePill icon={ShieldAlert} label="위험" tone={product.risk_score >= 70 ? 'danger' : 'warning'} value={product.risk_score} /></div>
        </Card>
      ))}</div> : <Card><EmptyState description="검색 조건을 바꾸거나 새 데이터 상품을 등록해 보세요." title="상품이 없습니다" /></Card>}

      <ProductEditor
        error={saveProduct.error}
        key={editing === 'new' ? 'new' : editing?.id ?? 'closed'}
        onClose={() => setEditing(null)}
        onSave={(payload) => saveProduct.mutate(payload)}
        open={editing !== null}
        pending={saveProduct.isPending}
        product={editing === 'new' ? undefined : editing ?? undefined}
      />
      <ConfirmDialog
        confirmLabel="상품 삭제"
        description={`‘${deleteTarget?.name_ko || deleteTarget?.product_key || ''}’ 상품 카탈로그 항목을 삭제합니다. 감사·실행 이력은 보존되며, 이 상품 키는 재사용할 수 없습니다.`}
        error={deleteProduct.error}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteProduct.mutate(deleteTarget.product_key)}
        open={Boolean(deleteTarget)}
        pending={deleteProduct.isPending}
        title="데이터 상품을 삭제할까요?"
      />
    </div>
  )
}

const emptyProduct: DataProductInput = {
  product_key: '', name_ko: '', name_en: '', short_name: '', description: '', executive_summary: '', sales_pitch: '',
  source_type: 'custom', source_ref: '', owner: '', allowed_teams: [], sensitivity: 'internal', status: 'draft',
  target_industries: [], target_customers: [], pricing_model: '', api_spec: '', poc_plan: '', risk_score: 0,
  revenue_score: 0, differentiation: '', similar_products: [],
}

function ProductEditor({ product, open, pending, error, onClose, onSave }: {
  product?: DataProduct
  open: boolean
  pending: boolean
  error: Error | null
  onClose: () => void
  onSave: (payload: DataProductInput) => void
}) {
  const [form, setForm] = useState<DataProductInput>(() => product ? { ...product } : emptyProduct)
  const [allowedTeams, setAllowedTeams] = useState(() => (product?.allowed_teams ?? []).join(', '))
  const [targetCustomers, setTargetCustomers] = useState(() => (product?.target_customers ?? []).join(', '))
  const submit = (event: FormEvent) => {
    event.preventDefault()
    onSave({ ...form, product_key: form.product_key.trim(), name_ko: form.name_ko.trim(), name_en: form.name_en?.trim(), owner: form.owner?.trim(), source_ref: form.source_ref?.trim(), allowed_teams: csv(allowedTeams), target_customers: csv(targetCustomers) })
  }
  return (
    <Dialog
      actions={<><Button disabled={pending} onClick={onClose} type="button" variant="secondary">취소</Button><Button disabled={pending || !form.product_key.trim() || !form.name_ko.trim()} form="product-editor-form" type="submit" variant="accent">{pending ? <LoaderCircle className="size-4 animate-spin" /> : null}{pending ? '저장 중…' : product ? '변경 저장' : '상품 등록'}</Button></>}
      busy={pending}
      className="!w-[min(100%,800px)]"
      description="출시 상태 변경은 상품 작업공간의 게이트와 생명주기 액션에서 별도로 처리합니다."
      onClose={onClose}
      open={open}
      title={product ? '데이터 상품 수정' : '새 데이터 상품 등록'}
    >
      <form className="grid gap-4 sm:grid-cols-2" id="product-editor-form" onSubmit={submit}>
        <Field label="상품 키"><input autoFocus={!product} className="field-input" disabled={Boolean(product)} onChange={(event) => setForm({ ...form, product_key: event.target.value })} placeholder="credit-insight" required value={form.product_key} /></Field>
        <Field label="한글 이름"><input autoFocus={Boolean(product)} className="field-input" onChange={(event) => setForm({ ...form, name_ko: event.target.value })} placeholder="고객 신용 인사이트" required value={form.name_ko} /></Field>
        <Field label="영문 이름"><input className="field-input" onChange={(event) => setForm({ ...form, name_en: event.target.value })} placeholder="Customer Credit Insight" value={form.name_en ?? ''} /></Field>
        <Field label="담당자"><input className="field-input" onChange={(event) => setForm({ ...form, owner: event.target.value })} placeholder="product-owner" value={form.owner ?? ''} /></Field>
        <Field label="원천 유형"><select className="field-input" onChange={(event) => setForm({ ...form, source_type: event.target.value })} value={form.source_type ?? 'custom'}>{['dataset', 'api', 'report', 'score', 'segment', 'model_feature', 'dashboard', 'saved_report', 'metric', 'golden_query', 'custom'].map((type) => <option key={type} value={type}>{type}</option>)}</select></Field>
        <Field label="원천 참조"><input className="field-input" onChange={(event) => setForm({ ...form, source_ref: event.target.value })} placeholder="finance.transactions" value={form.source_ref ?? ''} /></Field>
        <Field label="민감도"><select className="field-input" onChange={(event) => setForm({ ...form, sensitivity: event.target.value })} value={form.sensitivity ?? 'internal'}><option value="public">공개</option><option value="internal">내부</option><option value="restricted">제한</option><option value="personal_credit">개인신용정보</option><option value="pseudonymized">가명처리</option><option value="aggregated">집계</option></select></Field>
        <Field label="가격 정책"><input className="field-input" onChange={(event) => setForm({ ...form, pricing_model: event.target.value })} placeholder="호출당 과금" value={form.pricing_model ?? ''} /></Field>
        <Field label="수익 점수"><input className="field-input" max={100} min={0} onChange={(event) => setForm({ ...form, revenue_score: Number(event.target.value) })} type="number" value={form.revenue_score ?? 0} /></Field>
        <Field label="위험 점수"><input className="field-input" max={100} min={0} onChange={(event) => setForm({ ...form, risk_score: Number(event.target.value) })} type="number" value={form.risk_score ?? 0} /></Field>
        <Field className="sm:col-span-2" label="설명"><textarea className="field-input h-24 py-3" onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="사용자가 이해할 수 있는 상품 설명" value={form.description ?? ''} /></Field>
        <Field label="허용 팀 · 쉼표 구분"><input className="field-input" onChange={(event) => setAllowedTeams(event.target.value)} placeholder="risk, sales" value={allowedTeams} /></Field>
        <Field label="대상 고객 · 쉼표 구분"><input className="field-input" onChange={(event) => setTargetCustomers(event.target.value)} placeholder="은행, 핀테크" value={targetCustomers} /></Field>
        {error ? <p className="field-error sm:col-span-2" role="alert">{error.message}</p> : null}
      </form>
    </Dialog>
  )
}

function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return <label className={className}><span className="field-label">{label}</span>{children}</label>
}

function csv(value: string) { return value.split(',').map((item) => item.trim()).filter(Boolean) }

function ScorePill({ icon: Icon, label, value, tone }: { icon: typeof CircleDollarSign; label: string; value: number; tone: 'success' | 'warning' | 'danger' }) {
  return <div className="rounded-xl bg-[var(--surface-muted)] px-2.5 py-2"><p className="flex items-center gap-1 text-[11px] font-bold tracking-wider text-[var(--muted)]"><Icon className="size-2.5" />{label}</p><p className="mt-1 text-xs font-extrabold" style={{ color: `var(--${tone})` }}>{value}</p></div>
}
