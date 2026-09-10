# Data Works 운영 가이드

Data Works의 기동, 관측, 백업, publish gate 운영 절차를 정리합니다. Kubernetes 운영 허브 기능은 레거시 참고 영역이며, 기본 운영 대상은 Data Product Factory입니다.

## 1. 사전 준비

| 항목 | 값 |
| --- | --- |
| Go | `go.mod` 기준 1.25 |
| 기본 포트 | `:8080` |
| 기본 DB | SQLite `data/gateway.db` |
| 운영 DB | PostgreSQL 권장 |
| 필수 환경 변수 | `GATEWAY_SECRET`, `ADMIN_TOKEN` |

```powershell
$env:GATEWAY_SECRET = "replace-with-32-byte-secret"
$env:ADMIN_TOKEN    = "replace-with-admin-token"
```

`GATEWAY_SECRET`은 provider key 암호화에 쓰입니다. 운영 중 변경하면 기존 암호화 값을 복호화하지 못할 수 있으므로 안전하게 보관하세요.

## 2. 기동

### 로컬 개발

```powershell
$env:GATEWAY_SECRET = "dev-only-secret"
$env:ADMIN_TOKEN    = "dev-admin"
go run ./cmd/dataworks
```

정상 기동 로그:

```text
Data Works listening addr=:8080 database=sqlite
```

Admin UI: `http://localhost:8080/admin`

### 바이너리 빌드

```bash
go build -trimpath -ldflags "-s -w" -o dataworks ./cmd/dataworks
./dataworks
```

### Docker

```bash
docker build -t dataworks:dev .
docker run -d --name dataworks --restart=always \
  -p 8080:8080 \
  -v /opt/dataworks/data:/data \
  -e GATEWAY_SECRET="$(openssl rand -hex 32)" \
  -e ADMIN_TOKEN="$(openssl rand -hex 32)" \
  dataworks:dev
```

## 3. 운영 확인

| 확인 | 명령/API |
| --- | --- |
| 프로세스 상태 | `GET /healthz` |
| 메트릭 | `GET /metrics` |
| Data Works KPI | `GET /admin/dataworks/home` |
| Action Center | `GET /admin/dataworks/action-center` |
| Factory 실행 이력 | `GET /admin/dataworks/factory/runs` |
| Prompt Registry | `GET /admin/dataworks/prompt-templates` |
| Funnel 일별 추이 | `GET /admin/dataworks/analytics/funnel?days=30` |
| Portfolio graph | `GET /admin/dataworks/portfolio/graph` |
| 시스템 오류 | `GET /admin/system-errors` |

관리 API는 `Authorization: Bearer <ADMIN_TOKEN>` 또는 Admin UI 토큰 입력을 사용합니다.

## 4. Publish Gate 운영

High-risk 또는 민감 데이터 상품은 다음 조건 없이는 `published`로 전환되지 않습니다.

1. `source_ref`에 연결된 모든 자산의 Asset Readiness Score가 70 이상
2. `data_owner`, `legal`, `compliance` 승인 trace가 `approved` 또는 `waived`
3. Evidence Pack이 생성되어 있음
4. 승인 trace가 만료되지 않았음

운영 절차:

```bash
# 1. 자산 준비도 등록
curl -X POST "$BASE/admin/dataworks/assets/readiness" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"asset_key":"loan_history","schema_score":90,"freshness_score":90,"sample_score":90,"missingness_score":90,"sensitivity_score":90,"external_sharing_score":90,"api_readiness_score":90,"billing_readiness_score":90}'

# 2. 승인 trace 등록
curl -X POST "$BASE/admin/dataworks/products/dw_credit_score/approvals" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"step":"legal","status":"approved","evidence_ref":"legal-memo-2026-07"}'

# 3. Evidence Pack 생성
curl -X POST "$BASE/admin/dataworks/products/dw_credit_score/evidence-pack" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}'

# 4. Gate 확인
curl "$BASE/admin/dataworks/products/dw_credit_score/publish-gate" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

차단 시 `409 Conflict`와 함께 `publish_gate.blocked_reasons`가 내려옵니다.

## 5. Contract Scope와 API Entitlement

런타임 API 상품은 다음 조건을 모두 통과해야 `POST /v1/data-products/{key}/query`를 사용할 수 있습니다.

1. 상품 상태가 `published`
2. 호출 API 키에 해당 상품 Entitlement가 존재하고 `active`
3. Entitlement가 연결한 Contract Scope가 `active`이고 유효 기간 안에 있음
4. 요청 필드가 Contract Scope의 `allowed_fields` 안에 있음
5. 민감 상품은 Contract Scope에 `purpose`가 지정되어 있음

```bash
# 1. 고객 계약 Scope 등록
curl -X POST "$BASE/admin/dataworks/products/dw_credit_score/contract-scopes" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"contract_key":"ct_bank","customer_key":"cust_bank","allowed_fields":["score","risk_band"],"rate_limit":100,"valid_to":"2026-12-31T00:00:00Z","purpose":"credit risk monitoring"}'

# 2. API 키 Entitlement 등록
curl -X POST "$BASE/admin/dataworks/products/dw_credit_score/entitlements" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"api_key_id":"key_bank","customer_key":"cust_bank","contract_key":"ct_bank","scope":"data_product:query","expires_at":"2026-12-31T00:00:00Z"}'

# 3. 고객 런타임 호출
curl -X POST "$BASE/v1/data-products/dw_credit_score/query" \
  -H "Authorization: Bearer $CUSTOMER_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"fields":["score","risk_band"]}'
```

`fields`가 계약 범위를 벗어나면 `403`과 `forbidden_fields`가 반환됩니다.

`allowed_fields` 는 최소 한 개 이상이어야 하며(전체 스키마 허용은 `["*"]`), 비어 있거나 공백뿐이면
`400 invalid_allowed_fields` 로 거부됩니다. 값이 없는 Contract Scope 는 런타임에서 모든 조회를 `403`
(`empty_contract_scope` 또는 `forbidden_fields`)으로 막기 때문입니다. 저장 시 공백 제거와 대소문자 무시
중복 제거가 적용되고, 응답 `data` 의 키는 항상 계약에 적힌 표기를 씁니다 — 요청이 `Score` 로 와도
계약이 `score` 면 상품 OpenAPI 문서가 선언한 대로 `score` 로 내려갑니다.

Contract Scope 의 `status` 는 `active`(기본), `draft`, `suspended`, `revoked` 만 허용하며 그 외 값은
`400 invalid_contract_status` 로 거부됩니다(대소문자·앞뒤 공백은 정규화). 런타임은 `active` 인 계약만 서빙하므로
`actve`·`enabled` 같은 오타를 저장하면 계약이 곧바로 죽고 모든 조회가 `403 contract_scope_inactive` 가 되지만
admin 목록에는 고객 계약으로 그대로 보입니다.

`valid_from` 이 `valid_to` 보다 뒤인 창은 `400 invalid_access_window` 로 거부됩니다. 런타임 게이트는 현재
시각이 창 안에 있을 때만 조회를 허용하므로, 뒤집힌 창은 계약이 존재하는 내내 모든 조회를 막습니다.

Entitlement 의 `scope` 는 쉼표·공백으로 구분한 권한 목록이며, 런타임 조회는 `data_product:query`,
`data_product:*`, `query`, `*` 중 하나가 목록에 정확히 포함될 때만 허용됩니다(빈 값은 무제한).
`data_product:export` 처럼 조회 권한이 없는 값만 있으면 `403 scope_denied` 가 반환됩니다.

Entitlement 의 `status` 도 Contract Scope 와 같이 `active`(기본), `draft`, `suspended`, `revoked` 만 허용하며
그 외 값은 `400 invalid_entitlement_status` 로 거부됩니다(대소문자·앞뒤 공백은 정규화). 런타임은 `active` 인
Entitlement 만 인정하므로 `enabled` 같은 오타를 저장하면 고객 API 키가 곧바로 `403 inactive_entitlement` 를
받지만 admin 목록에는 정상 발급된 접근권으로 보입니다.

Entitlement 는 `id` 단위로 저장되므로 같은 API 키가 한 상품에 여러 행을 가질 수 있습니다(만료된 체험 계약 옆에
발급한 갱신 계약, 감사용으로 남겨 둔 `revoked` 행 등). 런타임 게이트는 그중 `active` 이고 만료되지 않은 행을 먼저
고르고, 그런 행이 여럿이면 가장 최근에 갱신된 것을 사용합니다. 활성 행이 하나도 없으면 가장 최근 행을 근거로
`403 inactive_entitlement` 가 반환됩니다.

각 Entitlement 는 자기 `contract_key` 를 가리키므로, 우선순위가 높은 행이 이미 종료된 계약을 가리키고 다른 행이
살아 있는 계약을 가리키는 경우가 생깁니다. 런타임은 후보를 순서대로 훑어 **Entitlement 가 활성이고 `scope` 가 조회를
허용하며, 그 계약이 이 상품의 것이고 `active` 이며 유효 기간 안에 있고(민감 상품이면 `purpose` 까지 있는)** 첫 행을
사용합니다. 조건을 모두 만족하는 행이 하나도 없으면 위 우선순위 1순위 행으로 판정해 `inactive_entitlement`,
`contract_scope_missing`, `contract_scope_inactive`, `missing_contract_purpose` 중 실제 원인을 응답합니다. 요청
본문에 따라 달라지는 `allowed_fields` 검사와 호출량을 소모하는 `rate_limit` 은 선택 기준에서 제외되므로, 후보를
훑는 과정이 분당 한도를 앞당겨 소진하지 않습니다.

### 마스킹 정책과 Publish Gate

Contract Scope 의 `masking_policy` 는 런타임이 실제로 구현한 `none`(기본), `redact`, `hash` 만 허용하며 그 외 값은
`400 invalid_masking_policy` 로 거부됩니다(대소문자·앞뒤 공백은 정규화). 자유 서술형 문구를 저장하면 응답은 원본
값 그대로 나가면서 민감 상품 Publish Gate 의 `masking_configured` 만 통과하기 때문입니다.

마스킹은 계약별로 적용되므로 민감 상품의 `masking_configured` 는 **아직 조회를 처리할 수 있는 계약이 모두**
`redact`·`hash` 를 가질 때만 참입니다. 계약 하나만 마스킹하고 다른 계약이 `none` 이면 그 고객은 원본 값을 그대로
받으므로 `masking_configured=false`(`missing_evidence: masking_policy`)로 막힙니다. `active` 가 아니거나 유효 기간이
이미 끝난 계약은 다시는 조회를 처리할 수 없으므로 판정에서 제외하고, 아직 시작되지 않은 계약은 나중에 조회를
처리하므로 그대로 포함합니다. 계약이 하나도 없으면 조회 자체가 불가능하므로 통과합니다.

### 만료 예정 계약·권한 확인

`GET /admin/dataworks/action-center` 는 기본적으로 30일 안에 만료되는 Contract Scope(`expiring_contracts`,
`contract_expiring`)와 API Entitlement(`expiring_access`, `entitlement_expiring`)를 함께 보고합니다. 이미 만료
되었거나 `active` 가 아닌 권한은 종전대로 `inactive_access` 로 집계됩니다.

권한의 활성 판정은 런타임 조회 게이트와 같은 규칙(`status` 는 대소문자·앞뒤 공백 무시, `expires_at` 은 앞뒤 공백을
무시하고 해석 불가하면 만료 취급)을 씁니다. 쓰기 경로가 `status` 를 정규화하기 전에 저장된 `"Active"` 같은 행은
런타임이 정상적으로 서빙하므로, 운영 화면이 이를 `inactive_access` 로 보고해 멀쩡한 접근권을 회수하게 만들지
않습니다.

만료 예고는 `active` 인 Contract Scope 에만 붙습니다. `draft`·`suspended`·`revoked` 계약은 런타임이 서빙하지
않으므로 갱신할 것이 없고, `valid_to` 는 시간이 갈수록 과거로 멀어지기만 해서 한 번 종료한 계약이 영구히
`contract_expiring`(만료된 창이므로 심각도 `high`)으로 남아 정말 갱신이 필요한 계약을 덮어 버립니다.

분기 단위 갱신 주기처럼 더 긴 예고가 필요하면 `expiring_within` 으로 조회 창을 지정합니다. `13w`, `45d` 같은
주·일 표기와 `72h` 같은 Go duration 표기를 받으며, 응답의 `expiring_within` 필드로 실제 적용된 창을 확인할 수
있습니다. 양수가 아니거나 해석할 수 없는 값은 기본값으로 되돌리지 않고 `400 invalid_expiring_within` 으로
거부합니다.

```bash
curl -s "$BASE/admin/dataworks/action-center?expiring_within=13w" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '.summary'
```

## 6. Watermark, Cost, Retirement 운영

운영자는 Action Center에서 stale 데이터, 음수 margin, 개선/폐기 후보를 같이 확인합니다.

주요 API:

- `GET/POST /admin/dataworks/products/{key}/sla`
- `GET/POST /admin/dataworks/products/{key}/watermarks`
- `GET/POST /admin/dataworks/products/{key}/costs`
- `GET/POST /admin/dataworks/products/{key}/proposal-ab`
- `GET/POST /admin/dataworks/products/{key}/retirement`

운영 기준:

1. Watermark의 `delay_status`가 `stale`, `delayed`, `failed`이면 제안/계약 전 데이터 최신성을 먼저 점검합니다.
2. Cost의 `estimated_margin`이 음수이면 가격, rate limit, LLM 사용량, 가공 비용을 재조정합니다.
3. Retirement 추천이 `improve` 또는 `retire`이면 상품 리뷰 회의 안건으로 올립니다.
4. Proposal A/B variant는 executive, technical, compliance 관점으로 생성되며 고객 반응 이벤트를 남깁니다.

## 7. 백업

SQLite 운영 시:

```bash
systemctl stop dataworks
cp /opt/dataworks/data/gateway.db /opt/dataworks/backups/gateway-$(date +%F).db
systemctl start dataworks
```

PostgreSQL 운영 시:

```bash
pg_dump "$DATABASE_URL" > dataworks-$(date +%F).sql
```

백업 대상에는 `data_products`, `factory_runs`, `dw_prompt_templates`, `dw_factory_eval_scores`, `dw_product_funnel_daily`, `dw_product_relationships`, `dw_asset_readiness_scores`, `dw_product_canvases`, `dw_approval_traces`, `dw_evidence_packs`, `dw_contract_versions`, `dw_customer_segments`, `dw_product_fit_scores`, `dw_product_versions`, `dw_contract_scopes`, `dw_api_entitlements`, `dw_product_sla`, `dw_data_watermarks`, `dw_product_costs`, `dw_customer_proposal_events`, `dw_retirement_candidates`가 포함되어야 합니다.

## 8. 장애 대응

| 증상 | 확인 |
| --- | --- |
| Admin UI 접근 불가 | `ADMIN_TOKEN`, 방화벽, `LISTEN_ADDR` 확인 |
| publish가 계속 차단됨 | `/publish-gate`의 `missing_approvals`, `missing_evidence`, `blocked_reasons` 확인 |
| Evidence Pack 생성 실패 | 상품 존재 여부, 최신 definition/risk/poc 조회 오류, DB 마이그레이션 상태 확인 |
| readiness 저장 실패 | `asset_key` 누락 여부, `dw_asset_readiness_scores` 테이블 존재 확인 |
| Factory replay 실패 | 원본 run ID, active prompt template의 `run_type`, 마이그레이션 71~85 적용 여부 확인 |
| DB 잠금 | SQLite busy timeout, 장기 트랜잭션, PostgreSQL 전환 검토 |

## 9. 레거시 K8s 기능

기존 K8s 수집/분석 기능은 `k8s_ops` 플래그 아래 격리된 레거시 기능입니다. 운영이 필요한 경우 다음 문서를 참고하세요.

- [K8s 운영 허브 가이드](K8S_OPERATIONS_HUB.md)
- [K8s Agent 가이드](K8S_AGENT.md)
