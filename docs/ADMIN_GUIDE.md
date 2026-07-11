# Data Works 관리자 가이드

이 문서는 Data Works Admin UI와 관리 API를 이용해 데이터 자산을 상품 후보, 승인 가능한 상품 정의, API 계약, Evidence Pack, 출시 상태로 운영하는 방법을 설명합니다.

## 1. 접속과 권한

- Admin UI: `http://<host>:8080/admin`
- 관리자 토큰: `ADMIN_TOKEN`
- 기본 홈: Factory Home
- 관리 API: `Authorization: Bearer <ADMIN_TOKEN>`

개발 환경 예시:

```powershell
$env:GATEWAY_SECRET = "dev-only-secret"
$env:ADMIN_TOKEN    = "dev-admin"
go run ./cmd/dataworks
```

## 2. Factory Home

Factory Home은 전체 상품화 funnel을 보는 시작 화면입니다.

확인할 지표:

- 등록된 데이터 자산 수
- 전체 상품 후보 수
- published 상품 수
- review/risk_review 대기 수
- high-risk 상품 수
- PoC 대기 수
- 평균 revenue score

API:

- `GET /admin/dataworks/home`
- `GET /admin/dataworks/funnel`
- `GET /admin/dataworks/analytics`

## 3. 데이터 자산

데이터 자산은 상품의 원재료입니다. 자산에는 `asset_key`, 이름, 도메인, owner, 컬럼 요약, 민감도, 갱신 주기를 기록합니다.

API:

- `GET /admin/dataworks/assets`
- `POST /admin/dataworks/assets`

예시:

```json
{
  "asset_key": "loan_history",
  "name": "Loan History",
  "domain": "credit",
  "owner": "data-platform",
  "columns_summary": "customer segment, repayment history, delinquency band",
  "sensitivity": "personal_credit",
  "refresh_cycle": "daily"
}
```

## 4. Asset Readiness Score

상품 출시 전 자산 준비도를 100점 기준으로 관리합니다. `overall_score`를 직접 입력하지 않으면 다음 항목의 평균으로 계산합니다.

- schema
- freshness
- sample
- missingness
- sensitivity
- external sharing
- API readiness
- billing readiness

API:

- `GET /admin/dataworks/assets/readiness`
- `GET /admin/dataworks/assets/readiness?asset_key=loan_history`
- `POST /admin/dataworks/assets/readiness`

High-risk/sensitive 상품은 연결 자산의 readiness가 70 미만이면 출시가 차단됩니다.

## 5. 아이디어와 상품 정의

아이디어 생성은 업종, 고객군, 시장 니즈, 데이터 자산을 입력해 후보를 만듭니다.

API:

- `POST /admin/dataworks/factory/ideas`
- `POST /admin/dataworks/factory/definitions`
- `POST /admin/dataworks/similarity/check`
- `POST /admin/dataworks/scoring/evaluate`

상품 정의 결과는 `/admin/dataworks/products`와 기존 `/admin/data-products` 카탈로그에서 함께 조회됩니다.

## 6. Product Canvas

Product Canvas는 상품 정의의 편집 가능한 사업 문서입니다.

관리 항목:

- 고객 문제
- 구매자
- 사용 시나리오
- 제공 데이터
- 차별점
- 가격 모델
- 리스크 메모
- PoC 성공 기준
- 예상 매출

API:

- `GET /admin/dataworks/products/{key}/canvas`
- `POST /admin/dataworks/products/{key}/canvas`

저장된 canvas가 없으면 현재 상품 메타데이터를 바탕으로 draft canvas를 반환합니다.

## 7. 리스크 검토

리스크 검토는 개인정보, 개인신용정보, 가명정보 결합, AI 사용, 외부 제공, 보안 리스크를 점검합니다.

API:

- `POST /admin/dataworks/risk/check`
- `GET /admin/dataworks/reviews`
- `POST /admin/dataworks/reviews/{key}/approve`
- `POST /admin/dataworks/reviews/{key}/reject`

`risk_score >= 70`인 상품은 strict publish gate 대상입니다.

## 8. Approval Trace Matrix

출시 승인 증적은 상품별 trace로 관리합니다.

필수 step:

- `data_owner`
- `legal`
- `compliance`

권장 step:

- `security`
- `sales`
- `contract`

API:

- `GET /admin/dataworks/products/{key}/approvals`
- `POST /admin/dataworks/products/{key}/approvals`

예시:

```json
{
  "step": "legal",
  "status": "approved",
  "required": true,
  "evidence_ref": "legal-memo-2026-07",
  "notes": "External sharing scope approved"
}
```

`expires_at`이 지난 승인은 publish gate에서 만료로 처리됩니다.

## 9. Evidence Pack

Evidence Pack은 출시·감사·고객 PoC 대응에 필요한 근거 묶음입니다.

포함 항목:

- 상품 메타데이터
- Product Canvas
- source asset lineage
- Asset Readiness Score
- 최신 상품 정의
- 최신 risk review
- approval trace
- PoC plan
- API contract
- contract version
- publish gate 결과

API:

- `GET /admin/dataworks/products/{key}/evidence-pack`
- `POST /admin/dataworks/products/{key}/evidence-pack`

본문 없이 `POST`하면 서버가 현재 저장된 정보를 기준으로 Evidence Pack JSON을 생성합니다.

## 10. API 상품

API 상품은 정의서의 `api_spec`을 기반으로 OpenAPI 3.1 문서를 생성할 수 있습니다.

API:

- `POST /admin/dataworks/factory/api-spec`
- `GET /admin/dataworks/products/{key}/openapi`
- `GET/POST /admin/dataworks/products/{key}/contract-versions`
- `GET/POST /admin/dataworks/products/{key}/contract-scopes`
- `GET/POST /admin/dataworks/products/{key}/entitlements`
- `POST /v1/data-products/{key}/query`

계약 버전은 고객 제공 범위, rate limit, 오류 정책, 과금 조건 변경 이력을 남기는 용도로 사용합니다.

## 11. 운영 전환 기능

Action Center는 출시 차단, 승인 대기, 낮은 Fit Score, 만료 임박 계약, 비활성 Entitlement를 한 화면/API에서 모아 보여줍니다.

API:

- `GET /admin/dataworks/action-center`
- `GET/POST /admin/dataworks/customer-segments`
- `GET/POST /admin/dataworks/products/{key}/fit-scores`
- `GET/POST /admin/dataworks/products/{key}/versions`
- `GET /admin/dataworks/products/{key}/version-diff?from=1&to=2`
- `GET/POST /admin/dataworks/products/{key}/sla`
- `GET/POST /admin/dataworks/products/{key}/watermarks`
- `GET/POST /admin/dataworks/products/{key}/costs`
- `GET/POST /admin/dataworks/products/{key}/proposal-ab`
- `GET/POST /admin/dataworks/products/{key}/retirement`

운영 순서:

1. 고객군을 `/customer-segments`에 등록합니다.
2. 상품별 `/fit-scores`를 계산해 제안 우선순위를 확인합니다.
3. 주요 변경 전후에 `/versions`로 스냅샷을 저장하고 `/version-diff`로 승인자에게 변경 요약을 제공합니다.
4. `/contract-scopes`에 고객별 허용 필드, 호출 한도, 기간, 목적을 등록합니다.
5. `/entitlements`로 API 키를 계약 Scope에 연결한 뒤 고객에게 `/v1/data-products/{key}/query`를 제공합니다.
6. `/sla`, `/watermarks`, `/costs`로 실제 운영 가능성과 수익성을 지속 관리합니다.
7. `/proposal-ab`로 고객 접점별 제안 문구를 만들고 `/retirement`로 저성과 상품의 개선/폐기 후보를 관리합니다.

## 12. 출시

출시 전 확인:

1. 상품 상태가 `approved`인지 확인
2. source asset readiness가 70 이상인지 확인
3. 필수 approval trace가 승인되었는지 확인
4. Evidence Pack이 생성되었는지 확인
5. `/publish-gate`로 차단 사유가 없는지 확인

API:

- `GET /admin/dataworks/products/{key}/publish-gate`
- `POST /admin/dataworks/products/{key}/publish`

기존 호환 경로에서도 동일한 gate가 적용됩니다.

- `POST /admin/factory/products/{key}/publish`
- `POST /admin/data-products` with `status=published`

## 13. PoC와 제안

PoC와 제안 패키지는 상품 출시 전 고객 검증과 영업 자료 생성을 돕습니다.

API:

- `POST /admin/dataworks/poc/plans`
- `POST /admin/dataworks/proposals`
- `POST /admin/dataworks/factory/report-spec`

PoC 결과 관리와 proposal feedback loop는 이후 단계에서 funnel 분석과 연결해 확장합니다.

## 14. 포트폴리오와 분석

운영자는 상품 상태별 포트폴리오와 관계 그래프를 확인합니다.

API:

- `GET /admin/dataworks/portfolio`
- `GET /admin/dataworks/portfolio/graph`
- `GET /admin/dataworks/funnel`
- `GET /admin/dataworks/analytics`
- `GET /admin/dataworks/factory/runs`
- `GET /admin/dataworks/analytics/funnel?days=30`

Portfolio graph는 product, asset, proposal feedback, PoC outcome node와 관계 edge 및 영속화된 relationship 목록을 반환합니다. Funnel 분석은 조회 시점의 일별 스냅샷을 저장해 기간별 추이를 함께 제공합니다.

## 15. Factory Run 재현성과 평가

Factory 단계별 프롬프트는 `template_key`와 자동 증가 `version`으로 관리합니다. 새 `active` 버전을 등록하면 같은 키의 기존 active 버전은 `retired`로 전환됩니다.

API:

- `GET /admin/dataworks/prompt-templates?run_type=products.define&status=active`
- `POST /admin/dataworks/prompt-templates`
- `POST /admin/dataworks/factory/runs/{id}/replay`
- `POST /admin/dataworks/factory/runs/{id}/evaluate`

재실행은 원본 `input_hash`와 `parent_run_id`를 보존하고 모델, 프롬프트 버전, 정책 판단, 토큰 비용을 별도 실행 이력으로 남깁니다. 평가는 정확도, 유용성, 리스크 통제, 출력 품질을 0~100점으로 기록하며 실행 목록에서 집계 점수를 확인할 수 있습니다.

## 16. 레거시 K8s 메뉴

K8s 운영 기능은 Data Works 기본 흐름이 아닙니다. 필요한 경우 `k8s_ops` 기능 플래그와 레거시 문서를 사용하세요.

- [K8s 운영 허브 가이드](K8S_OPERATIONS_HUB.md)
- [K8s Agent 가이드](K8S_AGENT.md)
