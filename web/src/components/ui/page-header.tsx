import type { ReactNode } from 'react'

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow?: string
  title: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <header className="mb-7 flex flex-col justify-between gap-4 md:flex-row md:items-end">
      <div>
        {eyebrow ? (
          <p className="mb-2 text-[11px] font-extrabold uppercase tracking-[.18em] text-[var(--accent)]">
            {eyebrow}
          </p>
        ) : null}
        <h1 className="text-[clamp(1.75rem,3vw,2.5rem)] font-[750] leading-[1.08] tracking-[-.045em] text-[var(--ink)]">
          {title}
        </h1>
        {description ? (
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--muted)]">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </header>
  )
}
