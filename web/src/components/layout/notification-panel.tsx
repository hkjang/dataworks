import type { UseQueryResult } from '@tanstack/react-query'
import { AlertCircle, ArrowUpRight, Bell, Clock3 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { actionMessageLabel, actionTypeLabel } from '@/lib/labels.ko'
import type { ActionItem, ActionSummary } from '@/types/dataworks'
import { useUIStore } from '@/stores/ui-store'

export function NotificationPanel({
  query,
}: {
  query: UseQueryResult<{ summary: ActionSummary; actions: ActionItem[] }, Error>
}) {
  const open = useUIStore((state) => state.notificationsOpen)
  const setOpen = useUIStore((state) => state.setNotificationsOpen)
  const navigate = useNavigate()
  if (!open) return null

  const actions = query.data?.actions.slice(0, 5) ?? []
  return (
    <section className="notification-panel" aria-label="알림 센터">
      <div className="flex items-center justify-between border-b border-[var(--line)] px-4 py-3">
        <div><p className="text-sm font-bold text-[var(--ink)]">확인이 필요한 항목</p><p className="text-[11px] text-[var(--muted)]">액션 센터에서 동기화됨</p></div>
        <span className="grid size-8 place-items-center rounded-xl bg-[var(--danger-soft)] text-[var(--danger)]"><Bell className="size-3.5" /></span>
      </div>
      <div className="max-h-80 overflow-auto p-2">
        {actions.length ? actions.map((action, index) => (
          <button key={`${action.type}-${action.product_key}-${index}`} className="notification-item" onClick={() => { navigate(action.product_key ? `/products/${encodeURIComponent(action.product_key)}` : '/review'); setOpen(false) }}>
            <span className={`mt-1 grid size-7 shrink-0 place-items-center rounded-lg ${action.severity === 'high' ? 'bg-[var(--danger-soft)] text-[var(--danger)]' : 'bg-[var(--warning-soft)] text-[var(--warning)]'}`}>
              {action.severity === 'high' ? <AlertCircle className="size-3.5" /> : <Clock3 className="size-3.5" />}
            </span>
            <span className="min-w-0 flex-1 text-left"><span className="block truncate text-xs font-bold text-[var(--ink)]">{action.title || action.product_key || actionTypeLabel(action.type)}</span><span className="mt-0.5 line-clamp-2 block text-[11px] leading-5 text-[var(--muted)]">{actionMessageLabel(action.next_action || action.blocked_reasons?.[0] || action.reason)}</span></span>
          </button>
        )) : <p className="px-3 py-10 text-center text-xs text-[var(--muted)]">새로운 알림이 없습니다.</p>}
      </div>
      <button className="flex w-full items-center justify-center gap-2 border-t border-[var(--line)] py-3 text-xs font-bold text-[var(--accent)]" onClick={() => { navigate('/review'); setOpen(false) }}>모든 액션 보기 <ArrowUpRight className="size-3.5" /></button>
    </section>
  )
}
