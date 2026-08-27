# Data Works Web

Data Works의 데이터 상품 운영 화면입니다. 공개 제품 소개 페이지인 `docs/`와 별개이며, Go API는 기존 `/admin/dataworks/*` 계약을 그대로 사용합니다.

## Stack

- React 19 + TypeScript + Vite
- React Router (UI 전용 `/dataworks/` namespace)
- TanStack Query + TanStack Table
- Zustand
- Tailwind CSS + shadcn 스타일의 로컬 UI primitives
- React Flow + Recharts
- React Hook Form + Zod
- Vitest + Playwright

## Commands

```bash
npm ci
npm run dev
npm run lint
npm test
npm run test:e2e
npm run build
```

개발 서버는 `/admin`, `/auth`, `/me` 요청을 `http://localhost:8080`으로 프록시합니다.

## Production delivery

Vite 결과는 `web/dist`에 생성됩니다. `web/embed.go`가 이를 embed하고 Go 서버가 `/dataworks/`에서 정적 자산과 BrowserRouter fallback을 제공합니다. `/admin`의 기존 Console과 `/admin/dataworks/*` API는 그대로 유지됩니다.

소스 checkout에는 Go 컴파일을 위한 `dist/.gitkeep`만 둡니다. 로컬 binary에 React UI를 포함하려면 반드시 `npm run build` 후 `go build` 또는 `go run`을 실행해야 합니다. Dockerfile은 이 순서를 자동화합니다.

## Current migration slice

- Home Workbench + Action Center
- Asset Catalog + readiness join
- Product portfolio and Product Workspace
- Lifecycle Stepper
- Publish Gate visualizer
- Approval and Evidence views
- Factory observability
- Data Product Graph
- Marketplace, Analytics, Governance shells

Admin OpenAPI 문서에는 아직 P0 response schema가 없으므로 현재 API DTO는 `src/types/dataworks.ts`에서 수동 관리합니다. 타입 자동 생성은 Admin OpenAPI schema 보강 후 전환합니다.
