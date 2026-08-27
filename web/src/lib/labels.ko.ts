const normalize = (value?: string) => value?.trim().toLowerCase() ?? ''

const STATUS_LABELS: Record<string, string> = {
  active: '활성',
  approved: '승인됨',
  archived: '보관됨',
  blocked: '차단됨',
  canceled: '취소됨',
  cancelled: '취소됨',
  closed: '종료됨',
  complete: '완료',
  completed: '완료',
  current: '진행 중',
  denied: '거부됨',
  draft: '초안',
  expired: '만료됨',
  failed: '실패',
  inactive: '비활성',
  missing: '누락',
  'not required': '불필요',
  not_required: '불필요',
  open: '열림',
  pass: '통과',
  paused: '일시 중지',
  pending: '대기 중',
  processing: '처리 중',
  published: '출시됨',
  queued: '대기열',
  ready: '준비됨',
  rejected: '반려됨',
  replayed: '재실행됨',
  review: '검토 중',
  risk_review: '위험 검토 중',
  running: '실행 중',
  submitted: '제출됨',
  success: '성공',
  succeeded: '성공',
  upcoming: '예정',
  waived: '면제됨',
  waiting: '대기 중',
  warning: '경고',
}

const SEVERITY_LABELS: Record<string, string> = {
  critical: '매우 높음',
  high: '높음',
  info: '정보',
  low: '낮음',
  medium: '보통',
  warning: '경고',
}

const ROLE_LABELS: Record<string, string> = {
  admin: '관리자',
  administrator: '관리자',
  compliance: '준법 담당자',
  data_owner: '데이터 오너',
  legacy: '토큰 사용자',
  legal: '법무 담당자',
  operator: '운영자',
  owner: '소유자',
  reviewer: '검토자',
  super_admin: '최고 관리자',
  viewer: '조회자',
}

const SENSITIVITY_LABELS: Record<string, string> = {
  confidential: '기밀',
  internal: '내부',
  identifier: '식별정보',
  personal: '개인정보',
  personal_credit: '개인신용정보',
  pseudonymized: '가명처리',
  public: '공개',
  restricted: '제한',
  sensitive: '민감',
}

const APPROVAL_LABELS: Record<string, string> = {
  compliance: '준법 승인',
  data_owner: '데이터 오너 승인',
  finance: '재무 승인',
  legal: '법무 승인',
  privacy: '개인정보 승인',
  product_owner: '상품 오너 승인',
  security: '보안 승인',
}

const ACTION_TYPE_LABELS: Record<string, string> = {
  approval_pending: '승인 대기',
  blocked_launches: '출시 차단',
  contract_expiring: '계약 만료 예정',
  entitlement_inactive: '비활성 접근 권한',
  expiring_contracts: '계약 만료 예정',
  inactive_access: '비활성 접근 권한',
  launch_blocked: '출시 차단',
  low_customer_fit: '낮은 고객 적합도',
  low_fit_scores: '낮은 고객 적합도',
  negative_margin: '마이너스 마진',
  retirement_candidate: '종료 검토 후보',
  retirement_candidates: '종료 검토 후보',
  stale_watermark: '최신성 지연',
  stale_watermarks: '최신성 지연',
}

const ACTION_MESSAGE_LABELS: Record<string, string> = {
  'adjust pricing, reduce query/llm cost, or restrict low-value usage': '가격을 조정하고 쿼리·LLM 비용을 줄이거나 저가치 사용을 제한하세요.',
  'adjust product positioning or target a better segment before proposal work': '제안 작업 전에 상품 포지셔닝을 조정하거나 더 적합한 고객군을 선택하세요.',
  'complete review approvals or return to draft with remediation notes': '검토 승인을 완료하거나 보완 사항을 기록해 초안으로 되돌리세요.',
  'refresh source data or pause proposals that depend on stale data': '원천 데이터를 갱신하거나 최신성이 지연된 데이터에 의존하는 제안을 일시 중지하세요.',
  'renew, narrow, or retire the customer contract scope': '고객 계약 범위를 갱신·축소하거나 종료하세요.',
  'resolve publish gate evidence before moving to published': '출시 상태로 전환하기 전에 출시 게이트 증적을 보완하세요.',
  'review product catalog status and decide improve or retire': '상품 카탈로그 상태를 검토하고 개선 또는 종료를 결정하세요.',
  'rotate, reactivate, or remove stale api product access': '오래된 API 상품 접근 권한을 교체·재활성화하거나 제거하세요.',
}

const GRAPH_TYPE_LABELS: Record<string, string> = {
  approval: '승인',
  asset: '자산',
  customer: '고객',
  poc_outcome: 'PoC 결과',
  product: '상품',
  proposal_feedback: '제안 피드백',
}

const GRAPH_RELATION_LABELS: Record<string, string> = {
  approval_trace: '승인 이력',
  feeds: '공급',
  poc_outcome: 'PoC 결과',
  proposal_feedback: '제안 피드백',
  uses_asset: '자산 사용',
}

const RUN_TYPE_LABELS: Record<string, string> = {
  'ideas.generate': '아이디어 생성',
  'products.define': '상품 정의',
}

const REFRESH_CYCLE_LABELS: Record<string, string> = {
  daily: '매일',
  hourly: '매시간',
  monthly: '매월',
  realtime: '실시간',
  weekly: '매주',
}

const POLICY_DECISION_LABELS: Record<string, string> = {
  allow: '허용',
  allowed: '허용',
  approved: '승인',
  block: '차단',
  blocked: '차단',
  deny: '거부',
  denied: '거부',
  review: '검토 필요',
  warn: '경고',
}

export function statusLabel(value?: string) {
  const key = normalize(value)
  return STATUS_LABELS[key] ?? (key ? '확인 필요' : '알 수 없음')
}

export function severityLabel(value?: string) {
  const key = normalize(value)
  return SEVERITY_LABELS[key] ?? (key ? '미분류' : '알 수 없음')
}

export function roleLabel(value?: string) {
  const key = normalize(value)
  return ROLE_LABELS[key] ?? (key ? '사용자' : '운영자')
}

export function sensitivityLabel(value?: string) {
  const key = normalize(value)
  return SENSITIVITY_LABELS[key] ?? (key ? '기타' : '내부')
}

export function approvalLabel(value?: string) {
  const key = normalize(value)
  return APPROVAL_LABELS[key] ?? (key ? '추가 승인' : '승인')
}

export function actionTypeLabel(value?: string) {
  const key = normalize(value)
  return ACTION_TYPE_LABELS[key] ?? '운영 조치'
}

export function actionMessageLabel(value?: string) {
  const key = normalize(value)
  if (!key) return ''
  return ACTION_MESSAGE_LABELS[key] ?? gateMessageLabel(value)
}

export function graphTypeLabel(value?: string) {
  const key = normalize(value)
  return GRAPH_TYPE_LABELS[key] ?? '연결 항목'
}

export function graphRelationLabel(value?: string) {
  const key = normalize(value)
  return GRAPH_RELATION_LABELS[key] ?? '연결'
}

export function runTypeLabel(value?: string) {
  const key = normalize(value)
  return RUN_TYPE_LABELS[key] ?? (key ? '팩토리 실행' : '실행 유형 미지정')
}

export function refreshCycleLabel(value?: string) {
  const key = normalize(value)
  return REFRESH_CYCLE_LABELS[key] ?? (value?.trim() || '미지정')
}

export function policyDecisionLabel(value?: string) {
  const key = normalize(value)
  return POLICY_DECISION_LABELS[key] ?? (key ? '정책 판정' : '미판정')
}

export function gateMessageLabel(value?: string) {
  const message = value?.trim() ?? ''
  const key = message.toLowerCase()
  if (!key) return ''

  const fixed: Record<string, string> = {
    'api contract (openapi spec) is not configured': 'API 계약(OpenAPI 명세)이 설정되지 않았습니다.',
    'at least one source data asset must be linked before publishing a strict-gated product': '엄격한 게이트가 적용된 상품을 출시하려면 원천 데이터 자산을 하나 이상 연결해야 합니다.',
    'evidence pack must be generated before publishing': '출시 전에 증적 패키지를 생성해야 합니다.',
    'no data quality execution results found for assets': '자산의 데이터 품질 실행 결과가 없습니다.',
    'pricing model or operational cost parameters are not configured': '가격 정책 또는 운영 비용 매개변수가 설정되지 않았습니다.',
    'risk review check must be completed before publishing': '출시 전에 위험 검토를 완료해야 합니다.',
    'sample response masking policy is not configured for sensitive product': '민감 상품의 샘플 응답 마스킹 정책이 설정되지 않았습니다.',
    'sla targets are not configured': 'SLA 목표가 설정되지 않았습니다.',
    'strict gate is not required for this product risk/sensitivity profile': '이 상품의 위험도·민감도 프로필에는 엄격한 출시 게이트가 필요하지 않습니다.',
  }
  if (fixed[key]) return fixed[key]

  const missingApproval = message.match(/^missing required approval:\s*(.+)$/i)
  if (missingApproval) return `필수 승인 누락: ${approvalLabel(missingApproval[1])}`

  const missingReadiness = message.match(/^missing asset readiness score for\s+(.+)$/i)
  if (missingReadiness) return `${missingReadiness[1]} 자산의 준비도 점수가 없습니다.`

  const lowReadiness = message.match(/^asset\s+(.+?)\s+readiness score\s+(\d+)\s+is below\s+(\d+)$/i)
  if (lowReadiness) return `${lowReadiness[1]} 자산의 준비도 점수 ${lowReadiness[2]}점이 기준 ${lowReadiness[3]}점보다 낮습니다.`

  const qualityFailure = message.match(/^data quality rule failed:\s*(.+)$/i)
  if (qualityFailure) return `데이터 품질 규칙 실패: ${qualityFailure[1]}`

  return message
}
