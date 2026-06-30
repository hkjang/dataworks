# Data Works

**Data Works**는 내부 데이터 자산을 판매 가능한 데이터 상품, API 상품, 분석 리포트, PoC 제안으로 전환하는 **Data Product Factory**입니다.

기존 Clustara의 OpenAI 호환 게이트웨이, Admin UI, SQLite/PostgreSQL 저장소, 감사 로그, 승인 흐름, Text2SQL 반복 질문 분석, ClickHouse 분석 기반을 재활용하되, 기본 경험은 Kubernetes 운영 허브가 아니라 데이터 상품화 워크플로우에 맞춰 전환했습니다.

## 핵심 흐름

| 단계 | 내용 |
| --- | --- |
| 아이디어 생성 | 업종, 고객군, 시장 니즈, 데이터 자산을 입력해 상품 후보 5~20개 생성 |
| 상품 정의서 | 상품명, 고객 문제, 제공 데이터, API 구조, 가격 모델, 기대 효과 생성 |
| 리스크 점검 | 개인정보, 개인신용정보, 가명정보, AI 활용, 외부 제공, 보안 리스크 체크 |
| PoC 설계 | 데이터 범위, 검증 방법, 성공 지표, 일정, 승인 항목 생성 |
| 우선순위 평가 | 매출 가능성, 반복 판매성, 구현 적합도, 리스크를 점수화 |
| 제안 패키지 | 은행, 카드, 보험, 핀테크, 공공기관 등 고객군별 제안 구조 생성 |
| 라이프사이클 | draft → review → risk_review → approved → published → archived 관리 |

## 주요 API

| Method | Path | 설명 |
| --- | --- | --- |
| `GET` | `/admin/dataworks/home` | Data Works Home 지표 |
| `GET` | `/admin/dataworks/assets` | 데이터 자산 목록 조회 및 업서트 |
| `POST` | `/admin/dataworks/factory/ideas` | 데이터 상품 아이디어 생성 |
| `POST` | `/admin/dataworks/factory/definitions` | 아이디어 기반 상품 정의서 생성 |
| `POST` | `/admin/dataworks/factory/api-spec` | API 규격 설계 생성 |
| `POST` | `/admin/dataworks/factory/report-spec` | 분석 리포트 규격 설계 생성 |
| `POST` | `/admin/dataworks/risk/check` | 규제·보안 리스크 체크리스트 |
| `POST` | `/admin/dataworks/poc/plans` | PoC 계획 생성 |
| `POST` | `/admin/dataworks/scoring/evaluate` | 우선순위·매출 가능성 평가 |
| `POST` | `/admin/dataworks/proposals` | 고객군별 제안 패키지 생성 |
| `POST` | `/admin/dataworks/similarity/check` | 기존 상품과 유사도 비교 |
| `GET` | `/admin/dataworks/reviews` | 승인 검토 대기 상품 목록 |
| `POST` | `/admin/dataworks/reviews/{key}/approve\|reject` | 상품 상태 승인/반려 |
| `GET` | `/admin/dataworks/portfolio` | 상태별 라이프사이클 상품 조회 |
| `GET` | `/admin/dataworks/analytics` | 상품 매출/리스크 성과 분석 |
| `GET` | `/admin/dataworks/factory/runs` | AI 워크런 수행 이력 조회 |
| `GET` | `/admin/dataworks/products` | 상품 후보·정의서 목록 |

기존 `/admin/data-products` 카탈로그는 확장 필드(`target_customers`, `pricing_model`, `api_spec`, `poc_plan`, `risk_score`, `revenue_score` 등)를 포함하도록 확장되었습니다.

## 실행

```powershell
$env:GATEWAY_SECRET = "dev-only-secret"
$env:ADMIN_TOKEN    = "dev-admin"
go run ./cmd/dataworks
```

기동 로그에 `Data Works listening addr=:8080 database=sqlite`가 보이면 정상입니다.

Admin UI: `http://localhost:8080/admin`

상단 관리자 토큰에 `ADMIN_TOKEN` 값을 입력하면 `Factory Home`이 첫 화면으로 열립니다.

## Docker

```bash
docker build -t dataworks:dev .
docker run -d --name dataworks --restart=always -p 8080:8080 -v $PWD/data:/data \
  -e GATEWAY_SECRET=$(openssl rand -hex 32) \
  -e ADMIN_TOKEN=$(openssl rand -hex 32) \
  dataworks:dev
```

`docker-compose.yml`은 기본 이미지명을 `dataworks`로 사용합니다.

## 저장소

| 저장소 | 용도 |
| --- | --- |
| SQLite | 개발 및 PoC 기본값 (`data/gateway.db`) |
| PostgreSQL | 운영용 저장소 |
| ClickHouse | 장기 분석 fact 적재 및 성과 분석 선택 구성 |

## 레거시 K8s 기능

Kubernetes 수집, Pod 로그, RCA, Stack, ConfigMap, CRD Discovery 등 기존 운영 허브 기능은 삭제하지 않고 `k8s_ops` 기능 플래그 뒤로 격리했습니다. 기본 메뉴와 기본 홈에서는 노출되지 않습니다.

관련 문서는 레거시 참고용으로 유지됩니다.

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
