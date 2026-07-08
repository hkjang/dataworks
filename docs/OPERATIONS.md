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
| Funnel | `GET /admin/dataworks/funnel` |
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

백업 대상에는 `data_products`, `dw_asset_readiness_scores`, `dw_product_canvases`, `dw_approval_traces`, `dw_evidence_packs`, `dw_contract_versions`, `dw_customer_segments`, `dw_product_fit_scores`, `dw_product_versions`, `dw_contract_scopes`, `dw_api_entitlements`, `dw_product_sla`, `dw_data_watermarks`, `dw_product_costs`, `dw_customer_proposal_events`, `dw_retirement_candidates`가 포함되어야 합니다.

## 8. 장애 대응

| 증상 | 확인 |
| --- | --- |
| Admin UI 접근 불가 | `ADMIN_TOKEN`, 방화벽, `LISTEN_ADDR` 확인 |
| publish가 계속 차단됨 | `/publish-gate`의 `missing_approvals`, `missing_evidence`, `blocked_reasons` 확인 |
| Evidence Pack 생성 실패 | 상품 존재 여부, 최신 definition/risk/poc 조회 오류, DB 마이그레이션 상태 확인 |
| readiness 저장 실패 | `asset_key` 누락 여부, `dw_asset_readiness_scores` 테이블 존재 확인 |
| DB 잠금 | SQLite busy timeout, 장기 트랜잭션, PostgreSQL 전환 검토 |

## 9. 레거시 K8s 기능

기존 K8s 수집/분석 기능은 `k8s_ops` 플래그 아래 격리된 레거시 기능입니다. 운영이 필요한 경우 다음 문서를 참고하세요.

- [K8s 운영 허브 가이드](K8S_OPERATIONS_HUB.md)
- [K8s Agent 가이드](K8S_AGENT.md)
