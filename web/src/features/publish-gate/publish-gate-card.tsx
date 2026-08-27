import { AlertTriangle, Check, ChevronRight, CircleDashed, LockKeyhole, ShieldCheck } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { approvalLabel, gateMessageLabel, statusLabel } from '@/lib/labels.ko'
import { formatDate } from '@/lib/utils'
import type { PublishGate } from '@/types/dataworks'

interface GateCheck {
  key: string
  label: string
  status: 'pass' | 'waiting' | 'warning' | 'blocked'
  detail?: string
}

export function PublishGateCard({ gate, onPublish, publishing }: { gate: PublishGate; onPublish?: () => void; publishing?: boolean }) {
  const assetPass = !gate.strict_gate || (gate.asset_readiness.length > 0 && gate.asset_readiness.every((asset) => asset.overall_score >= gate.minimum_readiness))
  const approvalChecks: GateCheck[] = gate.required_approvals.map((step) => {
    const status = (gate.approval_status[step] || (gate.strict_gate ? 'pending' : 'not required')).trim().toLowerCase()
    const pass = !gate.strict_gate || ['approved', 'waived'].includes(status)
    return { key: `approval-${step}`, label: approvalLabel(step), status: pass ? 'pass' : status === 'rejected' || status === 'expired' ? 'blocked' : 'waiting', detail: statusLabel(status) }
  })
  const checks: GateCheck[] = [
    { key: 'assets', label: '자산 준비도', status: assetPass ? 'pass' : 'blocked', detail: gate.asset_readiness.length ? `최저 점수 ${Math.min(...gate.asset_readiness.map((item) => item.overall_score))} / 기준 ${gate.minimum_readiness}` : '점수 없음' },
    ...approvalChecks,
    { key: 'evidence', label: '증적 패키지', status: gate.missing_evidence.includes('evidence_pack') ? 'blocked' : 'pass' },
    { key: 'quality', label: '데이터 품질', status: gate.quality_passed ? 'pass' : gate.blocked_reasons.some((item) => item.includes('quality')) ? 'blocked' : 'warning' },
    { key: 'risk', label: '위험 검토', status: gate.risk_reviewed ? 'pass' : 'warning' },
    { key: 'api', label: 'API 계약', status: gate.api_contract_configured ? 'pass' : 'warning' },
    { key: 'sla', label: 'SLA 목표', status: gate.sla_configured ? 'pass' : 'warning' },
    { key: 'pricing', label: '가격 및 비용', status: gate.pricing_model_configured ? 'pass' : 'warning' },
    { key: 'masking', label: '마스킹 정책', status: gate.masking_configured ? 'pass' : 'blocked' },
  ]
  const completion = Math.round((checks.filter((item) => item.status === 'pass').length / checks.length) * 100)

  return (
    <Card className={`release-gate ${gate.allowed ? 'is-open' : 'is-blocked'}`}>
      <CardHeader className="items-center">
        <div className="flex items-center gap-3">
          <span className={`grid size-11 place-items-center rounded-xl ${gate.allowed ? 'bg-[var(--success-soft)] text-[var(--success)]' : 'bg-[var(--danger-soft)] text-[var(--danger)]'}`}>
            {gate.allowed ? <ShieldCheck className="size-5" /> : <LockKeyhole className="size-5" />}
          </span>
          <div><p className="text-[11px] font-extrabold uppercase tracking-[.12em] text-[var(--muted)]">출시 게이트 · G-01</p><p className="mt-1 text-sm font-bold text-[var(--ink)]">구성 완료도 · {formatDate(gate.checked_at)}</p></div>
        </div>
        <div className="text-right"><p className="text-2xl font-[750] tracking-[-.05em] text-[var(--ink)]">{completion}%</p><Badge tone={gate.allowed ? 'success' : 'danger'}>{gate.allowed ? '준비 완료' : '차단됨'}</Badge></div>
      </CardHeader>
      <CardContent>
        <Progress value={completion} className="mb-5" />
        <div className="grid gap-x-8 md:grid-cols-2">
          {checks.map((check) => <GateRow key={check.key} check={check} />)}
        </div>

        {!gate.allowed ? (
          <div className="mt-5 rounded-xl border border-[color-mix(in_srgb,var(--danger)_20%,var(--line))] bg-[var(--danger-soft)] p-4">
            <div className="flex items-center gap-2 text-xs font-bold text-[var(--danger)]"><LockKeyhole className="size-4" /> 출시 차단됨</div>
            <ul className="mt-3 space-y-2">
              {gate.blocked_reasons.map((reason) => <li key={reason} className="flex gap-2 text-[11px] leading-5 text-[var(--ink)]"><ChevronRight className="mt-1 size-3 shrink-0 text-[var(--danger)]" />{gateMessageLabel(reason)}</li>)}
            </ul>
          </div>
        ) : (
          <div className="mt-5 flex flex-col justify-between gap-3 rounded-xl bg-[var(--success-soft)] p-4 sm:flex-row sm:items-center">
            <div><p className="text-xs font-bold text-[var(--success)]">모든 필수 게이트 조건을 통과했습니다</p><p className="mt-1 text-[11px] text-[var(--muted)]">서버의 최종 출시 게이트 판정을 통과했습니다.</p></div>
            {onPublish ? <Button size="sm" variant="accent" onClick={onPublish} disabled={publishing}>{publishing ? '출시 중…' : '상품 출시'}</Button> : null}
          </div>
        )}
        {gate.warnings.length ? (
          <div className="mt-4"><p className="flex items-center gap-2 text-[11px] font-bold tracking-[.08em] text-[var(--warning)]"><AlertTriangle className="size-3.5" /> 참고 경고</p><ul className="mt-2 space-y-1 text-[11px] leading-5 text-[var(--muted)]">{gate.warnings.map((warning) => <li key={warning}>• {gateMessageLabel(warning)}</li>)}</ul></div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function GateRow({ check }: { check: GateCheck }) {
  const config = {
    pass: { icon: Check, tone: 'success' as const, label: '통과' },
    waiting: { icon: CircleDashed, tone: 'warning' as const, label: '대기' },
    warning: { icon: AlertTriangle, tone: 'warning' as const, label: '경고' },
    blocked: { icon: LockKeyhole, tone: 'danger' as const, label: '차단' },
  }[check.status]
  return <div className="gate-row"><div className="flex min-w-0 items-center gap-2.5"><span className="grid size-7 shrink-0 place-items-center rounded-lg" style={{ background: `var(--${config.tone}-soft)`, color: `var(--${config.tone})` }}><config.icon className="size-3.5" /></span><div className="min-w-0"><p className="truncate text-[11px] font-semibold text-[var(--ink)]">{check.label}</p>{check.detail ? <p className="mt-0.5 truncate text-[11px] text-[var(--muted)]">{check.detail}</p> : null}</div></div><Badge tone={config.tone}>{config.label}</Badge></div>
}
