FROM oven/bun:1.4.0@sha256:5ff609364c049b54eb0ff560ec96319729a972078ef2c755d758f0c6ef89c2d6 AS builder

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY ./web ./
COPY ./VERSION /build/VERSION
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build

FROM golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0 GOWORK=off

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
# relaykit is a local submodule referenced via replace; its go.mod must be
# present for go mod download to resolve the main module graph.
ADD relaykit/go.mod ./relaykit/go.mod
RUN go mod download

COPY . .
COPY --from=builder /build/web/dist ./web/dist
RUN go build -ldflags "-s -w -X 'github.com/lza6/new-api-Max/common.Version=$(cat VERSION)'" -o new-api

FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates \
    && groupadd --gid 10001 app \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin app

COPY --from=builder2 /build/new-api /
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
# 挂载点预先建好并归属运行用户：即使宿主目录首次为空（或将来由别的工具创建），
# 容器内的进程也不会因为没有写权限而无法落盘。
RUN mkdir -p /data /app/logs && chown -R 10001:10001 /data /app/logs
EXPOSE 3000
WORKDIR /data
# 纵深防御：进程不以 root 运行。uid/gid 固定为 10001，部署脚本在宿主侧把
# 数据/日志目录 chown 给它（见 计划书/ops/deploy-zero-downtime.example.sh）。
USER app

ENTRYPOINT ["/new-api"]

# T11：容器健康检查（Caddy/编排依赖 /api/status 探活；v1.3.41 起该路径豁免 GA 限流）
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:3000/api/status | grep -q '"success":\s*true' || exit 1

