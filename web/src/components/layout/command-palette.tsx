import { useQuery } from '@tanstack/react-query'
import { Boxes, Database, Search, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { dataworksApi } from '@/api/dataworks'
import { Button } from '@/components/ui/button'
import { useUIStore } from '@/stores/ui-store'

export function CommandPalette() {
  const open = useUIStore((state) => state.commandOpen)
  const setOpen = useUIStore((state) => state.setCommandOpen)
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const products = useQuery({ queryKey: ['dataworks', 'products'], queryFn: dataworksApi.products, enabled: open, staleTime: 60_000 })
  const assets = useQuery({ queryKey: ['dataworks', 'assets'], queryFn: dataworksApi.assets, enabled: open, staleTime: 60_000 })

  const results = useMemo(() => {
    const query = search.trim().toLowerCase()
    const productItems = (products.data?.products ?? [])
      .filter((item) => !query || `${item.product_key} ${item.name_ko} ${item.name_en}`.toLowerCase().includes(query))
      .slice(0, 6)
      .map((item) => ({ type: '상품', label: item.name_ko || item.product_key, meta: item.product_key, to: `/products/${encodeURIComponent(item.product_key)}`, Icon: Boxes }))
    const assetItems = (assets.data?.assets ?? [])
      .filter((item) => !query || `${item.asset_key} ${item.name} ${item.domain}`.toLowerCase().includes(query))
      .slice(0, 5)
      .map((item) => ({ type: '자산', label: item.name || item.asset_key, meta: item.asset_key, to: `/assets?asset=${encodeURIComponent(item.asset_key)}`, Icon: Database }))
    return [...productItems, ...assetItems]
  }, [assets.data, products.data, search])

  if (!open) return null
  return (
    <div className="dialog-backdrop items-start pt-[12vh]" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setOpen(false) }}>
      <section className="command-dialog" role="dialog" aria-modal="true" aria-label="통합 검색">
        <div className="flex items-center gap-3 border-b border-[var(--line)] px-4">
          <Search className="size-4 text-[var(--muted)]" />
          <input autoFocus className="h-14 min-w-0 flex-1 bg-transparent text-sm text-[var(--ink)] outline-none placeholder:text-[var(--muted-soft)]" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="상품, 자산, 고객, 계약 검색…" />
          <Button variant="ghost" size="icon" onClick={() => setOpen(false)} aria-label="통합 검색 닫기"><X className="size-4" /></Button>
        </div>
        <div className="max-h-[420px] overflow-auto p-2">
          <p className="px-3 py-2 text-[11px] font-black tracking-[.12em] text-[var(--muted-soft)]">검색 결과</p>
          {results.length ? results.map((item) => (
            <button key={`${item.type}-${item.meta}`} className="command-result" onClick={() => { navigate(item.to); setOpen(false); setSearch('') }}>
              <span className="grid size-9 place-items-center rounded-xl bg-[var(--surface-muted)] text-[var(--muted)]"><item.Icon className="size-4" /></span>
              <span className="min-w-0 flex-1 text-left"><span className="block truncate text-sm font-semibold text-[var(--ink)]">{item.label}</span><span className="block truncate text-[11px] text-[var(--muted)]">{item.meta}</span></span>
              <span className="text-[11px] font-bold tracking-wider text-[var(--muted-soft)]">{item.type}</span>
            </button>
          )) : <p className="px-3 py-10 text-center text-sm text-[var(--muted)]">일치하는 결과가 없습니다.</p>}
        </div>
        <div className="flex items-center gap-4 border-t border-[var(--line)] px-4 py-2 text-[11px] text-[var(--muted-soft)]"><span>↵ 열기</span><span>Esc 닫기</span></div>
      </section>
    </div>
  )
}
