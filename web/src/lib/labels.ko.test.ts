import { describe, expect, it } from 'vitest'

import {
  actionMessageLabel,
  actionTypeLabel,
  approvalLabel,
  gateMessageLabel,
  graphRelationLabel,
  runTypeLabel,
  sensitivityLabel,
  severityLabel,
  statusLabel,
} from './labels.ko'

describe('한국어 UI 라벨', () => {
  it('상태와 분류 enum을 한국어로 표시한다', () => {
    expect(statusLabel('risk_review')).toBe('위험 검토 중')
    expect(statusLabel('completed')).toBe('완료')
    expect(severityLabel('high')).toBe('높음')
    expect(sensitivityLabel('personal_credit')).toBe('개인신용정보')
    expect(approvalLabel('legal')).toBe('법무 승인')
  })

  it('운영 enum을 한국어로 표시한다', () => {
    expect(actionTypeLabel('launch_blocked')).toBe('출시 차단')
    expect(graphRelationLabel('feeds')).toBe('공급')
    expect(runTypeLabel('ideas.generate')).toBe('아이디어 생성')
    expect(actionMessageLabel('resolve publish gate evidence before moving to published')).toContain('출시 게이트 증적')
  })

  it('게이트 사유의 동적 값을 보존하면서 문장을 번역한다', () => {
    expect(gateMessageLabel('missing required approval: compliance')).toBe('필수 승인 누락: 준법 승인')
    expect(gateMessageLabel('missing asset readiness score for FIN_TX')).toBe('FIN_TX 자산의 준비도 점수가 없습니다.')
    expect(gateMessageLabel('asset FIN_TX readiness score 62 is below 70')).toBe('FIN_TX 자산의 준비도 점수 62점이 기준 70점보다 낮습니다.')
  })
})
