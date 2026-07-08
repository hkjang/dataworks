# Data Works

**Data Works**는 내부 데이터 자산을 판매 가능한 데이터 상품, API 상품, 분석 리포트, PoC 제안 패키지로 전환하는 **Data Product Factory**입니다.

기존 게이트웨이 코어, Admin UI, SQLite/PostgreSQL 저장소, 감사 로그, 승인 흐름, Text2SQL 수요 분석, ClickHouse 분석 기반은 재활용하되, 기본 경험은 Kubernetes 운영 허브가 아니라 데이터 상품화 워크플로우에 맞춰 전환합니다.

## 핵심 흐름

| 단계 | 내용 |
| --- | --- |
| 자산 등록 | 데이터셋, 테이블, API, 리포트 후보를 내부 자산으로 등록 |
| 준비도 평가 | 스키마, 최신성, 샘플, 결측, 민감도, 외부 제공 가능성, API/과금 준비도를 100점 기준으로 평가 |
| 아이디어 생성 | 업종, 고객군, 시장 니즈, 데이터 자산을 기반으로 상품 후보 생성 |
| Product Canvas | 고객 문제, 구매자, 사용 시나리오, 제공 데이터, 차별점, 가격, 리스크, PoC 성공 기준 정리 |
| 리스크 검토 | 개인정보, 개인신용정보, 가명정보, AI 사용, 외부 제공, 보안 리스크 체크 |
| 승인 Trace | 데이터 오너, 법무, 컴플라이언스 승인과 증적 연결 |
| Evidence Pack | 상품 정의, 자산 출처, 준비도, 리스크 검토, 승인, API 계약, PoC 계획을 감사 가능한 JSON pack으로 생성 |
| 출시 Gate | high-risk/sensitive 상품은 준비도, 승인, 증적 없이는 `published` 전환 차단 |
| 운영 분석 | funnel, portfolio graph, factory run, 매출/리스크 지표 조회 |

## 주요 API

| Method | Path | 설명 |
| --- | --- | --- |
| `GET` | `/admin/dataworks/home` | Data Works Home 지표 |
| `GET` | `/admin/dataworks/assets` | 데이터 자산 목록 조회 및 업서트 |
| `POST` | `/admin/dataworks/assets/{key}/readiness/check` | 데이터 자산 상품화 준비도 평가 |
| `GET` | `/admin/dataworks/assets/{key}/lineage` | 데이터 자산 기반 상품 lineage 조회 |
| `GET` | `/admin/dataworks/home` | Data Works Home KPI |
| `GET` | `/admin/dataworks/action-center` | 운영 액션 센터: 출시 차단, 승인 대기, 만료 임박 계약, 비활성 권한 |
| `GET/POST` | `/admin/dataworks/customer-segments` | 고객 세그먼트 등록 및 조회 |
| `GET/POST` | `/admin/dataworks/assets` | 데이터 자산 목록 조회 및 업서트 |
| `GET/POST` | `/admin/dataworks/assets/readiness` | 자산 준비도 점수 조회 및 업서트 |
| `POST` | `/admin/dataworks/factory/ideas` | 데이터 상품 아이디어 생성 |
| `POST` | `/admin/dataworks/factory/definitions` | 아이디어 기반 상품 정의서 생성 |
| `POST` | `/admin/dataworks/factory/api-spec` | API 규격 설계 생성 |
| `POST` | `/admin/dataworks/factory/report-spec` | 분석 리포트 규격 설계 생성 |
| `GET/POST` | `/admin/dataworks/products/{key}/canvas` | Product Canvas 조회 및 저장 |
| `GET/POST` | `/admin/dataworks/products/{key}/approvals` | 승인 trace matrix 조회 및 저장 |
| `GET/POST` | `/admin/dataworks/products/{key}/evidence-pack` | Evidence Pack 조회 및 생성 |
| `GET` | `/admin/dataworks/products/{key}/publish-gate` | 출시 가능 여부와 차단 사유 조회 |
| `POST` | `/admin/dataworks/products/{key}/publish` | publish gate 통과 시 상품 출시 |
| `GET` | `/admin/dataworks/products/{key}/openapi` | 상품 API용 OpenAPI 3.1 문서 생성 |
| `GET/POST` | `/admin/dataworks/products/{key}/contract-versions` | 계약 버전 조회 및 추가 |
| `GET/POST` | `/admin/dataworks/products/{key}/fit-scores` | 고객 세그먼트별 Market Fit Score 계산 및 수동 보정 |
| `GET/POST` | `/admin/dataworks/products/{key}/versions` | 상품 정의 스냅샷 저장 및 변경 이력 관리 |
| `GET` | `/admin/dataworks/products/{key}/version-diff?from=1&to=2` | 상품 버전 스냅샷 비교 |
| `GET/POST` | `/admin/dataworks/products/{key}/contract-scopes` | 고객 계약별 허용 필드, 호출 한도, 기간, 목적, 제한 조건 관리 |
| `GET/POST` | `/admin/dataworks/products/{key}/entitlements` | API 키를 상품 계약 Scope에 연결 |
| `GET/POST` | `/admin/dataworks/products/{key}/sla` | 상품별 refresh cycle, latency, availability, support level 관리 |
| `GET/POST` | `/admin/dataworks/products/{key}/watermarks` | 상품/자산별 데이터 최신성 Watermark 관리 |
| `GET/POST` | `/admin/dataworks/products/{key}/costs` | 상품별 query, LLM, 운영, 가공 비용과 예상 margin 관리 |
| `GET/POST` | `/admin/dataworks/products/{key}/proposal-ab` | 고객별 제안서 A/B/C variant 생성 및 이벤트 이력 조회 |
| `GET/POST` | `/admin/dataworks/products/{key}/retirement` | 미사용, 저성과, 고위험, stale 데이터 기반 개선/폐기 추천 |
| `POST` | `/v1/data-products/{key}/query` | published 상태, Entitlement, Contract Scope, 만료 조건을 검사하는 런타임 상품 API |
| `POST` | `/admin/dataworks/risk/check` | 규제·보안 리스크 체크리스트 |
| `POST` | `/admin/dataworks/poc/plans` | PoC 계획 생성 |
| `POST` | `/admin/dataworks/poc/{id}/outcome` | PoC 결과 및 계약 후보 전환 상태 기록 |
| `POST` | `/admin/dataworks/scoring/evaluate` | 우선순위·매출 가능성 평가 |
| `POST` | `/admin/dataworks/proposals` | 고객군별 제안 패키지 생성 |
| `POST` | `/admin/dataworks/proposals/{id}/feedback` | 제안 결과, 반응, 거절 사유, 다음 액션 기록 |
| `POST` | `/admin/dataworks/similarity/check` | 기존 상품과 유사도 비교 |
| `GET` | `/admin/dataworks/reviews` | 승인 검토 대기 상품 목록 |
| `POST` | `/admin/dataworks/reviews/{key}/approve\|reject` | 상품 상태 승인/반려 |
| `GET` | `/admin/dataworks/portfolio` | 상태별 라이프사이클 상품 조회 |
| `GET` | `/admin/dataworks/portfolio/graph` | 자산·상품·제안·PoC 관계 그래프 조회 |
| `GET` | `/admin/dataworks/portfolio/graph` | 상품·자산·승인 관계 그래프 |
| `GET` | `/admin/dataworks/funnel` | 아이디어부터 출시까지 funnel 분석 |
| `GET` | `/admin/dataworks/analytics` | 상품 매출/리스크 성과 분석 |
| `GET` | `/admin/dataworks/analytics/funnel` | 아이디어 → 정의서 → 리스크 → 제안 → PoC → 출시 funnel |
| `GET` | `/admin/dataworks/factory/runs` | AI 워크런 수행 이력 조회 |
| `GET` | `/admin/dataworks/products` | 상품 후보·정의서 목록 |
| `POST` | `/admin/dataworks/products/{key}/canvas/generate` | Product Canvas 생성 및 저장 |
| `GET` | `/admin/dataworks/products/{key}/evidence` | Product Evidence Pack 조회 |
| `POST` | `/admin/dataworks/products/{key}/regulatory-trace` | Regulatory Trace Matrix 생성 또는 교체 |
| `POST` | `/admin/dataworks/products/{key}/api-contract` | OpenAPI 기반 API 상품 계약 생성 |
| `POST` | `/admin/dataworks/products/{key}/mock` | Mock API 응답 생성 및 호출 로그 기록 |
| `GET` | `/admin/dataworks/products/{key}/funnel` | 상품별 전환 funnel 조회 |
| `GET` | `/admin/dataworks/products` | 기존 카탈로그 호환 목록 |

기존 `/admin/data-products` 카탈로그는 확장 필드(`target_customers`, `pricing_model`, `api_spec`, `poc_plan`, `risk_score`, `revenue_score` 등)를 포함하며, `status=published` 업서트 시 동일한 publish gate를 적용합니다.

## Publish Gate

`risk_score >= 70` 이거나 `sensitivity`가 `restricted`, `personal_credit`, `pseudonymized` 등 민감 프로필인 상품은 엄격 게이트를 적용합니다.

출시 조건:

- 연결된 `source_ref` 자산마다 Asset Readiness Score `70` 이상
- `data_owner`, `legal`, `compliance` 승인 trace가 `approved` 또는 `waived`
- Evidence Pack 생성 완료
- 만료된 승인 trace는 통과 증적으로 인정하지 않음

조건을 만족하지 않으면 publish 요청은 `409 Conflict`와 `publish_gate.blocked_reasons`를 반환합니다.

## 실행

```powershell
$env:GATEWAY_SECRET = "dev-only-secret"
$env:ADMIN_TOKEN    = "dev-admin"
go run ./cmd/dataworks
```

기동 로그에 `Data Works listening`이 보이면 정상입니다.

Admin UI: `http://localhost:8080/admin`

## Docker

```bash
docker build -t dataworks:dev .
docker run -d --name dataworks --restart=always -p 8080:8080 -v "$PWD/data:/data" \
  -e GATEWAY_SECRET="$(openssl rand -hex 32)" \
  -e ADMIN_TOKEN="$(openssl rand -hex 32)" \
  dataworks:dev
```

`docker-compose.yml`은 기본 이미지명을 `dataworks`로 사용합니다.

## 저장소

| 저장소 | 용도 |
| --- | --- |
| SQLite | 개발 및 PoC 기본값 (`data/gateway.db`) |
| PostgreSQL | 운영 환경 권장 저장소 |
| ClickHouse | 장기 분석 fact 적재와 성과 분석 선택 구성 |

## 레거시 K8s 기능

Kubernetes 수집, Pod 로그, RCA, Stack, ConfigMap, CRD Discovery 등 기존 운영 허브 기능은 제거하지 않고 `k8s_ops` 기능 플래그 아래에 격리합니다. Data Works 기본 메뉴와 문서는 상품화 워크플로우를 우선합니다.

레거시 참고 문서:

- [K8s 운영 허브 가이드](docs/K8S_OPERATIONS_HUB.md)
- [K8s Agent 가이드](docs/K8S_AGENT.md)

## 문서

- [운영 가이드](docs/OPERATIONS.md)
- [관리자 가이드](docs/ADMIN_GUIDE.md)
- [안전 및 보안 거버넌스 가이드](docs/SAFETY_GUIDE.md)
- [PostgreSQL 가이드](docs/POSTGRES_GUIDE.md)
- [릴리즈 가이드](docs/RELEASE_GUIDE.md)

## 릴리즈

변경 이력은 [scripts/changelog.txt](scripts/changelog.txt)를 기준으로 관리합니다.
