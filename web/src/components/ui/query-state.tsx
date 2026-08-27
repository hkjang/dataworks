import { AlertTriangle, Inbox, LoaderCircle, RefreshCw } from 'lucide-react'

import { ApiError } from '@/api/client'
import { Button } from './button'

export function PageLoader({ label = '데이터를 불러오는 중' }: { label?: string }) {
  return (
    <div className="flex min-h-72 flex-col items-center justify-center gap-3 text-[var(--muted)]">
      <LoaderCircle className="size-6 animate-spin text-[var(--accent)]" />
      <p className="text-sm font-medium">{label}</p>
    </div>
  )
}

export function ErrorState({ error, retry }: { error: unknown; retry?: () => void }) {
  const unauthorized = error instanceof ApiError && error.status === 401
  return (
    <div className="flex min-h-72 flex-col items-center justify-center gap-3 px-6 text-center">
      <span className="grid size-11 place-items-center rounded-2xl bg-[var(--danger-soft)] text-[var(--danger)]">
        <AlertTriangle className="size-5" />
      </span>
      <div>
        <p className="font-bold text-[var(--ink)]">
          {unauthorized ? '접근 인증이 필요합니다' : '데이터를 불러오지 못했습니다'}
        </p>
        <p className="mt-1 max-w-md text-sm text-[var(--muted)]">
          {error instanceof Error ? error.message : '잠시 후 다시 시도해 주세요.'}
        </p>
      </div>
      {retry ? (
        <Button variant="secondary" size="sm" onClick={retry}>
          <RefreshCw className="size-3.5" /> 다시 시도
        </Button>
      ) : null}
    </div>
  )
}

export function EmptyState({
  title,
  description,
}: {
  title: string
  description: string
}) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center gap-3 px-6 text-center">
      <span className="grid size-11 place-items-center rounded-2xl bg-[var(--surface-muted)] text-[var(--muted)]">
        <Inbox className="size-5" />
      </span>
      <div>
        <p className="font-bold text-[var(--ink)]">{title}</p>
        <p className="mt-1 max-w-md text-sm text-[var(--muted)]">{description}</p>
      </div>
    </div>
  )
}
