import { AlertTriangle, Check, Circle, LockKeyhole } from 'lucide-react'

import { statusLabel } from '@/lib/labels.ko'
import type { ApprovalTrace, ContractVersion, DataProduct, EvidencePack, ProductCanvas, PublishGate } from '@/types/dataworks'

type StepState = 'complete' | 'current' | 'warning' | 'blocked' | 'upcoming'

interface LifecycleData {
  product: DataProduct
  canvas?: ProductCanvas
  canvasDraft?: boolean
  approvals?: ApprovalTrace[]
  evidence?: EvidencePack | null
  gate?: PublishGate
  contract?: ContractVersion | null
}

export function LifecycleStepper({ data }: { data: LifecycleData }) {
  const requiredApprovals = data.gate?.required_approvals ?? ['data_owner', 'legal', 'compliance']
  const approvalsComplete = requiredApprovals.every((step) => ['approved', 'waived'].includes(data.gate?.approval_status[step] ?? ''))
  const assetBlocked = data.gate?.blocked_reasons.some((reason) => reason.includes('asset') || reason.includes('source data')) ?? false

  const rawSteps: Array<{ label: string; complete: boolean; warning?: boolean; blocked?: boolean }> = [
    { label: '자산', complete: Boolean(data.product.source_ref) && !assetBlocked, blocked: assetBlocked },
    { label: '아이디어', complete: true },
    { label: '캔버스', complete: Boolean(data.canvas) && !data.canvasDraft },
    { label: '위험', complete: Boolean(data.gate?.risk_reviewed), warning: !data.gate?.risk_reviewed },
    { label: '승인', complete: approvalsComplete, blocked: Boolean(data.gate?.strict_gate && data.gate.missing_approvals.length) },
    { label: '증적', complete: Boolean(data.evidence), blocked: data.gate?.missing_evidence.includes('evidence_pack') },
    { label: '출시', complete: data.product.status === 'published', blocked: Boolean(data.gate && !data.gate.allowed) },
    { label: '계약', complete: Boolean(data.contract), warning: data.product.status === 'published' && !data.contract },
    { label: '운영', complete: false },
  ]
  const currentIndex = Math.max(0, rawSteps.findIndex((step) => !step.complete))
  const steps = rawSteps.map((step, index): { label: string; state: StepState } => ({
    label: step.label,
    state: step.complete
      ? 'complete'
      : step.blocked
        ? 'blocked'
        : step.warning
          ? 'warning'
          : index === currentIndex
            ? 'current'
            : 'upcoming',
  }))

  return (
    <div className="lifecycle-track" aria-label="상품 생명주기">
      {steps.map((step) => (
        <div key={step.label} className="lifecycle-step">
          <span className={`lifecycle-dot ${step.state}`} aria-label={`${step.label}: ${statusLabel(step.state)}`}>
            {step.state === 'complete' ? <Check className="size-3.5" /> : step.state === 'blocked' ? <LockKeyhole className="size-3.5" /> : step.state === 'warning' ? <AlertTriangle className="size-3.5" /> : <Circle className="size-2.5" fill="currentColor" />}
          </span>
          <span className="mt-2 block text-[11px] font-bold text-[var(--muted)]">{step.label}</span>
        </div>
      ))}
    </div>
  )
}
