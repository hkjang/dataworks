import { cva, type VariantProps } from 'class-variance-authority'
import type { HTMLAttributes } from 'react'

import { statusLabel } from '@/lib/labels.ko'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex h-6 items-center gap-1.5 rounded-full px-2.5 text-[11px] font-bold tracking-[.02em]',
  {
    variants: {
      tone: {
        neutral: 'bg-[var(--surface-muted)] text-[var(--muted)]',
        info: 'bg-[var(--info-soft)] text-[var(--info)]',
        success: 'bg-[var(--success-soft)] text-[var(--success)]',
        warning: 'bg-[var(--warning-soft)] text-[var(--warning)]',
        danger: 'bg-[var(--danger-soft)] text-[var(--danger)]',
        violet: 'bg-[var(--violet-soft)] text-[var(--violet)]',
      },
    },
    defaultVariants: { tone: 'neutral' },
  },
)

export interface BadgeProps
  extends HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, tone, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ tone }), className)} {...props} />
}

export function StatusBadge({ status }: { status?: string }) {
  const normalized = status?.trim().toLowerCase() ?? ''
  const tone =
    ['active', 'approved', 'complete', 'completed', 'pass', 'published', 'ready', 'success', 'succeeded', 'waived'].includes(normalized)
      ? 'success'
      : ['pending', 'queued', 'review', 'waiting', 'warning'].includes(normalized)
        ? 'warning'
        : ['blocked', 'denied', 'expired', 'failed', 'missing', 'rejected', 'risk_review'].includes(normalized)
          ? 'danger'
          : ['current', 'draft', 'processing', 'replayed', 'running', 'submitted'].includes(normalized)
            ? 'info'
            : 'neutral'
  return <Badge tone={tone}>{statusLabel(status)}</Badge>
}
