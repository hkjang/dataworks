# Data Works 릴리즈 가이드

Data Works는 Go API와 React Workbench를 하나의 Docker 이미지로 빌드합니다. GitHub Release에는 폐쇄망에서 `docker load`할 수 있는 `dataworks-vVERSION.tar.gz` 파일 하나만 custom asset으로 업로드합니다.

## 1. 릴리즈 전 체크리스트

- [ ] `web/`에서 `npm ci`, `npm run lint`, `npm test`, `npm run build` 통과
- [ ] `go test ./...` 통과
- [ ] `go build ./cmd/dataworks` 통과
- [ ] `internal/proxy/server.go`의 `AppVersion`, `scripts/changelog.txt` 최상단, `docs/K8S_OPERATIONS_HUB.md` 버전 일치
- [ ] `git status` 확인 후 의도한 변경만 커밋
- [ ] Docker Engine 및 GitHub CLI 사용 가능
- [ ] `gh auth status` 정상
- [ ] 운영 PostgreSQL 접속 정보와 bootstrap 관리자, 암호화 키 준비
- [ ] `docker-compose.yml` 이미지 태그가 이번 archive의 `dataworks:vVERSION`과 일치

## 2. 버전과 태그

[Semantic Versioning](https://semver.org/lang/ko/)을 따르고 Git 태그에는 `v` 접두사를 사용합니다.

```bash
git tag -a vVERSION -m "Release vVERSION"
git push origin main
git push origin vVERSION
```

태그를 생성하기 전에 해당 커밋의 테스트와 Docker 스모크 테스트를 모두 완료합니다.

## 3. 오프라인 Docker 이미지 빌드

### Linux / macOS

```bash
./scripts/release.sh -v vVERSION -p linux/amd64
```

### Windows / PowerShell

```powershell
pwsh -File scripts/release.ps1 -Version vVERSION -Platform linux/amd64
```

두 스크립트는 다음 작업만 수행합니다.

1. `dataworks:vVERSION` Docker 이미지 빌드
2. `docker save`로 이미지 archive 생성
3. gzip 압축

최종 산출물은 하나입니다.

```text
release/
└── dataworks-vVERSION.tar.gz
```

checksum 파일, 별도 README, 보고서, 소스 archive는 custom release asset으로 생성하거나 업로드하지 않습니다. GitHub가 릴리즈 페이지에 자동 표시하는 Source code archive는 이 정책의 custom asset에 포함되지 않습니다.

## 4. 산출물 검증

```bash
gzip -t release/dataworks-vVERSION.tar.gz
tar -xOzf release/dataworks-vVERSION.tar.gz manifest.json
docker image inspect dataworks:vVERSION
```

`manifest.json`의 `RepoTags`에 `dataworks:vVERSION`이 있어야 합니다.

빌드된 이미지를 실행해 포트와 Workbench를 확인합니다.

```bash
docker run -d --name dataworks-release-smoke --restart=no \
  -p 18080:8080 \
  -e POSTGRES_DSN='postgres://dataworks:change-me@postgres:5432/dataworks?sslmode=disable' \
  -e BOOTSTRAP_ADMIN='admin@dataworks.local' \
  -e BOOTSTRAP_ADMIN_PASSWORD='change-me' \
  -e ENCRYPTION_KEY='replace-with-64-hex-characters' \
  dataworks:vVERSION

curl -fsS http://localhost:18080/health
curl -fsS http://localhost:18080/dataworks/
docker rm -f dataworks-release-smoke
```

## 5. GitHub Release 생성

`scripts/changelog.txt`에 `vVERSION` 항목을 최상단에 추가한 후 릴리즈 스크립트를 실행합니다.

```powershell
pwsh -File scripts/gh_release.ps1 -Version vVERSION -PrevVersion vPREVIOUS
```

스크립트는 한국어 릴리즈 노트를 생성하지만 GitHub custom asset으로는 다음 파일 하나만 업로드합니다.

```text
dataworks-vVERSION.tar.gz
```

GitHub CLI를 직접 사용할 때도 asset 인자를 하나만 지정합니다.

```bash
gh release create vVERSION \
  release/dataworks-vVERSION.tar.gz \
  --repo hkjang/dataworks \
  --verify-tag \
  --title "vVERSION - Data Works" \
  --notes-file release/release-notes-vVERSION.md
```

게시 후 custom asset 목록을 확인합니다.

```bash
gh api repos/hkjang/dataworks/releases/tags/vVERSION \
  --jq '.assets | map(.name)'
```

기대값은 `["dataworks-vVERSION.tar.gz"]`입니다.

## 6. 폐쇄망 배포

`dataworks-vVERSION.tar.gz` 파일을 폐쇄망 Docker 호스트로 옮긴 후 적재합니다.

```bash
gunzip -c dataworks-vVERSION.tar.gz | docker load
```

정상 적재되면 `Loaded image: dataworks:vVERSION`이 표시됩니다. 실행 시 운영 환경에서 발급한 값으로 다음 네 항목을 모두 교체합니다.

```bash
docker run -d --name dataworks --restart=always \
  -p 8080:8080 \
  -e POSTGRES_DSN='postgres://dataworks:change-me@postgres.internal:5432/dataworks?sslmode=require' \
  -e BOOTSTRAP_ADMIN='admin@dataworks.local' \
  -e BOOTSTRAP_ADMIN_PASSWORD='change-me' \
  -e ENCRYPTION_KEY='replace-with-64-hex-characters' \
  dataworks:vVERSION
```

PostgreSQL 호스트는 폐쇄망 내에서 접근 가능해야 합니다. `ENCRYPTION_KEY`는 충분한 난수로 생성해 안전하게 보관하고, 운영 중 임의로 변경하지 않습니다.

### Docker Compose로 실행

저장소의 `docker-compose.yml`은 현재 오프라인 릴리즈 이미지 `dataworks:v0.9.36`에 고정되어 있습니다. `dataworks-v0.9.36.tar.gz`를 적재한 뒤 `.env`에는 다음 네 필수 항목만 설정합니다. `GATEWAY_VERSION` 같은 별도 이미지 태그 환경변수는 사용하지 않습니다.

```dotenv
POSTGRES_DSN=postgres://dataworks:change-me@postgres.internal:5432/dataworks?sslmode=require
BOOTSTRAP_ADMIN=admin@dataworks.local
BOOTSTRAP_ADMIN_PASSWORD=change-me
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

이미지 계약과 Compose 해석 결과를 확인한 후 기동합니다.

```bash
docker compose config --images
# dataworks:v0.9.36
docker compose up -d
```

새 릴리즈를 만들 때는 `docker-compose.yml`의 고정 태그도 `dataworks:vVERSION`으로 함께 갱신합니다.

## 7. 릴리즈 후 검증

```bash
curl -fsS http://<HOST>:8080/health
curl -fsS http://<HOST>:8080/ready
curl -fsS http://<HOST>:8080/dataworks/
```

- Data Product Workbench: `http://<HOST>:8080/dataworks/`
- 기존 Admin Console: `http://<HOST>:8080/admin`

## 8. 롤백

이전 `dataworks:vPREVIOUS` 이미지를 적재한 후 현재 컨테이너를 교체합니다. 데이터베이스 스키마와 하위 호환성을 먼저 확인하고, 동일한 PostgreSQL와 암호화 키를 사용합니다.

```bash
docker stop dataworks
docker rm dataworks
docker run -d --name dataworks --restart=always \
  -p 8080:8080 \
  -e POSTGRES_DSN='postgres://dataworks:change-me@postgres.internal:5432/dataworks?sslmode=require' \
  -e BOOTSTRAP_ADMIN='admin@dataworks.local' \
  -e BOOTSTRAP_ADMIN_PASSWORD='change-me' \
  -e ENCRYPTION_KEY='replace-with-64-hex-characters' \
  dataworks:vPREVIOUS
```
