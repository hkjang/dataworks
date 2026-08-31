import { LoaderCircle, X } from 'lucide-react'
import { useEffect, useId, useRef, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export function Dialog({
  open,
  title,
  description,
  children,
  actions,
  onClose,
  busy = false,
  className,
}: {
  open: boolean
  title: string
  description?: string
  children: ReactNode
  actions?: ReactNode
  onClose: () => void
  busy?: boolean
  className?: string
}) {
  const titleID = useId()
  const descriptionID = useId()
  const dialogRef = useRef<HTMLElement>(null)

  useEffect(() => {
    if (!open) return

    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const focusFrame = window.requestAnimationFrame(() => {
      const dialog = dialogRef.current
      if (!dialog || dialog.contains(document.activeElement)) return
      dialog.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')?.focus()
    })
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onClose()
      if (event.key !== 'Tab') return
      const focusable = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])') ?? [])
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      window.cancelAnimationFrame(focusFrame)
      document.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = previousOverflow
      previousFocus?.focus()
    }
  }, [busy, onClose, open])

  if (!open) return null
  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target && !busy) onClose()
      }}
      role="presentation"
    >
      <section
        aria-describedby={description ? descriptionID : undefined}
        aria-labelledby={titleID}
        aria-modal="true"
        className={cn('dialog-card max-h-[calc(100vh-40px)] overflow-y-auto', className)}
        ref={dialogRef}
        role="dialog"
      >
        <header className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-bold tracking-[-.03em] text-[var(--ink)]" id={titleID}>{title}</h2>
            {description ? <p className="mt-1.5 text-xs leading-5 text-[var(--muted)]" id={descriptionID}>{description}</p> : null}
          </div>
          <Button aria-label="닫기" disabled={busy} onClick={onClose} size="icon" type="button" variant="ghost"><X className="size-4" /></Button>
        </header>
        <div className="mt-5">{children}</div>
        {actions ? <footer className="mt-6 flex flex-wrap justify-end gap-2 border-t border-[var(--line)] pt-4">{actions}</footer> : null}
      </section>
    </div>
  )
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  confirmVariant = 'danger',
  onConfirm,
  onClose,
  pending = false,
  error,
  warning = '이 작업은 즉시 반영되며 되돌릴 수 없습니다.',
}: {
  open: boolean
  title: string
  description: string
  confirmLabel: string
  confirmVariant?: 'danger' | 'accent' | 'primary'
  onConfirm: () => void
  onClose: () => void
  pending?: boolean
  error?: Error | null
  warning?: string
}) {
  return (
    <Dialog
      actions={<><Button disabled={pending} onClick={onClose} type="button" variant="secondary">취소</Button><Button disabled={pending} onClick={onConfirm} type="button" variant={confirmVariant}>{pending ? <LoaderCircle className="size-4 animate-spin" /> : null}{pending ? '처리 중…' : confirmLabel}</Button></>}
      busy={pending}
      description={description}
      onClose={onClose}
      open={open}
      title={title}
    >
      {error ? <p className="field-error rounded-xl bg-[var(--danger-soft)] p-3" role="alert">{error.message}</p> : <p className="rounded-xl bg-[var(--warning-soft)] p-3 text-xs leading-5 text-[var(--warning)]">{warning}</p>}
    </Dialog>
  )
}
