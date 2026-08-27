import { Boxes, CheckCircle2, Database, Factory } from 'lucide-react'

import { cn } from '@/lib/utils'

interface ProductionFlowProps {
  raw: number
  ready: number
  building: number
  live: number
}

const stages = [
  { key: 'raw', number: '01', label: '원천 자산', hint: '데이터 원재료', icon: Database, tone: 'raw' },
  { key: 'ready', number: '02', label: '준비 완료', hint: '품질 통과', icon: CheckCircle2, tone: 'data' },
  { key: 'building', number: '03', label: '상품화 중', hint: '공장 가동', icon: Factory, tone: 'ai' },
  { key: 'live', number: '04', label: '운영 중', hint: '상품 운영', icon: Boxes, tone: 'product' },
] as const

export function ProductionFlow({ raw, ready, building, live }: ProductionFlowProps) {
  const values = { raw, ready, building, live }

  return (
    <section className="refinery-panel" aria-labelledby="production-flow-title">
      <div className="refinery-panel-header">
        <div>
          <p className="refinery-kicker">공장 관제 · 생산 현황</p>
          <h2 id="production-flow-title">데이터 상품 생산 흐름</h2>
          <p>원천 데이터가 검증되고 상품으로 전환되는 현재 공정 상태입니다.</p>
        </div>
        <span className="status-signal is-live"><span /> 실시간</span>
      </div>

      <div className="production-flow" role="list" aria-label="생산 단계">
        {stages.map((stage, index) => {
          const Icon = stage.icon
          const active = values[stage.key] > 0
          return (
            <div className="contents" key={stage.key}>
              <article className={cn('factory-station', `is-${stage.tone}`, active && 'is-active')} role="listitem">
                <div className="factory-station-topline">
                  <span>{stage.hint}</span>
                  <span>{stage.number}</span>
                </div>
                <div className="factory-station-body">
                  <span className="factory-station-icon"><Icon aria-hidden="true" /></span>
                  <div>
                    <strong>{values[stage.key].toLocaleString('ko-KR')}</strong>
                    <p>{stage.label}</p>
                  </div>
                </div>
              </article>
              {index < stages.length - 1 ? (
                <div className={cn('flow-pipe', values[stages[index + 1].key] > 0 && 'is-flowing')} aria-hidden="true">
                  <span className="flow-pipe-line" />
                  <span className="flow-pipe-pulse" />
                </div>
              ) : null}
            </div>
          )
        })}
      </div>
    </section>
  )
}
