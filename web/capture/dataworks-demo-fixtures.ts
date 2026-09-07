import type { Page, Route } from '@playwright/test'

export const DEMO_VERSION = 'v0.9.44'
export const PRIMARY_PRODUCT = 'sme-credit-insight'

const timestamp = '2026-08-28T09:30:00Z'

const assets = [
  { id: 'asset-1', asset_key: 'SME_PROFILE', name: '중소기업 프로필', domain: '기업금융', owner: '기업데이터팀', columns_summary: '기업 기본정보, 업력, 업종, 지역', sensitivity: 'pseudonymized', refresh_cycle: 'daily', created_at: '2026-01-10T00:00:00Z', updated_at: timestamp },
  { id: 'asset-2', asset_key: 'FIN_TX', name: '금융 거래 집계', domain: '금융', owner: '금융데이터팀', columns_summary: '월별 입출금, 잔액, 연체 신호', sensitivity: 'personal_credit', refresh_cycle: 'daily', created_at: '2026-01-12T00:00:00Z', updated_at: timestamp },
  { id: 'asset-3', asset_key: 'CARD_SIGNAL', name: '카드 소비 신호', domain: '금융', owner: '결제인사이트팀', columns_summary: '업종별 소비, 결제 주기, 변동성', sensitivity: 'pseudonymized', refresh_cycle: 'hourly', created_at: '2026-02-01T00:00:00Z', updated_at: '2026-08-28T08:45:00Z' },
  { id: 'asset-4', asset_key: 'FRAUD_EVENT', name: '이상 거래 이벤트', domain: '리스크', owner: '금융보안팀', columns_summary: '탐지 규칙, 이상 점수, 조치 결과', sensitivity: 'restricted', refresh_cycle: 'realtime', created_at: '2026-02-10T00:00:00Z', updated_at: '2026-08-28T09:20:00Z' },
  { id: 'asset-5', asset_key: 'COMMERCE_ORDER', name: '커머스 주문 집계', domain: '커머스', owner: '커머스데이터팀', columns_summary: '주문, 반품, 객단가, 카테고리', sensitivity: 'internal', refresh_cycle: 'hourly', created_at: '2026-03-01T00:00:00Z', updated_at: '2026-08-28T09:00:00Z' },
  { id: 'asset-6', asset_key: 'CUSTOMER_TOUCH', name: '고객 접점 분석', domain: '고객', owner: 'CX인사이트팀', columns_summary: '채널, 캠페인, 전환, 만족도', sensitivity: 'pseudonymized', refresh_cycle: 'daily', created_at: '2026-03-14T00:00:00Z', updated_at: '2026-08-27T23:00:00Z' },
  { id: 'asset-7', asset_key: 'CORP_CASHFLOW', name: '기업 현금흐름 지표', domain: '기업금융', owner: '기업데이터팀', columns_summary: '유동성, 매출 변동, 지급 여력', sensitivity: 'confidential', refresh_cycle: 'daily', created_at: '2026-04-01T00:00:00Z', updated_at: '2026-08-28T07:30:00Z' },
  { id: 'asset-8', asset_key: 'LEGACY_REPORT', name: '레거시 월간 리포트', domain: '공통', owner: '데이터운영팀', columns_summary: '월별 집계 리포트', sensitivity: 'internal', refresh_cycle: 'monthly', created_at: '2025-11-01T00:00:00Z', updated_at: '2026-07-01T00:00:00Z' },
]

const readiness = assets.map((asset, index) => {
  const score = [94, 84, 91, 88, 86, 76, 82, 55][index]
  return {
    asset_key: asset.asset_key,
    schema_score: Math.min(99, score + 2),
    freshness_score: index === 7 ? 42 : Math.min(99, score + 3),
    sample_score: Math.max(50, score - 3),
    missingness_score: Math.min(98, score + 1),
    sensitivity_score: Math.max(55, score - 8),
    external_sharing_score: Math.max(48, score - 10),
    api_readiness_score: Math.max(52, score - 2),
    billing_readiness_score: Math.max(50, score - 5),
    overall_score: score,
    notes: score >= 70 ? '상품화 기준을 충족합니다.' : '최신성과 문서화 개선이 필요합니다.',
    updated_by: 'demo-system',
    updated_at: timestamp,
  }
})

function product(overrides: Record<string, unknown>) {
  return {
    id: 'product-demo', product_key: 'demo-product', name_ko: '데모 데이터 상품', name_en: 'Demo Data Product', short_name: '데모 상품',
    description: '신뢰할 수 있는 데이터 신호를 API로 제공하는 데이터 상품입니다.', executive_summary: '검증된 데이터 자산을 상품화합니다.', sales_pitch: '빠르게 연결하고 안전하게 운영하는 데이터 인사이트',
    source_type: 'api', source_ref: 'SME_PROFILE', owner: '데이터상품팀', allowed_teams: ['상품운영팀'], sensitivity: 'internal', status: 'draft', version: 1,
    target_industries: ['fintech'], target_customers: ['data-team'], pricing_model: 'usage', api_spec: '{}', poc_plan: '4주 PoC', risk_score: 30, revenue_score: 70,
    differentiation: '거버넌스 증적과 운영 API를 함께 제공합니다.', similar_products: [], updated_by: 'demo-admin', created_at: '2026-05-01T00:00:00Z', updated_at: timestamp,
    ...overrides,
  }
}

const products = [
  product({ id: 'product-1', product_key: PRIMARY_PRODUCT, name_ko: '중소기업 신용 인사이트 API', name_en: 'SME Credit Insight API', short_name: 'SME 신용 인사이트', description: '기업 프로필과 거래 흐름을 결합해 실시간 신용 위험과 성장 신호를 제공하는 API입니다.', executive_summary: '중소기업 금융 심사 시간을 단축하는 고신뢰 데이터 상품', sales_pitch: '더 빠른 기업금융 심사를 위한 설명 가능한 신용 인사이트', source_ref: 'SME_PROFILE, FIN_TX', owner: '기업데이터상품팀', sensitivity: 'personal_credit', status: 'approved', version: 4, target_industries: ['핀테크', '은행', '보증'], target_customers: ['기업금융 심사역', '리스크 담당자'], pricing_model: 'tier_usage', risk_score: 78, revenue_score: 92 }),
  product({ id: 'product-2', product_key: 'fraud-signal-api', name_ko: '이상 거래 탐지 신호 API', name_en: 'Fraud Signal API', short_name: '이상 거래 신호', description: '결제 이상 징후와 위험 등급을 밀리초 단위로 제공합니다.', sales_pitch: '실시간 결제 리스크를 더 빠르게 차단하세요.', source_ref: 'FRAUD_EVENT, CARD_SIGNAL', owner: '금융보안상품팀', sensitivity: 'restricted', status: 'published', version: 7, target_industries: ['카드', '핀테크'], pricing_model: 'usage', risk_score: 44, revenue_score: 95 }),
  product({ id: 'product-3', product_key: 'cashflow-health-api', name_ko: '기업 현금흐름 건강도 API', name_en: 'Cashflow Health API', short_name: '현금흐름 건강도', description: '기업 유동성과 지급 여력 변화를 일별 지표로 제공합니다.', source_ref: 'CORP_CASHFLOW', owner: '기업데이터상품팀', sensitivity: 'confidential', status: 'published', version: 3, target_industries: ['은행', 'B2B SaaS'], pricing_model: 'subscription', risk_score: 32, revenue_score: 88 }),
  product({ id: 'product-4', product_key: 'commerce-trend-api', name_ko: '커머스 소비 트렌드 API', name_en: 'Commerce Trend API', short_name: '소비 트렌드', description: '카테고리별 주문과 반품 변화를 집계해 시장 흐름을 제공합니다.', source_ref: 'COMMERCE_ORDER', owner: '커머스데이터팀', status: 'published', version: 5, target_industries: ['리테일', '마케팅'], pricing_model: 'tier', risk_score: 18, revenue_score: 84 }),
  product({ id: 'product-5', product_key: 'customer-360-signal', name_ko: '고객 360 행동 신호', name_en: 'Customer 360 Signal', source_ref: 'CUSTOMER_TOUCH', owner: 'CX인사이트팀', sensitivity: 'pseudonymized', status: 'risk_review', version: 2, target_industries: ['마케팅'], pricing_model: 'usage', risk_score: 71, revenue_score: 79 }),
  product({ id: 'product-6', product_key: 'campaign-opportunity', name_ko: '캠페인 기회 추천', name_en: 'Campaign Opportunity', source_ref: 'CUSTOMER_TOUCH', owner: 'CX인사이트팀', status: 'review', version: 1, target_industries: ['마케팅'], pricing_model: 'subscription', risk_score: 38, revenue_score: 73 }),
  product({ id: 'product-7', product_key: 'legacy-monthly-report', name_ko: '월간 데이터 리포트', name_en: 'Monthly Data Report', source_type: 'report', source_ref: 'LEGACY_REPORT', owner: '데이터운영팀', status: 'archived', version: 9, target_industries: ['내부'], pricing_model: 'internal', risk_score: 12, revenue_score: 42, updated_at: '2026-07-01T00:00:00Z' }),
]

const actionCenter = {
  summary: { approval_pending: 3, blocked_launches: 2, low_fit_scores: 1, expiring_contracts: 2, inactive_access: 1, stale_watermarks: 1, negative_margin: 1, retirement_candidates: 1 },
  actions: [
    { type: 'launch_blocked', severity: 'high', product_key: PRIMARY_PRODUCT, title: '중소기업 신용 인사이트 출시 차단', next_action: '법무 승인 요청', blocked_reasons: ['missing required approval: legal'], missing_approvals: ['legal'] },
    { type: 'approval_pending', severity: 'high', product_key: 'customer-360-signal', title: '고객 360 준법 승인 대기', next_action: '준법 검토 담당자 지정' },
    { type: 'contract_expiring', severity: 'medium', product_key: 'fraud-signal-api', title: '핀테크 A 계약 만료 예정', contract_key: 'CONTRACT-DEMO-01', customer_key: 'fintech-a', valid_to: '2026-09-18T00:00:00Z', next_action: '계약 갱신 검토' },
    { type: 'stale_watermark', severity: 'medium', asset_key: 'LEGACY_REPORT', title: '월간 리포트 최신성 지연', delay_status: 'delayed', next_action: '수집 파이프라인 재실행' },
    { type: 'negative_margin', severity: 'medium', product_key: 'campaign-opportunity', title: '캠페인 기회 추천 마진 경고', estimated_margin: -8.4, next_action: '모델 비용과 가격 정책 검토' },
    { type: 'retirement_candidate', severity: 'low', product_key: 'legacy-monthly-report', title: '레거시 리포트 종료 검토', recommendation: 'retire', reason: '최근 90일 활성 사용자가 없습니다.' },
  ],
}

const factoryRuns = [
  { id: 'RUN-20260828-006', run_type: 'product_definition', model: 'qwen3-235b', prompt_version: 'blueprint-v12', input_hash: 'demo-input-006', output_ref: 'artifact://demo/blueprint-006', parent_run_id: '', policy_decision: 'allowed', token_cost: 42.8, status: 'running', latency_ms: 1840, created_by: 'factory-demo', created_at: '2026-08-28T09:25:00Z' },
  { id: 'RUN-20260828-005', run_type: 'risk_review', model: 'qwen3-235b', prompt_version: 'risk-v8', input_hash: 'demo-input-005', output_ref: 'artifact://demo/risk-005', parent_run_id: 'RUN-20260828-004', policy_decision: 'blocked', token_cost: 31.4, status: 'completed', latency_ms: 2310, created_by: 'factory-demo', created_at: '2026-08-28T09:10:00Z' },
  { id: 'RUN-20260828-004', run_type: 'canvas_generation', model: 'qwen3-235b', prompt_version: 'canvas-v14', input_hash: 'demo-input-004', output_ref: 'artifact://demo/canvas-004', parent_run_id: '', policy_decision: 'allowed', token_cost: 38.2, status: 'completed', latency_ms: 1980, created_by: 'factory-demo', created_at: '2026-08-28T08:50:00Z' },
  { id: 'RUN-20260827-003', run_type: 'evidence_pack', model: 'local-llm-70b', prompt_version: 'evidence-v6', input_hash: 'demo-input-003', output_ref: 'artifact://demo/evidence-003', parent_run_id: '', policy_decision: 'approved', token_cost: 22.7, status: 'success', latency_ms: 1420, created_by: 'factory-demo', created_at: '2026-08-27T17:20:00Z' },
  { id: 'RUN-20260827-002', run_type: 'opportunity_scan', model: 'local-llm-70b', prompt_version: 'opportunity-v9', input_hash: 'demo-input-002', output_ref: 'artifact://demo/opportunity-002', parent_run_id: '', policy_decision: 'allowed', token_cost: 51.1, status: 'completed', latency_ms: 2740, created_by: 'factory-demo', created_at: '2026-08-27T15:10:00Z' },
  { id: 'RUN-20260827-001', run_type: 'readiness_summary', model: 'qwen3-32b', prompt_version: 'readiness-v4', input_hash: 'demo-input-001', output_ref: 'artifact://demo/readiness-001', parent_run_id: '', policy_decision: 'allowed', token_cost: 12.5, status: 'completed', latency_ms: 980, created_by: 'factory-demo', created_at: '2026-08-27T13:30:00Z' },
]

const canvas = {
  product_key: PRIMARY_PRODUCT,
  customer_problem: '중소기업 금융 심사에 필요한 데이터가 여러 시스템에 흩어져 있어 의사결정이 늦어집니다.',
  buyer: '은행·핀테크의 기업금융 책임자와 신용 리스크 담당자',
  use_cases: '대출 사전 심사, 한도 조정, 조기경보, 포트폴리오 모니터링',
  provided_data: '기업 기본정보, 현금흐름 지수, 신용 위험 등급, 설명 가능한 주요 영향 요인',
  differentiation: '준비도와 승인 증적이 연결된 설명 가능한 일별 신용 신호',
  pricing_model: '월 기본료 + API 호출 구간별 사용량 과금',
  risk_notes: '개인신용정보 정책과 목적 제한을 적용하고 응답 필드를 계약 범위로 마스킹합니다.',
  poc_success_criteria: '심사 시간 30% 단축, 조기경보 적중률 15% 향상, 운영 오류율 1% 미만',
  expected_revenue: '출시 12개월 내 연간 반복 매출 4.8억원',
  owner: '기업데이터상품팀', updated_by: 'demo-admin', created_at: '2026-06-01T00:00:00Z', updated_at: timestamp,
}

const approvals = [
  { id: 'approval-1', product_key: PRIMARY_PRODUCT, step: 'data_owner', status: 'approved', required: true, evidence_ref: 'evidence://demo/data-owner', notes: '자산 사용 목적과 품질 기준 확인', decided_by: '데이터 오너 데모', expires_at: '2027-08-28T00:00:00Z', created_at: timestamp, updated_at: timestamp },
  { id: 'approval-2', product_key: PRIMARY_PRODUCT, step: 'legal', status: 'pending', required: true, evidence_ref: '', notes: '외부 제공 약관 검토 중', decided_by: '', expires_at: '', created_at: timestamp, updated_at: timestamp },
  { id: 'approval-3', product_key: PRIMARY_PRODUCT, step: 'compliance', status: 'approved', required: true, evidence_ref: 'evidence://demo/compliance', notes: '가명처리와 목적 제한 정책 확인', decided_by: '준법 담당자 데모', expires_at: '2027-06-30T00:00:00Z', created_at: timestamp, updated_at: timestamp },
]

const gate = {
  product_key: PRIMARY_PRODUCT, strict_gate: true, allowed: false, minimum_readiness: 70,
  required_approvals: ['data_owner', 'legal', 'compliance'], approval_status: { data_owner: 'approved', legal: 'pending', compliance: 'approved' }, missing_approvals: ['legal'], missing_evidence: ['product_sla'],
  blocked_reasons: ['missing required approval: legal'], warnings: ['SLA targets are not configured'], asset_readiness: readiness.filter((item) => ['SME_PROFILE', 'FIN_TX'].includes(item.asset_key)), checked_at: timestamp,
  quality_passed: true, risk_reviewed: true, api_contract_configured: true, sla_configured: false, pricing_model_configured: true, masking_configured: true,
}

const evidencePack = {
  product_key: PRIMARY_PRODUCT,
  pack_json: JSON.stringify({ pack_id: 'EVIDENCE-DEMO-20260828', readiness: { SME_PROFILE: 94, FIN_TX: 84 }, approvals: { data_owner: 'approved', legal: 'pending', compliance: 'approved' }, controls: { masking: true, purpose_limitation: true, contract_scope: true }, generated_for_demo: true }),
  artifact_ref: 'evidence://demo/sme-credit-insight/v4', created_by: 'evidence-demo', created_at: '2026-08-28T08:30:00Z', updated_at: timestamp,
}

const graph = {
  nodes: [
    { id: 'asset:SME_PROFILE', type: 'asset', label: '중소기업 프로필', domain: '기업금융', sensitivity: 'pseudonymized' },
    { id: 'asset:FIN_TX', type: 'asset', label: '금융 거래 집계', domain: '금융', sensitivity: 'personal_credit' },
    { id: 'asset:FRAUD_EVENT', type: 'asset', label: '이상 거래 이벤트', domain: '리스크', sensitivity: 'restricted' },
    { id: `product:${PRIMARY_PRODUCT}`, type: 'product', label: '중소기업 신용 인사이트', status: 'approved', risk_score: 78, revenue_score: 92 },
    { id: 'product:fraud-signal-api', type: 'product', label: '이상 거래 탐지 신호', status: 'published', risk_score: 44, revenue_score: 95 },
    { id: 'product:cashflow-health-api', type: 'product', label: '기업 현금흐름 건강도', status: 'published', risk_score: 32, revenue_score: 88 },
    { id: 'customer:fintech-a', type: 'poc_outcome', label: '핀테크 A · PoC 성과', success: true },
    { id: 'customer:bank-b', type: 'proposal', label: '은행 B · 도입 검토', success: false },
    { id: 'customer:credit-c', type: 'poc_outcome', label: '신용평가 C · PoC 성과', success: true },
  ],
  edges: [
    { from: 'asset:SME_PROFILE', to: `product:${PRIMARY_PRODUCT}`, relation_type: 'source_for' },
    { from: 'asset:FIN_TX', to: `product:${PRIMARY_PRODUCT}`, relation_type: 'source_for' },
    { from: 'asset:FRAUD_EVENT', to: 'product:fraud-signal-api', relation_type: 'source_for' },
    { from: 'asset:FIN_TX', to: 'product:cashflow-health-api', relation_type: 'source_for' },
    { from: `product:${PRIMARY_PRODUCT}`, to: 'customer:bank-b', relation_type: 'proposal' },
    { from: `product:${PRIMARY_PRODUCT}`, to: 'customer:credit-c', relation_type: 'poc' },
    { from: 'product:fraud-signal-api', to: 'customer:fintech-a', relation_type: 'poc' },
  ],
  relationships: [],
}

const personalDashboard = {
  user_id: 'demo-admin',
  today: { requests: 184, tokens: 428120, cost_krw: 18420, errors: 2 },
  month: { requests: 4820, tokens: 12840500, cost_krw: 428500, errors: 31 },
  profile: { requests: 4820, total_cost_krw: 428500, avg_cost_per_request: 88.9, avg_latency_ms: 684, success_rate: .993, error_rate: .007, cache_rate: .38, text2sql_usage_rate: .24, mcp_usage_rate: .57, risk_score: 8, summary: '내부 모델과 MCP 도구를 안정적으로 활용하고 있습니다. 만료 예정 키 1개를 미리 회전하세요.' },
  potential_savings_krw: 68200, potential_savings_model: 'local-llm-70b',
  key_alerts: [{ id: 'key-demo-2', name: '리서치 자동화', severity: 'medium', flags: ['expires_soon'] }], recent_failures: [],
}

const keyResponse = {
  role: '서비스 관리자',
  grantable_scopes: ['chat:completion', 'embeddings:create', 'models:read', 'routing:read', 'observability:read', 'costs:read', 'mcp:use', 'team:read'],
  api_keys: [
    { id: 'key-demo-analytics', name: '분석 노트북', role: 'operator', status: 'active', scopes: ['chat:completion', 'models:read', 'mcp:use'], allowed_ips: ['192.0.2.0/24'], allowed_models: ['qwen-*', 'local-*'], denied_models: ['external-*'], allowed_providers: ['internal'], denied_providers: ['external'], budget_limit_krw: 120000, expires_at: '2027-08-31T23:59:59Z', created_at: '2026-07-01T00:00:00Z' },
    { id: 'key-demo-research', name: '리서치 자동화', role: 'developer', status: 'active', scopes: ['chat:completion', 'embeddings:create'], allowed_ips: [], allowed_models: ['local-*'], denied_models: [], allowed_providers: ['internal'], denied_providers: [], budget_limit_krw: 50000, expires_at: '2026-09-30T23:59:59Z', created_at: '2026-08-01T00:00:00Z' },
  ],
}

function runtimeSetting(key: string, category: string, type: string, value: string, description: string) {
  return { key, category, type, is_secret: false, value, source: 'admin', effective_source: 'admin', restart_required: false, read_only: false, description, permission_group: 'admin', can_write: true }
}

const settings = [
  runtimeSetting('ai.default_stream', 'ai.runtime', 'bool', 'true', 'AI 응답을 기본적으로 실시간 스트리밍합니다.'),
  runtimeSetting('limits.max_output_tokens', 'limits.ai', 'int', '262144', '일반 AI 호출의 최대 출력 토큰입니다.'),
  runtimeSetting('limits.agent_max_tokens', 'limits.ai', 'int', '131072', '에이전트 실행의 최대 출력 토큰입니다.'),
  runtimeSetting('mcp.agentic_model', 'mcp.runtime', 'string', 'qwen3-235b', 'MCP 에이전트가 사용하는 기본 모델입니다.'),
  runtimeSetting('mcp.max_tokens', 'mcp.runtime', 'int', '65536', 'MCP 단일 응답 최대 토큰입니다.'),
  runtimeSetting('mcp.max_agent_steps', 'mcp.runtime', 'int', '12', 'MCP 에이전트의 최대 실행 단계입니다.'),
  runtimeSetting('mcp.max_tools', 'mcp.runtime', 'int', '24', '한 실행에서 사용할 수 있는 최대 도구 수입니다.'),
  runtimeSetting('mcp.force_tool_first', 'mcp.runtime', 'bool', 'false', '첫 단계 도구 사용 강제 여부입니다.'),
  runtimeSetting('security.session_timeout', 'security.session', 'duration', '8h', '관리자 세션 만료 시간입니다.'),
  runtimeSetting('observability.audit_retention_days', 'observability.audit', 'int', '365', '감사 이벤트 보존 기간입니다.'),
]

const providers = {
  providers: [
    { name: 'internal-qwen', base_url: 'https://ai-gateway.example.invalid/v1', api_key_configured: true, timeout_ms: 600000, enabled: true, model_patterns: 'qwen-*,local-*', created_at: '2026-07-01T00:00:00Z' },
    { name: 'offline-vllm', base_url: 'https://vllm.example.invalid/v1', api_key_configured: true, timeout_ms: 900000, enabled: true, model_patterns: 'offline-*', created_at: '2026-08-01T00:00:00Z' },
  ],
}

const keycloak = {
  enabled: true, issuer_url: 'https://sso.example.invalid/realms/dataworks', client_id: 'dataworks-demo', client_secret_set: true,
  redirect_uri: 'https://dataworks.example.invalid/auth/keycloak/callback', scopes: ['openid', 'profile', 'email'], default_role: 'developer', role_claim: 'realm_access.roles', group_claim: 'groups', allow_local_login: true,
  role_map: { dataworks_admin: 'service_admin', dataworks_operator: 'operator', dataworks_user: 'developer' }, source: 'db', updated_at: timestamp,
}

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: 'application/json; charset=utf-8', body: JSON.stringify(body) })
}

export async function installDemoRoutes(page: Page) {
  let authenticated = false
  const unexpected: string[] = []
  const localOrigin = 'http://127.0.0.1:5173'

  await page.route('**/*', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname
    const method = request.method()

    if (url.origin !== localOrigin) {
      unexpected.push(`${method} ${url.origin}${path}`)
      await route.abort('blockedbyclient')
      return
    }

    if (path === '/auth/me' && method === 'GET') {
      await json(route, authenticated
        ? { auth_enabled: true, version: DEMO_VERSION, user: { id: 'demo-admin', email: 'admin@dataworks.example', name: '데모 관리자', role: 'super_admin', team_id: 'demo-team', scopes: ['admin:read', 'admin:write'] } }
        : { error: 'authentication required' }, authenticated ? 200 : 401)
      return
    }
    if (path === '/auth/sso/status' && method === 'GET') {
      await json(route, { keycloak_enabled: true, login_url: '/auth/keycloak/login', version: DEMO_VERSION })
      return
    }
    if (path === '/admin/dataworks/home' && method === 'GET') {
      await json(route, { dashboard: { total_assets: assets.length, total_products: products.length, published_products: 3, review_pending: 3, high_risk: 2, poc_pending: 2, ideas_total: 14, avg_revenue_score: 82 }, top_products: products.slice(0, 5).map((item) => ({ product_key: item.product_key, name: item.name_ko, revenue_score: item.revenue_score, risk_score: item.risk_score, status: item.status })) })
      return
    }
    if (path === '/admin/dataworks/action-center' && method === 'GET') { await json(route, actionCenter); return }
    if (path === '/admin/dataworks/assets' && method === 'GET') { await json(route, { assets }); return }
    if (path === '/admin/dataworks/assets/readiness' && method === 'GET') { await json(route, { readiness }); return }
    if (path === '/admin/dataworks/products' && method === 'GET') { await json(route, { products }); return }
    if (path === '/admin/dataworks/factory/runs' && method === 'GET') { await json(route, { runs: factoryRuns, evaluation_summaries: { quality: 91, risk: 12 } }); return }
    if (path === '/admin/dataworks/portfolio/graph' && method === 'GET') { await json(route, { graph }); return }

    const workspaceMatch = path.match(/^\/admin\/dataworks\/products\/([^/]+)\/(canvas|approvals|evidence-pack|publish-gate|contract-versions)$/)
    if (workspaceMatch && method === 'GET') {
      const operation = workspaceMatch[2]
      if (operation === 'canvas') await json(route, { canvas, draft: false })
      if (operation === 'approvals') await json(route, { approvals })
      if (operation === 'evidence-pack') await json(route, { evidence_pack: evidencePack })
      if (operation === 'publish-gate') await json(route, { publish_gate: gate })
      if (operation === 'contract-versions') await json(route, { contract_version: { id: 'contract-demo-v3', product_key: PRIMARY_PRODUCT, version: 3, contract_json: '{"demo":true}', status: 'active', created_by: 'contract-demo', created_at: '2026-08-20T00:00:00Z' } })
      return
    }

    if (path === '/me/dashboard' && method === 'GET') { await json(route, personalDashboard); return }
    if (path === '/me/keys' && method === 'GET') { await json(route, keyResponse); return }
    if (path === '/admin/providers' && method === 'GET') { await json(route, providers); return }
    if (path === '/admin/settings/effective' && method === 'GET') { await json(route, { settings }); return }
    if (path === '/admin/sso/keycloak/config' && method === 'GET') { await json(route, keycloak); return }
    if (path === '/v1/chat/completions' && method === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream; charset=utf-8',
        body: [
          'data: {"choices":[{"delta":{"content":"현재 상품은 법무 승인이 대기 중이며 SLA 목표가 설정되지 않아 출시가 차단되었습니다. "}}]}',
          'data: {"choices":[{"delta":{"content":"법무 승인 요청 후 SLA 기준을 저장하고 출시 게이트를 다시 점검하세요."}}]}',
          'data: [DONE]',
          '',
        ].join('\n\n'),
      })
      return
    }

    const isAPI = ['/admin/', '/auth/', '/me/', '/v1/'].some((prefix) => path.startsWith(prefix))
    if (isAPI) {
      unexpected.push(`${method} ${path}`)
      await json(route, { error: 'unexpected demo request' }, 418)
      return
    }
    await route.continue()
  })

  return {
    setAuthenticated(value: boolean) { authenticated = value },
    unexpected,
  }
}
