# emotion-echo-web-bff Dockerfile
# HTTP (Gin :8894) — 前后端唯一 BFF（聚合 5 下游 + XTTS + ai-svc gRPC）
#
# 依赖 emotion-echo-shared (go.mod 里 replace ../emotion-echo-shared)
# Build context 必须是仓库根:
#   docker build -f emotion-echo-web-bff/Dockerfile .

# ============ Stage 1: builder ============
FROM golang:1.26-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace

# 复制 shared 依赖模块 (web-bff go.mod 里 replace ../emotion-echo-shared)
COPY emotion-echo-shared /workspace/emotion-echo-shared

# 复制 web-bff 源代码
COPY emotion-echo-web-bff /workspace/emotion-echo-web-bff

WORKDIR /workspace/emotion-echo-web-bff

RUN go mod download

# 编译 Linux 静态二进制
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/web-bff .

# ============ Stage 2: runtime ============
FROM alpine:3.19

ENV TZ=Asia/Shanghai \
    LANG=C.UTF-8 \
    HTTP_PORT=8894

RUN apk add --no-cache tini ca-certificates tzdata wget \
    && cp /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo "${TZ}" > /etc/timezone \
    && apk del tzdata

RUN addgroup -S -g 65532 app \
    && adduser  -S -u 65532 -G app -h /app -s /sbin/nologin app

WORKDIR /app

COPY --from=builder --chown=app:app /out/web-bff /app/web-bff
COPY --from=builder --chown=app:app /workspace/emotion-echo-web-bff/etc/web-bff.yaml /app/etc/web-bff.yaml

USER app:app

EXPOSE 8894

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:${HTTP_PORT}/health || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/app/web-bff", "-f", "/app/etc/web-bff.yaml"]
