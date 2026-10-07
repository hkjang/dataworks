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

### 웹 워크벤치 검증 (Node 요구 버전)

`web/package.json` 의 `engines.node` 는 `>=22.19.0` 입니다. 테스트 환경(jsdom)이 끌어오는 undici 8 이 `worker_threads.markAsUncloneable`(Node 22.10+)을 요구하므로 그보다 낮은 런타임에서는 jsdom 환경 테스트가 기동조차 못 합니다.

주의할 함정: npm 은 run-script 의 `PATH` 앞에 **상위 디렉터리의 `node_modules/.bin` 을 전부** 붙입니다(프로젝트 → … → 홈 디렉터리 → `/`). 홈 디렉터리에 `node` 패키지가 설치돼 있으면 그 구버전 `node` 가 nvm·CI 의 node 를 가려, `npm test` 가 `webidl.util.markAsUncloneable is not a function` 으로 exit 1 이 됩니다. 이를 막기 위해 `npm test` 는 `web/scripts/run-with-supported-node.mjs` 를 거쳐 npm 이 실행 중인 node(`npm_node_execpath`)로 vitest 를 띄우고, 하한 미달이면 후보 목록과 함께 즉시 멈춥니다.

런처는 다음 두 상태까지 견딥니다.

- **`web/node_modules` 가 없는 새 워크트리** — 요청한 로컬 bin 이 없으면 고른 node 로 `npm ci` 를 **한 번만** 돌린 뒤 이어서 실행합니다. 이미 설치돼 있으면 아무 것도 설치하지 않습니다(검증 시간 보호). 설치가 실패하면 그 종료 코드를 그대로 돌려주고 멈추므로, 네트워크·npm 캐시가 없는 환경에서는 `npm ci` 의 실패 원인이 그대로 드러납니다 — 조용히 통과하지 않습니다.
- **npm 자신이 가로채인 구버전 node 로 떠 있는 경우** — `npm_node_execpath` 와 `process.execPath` 가 둘 다 하한 미달이면, 시스템에 설치된 node(`$NVM_DIR/versions/node/*/bin/node`, `/usr/local/bin/node`, `/usr/bin/node`)에서 **하한을 넘기는 가장 낮은 버전**을 고릅니다. 상위 `node_modules/.bin` 은 후보에서 제외합니다(그게 문제의 근원입니다). 최신이 아니라 가장 낮은 버전을 고르는 이유는 `engines.node` 가 하한 선언일 뿐이고, 검증해 본 적 없는 최신 런타임으로 넘어가면 오히려 깨지기 때문입니다 — 실측으로 Node 25 에서는 `src/features/auth/silent-sso.test.ts` 가 실패하고 22.23.1·23.11.1 에서는 통과합니다.

릴리즈 검증을 `npm test --silent` 로 돌릴 때 주의할 점: `--silent` 는 `npm_config_loglevel=silent` 를 자식 프로세스까지 물려줍니다. 그래서 테스트가 띄운 중첩 npm 의 `> eslint .` 배너까지 사라집니다. 중첩 npm 의 출력을 검사하는 테스트는 그 환경 변수를 지우고 로그 수준을 직접 지정해야 합니다(`web/src/test/root-npm-scripts.test.ts` 참고) — 단정을 약하게 만들어 우회하지 마십시오.

```bash
cd web && npm ci && npm run lint && npm test && npm run build
```

저장소 루트에서도 같은 검증을 돌릴 수 있습니다. `.github/workflows/ci.yml` 의 web 잡은 `defaults.run.working-directory: web` 에 의존하므로, 그 명령을 그대로 뽑아 루트에서 돌리면 예전에는 npm 이 상위 디렉터리로 올라가 **저장소 밖의** `package.json` 을 집어 `Missing script: "lint"` 로 exit 1 이 됐습니다(의존성 미설치로 생기는 exit 127 `eslint: not found` 와는 다른 실패입니다). 이제 루트 `package.json` 의 `lint`·`test`·`build` 가 `scripts/web-run.mjs` 를 거쳐 `web/` 의 같은 스크립트를 부릅니다 — 위임만 하며 검증을 완화하지 않고, `web/node_modules` 가 없을 때만 `npm ci` 를 먼저 돌립니다. 루트 스크립트에 `|| true`, `--max-warnings`, `--passWithNoTests` 같은 플래그를 더하지 마십시오.

```bash
npm run lint && npm test && npm run build   # 저장소 루트에서
```

`npm run build` 뒤에는 `git status --short` 가 깨끗해야 합니다. 추적 대상인 `web/dist/.gitkeep` 은 `web/vite.config.ts` 의 `keepDistPlaceholder` 플러그인이 `emptyOutDir` 뒤에 원본 내용 그대로 되살립니다.

### 문서 사이트 배포 실패 진단 (`pages build and deployment`)

`docs/` 는 GitHub Pages 의 소스 폴더(`main` / `/docs`)입니다. 이 저장소에는 `_config.yml` 도 `.nojekyll` 도 없으므로 GitHub 이 **직접 관리하는** `pages build and deployment` 워크플로가 `docs/` 를 Jekyll(github-pages 젬)로 빌드해 배포합니다. 이 워크플로는 `.github/workflows/` 에 없고 저장소가 설정을 소유하지 않습니다 — 즉 실패했을 때 저장소 쪽에서 고칠 수 있는 것과 고칠 수 없는 것을 먼저 갈라야 합니다.

**1) 내용 때문인지 확인** — CI 가 쓰는 것과 같은 이미지로 로컬에서 같은 빌드를 돌립니다. `jekyll-optional-front-matter` 때문에 front matter 가 없는 `docs/*.md` 도 전부 페이지로 렌더되므로, 마크다운에 들어간 Liquid 구문(이중 중괄호, 중괄호+퍼센트)이나 깨진 인코딩이 여기서 exit 1 로 드러납니다. 코드 블록 안이라도 Liquid 가 먼저 해석되므로 백틱은 보호막이 아닙니다.

```bash
rm -rf /tmp/pages-ws && mkdir -p /tmp/pages-ws        # 이전 추출물을 지우고 새로 만든다
git archive HEAD | tar -x -C /tmp/pages-ws            # 작업 트리를 더럽히지 않는다
docker run --rm --user "$(id -u):$(id -g)" -v /tmp/pages-ws:/github/workspace \
  -e GITHUB_WORKSPACE=/github/workspace -e INPUT_SOURCE=docs \
  -e INPUT_DESTINATION=./_site -e INPUT_VERBOSE=true -e INPUT_FUTURE=false \
  -e GITHUB_REPOSITORY=hkjang/dataworks -e INPUT_TOKEN="$GITHUB_TOKEN" \
  ghcr.io/actions/jekyll-build-pages:v1.0.13
```

첫 줄의 `rm -rf … && mkdir -p` 를 빼지 마십시오. 디렉터리가 없으면 `tar` 가 `Cannot open: No such file or directory` 로 exit 2 를 내고, 디렉터리를 재활용하면 `git archive | tar -x` 가 HEAD 에 더 이상 없는 파일을 지우지 않아 이전 추출물이 남습니다. 이 절차의 표준 사용법이 **문서 수정 전·후로 두 번 빌드**하는 것이라 그 경로를 실제로 밟게 되는데, 남은 구 파일(예: 이미 삭제한 `docs/*.md`)이 Liquid 로 깨지면 CI 에는 없는 실패가 로컬에서만 재현되어 유령 원인을 쫓게 됩니다 — 오류 메시지가 없는 조용한 오진이라 알아채기 어렵습니다.

`--user` 도 빼지 마십시오. 이것이 없으면 컨테이너가 `_site/` 를 **root 소유로** 써 놓고, 다음 회차의 `rm -rf /tmp/pages-ws` 가 `Permission denied` + exit 1 로 막혀 `&&` 사슬이 거기서 멈춥니다 — 두 번째 빌드를 아예 돌릴 수 없게 되고, 풀려면 `sudo rm -rf /tmp/pages-ws` 가 필요합니다. 실측으로 `--user` 를 준 빌드도 exit 0 이고 `_site/` 가 호출자 소유로 남아 다음 회차가 그냥 돕니다.

이미지 태그는 실제 실행의 `Pull ghcr.io/actions/jekyll-build-pages:…` 단계에서 읽어 맞추십시오. 토큰이 없으면 `jekyll-github-metadata` 가 `The GitHub API credentials you provided aren't valid.` 로 멈춥니다 — 이것은 로컬 환경의 한계이고 저장소 결함이 아닙니다. 공개 저장소이므로 `GITHUB_TOKEN` 은 **스코프를 하나도 주지 않은** 토큰이면 충분합니다. 이 진단에 권한 있는 PAT 를 쓰지 마십시오.

**2) 러너를 못 받은 것인지 확인** — 공개 저장소이므로 인증 없이 잡 목록을 읽을 수 있습니다.

```bash
curl -s "https://api.github.com/repos/hkjang/dataworks/actions/runs/<RUN_ID>/jobs" \
  | python3 -c 'import json,sys; [print(j["name"], j["conclusion"], repr(j["runner_name"]), len(j.get("steps") or [])) for j in json.load(sys.stdin)["jobs"]]'
```

정상 실행의 `build` 잡은 `runner_name` 이 `GitHub Actions …` 이고 단계가 7개(`Pull …`, `Checkout`, `Build with Jekyll`, `Upload artifact` …)이며 20초 남짓에 끝납니다. `runner_name` 이 빈 문자열이고 **단계가 0개**인데 `cancelled` 로 끝났다면 그 잡은 호스티드 러너를 배정받지 못한 채 큐에서 대기하다 취소된 것입니다 — 저장소 내용을 한 줄도 읽지 않았으므로 코드·문서를 고쳐도 달라지지 않습니다. 이때 `build` 가 `cancelled`, `deploy` 가 `skipped` 가 되어 실행 전체는 `failure` 로 보입니다.

단, **concurrency 로 취소된 선행 실행도 모양이 똑같습니다**(`runner_name` 빈 문자열·단계 0개·`cancelled`). 둘을 가르려면 같은 브랜치에 그 뒤로 더 새로운 Pages 실행이 있는지 먼저 보십시오 — `curl -s "https://api.github.com/repos/hkjang/dataworks/actions/runs?branch=main&per_page=10"`. 더 새로운 실행이 있으면 이 취소는 정상이고 조치할 것이 없습니다(그 뒤 실행만 보면 됩니다). 가장 마지막 실행이 이 모양이면 러너 미배정입니다.

**조치**: 같은 커밋으로 워크플로를 **재실행**하거나 다음 push 를 기다리는 것뿐입니다. 재배포 전까지 사이트는 직전에 성공한 배포(= 이전 릴리즈의 `docs/index.html`)를 계속 서빙하므로, `curl -s https://hkjang.github.io/dataworks/ | grep -o 'v0\.9\.[0-9]*'` 로 사이트가 최신 릴리즈를 반영하는지 확인할 수 있습니다. 또한 짧은 간격으로 main 에 두 번 push 하면(머지 커밋 + 릴리즈 커밋) 앞선 실행이 뒤 실행에 의해 취소되므로, 릴리즈 뒤 Pages 실행은 **마지막 것 하나만** 보면 됩니다.

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

승인 trace 의 `expires_at` 판정은 계약 창·Entitlement 와 같은 규칙을 씁니다. 앞뒤 공백을 떼고 본 값이 비어 있으면
만료 없음(승인 상태 유지), 값이 있으면 RFC3339(나노초 허용)로 읽어 현재 시각보다 뒤일 때만 유효합니다. 판정에만
공백을 떼며 저장·API 응답의 `expires_at` 은 입력한 원문 그대로 남습니다. 공백을 떼고도 읽을 수 없는 값은 `expired`
로 닫혀 게이트를 막으므로(설정 오류를 통과시키지 않기 위함), 그런 행은 `POST …/approvals` 로 다시 등록하십시오.

`POST …/products/{key}/regulatory-trace` 로 규제 추적 행렬을 재생성해도 승인 trace 의 `expires_at` 은 지워지지 않습니다.
규제 추적 입력에는 만료일 개념이 없으므로, 재생성은 `status`·`decided_by`·`notes`·`evidence_ref` 만 새 값으로 덮고
만료일은 `POST …/approvals` 로 등록한 값을 그대로 유지합니다. 만료일을 비우거나 바꾸려면 `POST …/approvals` 를 쓰십시오.

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

### 증거 목록 새로고침 실패(`evidence_refresh_failed`)

`POST /admin/dataworks/products/{key}/evidence` 는 상품 정의서(`product_definitions`), 리스크 점검
(`product_risk_reviews`), PoC 계획(`product_poc_plans`)을 읽어 증거 목록을 다시 만들고, 기존 행을
지운 뒤 새로 넣습니다. 세 조회 중 하나라도 실패하면 `500` + `evidence_refresh_failed` 로 끊고 저장된
증거 목록은 그대로 둡니다 — 읽지 못한 출처를 "출처 없음" 으로 취급하면 `definition_version`,
`risk_basis`, `poc_success_metric` 행이 조용히 사라진 채로 새로고침이 성공한 것처럼 보이기 때문입니다.
`GET` 도 저장된 행이 없어 즉석에서 만들어야 할 때 같은 이유로 `500` + `evidence_failed` 를 돌려줍니다.
이 코드를 받으면 DB 연결과 위 세 테이블을 먼저 확인하고, 복구한 다음 다시 새로고침하세요.

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

`contract_key` 는 상품별이 아니라 플랫폼 전체에서 유일합니다. 다른 상품이 이미 쓰고 있는 키로 등록하면
`409 contract_key_taken` 으로 거부됩니다 — 같은 상품에 다시 보내는 것은 갱신이지만, 다른 상품으로 보내면 저장 시
`product_key` 가 덮여 계약이 그 상품으로 옮겨 가고, 원래 상품이 그 계약으로 발급한 Entitlement 는 다음 호출부터
`403 contract_scope_missing` 을 받으면서 admin 목록에는 그대로 남기 때문입니다. Entitlement 도 `id` 가 플랫폼
전체에서 유일하므로, 다른 상품의 행이 쓰는 `id` 를 직접 지정해 보내면 `409 entitlement_id_taken` 으로 거부됩니다.

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

후보를 훑다가 어떤 후보의 `contract_key` 를 **읽는 데 실패**하면(DB 장애, 손상된 행 등) 그것은 "그 계약이 없다" 와
다릅니다. 이 경우 런타임은 남은 후보를 끝까지 확인해 **사용 가능한 후보가 있으면 그 후보로 정상 서빙(200)** 하고,
사용 가능한 후보가 하나도 없을 때만 `500 contract_lookup_failed` 를 반환합니다. 즉 후보가 여럿인 상황에서도 이
코드가 나올 수 있으며, 그 응답은 접근권 설정 문제가 아니라 **서버·DB 쪽 장애 신호**이므로 계약을 수정하지 말고 DB
상태와 서버 로그를 먼저 확인해야 합니다(종전에는 이 실패가 1순위 후보의 `403 contract_scope_inactive` 등으로
보고돼 운영자가 틀린 사유를 봤습니다).

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

액션 센터의 적합도·계약·권한·Watermark·비용·폐기 후보 목록 조회가 실패하면 HTTP `500`과 해당 오류 코드를
반환합니다. 조회 실패를 경고 0건으로 표시하지 않으며, 저장소 복구 후 다시 조회해야 합니다. 정상적으로
조회된 빈 목록은 기존처럼 HTTP `200`과 0건 집계를 반환합니다.

`approved`·`review`·`risk_review` 상품의 퍼블리시 게이트 평가(자산 준비도·승인 이력·Evidence Pack 조회)가
실패하면 같은 방식으로 HTTP `500`과 `publish_gate_failed` 를 반환합니다. `GET …/publish-gate` 와
`POST …/publish` 가 같은 조회 실패에 `500`을 돌려주는데 액션 센터만 그 상품을 `blocked_launches` 에서
빼고 출시 가능한 것처럼 보여 주지 않도록 한 것입니다. 평가가 성공해 차단 사유가 없는 상품은 종전대로
집계되지 않습니다.

권한의 활성 판정은 런타임 조회 게이트와 같은 규칙(`status` 는 대소문자·앞뒤 공백 무시, `expires_at` 은 앞뒤 공백을
무시하고 해석 불가하면 만료 취급)을 씁니다. 쓰기 경로가 `status` 를 정규화하기 전에 저장된 `"Active"` 같은 행은
런타임이 정상적으로 서빙하므로, 운영 화면이 이를 `inactive_access` 로 보고해 멀쩡한 접근권을 회수하게 만들지
않습니다. Retirement 후보 평가(`POST /admin/dataworks/products/{key}/retirement`)의 "no active API entitlements"
판정도 같은 규칙을 쓰므로, 런타임이 서빙 중인 접근권이 있는 상품에 이 사유로 위험 점수가 +20 되지 않습니다.

계약의 `valid_to` 도 런타임과 같이 앞뒤 공백을 무시하며, 저장된 원문과 응답 필드는 유지합니다. 런타임 조회
게이트는 `valid_from` 도 같은 규칙으로 읽으므로, 앞뒤 공백이 섞인 레거시 행이라도 이미 열린 창이면 정상적으로
서빙합니다(아직 열리지 않은 창과 해석할 수 없는 `valid_from` 은 종전대로 `403 contract_scope_inactive`).

해석할 수 없는 `valid_from` 은 `valid_to` 와 같은 규칙으로 보고합니다. 런타임이 그 계약의 모든 조회를
`403 contract_scope_inactive` 로 영구히 막으므로, `valid_to` 가 비어 있거나 조회 창 밖의 먼 미래여도
`contract_expiring`(심각도 `high`)으로 한 번 실립니다. 두 값이 모두 해석 불가여도 계약당 항목은 하나이고
`expiring_contracts` 도 1만 늘어납니다. 액션에는 `valid_from`·`valid_to` 가 저장 원문 그대로 실립니다.
아직 열리지 않은(미래) `valid_from` 은 오류가 아니라 예정된 계약이므로 여기에 포함하지 않습니다.

`valid_from` 이 `valid_to` 보다 뒤인 뒤집힌 창도 같은 기준으로 `contract_expiring`(심각도 `high`)으로 보고합니다.
창이 열리기 전에 이미 닫히므로 계약이 존재하는 내내 모든 조회가 `403 contract_scope_inactive` 이고, `valid_to` 가
조회 창 밖의 먼 미래여도 마찬가지입니다. 쓰기 경로는 이런 창을 `400 invalid_access_window` 로 거부하므로 이
보고는 그 검사 이전에 저장된 레거시 행을 위한 것이고, 두 값이 같은 순간이면 쓰기 경로와 같이 정상으로 봅니다.

만료 예고는 `active` 인 Contract Scope 에만 붙습니다. `draft`·`suspended`·`revoked` 계약은 런타임이 서빙하지
않으므로 갱신할 것이 없고, `valid_to` 는 시간이 갈수록 과거로 멀어지기만 해서 한 번 종료한 계약이 영구히
`contract_expiring`(만료된 창이므로 심각도 `high`)으로 남아 정말 갱신이 필요한 계약을 덮어 버립니다.

분기 단위 갱신 주기처럼 더 긴 예고가 필요하면 `expiring_within` 으로 조회 창을 지정합니다. `13w`, `45d` 같은
주·일 표기와 `72h` 같은 Go duration 표기를 받으며, 응답의 `expiring_within` 필드로 실제 적용된 창을 확인할 수
있습니다. 양수가 아니거나 해석할 수 없는 값은 기본값으로 되돌리지 않고 `400 invalid_expiring_within` 으로
거부합니다. 표현할 수 있는 범위(`106751d`, `15250w`)를 넘는 값도 같은 오류로 거부합니다 — 그대로 계산하면
값이 감겨 요청한 것과 다른(때로는 음수인) 창으로 조용히 답하게 됩니다.

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
