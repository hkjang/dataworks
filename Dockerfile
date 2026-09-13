# syntax=docker/dockerfile:1.7
FROM node:24-alpine AS web-build
WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src

ENV CGO_ENABLED=0 \
    GOFLAGS=-trimpath \
    GO111MODULE=on

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-build /src/web/dist ./web/dist

ARG TARGETOS=linux
ARG TARGETARCH=amd64
# main.version 은 이 저장소에 없는 이름이라 링커가 조용히 무시하고 있었다 —
# 값이 들어가는 것처럼 보였을 뿐 어느 빌드인지 알 길이 없었다. 실제로 있는
# internal/buildinfo 에 새기고, 커밋과 빌드 시각도 함께 넣는다.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w \
      -X dataworks/internal/buildinfo.Version=${VERSION} \
      -X dataworks/internal/buildinfo.Commit=${COMMIT} \
      -X dataworks/internal/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/dataworks ./cmd/dataworks && \
    mkdir -p /out/data

FROM gcr.io/distroless/static:nonroot AS runtime
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="Data Works" \
      org.opencontainers.image.description="Data Works API gateway and governance server" \
      org.opencontainers.image.source="https://github.com/hkjang/dataworks" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_TIME}"
WORKDIR /app
USER nonroot:nonroot

COPY --chown=nonroot:nonroot --from=build /out/dataworks /app/dataworks
COPY --chown=nonroot:nonroot --from=build /out/data /data

ENV LISTEN_ADDR=:8080 \
    DB_DRIVER=sqlite \
    DB_DSN=/data/gateway.db \
    LOG_FALLBACK_PATH=/data/fallback.ndjson

VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/app/dataworks"]
